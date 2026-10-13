package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/cc/internal/config"
	"github.com/go-admin-kit/services/cc/internal/fsclient"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"gorm.io/gorm"
)

// 异步转写：入队 → 查询 → worker 处理 → 失败重试/判失败，以及流式转发
// （不再把整段录音读进内存）。

func newAsyncQcServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// 落盘临时库而不是 :memory:——后台 worker 从连接池里另开连接，纯内存库
	// 每条连接是独立的空库，会看不到已建的表。
	dsn := filepath.Join(t.TempDir(), "cc.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.Call{}, &store.CCTranscript{}, &store.CCQcKeyword{},
		&store.CCQcResult{}, &store.CCTranscribeJob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := &Server{Store: store.NewWithDB(db), Secret: "test-jwt-secret"}
	r := gin.New()
	r.POST("/api/v1/cc/qc/transcribe", srv.QcTranscribe)
	r.GET("/api/v1/cc/qc/transcribe/jobs", srv.QcTranscribeJobByCall)
	r.GET("/api/v1/cc/qc/transcribe/jobs/:id", srv.QcTranscribeJob)
	return srv, r
}

// seedCall 造一条本租户带录音的话单。
func seedCall(t *testing.T, srv *Server, tenantID uint64, callID, recording string) {
	t.Helper()
	if err := srv.Store.DB().Create(&store.Call{
		TenantID: tenantID, CallID: callID, Status: "hangup", Recording: recording,
	}).Error; err != nil {
		t.Fatalf("seed call: %v", err)
	}
}

// asrStub 一个 OpenAI 兼容的转写端点：断言收到的 multipart 体，回固定文本。
func asrStub(t *testing.T, onBody func(size int64)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "no file", http.StatusBadRequest)
			return
		}
		defer func() { _ = f.Close() }()
		n, _ := io.CopyN(io.Discard, f, 1<<30)
		if onBody != nil {
			onBody(n)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"您好 这里是客服 关于退款问题"}`))
	}))
}

// recordingStub 假的 control-api 录音端点，回 size 字节。
func recordingStub(t *testing.T, size int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/recordings/") {
			http.NotFound(w, r)
			return
		}
		chunk := make([]byte, 4096)
		for written := 0; written < size; written += len(chunk) {
			n := len(chunk)
			if size-written < n {
				n = size - written
			}
			if _, err := w.Write(chunk[:n]); err != nil {
				return
			}
		}
	}))
}

func TestQcTranscribeEnqueuesAndReturns202(t *testing.T) {
	srv, r := newAsyncQcServer(t)
	seedCall(t, srv, 1, "aaaa1111-bbbb-2222-cccc-333344445555", "/recordings/rec-1.wav")
	srv.FS = fsclient.New("http://127.0.0.1:1", "tok")
	srv.Speech = config.NewSpeechReader(srv.Store.DB(), config.SpeechSettings{
		ASRURL: "http://127.0.0.1:1", ASRModel: "whisper-1",
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodPost, "/api/v1/cc/qc/transcribe", `{"call_id":"aaaa1111-bbbb-2222-cccc-333344445555"}`))
	if w.Code != http.StatusAccepted {
		t.Fatalf("enqueue: %d want 202; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			JobID   uint64 `json:"job_id"`
			Status  string `json:"status"`
			Created bool   `json:"created"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.JobID == 0 || resp.Data.Status != store.TranscribeStatusPending || !resp.Data.Created {
		t.Fatalf("bad enqueue payload: %+v", resp.Data)
	}

	// 重复提交复用同一任务（created=false，不新增行）
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodPost, "/api/v1/cc/qc/transcribe", `{"call_id":"aaaa1111-bbbb-2222-cccc-333344445555"}`))
	var again struct {
		Data struct {
			JobID   uint64 `json:"job_id"`
			Created bool   `json:"created"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &again)
	if again.Data.JobID != resp.Data.JobID || again.Data.Created {
		t.Fatalf("resubmit should reuse job: %+v", again.Data)
	}
	var rows int64
	srv.Store.DB().Model(&store.CCTranscribeJob{}).Count(&rows)
	if rows != 1 {
		t.Fatalf("job rows = %d, want 1", rows)
	}

	// 查询端点：按 id 与按 call_id 都能取到
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodGet, "/api/v1/cc/qc/transcribe/jobs/1", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("get job by id: %d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodGet, "/api/v1/cc/qc/transcribe/jobs?call_id=aaaa1111-bbbb-2222-cccc-333344445555", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("get job by call: %d body=%s", w.Code, w.Body.String())
	}
}

// 跨租户：别的租户的通话不能入队，别的租户的任务查不到。
func TestQcTranscribeTenantIsolation(t *testing.T) {
	srv, r := newAsyncQcServer(t)
	seedCall(t, srv, 2, "bbbb1111-cccc-2222-dddd-333344445555", "/recordings/rec-2.wav")
	srv.FS = fsclient.New("http://127.0.0.1:1", "tok")
	srv.Speech = config.NewSpeechReader(srv.Store.DB(), config.SpeechSettings{
		ASRURL: "http://127.0.0.1:1", ASRModel: "whisper-1",
	})

	// 网关头 tenant=1，通话属于 tenant=2 → 404，不入队
	w := httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodPost, "/api/v1/cc/qc/transcribe", `{"call_id":"bbbb1111-cccc-2222-dddd-333344445555"}`))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant enqueue: %d want 404; body=%s", w.Code, w.Body.String())
	}
	var rows int64
	srv.Store.DB().Model(&store.CCTranscribeJob{}).Count(&rows)
	if rows != 0 {
		t.Fatalf("cross-tenant enqueue created %d job rows, want 0", rows)
	}

	// tenant=2 的任务，tenant=1 查不到
	job, _, err := srv.Store.EnqueueTranscribeJob(t.Context(), 2, "bbbb1111-cccc-2222-dddd-333344445555", "rec-2.wav", 9, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodGet, "/api/v1/cc/qc/transcribe/jobs?call_id=bbbb1111-cccc-2222-dddd-333344445555", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant job read: %d want 404", w.Code)
	}
	if job.TenantID != 2 {
		t.Fatalf("job tenant = %d, want 2", job.TenantID)
	}
}

// worker 全链路：claim → 流式转发 → 存转写 → 质检 → done + 得分。
// 同时断言 ASR 侧收到的字节数等于录音大小（流式转发不丢不缓冲）。
func TestTranscribeWorkerHappyPath(t *testing.T) {
	srv, _ := newAsyncQcServer(t)
	const recSize = 3 << 20 // 3MB 录音
	var gotSize int64
	asr := asrStub(t, func(n int64) { gotSize = n })
	defer asr.Close()
	rec := recordingStub(t, recSize)
	defer rec.Close()

	srv.FS = fsclient.New(rec.URL, "tok")
	srv.Speech = config.NewSpeechReader(srv.Store.DB(), config.SpeechSettings{
		ASRURL: asr.URL, ASRModel: "whisper-1",
	})
	if err := srv.Store.CreateKeyword(t.Context(), &store.CCQcKeyword{
		TenantID: 1, Word: "退款", Type: "forbidden",
	}); err != nil {
		t.Fatal(err)
	}
	seedCall(t, srv, 1, "cccc1111-dddd-2222-eeee-333344445555", "/recordings/rec-w.wav")

	job, _, err := srv.Store.EnqueueTranscribeJob(t.Context(), 1, "cccc1111-dddd-2222-eeee-333344445555", "rec-w.wav", 1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := srv.Store.ClaimTranscribeJob(t.Context())
	if err != nil || claimed == nil {
		t.Fatalf("claim: job=%v err=%v", claimed, err)
	}
	if claimed.Status != store.TranscribeStatusRunning || claimed.Attempts != 1 {
		t.Fatalf("claimed job = %+v, want running attempts=1", claimed)
	}
	// 队列已空：第二次 claim 返回 nil
	if next, err := srv.Store.ClaimTranscribeJob(t.Context()); err != nil || next != nil {
		t.Fatalf("second claim = %v, %v; want nil,nil", next, err)
	}

	if err := srv.processTranscribeJob(t.Context(), claimed); err != nil {
		t.Fatalf("process: %v", err)
	}
	if gotSize != recSize {
		t.Fatalf("ASR received %d bytes, want %d (streamed through)", gotSize, recSize)
	}

	done, err := srv.Store.GetTranscribeJob(t.Context(), 1, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != store.TranscribeStatusDone || done.Score == nil {
		t.Fatalf("job after process = %+v, want done with score", done)
	}
	// 命中 1 次违禁词「退款」→ 100-20=80
	if *done.Score != 80 {
		t.Fatalf("score = %d, want 80", *done.Score)
	}
	if done.FinishedAt == nil {
		t.Fatal("finished_at not set")
	}
	tr, err := srv.Store.GetTranscript(t.Context(), 1, "cccc1111-dddd-2222-eeee-333344445555")
	if err != nil || tr.Status != "done" || tr.Text == "" {
		t.Fatalf("transcript = %+v, err=%v", tr, err)
	}
}

// 失败路径：ASR 500 → 前两次退回 pending 重试，第三次判 failed 并留档。
func TestTranscribeWorkerRetryThenFail(t *testing.T) {
	srv, _ := newAsyncQcServer(t)
	asr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer asr.Close()
	rec := recordingStub(t, 1024)
	defer rec.Close()
	srv.FS = fsclient.New(rec.URL, "tok")
	srv.Speech = config.NewSpeechReader(srv.Store.DB(), config.SpeechSettings{
		ASRURL: asr.URL, ASRModel: "whisper-1",
	})

	job, _, err := srv.Store.EnqueueTranscribeJob(t.Context(), 1, "dddd1111-eeee-2222-ffff-333344445555", "rec-f.wav", 1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		claimed, err := srv.Store.ClaimTranscribeJob(t.Context())
		if err != nil || claimed == nil {
			t.Fatalf("attempt %d claim: %v %v", attempt, claimed, err)
		}
		if err := srv.processTranscribeJob(t.Context(), claimed); err == nil {
			t.Fatalf("attempt %d: expected error from failing ASR", attempt)
		}
		cur, err := srv.Store.GetTranscribeJob(t.Context(), 1, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		wantStatus := store.TranscribeStatusPending
		if attempt == 3 {
			wantStatus = store.TranscribeStatusFailed
		}
		if cur.Status != wantStatus {
			t.Fatalf("attempt %d status = %q, want %q (attempts=%d)", attempt, cur.Status, wantStatus, cur.Attempts)
		}
		if cur.Error == "" {
			t.Fatalf("attempt %d: error not recorded", attempt)
		}
	}
	// 终态才留档 failed 转写
	tr, err := srv.Store.GetTranscript(t.Context(), 1, "dddd1111-eeee-2222-ffff-333344445555")
	if err != nil || tr.Status != "failed" {
		t.Fatalf("transcript = %+v err=%v, want failed", tr, err)
	}
	// 判死后队列为空，不再无限重试
	if next, err := srv.Store.ClaimTranscribeJob(t.Context()); err != nil || next != nil {
		t.Fatalf("claim after exhaustion = %v %v, want nil nil", next, err)
	}
}

// 进程重启：卡在 running 的任务被回收（未用满重试→pending，用满→failed）。
func TestRequeueStaleTranscribeJobs(t *testing.T) {
	srv, _ := newAsyncQcServer(t)
	ctx := t.Context()
	stale := time.Now().Add(-time.Hour)

	retryable, _, err := srv.Store.EnqueueTranscribeJob(ctx, 1, "eeee1111-ffff-2222-aaaa-333344445555", "a.wav", 1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	exhausted, _, err := srv.Store.EnqueueTranscribeJob(ctx, 1, "ffff1111-aaaa-2222-bbbb-333344445555", "b.wav", 1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	// 手工摆成「上个进程死时的 running」状态
	srv.Store.DB().Model(&store.CCTranscribeJob{}).Where("id = ?", retryable.ID).
		Updates(map[string]any{"status": store.TranscribeStatusRunning, "started_at": stale, "attempts": 1})
	srv.Store.DB().Model(&store.CCTranscribeJob{}).Where("id = ?", exhausted.ID).
		Updates(map[string]any{"status": store.TranscribeStatusRunning, "started_at": stale, "attempts": 3})

	requeued, failed, err := srv.Store.RequeueStaleTranscribeJobs(ctx, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 1 || failed != 1 {
		t.Fatalf("requeued=%d failed=%d, want 1 and 1", requeued, failed)
	}
	a, _ := srv.Store.GetTranscribeJob(ctx, 1, retryable.ID)
	b, _ := srv.Store.GetTranscribeJob(ctx, 1, exhausted.ID)
	if a.Status != store.TranscribeStatusPending || a.StartedAt != nil {
		t.Fatalf("retryable job = %+v, want pending with nil started_at", a)
	}
	if b.Status != store.TranscribeStatusFailed {
		t.Fatalf("exhausted job status = %q, want failed", b.Status)
	}
	// 刚起跑（started_at=now）的任务不该被误回收
	fresh, _, err := srv.Store.EnqueueTranscribeJob(ctx, 1, "abcd1111-aaaa-2222-bbbb-333344445555", "c.wav", 1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	srv.Store.DB().Model(&store.CCTranscribeJob{}).Where("id = ?", fresh.ID).
		Updates(map[string]any{"status": store.TranscribeStatusRunning, "started_at": time.Now(), "attempts": 1})
	if requeued, failed, err := srv.Store.RequeueStaleTranscribeJobs(ctx, 15*time.Minute); err != nil ||
		requeued != 0 || failed != 0 {
		t.Fatalf("fresh job touched: requeued=%d failed=%d err=%v", requeued, failed, err)
	}
	cur, err := srv.Store.GetTranscribeJob(ctx, 1, fresh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Status != store.TranscribeStatusRunning {
		t.Fatalf("fresh job status = %q, want running (untouched)", cur.Status)
	}
}

// 队列容量闸门：满了拒绝新任务（而不是排到内存/ASR 打爆）。
func TestEnqueueQueueCap(t *testing.T) {
	srv, _ := newAsyncQcServer(t)
	ctx := t.Context()
	for i := 0; i < 2; i++ {
		if _, _, err := srv.Store.EnqueueTranscribeJob(ctx, 1, "call-cap-"+string(rune('a'+i)), "x.wav", 1, 3, 2); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	_, _, err := srv.Store.EnqueueTranscribeJob(ctx, 1, "cafe1111-aaaa-2222-bbbb-333344445555", "x.wav", 1, 3, 2)
	if !errors.Is(err, store.ErrTranscribeQueueFull) {
		t.Fatalf("err = %v, want ErrTranscribeQueueFull", err)
	}
	// 已有的进行中任务重复提交不受容量限制（复用行）
	if _, created, err := srv.Store.EnqueueTranscribeJob(ctx, 1, "call-cap-a", "x.wav", 1, 3, 2); err != nil || created {
		t.Fatalf("reuse under cap: created=%v err=%v", created, err)
	}
}

// RunTranscribeLoop 端到端：入队的任务被后台 loop 自己捞起来跑完（覆盖抽干
// 逻辑与并发闸门，不只是手工 claim+process）。
func TestRunTranscribeLoopDrainsQueue(t *testing.T) {
	srv, _ := newAsyncQcServer(t)
	asr := asrStub(t, nil)
	defer asr.Close()
	rec := recordingStub(t, 8192)
	defer rec.Close()
	srv.FS = fsclient.New(rec.URL, "tok")
	srv.Speech = config.NewSpeechReader(srv.Store.DB(), config.SpeechSettings{
		ASRURL: asr.URL, ASRModel: "whisper-1",
	})

	ids := make([]uint64, 0, 3)
	for _, callID := range []string{
		"1111aaaa-bbbb-2222-cccc-333344445555",
		"2222aaaa-bbbb-2222-cccc-333344445555",
		"3333aaaa-bbbb-2222-cccc-333344445555",
	} {
		job, _, err := srv.Store.EnqueueTranscribeJob(t.Context(), 1, callID, "loop.wav", 1, 3, 100)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, job.ID)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go srv.RunTranscribeLoop(ctx, TranscribeWorkerOptions{
		Concurrency: 2, PollInterval: 10 * time.Millisecond, StaleAfter: time.Minute,
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		doneCount := 0
		for _, id := range ids {
			if j, err := srv.Store.GetTranscribeJob(t.Context(), 1, id); err == nil &&
				j.Status == store.TranscribeStatusDone {
				doneCount++
			}
		}
		if doneCount == len(ids) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("loop finished only %d/%d jobs", doneCount, len(ids))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// 转写不再把整段录音读进内存：qc.go/qc_worker.go 里不得出现对录音体的
// io.ReadAll，且转发必须走 io.Pipe。源码断言——内存回归很容易被后续改动
// 悄悄带回来，而这条断言会立刻失败。
func TestTranscribeDoesNotBufferRecording(t *testing.T) {
	for _, name := range []string{"qc.go", "qc_worker.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if bytes.Contains(src, []byte("io.ReadAll(io.LimitReader(resp.Body, 100<<20)")) {
			t.Fatalf("%s still buffers the whole recording with io.ReadAll", name)
		}
	}
	worker, err := os.ReadFile("qc_worker.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(worker, []byte("io.Pipe()")) {
		t.Fatal("qc_worker.go must stream via io.Pipe")
	}
}

// 大录音走流式：20MB 录音端到端转发成功，且 ASR 收到的字节数一致。
// 老实现会在此处占住 20MB+multipart 副本；流式实现只过固定缓冲。
func TestTranscribeStreamsLargeRecording(t *testing.T) {
	srv, _ := newAsyncQcServer(t)
	const recSize = 20 << 20
	var gotSize int64
	asr := asrStub(t, func(n int64) { gotSize = n })
	defer asr.Close()
	rec := recordingStub(t, recSize)
	defer rec.Close()
	srv.FS = fsclient.New(rec.URL, "tok")
	srv.Speech = config.NewSpeechReader(srv.Store.DB(), config.SpeechSettings{
		ASRURL: asr.URL, ASRModel: "whisper-1",
	})

	text, _, err := srv.transcribeRecording(t.Context(), srv.Speech.Settings(t.Context()), "big.wav")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text == "" {
		t.Fatal("empty text")
	}
	if gotSize != recSize {
		t.Fatalf("ASR received %d bytes, want %d", gotSize, recSize)
	}
}
