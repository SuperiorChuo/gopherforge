package system

import (
	"context"
	"strings"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"gorm.io/gorm"
)

// PermissionMenuBinding describes a menu guarded by or explicitly linked to a permission.
type PermissionMenuBinding struct {
	ID         uint   `json:"id"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Component  string `json:"component"`
	ParentID   uint   `json:"parent_id"`
	Status     int8   `json:"status"`
	Hidden     int8   `json:"hidden"`
	Permission string `json:"permission"`
}

// PermissionMenuDiagnosticResult contains resource-side menu bindings.
type PermissionMenuDiagnosticResult struct {
	Permission string                  `json:"permission"`
	Menus      []PermissionMenuBinding `json:"menus"`
}

type PermissionMenuDiagnosticService struct {
	dao *systemdao.PermissionDiagnosticDAO
}

func NewPermissionMenuDiagnosticServiceWithDB(db *gorm.DB) PermissionMenuDiagnosticService {
	return PermissionMenuDiagnosticService{dao: systemdao.NewPermissionDiagnosticDAO(db)}
}

func (s *PermissionMenuDiagnosticService) DiagnoseContext(ctx context.Context, permission string) (*PermissionMenuDiagnosticResult, error) {
	permission = strings.TrimSpace(permission)
	menus, err := s.dao.ListMenusByPermissionContext(ctx, permission)
	if err != nil {
		return nil, err
	}
	result := &PermissionMenuDiagnosticResult{
		Permission: permission,
		Menus:      make([]PermissionMenuBinding, 0, len(menus)),
	}
	for _, menu := range menus {
		result.Menus = append(result.Menus, PermissionMenuBinding{
			ID: menu.ID, Title: menu.Title, Path: menu.Path, Component: menu.Component,
			ParentID: menu.ParentID, Status: menu.Status, Hidden: menu.Hidden,
			Permission: menu.Permission,
		})
	}
	return result, nil
}
