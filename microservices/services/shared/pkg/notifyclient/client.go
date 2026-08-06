// Package notifyclient 经 HTTP 调 notify-service internal send 发站内信
// （X-Internal-Token，内网直连 notify 容器 :8095），bpm/ticket/crm/im/cc/pay/
// visibility 共用。token 未配置时 Enabled()=false，调用方静默跳过，不阻断主流程。
//
// 传输层在 shared/pkg/internalhttp：连接池复用（裸 http.Client 用
// DefaultTransport，MaxIdleConnsPerHost 只有 2）与 5xx/429 退避重试都在那里，
// 本包只留 payload 定义与端点路径。
package notifyclient

import (
	"context"
	"net/http"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/internalhttp"
)

type Client struct {
	http *internalhttp.Client
}

func New(base, token string) *Client {
	return &Client{http: internalhttp.New(base, token, internalhttp.Options{
		Timeout: 8 * time.Second,
	})}
}

func (c *Client) Enabled() bool {
	return c != nil && c.http.Enabled()
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

func (c *Client) Send(ctx context.Context, in SendInput) (*SendResult, error) {
	if !c.Enabled() {
		return &SendResult{}, nil
	}
	var out SendResult
	if err := c.http.DoJSON(ctx, http.MethodPost,
		"/api/v1/notify/internal/send", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
