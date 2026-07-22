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
	dao    *system.TenantDAO
	pkgDAO *system.TenantPackageDAO
}

func NewTenantServiceWithDB(db *gorm.DB) *TenantService {
	return &TenantService{
		dao:    system.NewTenantDAO(db),
		pkgDAO: system.NewTenantPackageDAO(db),
	}
}

// resolvePackageID 归一化套餐绑定入参：nil/0 → 不绑定（NULL）；>0 校验套餐存在后绑定。
func (s *TenantService) resolvePackageID(ctx context.Context, packageID *uint) (*uint, error) {
	if packageID == nil || *packageID == 0 {
		return nil, nil
	}
	if s.pkgDAO == nil {
		return nil, ErrTenantPackageNotFound
	}
	if _, err := s.pkgDAO.GetByIDContext(ctx, *packageID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantPackageNotFound
		}
		return nil, err
	}
	id := *packageID
	return &id, nil
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
	// PackageID 租户套餐（权限包）；缺省/0 = 不限。
	PackageID *uint `json:"package_id"`
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
	// PackageID 租户套餐：缺省 = 不改；0 = 解绑（置 NULL）；>0 = 绑定（校验存在）。
	PackageID *uint `json:"package_id"`
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
	packageID, err := s.resolvePackageID(ctx, req.PackageID)
	if err != nil {
		return nil, err
	}
	t := &model.Tenant{
		Code:      code,
		Name:      name,
		Status:    status,
		Plan:      plan,
		MaxUsers:  maxUsers,
		PackageID: packageID,
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
	if req.PackageID != nil {
		packageID, err := s.resolvePackageID(ctx, req.PackageID)
		if err != nil {
			return nil, err
		}
		t.PackageID = packageID
	}
	if req.Status != nil {
		// Default tenant (id=1) must stay enabled: disabling it would lock out
		// the platform admin and every default-tenant user at next refresh.
		if id == 1 && *req.Status != 1 {
			return nil, ErrDefaultTenantLocked
		}
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
