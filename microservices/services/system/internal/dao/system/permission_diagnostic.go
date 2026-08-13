package system

import (
	"context"
	"strings"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// PermissionDiagnosticDAO loads menu bindings for a registered permission.
type PermissionDiagnosticDAO struct {
	db *gorm.DB
}

func NewPermissionDiagnosticDAO(db *gorm.DB) *PermissionDiagnosticDAO {
	return &PermissionDiagnosticDAO{db: db}
}

func (d *PermissionDiagnosticDAO) ListMenusByPermissionContext(ctx context.Context, permission string) ([]model.Menu, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return []model.Menu{}, nil
	}

	var menus []model.Menu
	err := d.db.WithContext(ctx).
		Model(&model.Menu{}).
		Distinct("menus.*").
		Joins("LEFT JOIN menu_permissions mp ON mp.menu_id = menus.id").
		Joins("LEFT JOIN permissions p ON p.id = mp.permission_id").
		Where("menus.permission = ? OR p.code = ?", permission, permission).
		Order("menus.parent_id ASC, menus.sort ASC, menus.id ASC").
		Find(&menus).Error
	return menus, err
}
