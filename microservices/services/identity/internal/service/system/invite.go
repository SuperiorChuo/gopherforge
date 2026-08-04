package system

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/tenant"
	"gorm.io/gorm"
)

var (
	ErrInviteNotFound        = errors.New("invite not found")
	ErrInviteRoleNotInTenant = errors.New("role does not belong to this tenant")
)

const defaultInviteTTL = 7 * 24 * time.Hour

// InviteService 管理员侧邀请管理：创建（返回一次性 token/链接）/列表/撤销。
// 注册消费（校验 + 原子 used_at）在 auth 服务。
type InviteService struct {
	dao *systemdao.InviteDAO
}

func NewInviteServiceWithDB(db *gorm.DB) *InviteService {
	return &InviteService{dao: systemdao.NewInviteDAO(db)}
}

type CreateInviteRequest struct {
	RoleID    *uint      `json:"role_id"`
	Email     string     `json:"email"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type InviteResult struct {
	ID        uint      `json:"id"`
	Token     string    `json:"token"` // 一次性明文，仅此一次返回
	Link      string    `json:"link"`
	RoleID    uint      `json:"role_id,omitempty"`
	Email     string    `json:"email,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *InviteService) Create(ctx context.Context, req CreateInviteRequest) (*InviteResult, error) {
	tenantID := tenant.Normalize(tenant.FromContext(ctx))
	if req.RoleID != nil && *req.RoleID > 0 {
		ok, err := s.dao.RoleInTenantContext(ctx, *req.RoleID, tenantID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInviteRoleNotInTenant
		}
	}
	token, err := generateInviteToken()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(token))
	expiresAt := req.ExpiresAt
	if expiresAt == nil {
		t := time.Now().Add(defaultInviteTTL)
		expiresAt = &t
	}
	inv := &model.Invite{
		TenantID:  tenantID,
		Email:     strings.TrimSpace(req.Email),
		TokenHash: fmt.Sprintf("%x", hash[:]),
		ExpiresAt: *expiresAt,
		CreatedBy: 0,
	}
	if req.RoleID != nil {
		inv.RoleID = *req.RoleID
	}
	if err := s.dao.CreateContext(ctx, inv); err != nil {
		return nil, err
	}
	return &InviteResult{
		ID:        inv.ID,
		Token:     token,
		Link:      "/register?invite=" + token,
		RoleID:    inv.RoleID,
		Email:     inv.Email,
		ExpiresAt: inv.ExpiresAt,
	}, nil
}

func (s *InviteService) List(ctx context.Context) ([]model.Invite, error) {
	return s.dao.ListByTenantContext(ctx, tenant.Normalize(tenant.FromContext(ctx)))
}

func (s *InviteService) Revoke(ctx context.Context, id uint) error {
	if err := s.dao.RevokeContext(ctx, tenant.Normalize(tenant.FromContext(ctx)), id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInviteNotFound
		}
		return err
	}
	return nil
}

// generateInviteToken 生成 32 字节随机 token（base64url，不可预测）。
func generateInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
