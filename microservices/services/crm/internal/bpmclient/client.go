// Package bpmclient 经 HTTP 调 bpm-service internal API（X-Internal-Token，
// 内网直连 bpm 容器 :8096）：发起审批实例 + 按业务对象反查实例。
// 模式同 notifyclient / cc 的 crmclient；base 或 token 为空即禁用。
package bpmclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	bpmv1 "github.com/go-admin-kit/services/api/gen/bpm/v1"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
)

type Client struct {
	base     string
	token    string
	http     *http.Client
	resolver *grpcx.Resolver
	pool     *grpcx.ConnPool     // Phase 4 收尾：gRPC 连接复用
	breaker  *resilience.Breaker // Phase 4：gRPC 路径熔断器
}

func New(base, token string) *Client {
	var resolver *grpcx.Resolver
	if r, err := grpcx.NewResolver(""); err == nil {
		resolver = r
	} else {
		log.Printf("bpmclient: consul resolver 初始化失败（gRPC 路径禁用，仅 HTTP 回退）: %v", err)
	}
	return &Client{
		base:     strings.TrimRight(strings.TrimSpace(base), "/"),
		token:    strings.TrimSpace(token),
		http:     &http.Client{Timeout: 10 * time.Second},
		resolver: resolver,
		pool:     grpcx.NewConnPool(resolver, "bpm-service"),
		breaker:  resilience.NewBreaker(5, 10*time.Second),
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.base != "" && c.token != ""
}

// StartInput 发起审批（bpm internal instances 契约）。
type StartInput struct {
	DefinitionKey  string          `json:"definition_key"`
	Title          string          `json:"title"`
	BizType        string          `json:"biz_type"`
	BizID          string          `json:"biz_id"`
	FormSnapshot   json.RawMessage `json:"form_snapshot"`
	Variables      json.RawMessage `json:"variables,omitempty"`
	InitiatorID    uint64          `json:"initiator_id"`
	InitiatorDept  uint64          `json:"initiator_dept,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

type StartResult struct {
	InstanceID uint64 `json:"instance_id"`
	Status     string `json:"status"`
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// startGRPC 经 Consul 发现 + gRPC 调 bpm StartInstance。
// Phase 4/5：套 resilience——熔断 + 10s 总超时（对齐 HTTP 超时）。写类
// gRPC 不重试（MaxRetries=0），但由 StartInstance 生成的幂等键会贯穿
// gRPC 与 HTTP fallback，因此服务端可安全返回首次实例；瞬时故障仍计熔断。
func (c *Client) startGRPC(ctx context.Context, tenantID uint64, in StartInput) (*bpmv1.StartInstanceResponse, error) {
	if c.resolver == nil {
		return nil, errNoResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return (resilience.Options{
		MaxRetries:     0,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: c.breaker,
	}).DoResult(ctx, func(ctx context.Context) (*bpmv1.StartInstanceResponse, error) {
		conn, err := c.pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := bpmv1.NewBpmServiceClient(conn).StartInstance(ctx, &bpmv1.StartInstanceRequest{
			TenantId:       tenantID,
			DefinitionKey:  in.DefinitionKey,
			Title:          in.Title,
			BizType:        in.BizType,
			BizId:          in.BizID,
			FormSnapshot:   in.FormSnapshot,
			Variables:      in.Variables,
			InitiatorId:    in.InitiatorID,
			InitiatorDept:  in.InitiatorDept,
			IdempotencyKey: in.IdempotencyKey,
		})
		if err == nil {
			return resp, nil
		}
		if grpcx.Retryable(err) || grpcx.IsTransient(err) {
			if grpcx.Retryable(err) {
				c.pool.Invalidate()
			}
			return nil, fmt.Errorf("%w: %w", resilience.ErrTransient, err) // 计熔断，不重试
		}
		return nil, fmt.Errorf("%w: %w", resilience.ErrNoRetry, err)
	})
}

var errNoResolver = &noResolverError{}

type noResolverError struct{}

func (*noResolverError) Error() string { return "bpmclient: no consul resolver" }

// StartInstance 发起流程实例（服务端到服务端，表单快照由 CRM 权威生成）。
// Phase 3/5：gRPC 优先（Consul 发现 bpm-service），失败回退 HTTP；两条
// 写路径复用同一幂等键。
func (c *Client) StartInstance(ctx context.Context, tenantID uint64, in StartInput) (*StartResult, error) {
	key := strings.TrimSpace(in.IdempotencyKey)
	if key == "" {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, fmt.Errorf("生成 BPM 幂等键失败: %w", err)
		}
		key = hex.EncodeToString(raw[:])
	}
	in.IdempotencyKey = key
	if resp, err := c.startGRPC(ctx, tenantID, in); err == nil {
		return &StartResult{InstanceID: resp.GetInstanceId(), Status: resp.GetStatus()}, nil
	}
	data, err := c.doWithIdempotency(ctx, http.MethodPost, "/api/v1/bpm/internal/instances", tenantID, in, key)
	if err != nil {
		return nil, err
	}
	var out StartResult
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InstancesByBiz 按 (biz_type,biz_id) 反查实例列表（原样 JSON 透传，
// CRM 详情页审批进度入口）。Phase 3 续：gRPC 优先（Consul 发现 bpm-service），失败回退 HTTP。
func (c *Client) InstancesByBiz(ctx context.Context, tenantID uint64, bizType, bizID string) (json.RawMessage, error) {
	if c.resolver != nil {
		if raw, err := c.instancesByBizGRPC(ctx, tenantID, bizType, bizID); err == nil {
			return raw, nil
		}
	}
	path := "/api/v1/bpm/internal/instances/by-biz?biz_type=" + url.QueryEscape(bizType) +
		"&biz_id=" + url.QueryEscape(bizID)
	return c.do(ctx, http.MethodGet, path, tenantID, nil)
}

// instancesByBizGRPC 经 Consul 发现 + gRPC 调 bpm InstancesByBiz（读类，重试瞬时故障）。
func (c *Client) instancesByBizGRPC(ctx context.Context, tenantID uint64, bizType, bizID string) (json.RawMessage, error) {
	if c.resolver == nil {
		return nil, errNoResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return (resilience.Options{
		MaxRetries:     2,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: c.breaker,
	}).DoResult(ctx, func(ctx context.Context) (json.RawMessage, error) {
		conn, err := c.pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := bpmv1.NewBpmServiceClient(conn).InstancesByBiz(ctx, &bpmv1.InstancesByBizRequest{
			TenantId: tenantID,
			BizType:  bizType,
			BizId:    bizID,
		})
		if err == nil {
			return json.RawMessage(resp.GetJson()), nil
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

func (c *Client) do(ctx context.Context, method, path string, tenantID uint64, body any) (json.RawMessage, error) {
	return c.doWithIdempotency(ctx, method, path, tenantID, body, "")
}

func (c *Client) doWithIdempotency(ctx context.Context, method, path string, tenantID uint64, body any, idempotencyKey string) (json.RawMessage, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("bpm client 未启用（缺 BPM_API_BASE / BPM_INTERNAL_TOKEN）")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	req.Header.Set("X-Tenant-ID", strconv.FormatUint(tenantID, 10))
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		req.Header.Set("X-Idempotency-Key", key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("bpm HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		return nil, err
	}
	if resp.StatusCode >= 300 || (env.Code != 0 && env.Code != 200) {
		return nil, fmt.Errorf("bpm: %s", env.Message)
	}
	return env.Data, nil
}
