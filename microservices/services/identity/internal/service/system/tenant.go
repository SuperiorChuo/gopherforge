package system

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"github.com/go-admin-kit/services/identity/internal/pkg/tenant"
	"gorm.io/gorm"
)

var (
	ErrTenantNotFound      = errors.New("tenant not found")
	ErrTenantCodeExists    = errors.New("tenant code already exists")
	ErrTenantCodeInvalid   = errors.New("tenant code must be lowercase letters, digits, hyphen (2-64)")
	ErrTenantNameRequired  = errors.New("tenant name required")
	ErrDefaultTenantLocked = errors.New("default tenant code cannot be changed")
)

var tenantCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// TenantService manages SaaS tenants.
type TenantService struct {
	dao *system.TenantDAO
}

func NewTenantServiceWithDB(db *gorm.DB) *TenantService {
	return &TenantService{dao: system.NewTenantDAO(db)}
}

type TenantListRequest struct {
	pagination.PageRequest
	Keyword string
	Status  *int8
}

type CreateTenantRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Plan     string `json:"plan"`
	MaxUsers int64  `json:"max_users"`
	Status   int8   `json:"status"`
}

// PlanDefaultMaxUsers returns a soft default quota when max_users is 0 on create.
func PlanDefaultMaxUsers(plan string) int64 {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "pro":
		return 50
	case "enterprise":
		return 0 // unlimited
	default: // free
		return 10
	}
}

type UpdateTenantRequest struct {
	Name     *string `json:"name"`
	Plan     *string `json:"plan"`
	MaxUsers *int64  `json:"max_users"`
	Status   *int8   `json:"status"`
}

func (s *TenantService) List(ctx context.Context, req TenantListRequest) ([]model.Tenant, int64, error) {
	// Platform-wide catalog: disable row tenant plugin (tenants table has no tenant_id).
	ctx = tenant.DisableScope(ctx)
	return s.dao.ListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

func (s *TenantService) Get(ctx context.Context, id uint) (*model.Tenant, error) {
	ctx = tenant.DisableScope(ctx)
	t, err := s.dao.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *TenantService) Create(ctx context.Context, req CreateTenantRequest) (*model.Tenant, error) {
	ctx = tenant.DisableScope(ctx)
	code := strings.ToLower(strings.TrimSpace(req.Code))
	name := strings.TrimSpace(req.Name)
	if !tenantCodeRe.MatchString(code) {
		return nil, ErrTenantCodeInvalid
	}
	if name == "" {
		return nil, ErrTenantNameRequired
	}
	if _, err := s.dao.GetByCodeContext(ctx, code); err == nil {
		return nil, ErrTenantCodeExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	plan := strings.TrimSpace(req.Plan)
	if plan == "" {
		plan = "free"
	}
	maxUsers := req.MaxUsers
	if maxUsers == 0 && plan != "enterprise" {
		// 0 means "use plan default" on create (enterprise stays unlimited).
		maxUsers = PlanDefaultMaxUsers(plan)
	}
	t := &model.Tenant{
		Code:     code,
		Name:     name,
		Status:   status,
		Plan:     plan,
		MaxUsers: maxUsers,
	}
	if err := s.dao.CreateContext(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TenantService) Update(ctx context.Context, id uint, req UpdateTenantRequest) (*model.Tenant, error) {
	ctx = tenant.DisableScope(ctx)
	t, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrTenantNameRequired
		}
		t.Name = name
	}
	if req.Plan != nil {
		plan := strings.TrimSpace(*req.Plan)
		if plan == "" {
			plan = "free"
		}
		t.Plan = plan
	}
	if req.MaxUsers != nil {
		t.MaxUsers = *req.MaxUsers
	}
	if req.Status != nil {
		// allow disabling non-default; default can be disabled carefully
		t.Status = *req.Status
	}
	if err := s.dao.UpdateContext(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TenantService) UserCount(ctx context.Context, tenantID uint) (int64, error) {
	// Count users of another tenant requires bypassing the actor tenant filter.
	ctx = tenant.DisableScope(ctx)
	return s.dao.CountUsersContext(ctx, tenantID)
}
