package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SystemSMS delegates provider-specific delivery and audit logging to system-service.
type SystemSMS struct {
	URL    string
	Token  string
	Client *http.Client
}

func NewSystemSMS(url, token string, client *http.Client) *SystemSMS {
	return &SystemSMS{URL: strings.TrimSpace(url), Token: strings.TrimSpace(token), Client: client}
}

func (s *SystemSMS) Name() string { return "system" }

func (s *SystemSMS) Enabled() bool {
	return s != nil && s.URL != "" && s.Token != ""
}

func (s *SystemSMS) Send(ctx context.Context, phone string, p Payload) error {
	if !s.Enabled() {
		return fmt.Errorf("system sms is not configured")
	}
	if strings.TrimSpace(phone) == "" || p.TenantID == 0 || strings.TrimSpace(p.TemplateCode) == "" {
		return fmt.Errorf("system sms requires phone, tenant_id, and template_code")
	}
	body, err := json.Marshal(map[string]any{
		"tenant_id": p.TenantID, "mobile": phone, "template_code": p.TemplateCode, "params": p.Params,
	})
	if err != nil {
		return fmt.Errorf("encode system sms request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create system sms request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", s.Token)
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("call system sms: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read system sms response: %w", err)
	}
	var envelope struct {
		Message string `json:"message"`
		Data    struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode system sms response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("system sms returned %d: %s", resp.StatusCode, envelope.Message)
	}
	if envelope.Data.Status != "success" {
		return fmt.Errorf("system sms delivery failed: %s", envelope.Data.Error)
	}
	return nil
}
