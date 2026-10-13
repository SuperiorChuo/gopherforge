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
	"fmt"
	"log"
	"net/http"
	"time"

	notifyv1 "github.com/go-admin-kit/services/api/gen/notify/v1"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/internalhttp"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
)

type Client struct {
	http     *internalhttp.Client
	resolver *grpcx.Resolver
	pool     *grpcx.ConnPool     // Phase 4 收尾：gRPC 连接复用
	breaker  *resilience.Breaker // Phase 4：gRPC 路径熔断器
}

func New(base, token string) *Client {
	// Consul 解析器懒建（不连接不失败）。
	var resolver *grpcx.Resolver
	if r, err := grpcx.NewResolver(""); err == nil {
		resolver = r
	} else {
		log.Printf("notifyclient: consul resolver 初始化失败（gRPC 路径禁用，仅 HTTP 回退）: %v", err)
	}
	return &Client{
		http: internalhttp.New(base, token, internalhttp.Options{
			Timeout: 8 * time.Second,
		}),
		resolver: resolver,
		pool:     grpcx.NewConnPool(resolver, "notify-service"),
		breaker:  resilience.NewBreaker(5, 10*time.Second),
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.http.Enabled()
}

var errNoResolver = &noResolverError{}

type noResolverError struct{}

func (*noResolverError) Error() string { return "notifyclient: no consul resolver" }

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
	// Phase 3：gRPC 优先（Consul 发现 notify-service），失败回退 HTTP。
	if resp, err := c.sendGRPC(ctx, in); err == nil {
		return &SendResult{Created: int(resp.GetCreated()), Skipped: int(resp.GetSkipped())}, nil
	}
	var out SendResult
	if err := c.http.DoJSON(ctx, http.MethodPost,
		"/api/v1/notify/internal/send", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// sendGRPC 经 Consul 发现 + gRPC 调 notify InternalSend。
// Phase 4：套 resilience——重试（仅瞬时故障）+ 熔断 + 8s 总超时（对齐 HTTP 超时）。
func (c *Client) sendGRPC(ctx context.Context, in SendInput) (*notifyv1.InternalSendResponse, error) {
	if c.resolver == nil {
		return nil, errNoResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return (resilience.Options{
		MaxRetries:     2,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: c.breaker,
	}).DoResult(ctx, func(ctx context.Context) (*notifyv1.InternalSendResponse, error) {
		conn, err := c.pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := notifyv1.NewNotifyServiceClient(conn).InternalSend(ctx, &notifyv1.InternalSendRequest{
			TenantId:     in.TenantID,
			UserId:       in.UserID,
			UserIds:      in.UserIDs,
			TemplateCode: in.TemplateCode,
			Type:         in.Type,
			RefType:      in.RefType,
			RefId:        in.RefID,
			Vars:         in.Vars,
			Title:        in.Title,
			Content:      in.Content,
			Link:         in.Link,
		})
		if err == nil {
			return resp, nil
		}
		if grpcx.Retryable(err) {
			c.pool.Invalidate()
			return nil, err
		}
		if grpcx.IsTransient(err) {
			return nil, fmt.Errorf("%w: %w", resilience.ErrTransient, err)
		}
		return nil, fmt.Errorf("%w: %w", resilience.ErrNoRetry, err)
	})
}
