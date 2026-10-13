package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"gorm.io/gorm"
)

// 录音转写质检：
//   - transcribe：校验通话归属（本租户 + 有录音）→ 入队 cc_transcribe_jobs →
//     202 + job id 返回。实际转写由 qc_worker.go 的后台 loop 串行消费：
//     control-api 录音流经 io.Pipe 直接转发 OpenAI 兼容
//     /v1/audio/transcriptions（不缓冲整段 wav）→ 存转写 → 跑关键词质检。
//   - jobs 查询（按 job id 或 call_id）、关键词库 CRUD、转写单查、质检结果列表。
//
// ASR 未配置（CC_ASR_URL 空）时 transcribe 直接 400（不入队一堆必失败的任务）。

var (
	qcCallIDRe  = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)
	qcRecFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\.wav$`)
	qcWordRe    = regexp.MustCompile(`^\S.{0,126}\S$|^\S$`) // 1-128 非纯空白
)

const (
	// transcribeTimeout 单次转写（拉录音 + ASR）的总超时。
	transcribeTimeout = 5 * time.Minute
	// maxRecordingBytes 单个录音的转发上限（流式，不进内存；仅作异常闸门）。
	maxRecordingBytes int64 = 200 << 20
	// transcribeMaxAttempts 单任务最多尝试次数（首次 + 2 次重试）。
	transcribeMaxAttempts = 3
	// transcribeQueueCap 待处理 + 在跑任务数上限，超过即 429，防止把 ASR
	// 和本进程排到天荒地老。
	transcribeQueueCap = 200
)

// QcHit 质检命中明细（存 CCQcResult.Hits 的 JSON 元素）。
type QcHit struct {
	Word  string `json:"word"`
	Type  string `json:"type"` // forbidden | required
	Count int    `json:"count"`
}

type qcTranscribeReq struct {
	CallID string `json:"call_id"`
}

// QcTranscribe handles POST /api/v1/cc/qc/transcribe — 只入队，202 返回 job id。
// 归属校验保留：通话必须属于调用方租户（GetCall 带 tenant_id 条件），录音文件名
// 仍按白名单正则校验后才落进任务行。
func (s *Server) QcTranscribe(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	sp := s.Speech.Settings(c.Request.Context())
	if sp.ASRURL == "" {
		fail(c, http.StatusBadRequest, "ASR 未配置，请在 .env 配置 CC_ASR_*")
		return
	}
	if !s.FS.Enabled() {
		fail(c, http.StatusBadRequest, "CC_FS_API_BASE is not configured")
		return
	}
	var req qcTranscribeReq
	if err := c.ShouldBindJSON(&req); err != nil || !qcCallIDRe.MatchString(req.CallID) {
		fail(c, http.StatusBadRequest, "invalid call_id")
		return
	}

	// 查 Call 拿录音文件名（取 basename，走录音流式代理）
	call, err := s.Store.GetCall(c.Request.Context(), agent.TenantID, req.CallID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "call not found")
			return
		}
		fail(c, http.StatusInternalServerError, "load call failed")
		return
	}
	if call.Recording == "" {
		fail(c, http.StatusBadRequest, "该通话无录音，无法转写")
		return
	}
	recFile := filepath.Base(call.Recording)
	if !qcRecFileRe.MatchString(recFile) {
		fail(c, http.StatusBadRequest, "录音文件名非法")
		return
	}

	// 入队：转写本身由后台 worker 做（同步做会把整段 wav 读进内存并占住请求线程）。
	job, created, err := s.enqueueTranscribe(c.Request.Context(), agent.TenantID, req.CallID, recFile, agent.UserID)
	if err != nil {
		if errors.Is(err, store.ErrTranscribeQueueFull) {
			fail(c, http.StatusTooManyRequests, "转写队列已满，请稍后再试")
			return
		}
		fail(c, http.StatusInternalServerError, "转写任务入队失败")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"code": http.StatusAccepted, "message": "accepted", "data": gin.H{
		"job_id":  job.ID,
		"call_id": job.CallID,
		"status":  job.Status,
		"created": created, // false = 复用了已在进行中的同通话任务
	}})
}

// QcTranscribeJob handles GET /api/v1/cc/qc/transcribe/jobs/:id — 查任务状态。
// 完成时带上转写文本与质检结论，前端一次轮询即可拿到最终结果。
func (s *Server) QcTranscribeJob(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	job, err := s.Store.GetTranscribeJob(c.Request.Context(), agent.TenantID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "job not found")
			return
		}
		fail(c, http.StatusInternalServerError, "load job failed")
		return
	}
	s.respondTranscribeJob(c, agent.TenantID, job)
}

// QcTranscribeJobByCall handles GET /api/v1/cc/qc/transcribe/jobs?call_id= —
// 按通话查最近一次任务（前端不必记 job id）。
func (s *Server) QcTranscribeJobByCall(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	callID := c.Query("call_id")
	if !qcCallIDRe.MatchString(callID) {
		fail(c, http.StatusBadRequest, "invalid call_id")
		return
	}
	job, err := s.Store.GetTranscribeJobByCall(c.Request.Context(), agent.TenantID, callID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "尚无转写任务")
			return
		}
		fail(c, http.StatusInternalServerError, "load job failed")
		return
	}
	s.respondTranscribeJob(c, agent.TenantID, job)
}

// respondTranscribeJob 统一任务响应：done 时附带转写文本与质检得分/命中明细。
func (s *Server) respondTranscribeJob(c *gin.Context, tenantID uint64, job *store.CCTranscribeJob) {
	resp := gin.H{"job": job}
	if job.Status == store.TranscribeStatusDone {
		if t, err := s.Store.GetTranscript(c.Request.Context(), tenantID, job.CallID); err == nil {
			resp["text"] = t.Text
			resp["segments"] = t.Segments
		}
		if r, err := s.Store.GetQcResult(c.Request.Context(), tenantID, job.CallID); err == nil {
			resp["score"] = r.Score
			var hits []QcHit
			if r.Hits != "" {
				_ = json.Unmarshal([]byte(r.Hits), &hits)
			}
			resp["hits"] = hits
		}
	}
	ok(c, resp)
}

// runQc 遍历关键词库对 text 数命中，算分并存质检结论。
// 评分规则：满分 100；每个违禁词命中一次 -20；每个必说词缺失（未出现）-10；下限 0。
func (s *Server) runQc(ctx context.Context, tenantID uint64, callID, text string) (int, []QcHit, error) {
	keywords, err := s.Store.ListKeywords(ctx, tenantID)
	if err != nil {
		return 0, nil, err
	}
	score := 100
	hits := make([]QcHit, 0)
	for _, k := range keywords {
		count := strings.Count(text, k.Word)
		switch k.Type {
		case "forbidden":
			if count > 0 {
				score -= 20 * count
				hits = append(hits, QcHit{Word: k.Word, Type: k.Type, Count: count})
			}
		case "required":
			if count == 0 {
				score -= 10
				hits = append(hits, QcHit{Word: k.Word, Type: k.Type, Count: 0})
			} else {
				hits = append(hits, QcHit{Word: k.Word, Type: k.Type, Count: count})
			}
		}
	}
	if score < 0 {
		score = 0
	}
	hitsJSON, _ := json.Marshal(hits)
	if err := s.Store.UpsertQcResult(ctx, &store.CCQcResult{
		TenantID: tenantID, CallID: callID, Score: score, Hits: string(hitsJSON),
	}); err != nil {
		return 0, nil, err
	}
	return score, hits, nil
}

// parseASRResponse 解析 OpenAI 兼容 /v1/audio/transcriptions 的响应，
// 返回 (text, segmentsJSON)。响应体是文本（≤10MB），全量读取无内存风险。
func parseASRResponse(resp *http.Response) (string, string, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("ASR provider status %d: %s", resp.StatusCode, truncateErr(data))
	}
	var parsed struct {
		Text     string            `json:"text"`
		Segments []json.RawMessage `json:"segments"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", "", fmt.Errorf("ASR 返回解析失败: %w", err)
	}
	if strings.TrimSpace(parsed.Text) == "" {
		return "", "", fmt.Errorf("ASR 返回空文本")
	}
	var segments string
	if len(parsed.Segments) > 0 {
		if raw, err := json.Marshal(parsed.Segments); err == nil {
			segments = string(raw)
		}
	}
	return parsed.Text, segments, nil
}

// QcTranscript handles GET /api/v1/cc/qc/transcripts?call_id=.
func (s *Server) QcTranscript(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	callID := c.Query("call_id")
	if !qcCallIDRe.MatchString(callID) {
		fail(c, http.StatusBadRequest, "invalid call_id")
		return
	}
	t, err := s.Store.GetTranscript(c.Request.Context(), agent.TenantID, callID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "尚无转写")
			return
		}
		fail(c, http.StatusInternalServerError, "load transcript failed")
		return
	}
	ok(c, t)
}

// QcResults handles GET /api/v1/cc/qc/results?page=&page_size=.
func (s *Server) QcResults(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, total, err := s.Store.ListQcResults(c.Request.Context(), agent.TenantID, page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list qc results failed")
		return
	}
	ok(c, gin.H{"list": rows, "total": total, "page": page, "page_size": pageSize})
}

// QcKeywords handles GET /api/v1/cc/qc/keywords.
func (s *Server) QcKeywords(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	list, err := s.Store.ListKeywords(c.Request.Context(), agent.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list keywords failed")
		return
	}
	ok(c, gin.H{"list": list, "total": len(list)})
}

type qcKeywordReq struct {
	Word   string `json:"word"`
	Type   string `json:"type"`
	Remark string `json:"remark"`
}

// QcCreateKeyword handles POST /api/v1/cc/qc/keywords.
func (s *Server) QcCreateKeyword(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	var req qcKeywordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	req.Word = strings.TrimSpace(req.Word)
	if !qcWordRe.MatchString(req.Word) {
		fail(c, http.StatusBadRequest, "关键词需 1-128 字符且非纯空白")
		return
	}
	if req.Type != "forbidden" && req.Type != "required" {
		fail(c, http.StatusBadRequest, "type must be forbidden or required")
		return
	}
	k := &store.CCQcKeyword{
		TenantID: agent.TenantID, Word: req.Word, Type: req.Type, Remark: strings.TrimSpace(req.Remark),
	}
	if err := s.Store.CreateKeyword(c.Request.Context(), k); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "UNIQUE") {
			fail(c, http.StatusConflict, "关键词已存在")
			return
		}
		fail(c, http.StatusInternalServerError, "create keyword failed")
		return
	}
	ok(c, k)
}

// QcDeleteKeyword handles DELETE /api/v1/cc/qc/keywords/:id.
func (s *Server) QcDeleteKeyword(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.DeleteKeyword(c.Request.Context(), agent.TenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusNotFound, "keyword not found")
			return
		}
		fail(c, http.StatusInternalServerError, "delete keyword failed")
		return
	}
	ok(c, gin.H{"ok": true})
}
