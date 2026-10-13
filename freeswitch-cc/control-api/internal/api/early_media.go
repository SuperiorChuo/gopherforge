package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const earlyMediaFSDir = "/var/lib/freeswitch/early-media"

type earlyMediaJob struct {
	cancel context.CancelFunc
	rootID string
	voice  string
	tenant string
}

type earlyMediaState struct {
	mu   sync.Mutex
	jobs map[string]*earlyMediaJob
	seen map[string]struct{}
}

// HandleEarlyMedia 只消费拨号侧显式导出的 cc_early_media=1。
// CHANNEL_ANSWER/HANGUP 会取消 ASR 请求，确保接通后音频不送识别。
func (s *Server) HandleEarlyMedia(ev map[string]string) {
	if s == nil || strings.TrimSpace(s.Cfg.EarlyMediaURL) == "" {
		return
	}
	uuid := ev["Unique-ID"]
	if !guardUUIDRe.MatchString(uuid) {
		return
	}
	s.ensureEarlyState()
	switch ev["Event-Name"] {
	case "CHANNEL_ANSWER":
		s.cancelEarlyMedia(uuid, false)
		return
	case "CHANNEL_HANGUP_COMPLETE":
		s.cancelEarlyMedia(uuid, true)
		return
	case "CHANNEL_PROGRESS_MEDIA":
		if ev["variable_cc_early_media"] != "1" {
			return
		}
	default:
		return
	}

	s.earlyMedia.mu.Lock()
	if _, ok := s.earlyMedia.seen[uuid]; ok {
		s.earlyMedia.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &earlyMediaJob{
		cancel: cancel,
		rootID: earlyFirstNonEmpty(ev["Channel-Call-UUID"], uuid),
		voice:  ev["variable_cc_voice_id"],
		tenant: ev["variable_cc_tenant_id"],
	}
	s.earlyMedia.seen[uuid] = struct{}{}
	s.earlyMedia.jobs[uuid] = job
	s.earlyMedia.mu.Unlock()
	go s.sampleEarlyMedia(ctx, uuid, job)
}

func (s *Server) ensureEarlyState() {
	if s.earlyMedia == nil {
		s.earlyMedia = &earlyMediaState{jobs: map[string]*earlyMediaJob{}, seen: map[string]struct{}{}}
	}
}

func (s *Server) cancelEarlyMedia(uuid string, forget bool) {
	s.earlyMedia.mu.Lock()
	job := s.earlyMedia.jobs[uuid]
	delete(s.earlyMedia.jobs, uuid)
	if forget {
		delete(s.earlyMedia.seen, uuid)
	}
	s.earlyMedia.mu.Unlock()
	if job != nil {
		job.cancel()
	}
}

func (s *Server) sampleEarlyMedia(ctx context.Context, uuid string, job *earlyMediaJob) {
	seconds := s.Cfg.EarlyMediaSeconds
	if seconds < 2 {
		seconds = 2
	}
	if seconds > 8 {
		seconds = 8
	}
	if err := os.MkdirAll(s.Cfg.EarlyMediaDir, 0o750); err != nil {
		log.Printf("early media: mkdir: %v", err)
		s.finishEarlyMedia(uuid, job)
		return
	}
	name := "early-" + strings.ToLower(uuid) + ".wav"
	localPath := filepath.Join(s.Cfg.EarlyMediaDir, name)
	fsPath := filepath.Join(earlyMediaFSDir, name)
	defer func() {
		_ = os.Remove(localPath)
		s.finishEarlyMedia(uuid, job)
	}()

	if err := s.runEarlyCommand("uuid_record " + uuid + " start " + fsPath); err != nil {
		log.Printf("early media: start uuid=%s: %v", uuid, err)
		return
	}
	sampleDuration := time.Duration(seconds) * time.Second
	if s.earlySampleDuration > 0 {
		sampleDuration = s.earlySampleDuration
	}
	completed := false
	select {
	case <-ctx.Done():
	case <-time.After(sampleDuration):
		completed = true
	}
	_ = s.runEarlyCommand("uuid_record " + uuid + " stop " + fsPath)
	if !completed || ctx.Err() != nil {
		return
	}
	raw, err := readSettledWAV(ctx, localPath)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("early media: read uuid=%s: %v", uuid, err)
		}
		return
	}
	// 样本读入有界内存后先删卷文件，再发 ASR；不让外部请求时延延长留存。
	_ = os.Remove(localPath)
	class, blocked, err := s.classifyEarlyMedia(ctx, raw)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("early media: classify uuid=%s: %v", uuid, err)
		}
		return // ASR/中台异常 fail-open
	}
	if !blocked || !validEarlyClass(class) || ctx.Err() != nil {
		return
	}
	payload := map[string]any{"call_id": job.rootID, "class": class}
	if id, err := strconv.ParseUint(job.voice, 10, 64); err == nil && id > 0 {
		payload["voice_id"] = id
	}
	if id, err := strconv.ParseUint(job.tenant, 10, 64); err == nil && id > 0 {
		payload["tenant_id"] = id
	}
	if s.Hook != nil && s.Hook.Enabled() {
		if err := s.Hook.Send("call.early_media", payload); err != nil {
			log.Printf("early media: webhook uuid=%s: %v", uuid, err)
		}
	}
	log.Printf("early media: block uuid=%s class=%s", uuid, class)
	if s.killChannel != nil {
		_ = s.killChannel(uuid)
		return
	}
	if s.ESL != nil {
		_, _ = s.ESL.API("uuid_kill " + uuid + " CALL_REJECTED")
	}
}

func (s *Server) finishEarlyMedia(uuid string, job *earlyMediaJob) {
	s.earlyMedia.mu.Lock()
	if s.earlyMedia.jobs[uuid] == job {
		delete(s.earlyMedia.jobs, uuid)
	}
	s.earlyMedia.mu.Unlock()
	// 正常会由 HANGUP 清 seen；异常缺事件时有界回收，避免常驻增长。
	time.AfterFunc(2*time.Minute, func() {
		s.earlyMedia.mu.Lock()
		delete(s.earlyMedia.seen, uuid)
		s.earlyMedia.mu.Unlock()
	})
}

func (s *Server) runEarlyCommand(cmd string) error {
	if s.earlyCommand != nil {
		return s.earlyCommand(cmd)
	}
	if s.ESL == nil {
		return fmt.Errorf("ESL unavailable")
	}
	out, err := s.ESL.API(cmd)
	if err != nil {
		return err
	}
	if strings.HasPrefix(out, "-ERR") {
		return fmt.Errorf("%s", out)
	}
	return nil
}

func readSettledWAV(ctx context.Context, path string) ([]byte, error) {
	for i := 0; i < 10; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		f, err := os.Open(path)
		if err == nil {
			raw, readErr := io.ReadAll(io.LimitReader(f, (2<<20)+1))
			_ = f.Close()
			if len(raw) > 2<<20 {
				return nil, fmt.Errorf("sample exceeds 2 MiB")
			}
			if readErr == nil && len(raw) >= 44 {
				return raw, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("sample not settled")
}

func (s *Server) classifyEarlyMedia(ctx context.Context, wav []byte) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Cfg.EarlyMediaURL, bytes.NewReader(wav))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "audio/wav")
	if s.Cfg.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(s.Cfg.WebhookSecret))
		_, _ = mac.Write(wav)
		req.Header.Set("X-CC-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	client := s.earlyHTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return "", false, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var env struct {
		Data struct {
			Class   string `json:"class"`
			Blocked bool   `json:"blocked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", false, err
	}
	return env.Data.Class, env.Data.Blocked, nil
}

func validEarlyClass(class string) bool {
	return class == "empty" || class == "unavailable" || class == "busy"
}

func earlyFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
