package system

import (
	"context"
	"errors"
	"strings"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/pagination"
	"github.com/go-admin-kit/services/identity/internal/pkg/tenant"
	"gorm.io/gorm"
)

var (
	ErrTenantPackageNotFound     = errors.New("tenant package not found")
	ErrTenantPackageNameRequired = errors.New("tenant package name required")
	ErrTenantPackageNameExists   = errors.New("tenant package name already exists")
	ErrTenantPackageInUse        = errors.New("tenant package is bound by tenants, unbind before delete")
)

// PermissionsExceedPackageError 角色分配的权限超出租户套餐范围（携带越界权限码，供前端明确提示）。
type PermissionsExceedPackageError struct {
	Codes []string
}

func (e *PermissionsExceedPackageError) Error() string {
	return "permissions exceed tenant package: " + strings.Join(e.Codes, ", ")
}

// TenantPackageService 管理租户套餐（权限包），平台级目录。
type TenantPackageService struct {
	dao *systemdao.TenantPackageDAO
}

func NewTenantPackageServiceWithDB(db *gorm.DB) *TenantPackageService {
	return &TenantPackageService{dao: systemdao.NewTenantPackageDAO(db)}
}

type TenantPackageListRequest struct {
	pagination.PageRequest
	Keyword string
	Status  *int8
}

type CreateTenantPackageRequest struct {
	Name            string   `json:"name"`
	PermissionCodes []string `json:"permission_codes"`
	Status          int8     `json:"status"`
	Remark          string   `json:"remark"`
}

type UpdateTenantPackageRequest struct {
	Name            *string   `json:"name"`
	PermissionCodes *[]string `json:"permission_codes"`
	Status          *int8     `json:"status"`
	Remark          *string   `json:"remark"`
}

// normalizePermissionCodes 去空白、去重，保持原始顺序。
func normalizePermissionCodes(codes []string) model.StringList {
	out := make(model.StringList, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func (s *TenantPackageService) List(ctx context.Context, req TenantPackageListRequest) ([]model.TenantPackage, int64, error) {
	// 平台级目录：关闭行级租户过滤（tenant_packages 无 tenant_id 列）。
	ctx = tenant.DisableScope(ctx)
	return s.dao.ListContext(ctx, req.PageRequest, req.Keyword, req.Status)
}

func (s *TenantPackageService) GetAll(ctx context.Context) ([]model.TenantPackage, error) {
	ctx = tenant.DisableScope(ctx)
	return s.dao.GetAllContext(ctx)
}

func (s *TenantPackageService) Get(ctx context.Context, id uint) (*model.TenantPackage, error) {
	ctx = tenant.DisableScope(ctx)
	p, err := s.dao.GetByIDContext(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantPackageNotFound
		}
		return nil, err
	}
	return p, nil
}

func (s *TenantPackageService) Create(ctx context.Context, req CreateTenantPackageRequest) (*model.TenantPackage, error) {
	ctx = tenant.DisableScope(ctx)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrTenantPackageNameRequired
	}
	if _, err := s.dao.GetByNameContext(ctx, name); err == nil {
		return nil, ErrTenantPackageNameExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	p := &model.TenantPackage{
		Name:            name,
		PermissionCodes: normalizePermissionCodes(req.PermissionCodes),
		Status:          status,
		Remark:          strings.TrimSpace(req.Remark),
	}
	if err := s.dao.CreateContext(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *TenantPackageService) Update(ctx context.Context, id uint, req UpdateTenantPackageRequest) (*model.TenantPackage, error) {
	ctx = tenant.DisableScope(ctx)
	p, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrTenantPackageNameRequired
		}
		if name != p.Name {
			if _, err := s.dao.GetByNameContext(ctx, name); err == nil {
				return nil, ErrTenantPackageNameExists
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			p.Name = name
		}
	}
	if req.PermissionCodes != nil {
		// 语义说明：套餐改小不回收租户内已有角色的越界权限，仅拦截后续新分配（M1）。
		p.PermissionCodes = normalizePermissionCodes(*req.PermissionCodes)
	}
	if req.Status != nil {
		p.Status = *req.Status
	}
	if req.Remark != nil {
		p.Remark = strings.TrimSpace(*req.Remark)
	}
	if err := s.dao.UpdateContext(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *TenantPackageService) Delete(ctx context.Context, id uint) error {
	ctx = tenant.DisableScope(ctx)
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	n, err := s.dao.CountTenantsByPackageContext(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrTenantPackageInUse
	}
	return s.dao.DeleteContext(ctx, id)
}
