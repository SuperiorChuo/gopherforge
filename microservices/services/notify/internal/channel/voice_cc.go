package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CCVoiceSender 把语音通知入队到 cc-service 内部端点。空 base/token = 未启用。
type CCVoiceSender struct {
	Base   string
	Token  string
	Client *http.Client
}

func NewCCVoice(base, token string) *CCVoiceSender {
	return &CCVoiceSender{
		Base:   strings.TrimRight(strings.TrimSpace(base), "/"),
		Token:  strings.TrimSpace(token),
		Client: &http.Client{Timeout: 8 * time.Second},
	}
}

func (s *CCVoiceSender) Enabled() bool {
	return s != nil && s.Base != "" && s.Token != ""
}

func (s *CCVoiceSender) Name() string { return "cc-tts" }

func (s *CCVoiceSender) Send(ctx context.Context, phone string, p Payload) error {
	if !s.Enabled() {
		return fmt.Errorf("voice channel is not configured")
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return fmt.Errorf("empty phone")
	}
	text := strings.TrimSpace(p.Content)
	if text == "" {
		text = strings.TrimSpace(p.Title)
	}
	if text == "" {
		return fmt.Errorf("empty text")
	}
	body, err := json.Marshal(map[string]any{
		"to":       phone,
		"text":     text,
		"ref_type": p.TemplateCode,
		"ref_id":   "",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Base+"/api/v1/cc/internal/voice-notify", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", s.Token)
	if p.TenantID > 0 {
		req.Header.Set("X-Tenant-ID", strconv.FormatUint(p.TenantID, 10))
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("cc voice-notify status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}
