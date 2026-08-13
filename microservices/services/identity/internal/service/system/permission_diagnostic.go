package system

import (
	"context"
	"errors"
	"sort"
	"strings"

	systemdao "github.com/go-admin-kit/services/identity/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// PermissionDiagnosticService explains the effective RBAC result without changing authorization state.
type PermissionDiagnosticService struct {
	dao               *systemdao.PermissionDiagnosticDAO
	dataScopeResolver *authz.DataScopeResolver
}

func NewPermissionDiagnosticServiceWithDB(db *gorm.DB) PermissionDiagnosticService {
	return PermissionDiagnosticService{
		dao:               systemdao.NewPermissionDiagnosticDAO(db),
		dataScopeResolver: authz.NewDataScopeResolver(authz.NewDatabaseDataScopeStore(db)),
	}
}

type PermissionDiagnosticRequest struct {
	UserID     uint   `json:"user_id" binding:"required"`
	Permission string `json:"permission" binding:"required"`
}

type PermissionDiagnosticUser struct {
	ID         uint   `json:"id"`
	TenantID   uint   `json:"tenant_id"`
	Username   string `json:"username"`
	Nickname   string `json:"nickname"`
	Department uint   `json:"department_id"`
	Status     int8   `json:"status"`
}

type PermissionDiagnosticRole struct {
	ID            uint     `json:"id"`
	Name          string   `json:"name"`
	Code          string   `json:"code"`
	DataScope     string   `json:"data_scope"`
	PermissionIDs []uint   `json:"permission_ids"`
	Permissions   []string `json:"permissions"`
	Matches       bool     `json:"matches"`
	MatchReason   string   `json:"match_reason,omitempty"`
}

type PermissionDiagnosticPackage struct {
	Bound              bool   `json:"bound"`
	ID                 uint   `json:"id,omitempty"`
	Name               string `json:"name,omitempty"`
	Status             int8   `json:"status,omitempty"`
	AllowsPermission   bool   `json:"allows_permission"`
	HasExistingOverrun bool   `json:"has_existing_overrun"`
}

type PermissionDiagnosticDataScope struct {
	Scope         string `json:"scope"`
	DepartmentID  uint   `json:"department_id"`
	DepartmentIDs []uint `json:"department_ids"`
}

type PermissionDiagnosticResult struct {
	Allowed              bool                          `json:"allowed"`
	Reason               string                        `json:"reason"`
	RequestedPermission  string                        `json:"requested_permission"`
	MatchedBy            string                        `json:"matched_by,omitempty"`
	User                 PermissionDiagnosticUser      `json:"user"`
	Roles                []PermissionDiagnosticRole    `json:"roles"`
	EffectivePermissions []string                      `json:"effective_permissions"`
	Package              PermissionDiagnosticPackage   `json:"package"`
	DataScope            PermissionDiagnosticDataScope `json:"data_scope"`
}

var ErrPermissionDiagnosticUserNotFound = errors.New("permission diagnostic user not found")

func (s *PermissionDiagnosticService) DiagnoseContext(ctx context.Context, req PermissionDiagnosticRequest) (*PermissionDiagnosticResult, error) {
	permission := strings.TrimSpace(req.Permission)
	user, err := s.dao.GetUserContext(tenant.DisableScope(ctx), req.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPermissionDiagnosticUserNotFound
		}
		return nil, err
	}
	if !isPlatformAdminContext(ctx) && user.TenantID != tenant.Normalize(tenant.FromContext(ctx)) {
		return nil, ErrPermissionDiagnosticUserNotFound
	}

	targetContext := tenant.WithContext(ctx, user.TenantID)
	roles, err := s.dao.GetRolesContext(tenant.DisableScope(targetContext), user.ID)
	if err != nil {
		return nil, err
	}

	result := &PermissionDiagnosticResult{
		RequestedPermission: permission,
		Reason:              "no assigned role grants the requested permission",
		User: PermissionDiagnosticUser{
			ID: user.ID, TenantID: user.TenantID, Username: user.Username,
			Nickname: user.Nickname, Department: user.DepartmentID, Status: user.Status,
		},
		Roles: make([]PermissionDiagnosticRole, 0, len(roles)),
	}
	permissionSet := make(map[string]struct{})
	for _, role := range roles {
		diagnosticRole := PermissionDiagnosticRole{
			ID: role.ID, Name: role.Name, Code: role.Code, DataScope: role.DataScope,
			PermissionIDs: make([]uint, 0, len(role.Permissions)),
			Permissions:   make([]string, 0, len(role.Permissions)),
		}
		if role.Code == "super_admin" {
			diagnosticRole.Matches = true
			diagnosticRole.MatchReason = "super_admin role bypasses permission checks"
			if !result.Allowed {
				result.Allowed = true
				result.MatchedBy = "role:super_admin"
				result.Reason = diagnosticRole.MatchReason
			}
		}
		for _, granted := range role.Permissions {
			code := strings.TrimSpace(granted.Code)
			if code == "" {
				continue
			}
			diagnosticRole.PermissionIDs = append(diagnosticRole.PermissionIDs, granted.ID)
			diagnosticRole.Permissions = append(diagnosticRole.Permissions, code)
			permissionSet[code] = struct{}{}
			if matchesPermission(code, permission) {
				diagnosticRole.Matches = true
				diagnosticRole.MatchReason = "role grants " + code
				if !result.Allowed {
					result.Allowed = true
					result.MatchedBy = "permission:" + code
					result.Reason = diagnosticRole.MatchReason
				}
			}
		}
		sort.Strings(diagnosticRole.Permissions)
		result.Roles = append(result.Roles, diagnosticRole)
	}
	for code := range permissionSet {
		result.EffectivePermissions = append(result.EffectivePermissions, code)
	}
	sort.Strings(result.EffectivePermissions)

	if err := s.fillPackageContext(targetContext, user.TenantID, permission, result); err != nil {
		return nil, err
	}
	if err := s.fillDataScopeContext(targetContext, user, roles, result); err != nil {
		return nil, err
	}
	if user.Status != userStatusEnabled {
		result.Allowed = false
		result.MatchedBy = ""
		result.Reason = "user account is disabled"
	}
	return result, nil
}

func (s *PermissionDiagnosticService) fillPackageContext(ctx context.Context, tenantID uint, permission string, result *PermissionDiagnosticResult) error {
	result.Package.AllowsPermission = true
	target, err := s.dao.GetTenantContext(tenant.DisableScope(ctx), tenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if target.PackageID == nil || *target.PackageID == 0 {
		return nil
	}
	pkg, err := s.dao.GetTenantPackageContext(tenant.DisableScope(ctx), *target.PackageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result.Package = PermissionDiagnosticPackage{Bound: true, ID: *target.PackageID, AllowsPermission: false}
			return nil
		}
		return err
	}
	result.Package = PermissionDiagnosticPackage{Bound: true, ID: pkg.ID, Name: pkg.Name, Status: pkg.Status}
	for _, code := range pkg.PermissionCodes {
		if matchesPermission(strings.TrimSpace(code), permission) {
			result.Package.AllowsPermission = true
			break
		}
	}
	result.Package.HasExistingOverrun = result.Allowed && !result.Package.AllowsPermission
	return nil
}

func (s *PermissionDiagnosticService) fillDataScopeContext(ctx context.Context, user *localmodel.User, roles []model.Role, result *PermissionDiagnosticResult) error {
	user.Roles = roles
	scope, err := s.dataScopeResolver.ResolveUserDataScopeContext(ctx, user)
	if err != nil {
		return err
	}
	result.DataScope = PermissionDiagnosticDataScope{
		Scope: string(scope.Scope), DepartmentID: scope.DepartmentID,
		DepartmentIDs: scope.DepartmentIDs,
	}
	return nil
}

func matchesPermission(granted, required string) bool {
	return granted == required || granted == "*" || granted == "*:*:*"
}
