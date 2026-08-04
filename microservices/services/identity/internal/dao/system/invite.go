package system

import (
	"context"
	"time"

	"github.com/go-admin-kit/services/identity/internal/model"
	"gorm.io/gorm"
)

// InviteDAO 读写 invites 表（管理员侧：创建/列表/撤销；注册消费在 auth 服务）。
// 租户插件按 tenant_id 自动过滤，显式 tenantID 参数与 ctx 租户一致。
type InviteDAO struct {
	db *gorm.DB
}

func NewInviteDAO(db *gorm.DB) *InviteDAO {
	return &InviteDAO{db: db}
}

func (d *InviteDAO) CreateContext(ctx context.Context, inv *model.Invite) error {
	return d.db.WithContext(ctx).Create(inv).Error
}

func (d *InviteDAO) ListByTenantContext(ctx context.Context, tenantID uint) ([]model.Invite, error) {
	var list []model.Invite
	err := d.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("id DESC").
		Find(&list).Error
	return list, err
}

// RevokeContext 撤销（置 revoked_at），仅未撤销时可撤销。
func (d *InviteDAO) RevokeContext(ctx context.Context, tenantID, id uint) error {
	result := d.db.WithContext(ctx).
		Model(&model.Invite{}).
		Where("id = ? AND tenant_id = ? AND revoked_at IS NULL", id, tenantID).
		Update("revoked_at", time.Now())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// RoleInTenantContext 校验角色属于该租户（发邀请选角色时用）。
func (d *InviteDAO) RoleInTenantContext(ctx context.Context, roleID, tenantID uint) (bool, error) {
	var cnt int64
	err := d.db.WithContext(ctx).Model(&model.Role{}).
		Where("id = ? AND tenant_id = ?", roleID, tenantID).
		Count(&cnt).Error
	return cnt > 0, err
}
