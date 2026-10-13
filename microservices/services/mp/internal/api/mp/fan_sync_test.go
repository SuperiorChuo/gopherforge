package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

type fanBatchRequest struct {
	UserList []struct {
		OpenID string `json:"openid"`
		Lang   string `json:"lang"`
	} `json:"user_list"`
}

func makeOpenIDs(prefix string, count int) []string {
	result := make([]string, count)
	for i := range result {
		result[i] = fmt.Sprintf("%s-%03d", prefix, i+1)
	}
	return result
}

func fanListPayload(total int, openIDs []string, next string) []byte {
	payload := map[string]any{
		"total": total, "count": len(openIDs), "data": map[string]any{"openid": openIDs}, "next_openid": next,
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func fanProfilesPayload(openIDs []string) []byte {
	profiles := make([]map[string]any, 0, len(openIDs))
	for i, openID := range openIDs {
		profiles = append(profiles, map[string]any{
			"subscribe": 1, "openid": openID, "nickname": fmt.Sprintf("粉丝-%d", i+1),
			"city": "杭州", "province": "浙江", "country": "中国",
			"headimgurl": "https://example.test/avatar", "subscribe_time": 1_700_000_000,
			"tagid_list": []int64{7, 8},
		})
	}
	raw, _ := json.Marshal(map[string]any{"user_info_list": profiles})
	return raw
}

func makeFanSyncDue(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.Store.DB().Model(&store.MpFanSyncJob{}).
		Where("status = ?", "pending").Update("next_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("make fan sync due: %v", err)
	}
}

func doFanSyncWithPermission(r http.Handler, method, path, permission string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "42")
	req.Header.Set("X-Auth-Username", "tester")
	req.Header.Set("X-Auth-Tenant-ID", "1")
	req.Header.Set("X-Auth-Permissions", permission)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func runFanSyncUntilTerminal(t *testing.T, srv *Server, tenantID, accountID uint64) *store.MpFanSyncJob {
	t.Helper()
	for i := 0; i < 20; i++ {
		makeFanSyncDue(t, srv)
		if err := srv.ReconcileFanSyncJobs(context.Background(), 2); err != nil {
			t.Fatalf("reconcile step %d: %v", i, err)
		}
		job, err := srv.Store.LatestFanSyncJob(context.Background(), tenantID, accountID)
		if err != nil {
			t.Fatalf("load job step %d: %v", i, err)
		}
		if job != nil && (job.Status == "succeeded" || job.Status == "failed") {
			return job
		}
	}
	t.Fatal("fan sync did not reach terminal status")
	return nil
}

func TestFanSyncAPIAndWorkerCompleteMultiplePagesInBatches(t *testing.T) {
	pageOne := makeOpenIDs("page-one", 101)
	pageTwo := makeOpenIDs("page-two", 2)
	var mu sync.Mutex
	var cursors []string
	var batchSizes []int
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/user/get":
			cursor := r.URL.Query().Get("next_openid")
			mu.Lock()
			cursors = append(cursors, cursor)
			mu.Unlock()
			if cursor == "" {
				_, _ = w.Write(fanListPayload(103, pageOne, "cursor-2"))
				return
			}
			if cursor == "cursor-2" {
				_, _ = w.Write(fanListPayload(103, pageTwo, ""))
				return
			}
			_, _ = w.Write([]byte(`{"errcode":40003,"errmsg":"bad cursor"}`))
		case "/cgi-bin/user/info/batchget":
			var body fanBatchRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode batch: %v", err)
				return
			}
			openIDs := make([]string, len(body.UserList))
			for i, item := range body.UserList {
				openIDs[i] = item.OpenID
				if item.Lang != "zh_CN" {
					t.Errorf("batch lang=%q", item.Lang)
				}
			}
			mu.Lock()
			batchSizes = append(batchSizes, len(openIDs))
			mu.Unlock()
			_, _ = w.Write(fanProfilesPayload(openIDs))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	srv, router := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, router, "wxFANSYNC", "全量同步号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/fans/sync", accountID)

	w := doAuthed(router, http.MethodGet, path, "", 1)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"job":null`) {
		t.Fatalf("empty status: code=%d body=%s", w.Code, w.Body.String())
	}
	w = doAuthed(router, http.MethodPost, path, "{}", 1)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"reused":false`) {
		t.Fatalf("enqueue: code=%d body=%s", w.Code, w.Body.String())
	}
	w = doAuthed(router, http.MethodPost, path, "{}", 1)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"reused":true`) {
		t.Fatalf("reuse active: code=%d body=%s", w.Code, w.Body.String())
	}
	if w := doAuthed(router, http.MethodGet, path, "", 2); w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant status code=%d body=%s", w.Code, w.Body.String())
	}

	job := runFanSyncUntilTerminal(t, srv, 1, accountID)
	if job.Status != "succeeded" || job.TotalCount != 103 || job.ListedCount != 103 || job.SyncedCount != 103 {
		t.Fatalf("completed job=%+v", job)
	}
	mu.Lock()
	gotCursors := append([]string(nil), cursors...)
	gotBatchSizes := append([]int(nil), batchSizes...)
	mu.Unlock()
	if fmt.Sprint(gotCursors) != "[ cursor-2]" {
		t.Fatalf("cursors=%v", gotCursors)
	}
	if fmt.Sprint(gotBatchSizes) != "[100 1 2]" {
		t.Fatalf("batch sizes=%v", gotBatchSizes)
	}
	var fans []store.MpFan
	if err := srv.Store.DB().Where("tenant_id = ? AND account_id = ?", 1, accountID).Find(&fans).Error; err != nil {
		t.Fatalf("load fans: %v", err)
	}
	if len(fans) != 103 || fans[0].Tags != "7,8" {
		t.Fatalf("fans count=%d first=%+v", len(fans), fans[0])
	}
	w = doAuthed(router, http.MethodGet, path, "", 1)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "cursor-2") || strings.Contains(w.Body.String(), "page-one") {
		t.Fatalf("status leaked cursor/buffer: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestFanSyncRoutesRequireMethodSpecificPermissions(t *testing.T) {
	_, router := newAdminServer(t)
	accountID := createAccount(t, router, "wxFANPERM", "权限号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/fans/sync", accountID)

	if w := doFanSyncWithPermission(router, http.MethodGet, path, "social:read"); w.Code != http.StatusOK {
		t.Fatalf("read permission GET code=%d body=%s", w.Code, w.Body.String())
	}
	if w := doFanSyncWithPermission(router, http.MethodGet, path, "social:write"); w.Code != http.StatusForbidden {
		t.Fatalf("write-only GET code=%d body=%s", w.Code, w.Body.String())
	}
	if w := doFanSyncWithPermission(router, http.MethodPost, path, "social:read"); w.Code != http.StatusForbidden {
		t.Fatalf("read-only POST code=%d body=%s", w.Code, w.Body.String())
	}
	if w := doFanSyncWithPermission(router, http.MethodPost, path, "social:write"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("write permission POST code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestFanSyncWorkerRetriesTransientBatchFailure(t *testing.T) {
	failedOnce := false
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/user/get":
			_, _ = w.Write(fanListPayload(1, []string{"retry-openid"}, ""))
		case "/cgi-bin/user/info/batchget":
			if !failedOnce {
				failedOnce = true
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"errcode":0}`))
				return
			}
			_, _ = w.Write(fanProfilesPayload([]string{"retry-openid"}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	srv, router := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, router, "wxFANRETRY", "重试号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/fans/sync", accountID)
	if w := doAuthed(router, http.MethodPost, path, "{}", 1); w.Code != http.StatusAccepted {
		t.Fatalf("enqueue code=%d body=%s", w.Code, w.Body.String())
	}
	makeFanSyncDue(t, srv)
	if err := srv.ReconcileFanSyncJobs(context.Background(), 1); err != nil {
		t.Fatalf("stage page: %v", err)
	}
	makeFanSyncDue(t, srv)
	if err := srv.ReconcileFanSyncJobs(context.Background(), 1); err == nil {
		t.Fatal("expected first batch failure")
	}
	job, _ := srv.Store.LatestFanSyncJob(context.Background(), 1, accountID)
	if job.Status != "pending" || job.Attempts != 1 || job.PageOffset != 0 || job.LastError == "" {
		t.Fatalf("retry job=%+v", job)
	}
	job = runFanSyncUntilTerminal(t, srv, 1, accountID)
	if job.Status != "succeeded" || job.SyncedCount != 1 {
		t.Fatalf("recovered job=%+v", job)
	}
}

func TestFanSyncWorkerRejectsPrematureTerminalCursor(t *testing.T) {
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/user/get":
			_, _ = w.Write(fanListPayload(2, []string{"only-one"}, ""))
		case "/cgi-bin/user/info/batchget":
			_, _ = w.Write(fanProfilesPayload([]string{"only-one"}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	srv, router := newAdminServer(t)
	srv.WX = testWXClient(wxServer.URL)
	accountID := createAccount(t, router, "wxFANSHORT", "提前结束号", 1)
	path := fmt.Sprintf("/api/v1/mp/accounts/%d/fans/sync", accountID)
	if w := doAuthed(router, http.MethodPost, path, "{}", 1); w.Code != http.StatusAccepted {
		t.Fatalf("enqueue code=%d body=%s", w.Code, w.Body.String())
	}
	makeFanSyncDue(t, srv)
	if err := srv.ReconcileFanSyncJobs(context.Background(), 1); err != nil {
		t.Fatalf("stage page: %v", err)
	}
	makeFanSyncDue(t, srv)
	if err := srv.ReconcileFanSyncJobs(context.Background(), 1); !errors.Is(err, store.ErrFanSyncIncomplete) {
		t.Fatalf("reconcile err=%v, want incomplete", err)
	}
	job, err := srv.Store.LatestFanSyncJob(context.Background(), 1, accountID)
	if err != nil || job.Status != "failed" || job.ActiveKey != nil || !strings.Contains(job.LastError, "请重新同步") {
		t.Fatalf("failed job=%+v err=%v", job, err)
	}
}
