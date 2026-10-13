package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

const voiceFromExt = "notify"

type createVoiceReq struct {
	To      string `json:"to"`
	Text    string `json:"text"`
	Gateway string `json:"gateway"`
	RefType string `json:"ref_type"`
	RefID   string `json:"ref_id"`
}

func (s *Server) requireInternal(c *gin.Context) bool {
	if s.InternalToken == "" {
		fail(c, http.StatusServiceUnavailable, "internal endpoint disabled")
		return false
	}
	got := c.GetHeader("X-Internal-Token")
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.InternalToken)) != 1 {
		fail(c, http.StatusUnauthorized, "invalid internal token")
		return false
	}
	return true
}

func normalizeVoiceText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("text required")
	}
	if utf8.RuneCountInString(text) > 500 {
		return "", fmt.Errorf("text too long")
	}
	return text, nil
}

func (s *Server) enqueueVoiceNotify(ctx context.Context, tenantID, operatorID uint64, req createVoiceReq) (*store.CCVoiceNotify, int, string) {
	if !fsNumberRe.MatchString(req.To) {
		return nil, http.StatusBadRequest, "invalid to"
	}
	if req.Gateway != "" && !fsGatewayNameRe.MatchString(req.Gateway) {
		return nil, http.StatusBadRequest, "invalid gateway"
	}
	text, err := normalizeVoiceText(req.Text)
	if err != nil {
		return nil, http.StatusBadRequest, err.Error()
	}
	acc, err := s.Store.GetOrCreateAccount(ctx, tenantID)
	if err != nil {
		return nil, http.StatusInternalServerError, "load billing account failed"
	}
	if acc.Status == "frozen" {
		return nil, http.StatusForbidden, "账户已冻结"
	}
	rate, err := s.Store.MatchRate(ctx, tenantID, req.To)
	if err != nil {
		return nil, http.StatusInternalServerError, "match rate failed"
	}
	var ratePerMin int64
	if rate != nil {
		ratePerMin = rate.RatePerMin
	}
	d, err := s.evaluateOutboundRisk(ctx, tenantID, voiceFromExt, req.To, ratePerMin, false)
	if err != nil {
		return nil, http.StatusInternalServerError, "risk check failed"
	}
	if !d.Allow {
		status := d.Status
		if status == 0 {
			status = http.StatusForbidden
		}
		return nil, status, d.Reason
	}
	job := &store.CCVoiceNotify{
		TenantID:   tenantID,
		To:         req.To,
		Text:       text,
		Gateway:    req.Gateway,
		OperatorID: operatorID,
		RefType:    strings.TrimSpace(req.RefType),
		RefID:      strings.TrimSpace(req.RefID),
	}
	if err := s.Store.CreateVoiceNotify(ctx, job); err != nil {
		return nil, http.StatusInternalServerError, "enqueue voice notify failed"
	}
	return job, http.StatusAccepted, ""
}

// CreateVoiceNotify handles POST /api/v1/cc/voice — 管理面入队 TTS 语音通知。
func (s *Server) CreateVoiceNotify(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	var req createVoiceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	job, status, msg := s.enqueueVoiceNotify(c.Request.Context(), agent.TenantID, agent.UserID, req)
	if job == nil {
		fail(c, status, msg)
		return
	}
	ok(c, job)
}

// ListVoiceNotifies handles GET /api/v1/cc/voice。
func (s *Server) ListVoiceNotifies(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.ListVoiceNotifies(c.Request.Context(), agent.TenantID, page, ps)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list voice notifies failed")
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

// InternalVoiceNotify handles POST /api/v1/cc/internal/voice-notify（notify → cc）。
func (s *Server) InternalVoiceNotify(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	var req createVoiceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	tenantID := uint64(1)
	if v := c.GetHeader("X-Tenant-ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil && id > 0 {
			tenantID = id
		}
	}
	if req.To == "" {
		fail(c, http.StatusBadRequest, "to required")
		return
	}
	job, status, msg := s.enqueueVoiceNotify(c.Request.Context(), tenantID, 0, req)
	if job == nil {
		fail(c, status, msg)
		return
	}
	ok(c, gin.H{"id": job.ID, "status": job.Status})
}

func (s *Server) RunVoiceNotifyLoop(ctx context.Context) {
	log.Printf("cc voice notify worker: enabled poll=3s")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	stale := time.NewTicker(10 * time.Minute)
	defer stale.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stale.C:
			if n, err := s.Store.RequeueStaleVoiceNotifies(ctx, 10*time.Minute); err != nil {
				log.Printf("cc voice notify: requeue stale: %v", err)
			} else if n > 0 {
				log.Printf("cc voice notify: requeued %d stale jobs", n)
			}
		case <-ticker.C:
		}
		for {
			job, err := s.Store.ClaimNextVoiceNotify(ctx)
			if err != nil {
				log.Printf("cc voice notify: claim: %v", err)
				break
			}
			if job == nil {
				break
			}
			s.processVoiceNotify(ctx, job)
		}
	}
}

func (s *Server) maybeEnqueueVoiceQC(ctx context.Context, rec *store.Call) {
	if rec == nil || rec.CallID == "" || rec.Recording == "" || s.Speech == nil || !s.FS.Enabled() {
		return
	}
	if s.Speech.Settings(ctx).ASRURL == "" {
		return
	}
	job, err := s.Store.GetVoiceNotifyByCallID(ctx, rec.CallID)
	if err != nil || job == nil {
		return
	}
	recFile := filepath.Base(rec.Recording)
	if !qcRecFileRe.MatchString(recFile) {
		return
	}
	if _, _, err := s.enqueueTranscribe(ctx, job.TenantID, rec.CallID, recFile, job.OperatorID); err != nil {
		log.Printf("cc voice qc: enqueue call=%s: %v", rec.CallID, err)
	}
}

func (s *Server) syncVoiceFromCall(ctx context.Context, event string, rec *store.Call, voiceID uint64) {
	if voiceID == 0 || rec == nil {
		return
	}
	var err error
	switch event {
	case "call.answered":
		_, err = s.Store.ApplyVoiceAnswered(ctx, voiceID, rec.CallID)
	case "call.hangup", "cdr.json":
		answered := rec.AnsweredAt != nil || rec.DurationSec > 0
		_, err = s.Store.ApplyVoiceHangup(ctx, voiceID, rec.CallID, rec.HangupCause, rec.DurationSec, answered)
	default:
		return
	}
	if err != nil {
		log.Printf("cc voice notify: apply %s id=%d: %v", event, voiceID, err)
	}
}

func (s *Server) processVoiceNotify(ctx context.Context, job *store.CCVoiceNotify) {
	rate, err := s.Store.MatchRate(ctx, job.TenantID, job.To)
	if err != nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "match rate failed")
		return
	}
	var ratePerMin int64
	if rate != nil {
		ratePerMin = rate.RatePerMin
	}
	d, err := s.evaluateOutboundRisk(ctx, job.TenantID, voiceFromExt, job.To, ratePerMin, true)
	if err != nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "risk check failed")
		return
	}
	if !d.Allow {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "denied", "", "", d.Reason)
		return
	}
	if !s.FS.Enabled() {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "CC_FS_API_BASE is not configured")
		return
	}
	if s.Speech == nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "TTS 未配置")
		return
	}
	sp := s.Speech.Settings(ctx)
	if sp.TTSURL == "" {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "TTS 未配置")
		return
	}
	audio, err := s.synthesizeSpeech(ctx, sp, job.Text, sp.TTSVoice)
	if err != nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "TTS 合成失败："+err.Error())
		return
	}
	wav, err := transcodeTo8kWav(ctx, audio)
	if err != nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "音频转码失败："+err.Error())
		return
	}
	name := fmt.Sprintf("vn%d.wav", job.ID)
	body, contentType, err := buildPromptMultipart(name, wav)
	if err != nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", err.Error())
		return
	}
	if _, err := s.FS.PostMultipart(ctx, "/v1/prompts", contentType, body); err != nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", "", "", "upload prompt: "+err.Error())
		return
	}
	out, err := s.FS.Post(ctx, "/v1/calls/announce", gin.H{
		"to": job.To, "file": name, "gateway": job.Gateway, "tenant_id": job.TenantID, "voice_id": job.ID,
	})
	if err != nil {
		_ = s.Store.FinishVoiceNotify(ctx, job.ID, "failed", name, "", err.Error())
		return
	}
	reply, _ := out["reply"].(string)
	_ = s.Store.FinishVoiceNotify(ctx, job.ID, "queued", name, reply, "")
}
