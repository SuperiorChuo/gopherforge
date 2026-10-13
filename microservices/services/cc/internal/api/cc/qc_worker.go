package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/go-admin-kit/services/cc/internal/config"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
)

// 录音转写异步 worker。
//
// 历史实现是同步的：请求线程 io.ReadAll 整段 wav（上限 100MB）进内存，再作为
// multipart 二次缓冲发给 ASR，等最多 120s。服务内存限额 256M，两个并发请求就
// OOM。现在：
//   - HTTP 侧只入队（cc_transcribe_jobs）+ 202 返回 job id；
//   - 本 loop 串行消费（TranscribeConcurrency 上限，默认 1）；
//   - 传输用 io.Pipe 把「媒体面 → ASR」直接对接，multipart 边写边发，
//     进程常驻内存与录音大小解耦（只占 32KB 级 copy 缓冲）。

// TranscribeWorkerOptions 转写 worker 配置。
type TranscribeWorkerOptions struct {
	// Concurrency 同时在跑的转写数上限（默认 1）。内存/ASR 配额的总闸门。
	Concurrency int
	// PollInterval 队列空时的轮询间隔（默认 5s）。
	PollInterval time.Duration
	// StaleAfter running 任务超过此时长即视为中断（进程被 kill / 卡死），
	// 退回 pending 重排；默认 15 分钟（> ASR 单次超时 120s，留足余量）。
	StaleAfter time.Duration
}

func (o TranscribeWorkerOptions) normalized() TranscribeWorkerOptions {
	if o.Concurrency <= 0 {
		o.Concurrency = 1
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 5 * time.Second
	}
	if o.StaleAfter <= 0 {
		o.StaleAfter = 15 * time.Minute
	}
	return o
}

// RunTranscribeLoop 转写任务消费循环（main.go 起一个 goroutine）。
// 启动先做一次 stale 回收——上次进程退出时 running 的任务不会永久悬挂。
func (s *Server) RunTranscribeLoop(ctx context.Context, opts TranscribeWorkerOptions) {
	opts = opts.normalized()
	log.Printf("cc transcribe worker: enabled concurrency=%d poll=%s stale_after=%s",
		opts.Concurrency, opts.PollInterval, opts.StaleAfter)

	// 进程重启后的第一次回收：把上一条命留下的 running 任务捞回来。
	s.requeueStale(ctx, opts.StaleAfter)

	slots := make(chan struct{}, opts.Concurrency)
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()
	staleTicker := time.NewTicker(opts.StaleAfter)
	defer staleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-staleTicker.C:
			s.requeueStale(ctx, opts.StaleAfter)
			continue
		case <-ticker.C:
		}
		// 一轮把队列抽干（受 slots 限流），空了就等下一个 tick。
		for {
			select {
			case <-ctx.Done():
				return
			case slots <- struct{}{}:
			}
			job, err := s.Store.ClaimTranscribeJob(ctx)
			if err != nil {
				<-slots
				log.Printf("cc transcribe worker: claim failed: %v", err)
				break
			}
			if job == nil {
				<-slots
				break
			}
			go func(job *store.CCTranscribeJob) {
				defer func() { <-slots }()
				start := time.Now()
				err := s.processTranscribeJob(ctx, job)
				jobbeat.Report(s.Store.DB(), jobbeat.Run{
					Key: "cc.transcribe", Service: "cc-service",
					Description: "录音转写质检异步任务", IntervalSec: 0,
					StartedAt: start, Err: err,
				})
			}(job)
		}
	}
}

func (s *Server) requeueStale(ctx context.Context, staleAfter time.Duration) {
	requeued, failed, err := s.Store.RequeueStaleTranscribeJobs(ctx, staleAfter)
	if err != nil {
		log.Printf("cc transcribe worker: requeue stale failed: %v", err)
		return
	}
	if requeued > 0 || failed > 0 {
		log.Printf("cc transcribe worker: stale jobs requeued=%d failed=%d", requeued, failed)
	}
}

// processTranscribeJob 跑一条任务：流式拉录音 → 流式转发 ASR → 存转写 → 跑质检。
// 失败按 attempts 退回 pending 重试或判 failed（都留档到 cc_transcripts）。
func (s *Server) processTranscribeJob(ctx context.Context, job *store.CCTranscribeJob) error {
	sp := s.Speech.Settings(ctx)
	if sp.ASRURL == "" {
		return s.failJob(ctx, job, errors.New("ASR 未配置（CC_ASR_URL 为空）"))
	}
	if !s.FS.Enabled() {
		return s.failJob(ctx, job, errors.New("CC_FS_API_BASE 未配置，无法拉取录音"))
	}

	text, segments, err := s.transcribeRecording(ctx, sp, job.RecordingFile)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	if err := s.Store.UpsertTranscript(ctx, &store.CCTranscript{
		TenantID: job.TenantID, CallID: job.CallID, Text: text, Segments: segments,
		Status: "done",
	}); err != nil {
		return s.failJob(ctx, job, fmt.Errorf("保存转写失败: %w", err))
	}
	score, _, err := s.runQc(ctx, job.TenantID, job.CallID, text)
	if err != nil {
		return s.failJob(ctx, job, fmt.Errorf("质检失败: %w", err))
	}
	if err := s.Store.FinishTranscribeJob(ctx, job.ID, score); err != nil {
		return err
	}
	log.Printf("cc transcribe: job=%d tenant=%d call=%s done score=%d", job.ID, job.TenantID, job.CallID, score)
	return nil
}

// failJob 留档失败原因（任务行 + cc_transcripts），并按重试策略回队或判死。
// 返回原始错误供心跳上报（error 状态）。
func (s *Server) failJob(ctx context.Context, job *store.CCTranscribeJob, cause error) error {
	willRetry, err := s.Store.FailTranscribeJob(ctx, job.ID, cause.Error())
	if err != nil {
		log.Printf("cc transcribe: job=%d mark failed error: %v", job.ID, err)
	}
	// 终态才覆盖转写留档；重试中保留 pending 语义，避免前端看到闪烁的 failed。
	if !willRetry {
		_ = s.Store.UpsertTranscript(ctx, &store.CCTranscript{
			TenantID: job.TenantID, CallID: job.CallID, Status: "failed",
			Error: truncateErr([]byte(cause.Error())),
		})
	}
	log.Printf("cc transcribe: job=%d call=%s failed (retry=%v): %v", job.ID, job.CallID, willRetry, cause)
	return cause
}

// transcribeRecording 把「control-api 录音流」经 io.Pipe 直接接到 ASR 的
// multipart 请求体上：录音字节不落内存也不落盘，只过 io.Copy 的固定缓冲。
// 返回 (text, segmentsJSON)。
func (s *Server) transcribeRecording(ctx context.Context, sp config.SpeechSettings, recFile string) (string, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, transcribeTimeout)
	defer cancel()

	resp, err := s.FS.Stream(reqCtx, "/v1/recordings/"+recFile)
	if err != nil {
		return "", "", fmt.Errorf("拉取录音失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("录音不可用（可能已清理），control-api status %d", resp.StatusCode)
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	// 生产侧：多段表单边写边被 HTTP client 读走。录音超过 maxRecordingBytes
	// 视为异常（媒体面串流没有 Content-Length 时的兜底闸门），直接掐断。
	go func() {
		var copied int64
		err := func() error {
			part, err := mw.CreateFormFile("file", recFile)
			if err != nil {
				return err
			}
			copied, err = io.Copy(part, io.LimitReader(resp.Body, maxRecordingBytes+1))
			if err != nil {
				return fmt.Errorf("读取录音失败: %w", err)
			}
			if copied > maxRecordingBytes {
				return fmt.Errorf("录音超过 %d MB 上限，拒绝转写", maxRecordingBytes>>20)
			}
			if copied == 0 {
				return errors.New("录音为空（可能已清理）")
			}
			if err := mw.WriteField("model", sp.ASRModel); err != nil {
				return err
			}
			// 让 verbose_json 带回 segments（provider 不支持时忽略即可）
			_ = mw.WriteField("response_format", "verbose_json")
			return mw.Close()
		}()
		// CloseWithError(nil) 等价于正常 EOF；出错则让 ASR 请求带错终止。
		_ = pw.CloseWithError(err)
	}()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		sp.ASRURL+"/v1/audio/transcriptions", pr)
	if err != nil {
		_ = pr.CloseWithError(err)
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	if sp.ASRKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+sp.ASRKey)
	}
	asrResp, err := (&http.Client{Timeout: transcribeTimeout}).Do(httpReq)
	if err != nil {
		_ = pr.CloseWithError(err)
		return "", "", err
	}
	defer func() { _ = asrResp.Body.Close() }()
	return parseASRResponse(asrResp)
}

// enqueueTranscribe 入队一条任务（供 API 层调用）。归属校验由调用方完成。
func (s *Server) enqueueTranscribe(ctx context.Context, tenantID uint64, callID, recFile string, userID uint64) (*store.CCTranscribeJob, bool, error) {
	return s.Store.EnqueueTranscribeJob(ctx, tenantID, callID, recFile, userID,
		transcribeMaxAttempts, transcribeQueueCap)
}
