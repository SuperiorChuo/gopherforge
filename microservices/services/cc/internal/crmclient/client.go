// Package crmclient calls crm-service internal endpoints over HTTP
// (X-Internal-Token auth, 内网直连 crm 容器 :8091)。用于来电号码识客聚合与
// 挂机写 CRM 跟进——服务解耦，不直接读/写 crm 的库。
package crmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	crmv1 "github.com/go-admin-kit/services/api/gen/crm/v1"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/idempotency"
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
		log.Printf("crmclient: consul resolver 初始化失败（gRPC 路径禁用，仅 HTTP 回退）: %v", err)
	}
	return &Client{
		base:     strings.TrimRight(base, "/"),
		token:    token,
		http:     &http.Client{Timeout: 5 * time.Second},
		resolver: resolver,
		pool:     grpcx.NewConnPool(resolver, "crm-service"),
		breaker:  resilience.NewBreaker(5, 10*time.Second),
	}
}

// Enabled 仅当 base 与 token 都配置了才启用（缺一即视为未接 CRM）。
func (c *Client) Enabled() bool { return c != nil && c.base != "" && c.token != "" }

// envelope 拆包 crm 的 {code,message,data}。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Customer 是识客卡需要的 CRM 客户子集（字段名对齐 crm 模型 JSON）。
type Customer struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Phone       string `json:"phone"`
	Level       string `json:"level"`
	Industry    string `json:"industry"`
	OwnerUserID uint64 `json:"owner_user_id"`
}

// Lead 是识客卡需要的 CRM 线索子集。
type Lead struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Status string `json:"status"`
}

// LookupResult 对齐 crm InternalLookup 返回（PhoneLookup）。
type LookupResult struct {
	Matched  string    `json:"matched"` // customer | lead | none
	Customer *Customer `json:"customer,omitempty"`
	Lead     *Lead     `json:"lead,omitempty"`
}

// Lookup 按号码查 CRM 归属；未配置返回 (nil,nil) 让调用方走 has_crm=false。
// Phase 3：gRPC 优先（Consul 发现 crm-service），失败回退 HTTP。
func (c *Client) Lookup(ctx context.Context, tenantID uint64, phone string) (*LookupResult, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if res, err := c.lookupGRPC(ctx, tenantID, phone); err == nil {
		return res, nil
	}
	u := c.base + "/api/v1/crm/internal/lookup?phone=" + url.QueryEscape(phone)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, tenantID)
	var out LookupResult
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// lookupGRPC 经 Consul 发现 + gRPC 调 crm LookupByPhone。
// Phase 4：套 resilience——重试（仅瞬时故障）+ 熔断 + 5s 总超时（对齐 HTTP 超时）。
func (c *Client) lookupGRPC(ctx context.Context, tenantID uint64, phone string) (*LookupResult, error) {
	if c.resolver == nil {
		return nil, errNoResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return (resilience.Options{
		MaxRetries:     2,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: c.breaker,
	}).DoResult(ctx, func(ctx context.Context) (*LookupResult, error) {
		conn, err := c.pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		res, err := crmv1.NewCrmServiceClient(conn).LookupByPhone(ctx, &crmv1.LookupByPhoneRequest{TenantId: tenantID, Phone: phone})
		if err != nil {
			if grpcx.Retryable(err) {
				c.pool.Invalidate()
				return nil, err
			}
			if grpcx.IsTransient(err) {
				return nil, fmt.Errorf("%w: %w", resilience.ErrTransient, err)
			}
			return nil, fmt.Errorf("%w: %w", resilience.ErrNoRetry, err)
		}
		out := &LookupResult{Matched: res.GetMatched()}
		if res.GetCustomer() != nil {
			out.Customer = &Customer{ID: res.GetCustomer().GetId(), Name: res.GetCustomer().GetName(), Phone: res.GetCustomer().GetPhone(), OwnerUserID: res.GetCustomer().GetOwnerUserId()}
		}
		if res.GetLead() != nil {
			out.Lead = &Lead{ID: res.GetLead().GetId(), Name: res.GetLead().GetName(), Phone: res.GetLead().GetPhone()}
		}
		return out, nil
	})
}

var errNoResolver = &noResolverError{}

type noResolverError struct{}

func (*noResolverError) Error() string { return "crmclient: no consul resolver" }

// CreateFollowUp 给 CRM 目标写一条跟进（挂机留痕）。targetType 目前仅 "customer"。
// Phase 5：一次逻辑操作生成幂等键，贯穿 gRPC + HTTP 回退，服务端只写一次。
func (c *Client) CreateFollowUp(ctx context.Context, tenantID uint64, targetType string, targetID, byUserID uint64, content string) error {
	if !c.Enabled() {
		return nil
	}
	key, err := idempotency.NewKey()
	if err != nil {
		return fmt.Errorf("crmclient: 生成幂等键: %w", err)
	}
	// Phase 3：gRPC 优先，失败回退 HTTP。Phase 4：gRPC 路径套 resilience。
	if c.resolver != nil {
		if err := c.createFollowUpGRPC(ctx, tenantID, targetType, targetID, byUserID, content, key); err == nil {
			return nil
		}
	}
	body, _ := json.Marshal(map[string]any{
		"target_type":     targetType,
		"target_id":       targetID,
		"content":         content,
		"by_user_id":      byUserID,
		"idempotency_key": key,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/api/v1/crm/internal/followups", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", key)
	c.setHeaders(req, tenantID)
	return c.do(req, nil)
}

// createFollowUpGRPC 经 Consul 发现 + gRPC 调 crm CreateFollowUp。
// 写类带幂等键：gRPC 不重试（MaxRetries=0），失败回退 HTTP 复用同一 key。
func (c *Client) createFollowUpGRPC(ctx context.Context, tenantID uint64, targetType string, targetID, byUserID uint64, content, idemKey string) error {
	if c.resolver == nil {
		return errNoResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return resilience.Do(ctx, resilience.Options{
		MaxRetries:     0,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: c.breaker,
	}, func(ctx context.Context) error {
		conn, err := c.pool.Get(ctx)
		if err != nil {
			return err
		}
		_, err = crmv1.NewCrmServiceClient(conn).CreateFollowUp(ctx, &crmv1.CreateFollowUpRequest{
			TenantId:       tenantID,
			TargetType:     targetType,
			TargetId:       targetID,
			Content:        content,
			ByUserId:       byUserID,
			IdempotencyKey: idemKey,
		})
		if err == nil {
			return nil
		}
		if grpcx.Retryable(err) || grpcx.IsTransient(err) {
			if grpcx.Retryable(err) {
				c.pool.Invalidate()
			}
			return fmt.Errorf("%w: %w", resilience.ErrTransient, err) // 计熔断，不重试
		}
		return fmt.Errorf("%w: %w", resilience.ErrNoRetry, err)
	})
}

func (c *Client) setHeaders(req *http.Request, tenantID uint64) {
	req.Header.Set("X-Internal-Token", c.token)
	if tenantID == 0 {
		tenantID = 1
	}
	req.Header.Set("X-Tenant-ID", strconv.FormatUint(tenantID, 10))
}

// do 发请求并拆 envelope；out 为 nil 时只校验状态码。
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("crm internal %s: status %d: %s", req.URL.Path, resp.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}
