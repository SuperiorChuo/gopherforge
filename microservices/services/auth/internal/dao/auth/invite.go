package auth

import (
	"context"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// InviteDAO 读写 invites 表。auth 只负责按 hash 查 + 原子消费；
// 邀请创建/列表/撤销在 identity 服务（管理员所在）。
type InviteDAO struct {
	db *gorm.DB
}

func NewInviteDAO(db *gorm.DB) *InviteDAO {
	return &InviteDAO{db: db}
}

func (d *InviteDAO) GetByTokenHashContext(ctx context.Context, tokenHash string) (*model.Invite, error) {
	var inv model.Invite
	result := d.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&inv)
	return &inv, result.Error
}

// MarkUsedContext 原子消费：仅未用、未撤销、未过期时置 used_at（并发双用仅一次成功）。
func (d *InviteDAO) MarkUsedContext(ctx context.Context, id uint) error {
	result := d.db.WithContext(ctx).
		Model(&model.Invite{}).
		Where("id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", id, time.Now()).
		Update("used_at", time.Now())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// AssignRoleContext 校验角色属于该租户后绑定到用户（user_roles）。
func (d *InviteDAO) AssignRoleContext(ctx context.Context, userID, roleID, tenantID uint) error {
	var cnt int64
	if err := d.db.WithContext(ctx).Model(&model.Role{}).
		Where("id = ? AND tenant_id = ?", roleID, tenantID).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return gorm.ErrRecordNotFound
	}
	return d.db.WithContext(ctx).
		Exec("INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)", userID, roleID).Error
}
