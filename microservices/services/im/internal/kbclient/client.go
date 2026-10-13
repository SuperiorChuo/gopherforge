// 包 kbclient 调用 ai-service 的向量检索，供 IM 机器人 RAG（M1.5）使用。
package kbclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	aiv1 "github.com/go-admin-kit/services/api/gen/ai/v1"
	"github.com/go-admin-kit/services/im/internal/store/im"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
)

// Client 与 ai-service 的 POST /api/v1/ai/internal/kb/search 交互。
// Phase 3：gRPC 优先（Consul 发现 ai-service），失败回退 HTTP。
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	// MinScore 过滤较弱的余弦命中（1 - distance）。默认 0.28。
	MinScore float64
	resolver *grpcx.Resolver
	pool     *grpcx.ConnPool // Phase 4 收尾：gRPC 连接复用
	breaker  *resilience.Breaker
}

func New(baseURL, token string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	var resolver *grpcx.Resolver
	if r, err := grpcx.NewResolver(""); err == nil {
		resolver = r
	} else {
		log.Printf("kbclient: consul resolver 初始化失败（gRPC 路径禁用，仅 HTTP）: %v", err)
	}
	return &Client{
		BaseURL:  baseURL,
		Token:    strings.TrimSpace(token),
		HTTP:     &http.Client{Timeout: 12 * time.Second},
		MinScore: 0.28,
		resolver: resolver,
		pool:     grpcx.NewConnPool(resolver, "ai-service"),
		breaker:  resilience.NewBreaker(5, 10*time.Second),
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.BaseURL != "" && c.Token != ""
}

type searchReq struct {
	Query    string `json:"query"`
	TopK     int    `json:"top_k"`
	TenantID uint64 `json:"tenant_id"`
}

type match struct {
	DocumentID uint64  `json:"document_id"`
	Title      string  `json:"title"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		List   []match `json:"list"`
		Source string  `json:"source"`
	} `json:"data"`
}

// SearchVector 返回来自 ai-service embeddings 的命中结果。
// Phase 3：gRPC 优先（Consul 发现 ai-service），失败回退 HTTP。
func (c *Client) SearchVector(ctx context.Context, tenantID uint64, query string, topK int) ([]store.KBHit, error) {
	if c == nil {
		return nil, fmt.Errorf("kb vector client disabled")
	}
	if topK <= 0 {
		topK = 3
	}
	if c.resolver != nil {
		if hits, err := c.searchGRPC(ctx, tenantID, query, topK); err == nil {
			return hits, nil
		}
	}
	if !c.Enabled() {
		return nil, fmt.Errorf("kb vector client disabled")
	}
	body, _ := json.Marshal(searchReq{Query: query, TopK: topK, TenantID: tenantID})
	url := c.BaseURL + "/api/v1/ai/internal/kb/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.Token)
	req.Header.Set("X-Auth-Tenant-ID", fmt.Sprintf("%d", tenantID))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ai kb search status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	return filterHits(toProtoMatches(env.Data.List), c.MinScore), nil
}

// searchGRPC 经 Consul 发现 + gRPC 调 ai InternalKBSearch（读类，重试瞬时故障）。
func (c *Client) searchGRPC(ctx context.Context, tenantID uint64, query string, topK int) ([]store.KBHit, error) {
	if c.resolver == nil {
		return nil, errNoResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	return (resilience.Options{
		MaxRetries:     2,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: c.breaker,
	}).DoResult(ctx, func(ctx context.Context) ([]store.KBHit, error) {
		conn, err := c.pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := aiv1.NewAiServiceClient(conn).InternalKBSearch(ctx, &aiv1.InternalKBSearchRequest{
			Query: query, TopK: int32(topK), TenantId: tenantID,
		})
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
		return filterHits(resp.GetList(), c.MinScore), nil
	})
}

// filterHits 按 MinScore 过滤 + 摘要截断，返回 IM 需要的 KBHit 列表。
func filterHits(list []*aiv1.KBSearchMatch, minScore float64) []store.KBHit {
	min := minScore
	if min <= 0 {
		min = 0.28
	}
	out := make([]store.KBHit, 0, len(list))
	for _, m := range list {
		if m == nil || m.Score < min {
			continue
		}
		snippet := m.Content
		if r := []rune(snippet); len(r) > 220 {
			snippet = string(r[:220]) + "…"
		}
		out = append(out, store.KBHit{
			DocumentID: m.DocumentId,
			Title:      m.Title,
			Snippet:    snippet,
			Score:      m.Score,
		})
	}
	return out
}

// toProtoMatches 把 HTTP 响应的 match 列表转成 proto match（供 filterHits 统一处理）。
func toProtoMatches(list []match) []*aiv1.KBSearchMatch {
	out := make([]*aiv1.KBSearchMatch, 0, len(list))
	for _, m := range list {
		out = append(out, &aiv1.KBSearchMatch{
			DocumentId: m.DocumentID,
			Title:      m.Title,
			ChunkIndex: int32(m.ChunkIndex),
			Content:    m.Content,
			Score:      m.Score,
		})
	}
	return out
}

var errNoResolver = &noResolverError{}

type noResolverError struct{}

func (*noResolverError) Error() string { return "kbclient: no consul resolver" }
