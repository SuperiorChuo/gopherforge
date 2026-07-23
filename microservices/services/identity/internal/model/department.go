package model

import "time"

// Department stores organization hierarchy data.
type Department struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	TenantID uint   `gorm:"not null;default:1;uniqueIndex:ux_depts_tenant_code,priority:1;index" json:"tenant_id"`
	Name     string `gorm:"size:100;not null" json:"name"`
	Code     string `gorm:"size:50;uniqueIndex:ux_depts_tenant_code,priority:2" json:"code"`
	ParentID uint   `gorm:"default:0;index" json:"parent_id"`
	Leader   string `gorm:"size:50" json:"leader"`
	// LeaderUserID 部门主管用户 id（0=未设）。bpm dept_leader 审批人规则据此
	// 取主管（Leader 字符串仅展示用）；不做级联校验，用户删除后由审批侧
	// emptyFallback 兜底。列由 goose 迁移 000025 添加（identity/migrations）。
	LeaderUserID uint64       `gorm:"not null;default:0" json:"leader_user_id"`
	Phone     string       `gorm:"size:20" json:"phone"`
	Email     string       `gorm:"size:100" json:"email"`
	Sort      int          `gorm:"default:0" json:"sort"`
	Status    int8         `gorm:"default:1" json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Children  []Department `gorm:"-" json:"children,omitempty"`
}
