// Package notifyclient 经 HTTP 调站内信服务的 internal send 端点发通知
// （X-Internal-Token，内网直连）。base 或 token 未配置时 Enabled()=false，
// 调用方静默跳过，不阻断审批主流程（脚手架默认无通知服务，两者留空即禁用）。
package notifyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string) *Client {
	return &Client{
		base:  strings.TrimRight(strings.TrimSpace(base), "/"),
		token: strings.TrimSpace(token),
		http:  &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.base != "" && c.token != ""
}

type SendInput struct {
	TenantID     uint64            `json:"tenant_id"`
	UserID       uint64            `json:"user_id,omitempty"`
	UserIDs      []uint64          `json:"user_ids,omitempty"`
	TemplateCode string            `json:"template_code"`
	Type         string            `json:"type,omitempty"`
	RefType      string            `json:"ref_type,omitempty"`
	RefID        string            `json:"ref_id,omitempty"`
	Vars         map[string]string `json:"vars,omitempty"`
	Title        string            `json:"title,omitempty"`
	Content      string            `json:"content,omitempty"`
	Link         string            `json:"link,omitempty"`
}

type SendResult struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) Send(ctx context.Context, in SendInput) (*SendResult, error) {
	if !c.Enabled() {
		return &SendResult{}, nil
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/api/v1/notify/internal/send", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("notify send HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.Code != 0 && env.Code != 200 {
		return nil, fmt.Errorf("notify send: %s", env.Message)
	}
	var out SendResult
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &out)
	}
	return &out, nil
}
