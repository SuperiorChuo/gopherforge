package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	identityv1 "github.com/go-admin-kit/services/api/gen/identity/v1"
	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
)

var errNoIdentityResolver = &noIdentityResolverError{}

type noIdentityResolverError struct{}

func (*noIdentityResolverError) Error() string { return "notify: identity resolver 未初始化" }

// userContacts 调 identity owner API 批量取用户联系方式 + 租户归属（Phase 3 走 gRPC）。
// 经 Consul 发现 identity-service → gRPC BatchUserContacts。gRPC 不可达回退 HTTP，
// 两者都失败软读降级（放行 + 空联系方式）。
func (s *Store) userContacts(ctx context.Context, tenantID uint64, ids []uint64) (map[uint64]bool, map[uint64]channel.Contact) {
	allowed := make(map[uint64]bool, len(ids))
	contacts := make(map[uint64]channel.Contact, len(ids))
	if len(ids) == 0 {
		return allowed, contacts
	}

	if resp, err := s.userContactsGRPC(ctx, tenantID, ids); err == nil {
		for id, ct := range resp.GetContacts() {
			allowed[id] = true
			contacts[id] = channel.Contact{
				UserID: id,
				Email:  strings.TrimSpace(ct.GetEmail()),
				Phone:  strings.TrimSpace(ct.GetPhone()),
			}
		}
		return allowed, contacts
	}

	// 回退 HTTP（Phase 2C 实现保留）。
	return s.userContactsHTTP(ctx, tenantID, ids)
}

// userContactsGRPC 经 Consul 发现 + gRPC 调 identity BatchUserContacts。
// Phase 4：套 resilience——重试（仅瞬时故障）+ 熔断 + 3s 总超时；resolver 用缓存。
func (s *Store) userContactsGRPC(ctx context.Context, tenantID uint64, ids []uint64) (*identityv1.BatchUserContactsResponse, error) {
	if s.identityResolver == nil {
		return nil, errNoIdentityResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return (resilience.Options{
		MaxRetries:     2,
		BaseDelay:      100 * time.Millisecond,
		CircuitBreaker: s.identityBreaker,
	}).DoResult(ctx, func(ctx context.Context) (*identityv1.BatchUserContactsResponse, error) {
		conn, err := s.identityPool.Get(ctx)
		if err != nil {
			log.Printf("notify: identity grpc dial failed: %v", err)
			return nil, err
		}
		resp, err := identityv1.NewIdentityServiceClient(conn).BatchUserContacts(ctx, &identityv1.BatchUserContactsRequest{
			TenantId: tenantID,
			Ids:      ids,
		})
		if err == nil {
			return resp, nil
		}
		if grpcx.Retryable(err) {
			s.identityPool.Invalidate()
			return nil, err
		}
		if grpcx.IsTransient(err) {
			return nil, fmt.Errorf("%w: %w", resilience.ErrTransient, err)
		}
		return nil, fmt.Errorf("%w: %w", resilience.ErrNoRetry, err)
	})
}

// userContactsHTTP 回退：identity HTTP 内部端点（Phase 2C 实现）。
func (s *Store) userContactsHTTP(ctx context.Context, tenantID uint64, ids []uint64) (map[uint64]bool, map[uint64]channel.Contact) {
	allowed := make(map[uint64]bool, len(ids))
	contacts := make(map[uint64]channel.Contact, len(ids))
	base := s.identityAPIBase
	if base == "" {
		for _, id := range ids {
			allowed[id] = true
		}
		return allowed, contacts
	}

	body, _ := json.Marshal(map[string]any{"tenant_id": tenantID, "ids": ids})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/internal/users/contacts", bytes.NewReader(body))
	if err != nil {
		return degradeContacts(allowed, contacts, ids)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", s.identityInternalToken)

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return degradeContacts(allowed, contacts, ids)
	}
	defer resp.Body.Close()
	var out struct {
		Contacts map[string]struct {
			Email string `json:"email"`
			Phone string `json:"phone"`
		} `json:"contacts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return degradeContacts(allowed, contacts, ids)
	}
	for idStr, ct := range out.Contacts {
		id, _ := strconv.ParseUint(idStr, 10, 64)
		allowed[id] = true
		contacts[id] = channel.Contact{UserID: id, Email: strings.TrimSpace(ct.Email), Phone: strings.TrimSpace(ct.Phone)}
	}
	return allowed, contacts
}

// degradeContacts 软读降级：全部放行、联系方式留空。
func degradeContacts(allowed map[uint64]bool, contacts map[uint64]channel.Contact, ids []uint64) (map[uint64]bool, map[uint64]channel.Contact) {
	for _, id := range ids {
		allowed[id] = true
	}
	return allowed, contacts
}
