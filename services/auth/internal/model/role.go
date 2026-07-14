package model

import "time"

// Role stores RBAC role metadata and data-scope settings.
type Role struct {
	ID                     uint                      `gorm:"primaryKey" json:"id"`
	Name                   string                    `gorm:"size:50;not null" json:"name"`
	Code                   string                    `gorm:"size:50;not null;uniqueIndex" json:"code"`
	Description            string                    `gorm:"size:255" json:"description"`
	DataScope              string                    `gorm:"size:32;not null;default:self;index" json:"data_scope"`
	DataScopeDepartmentIDs []uint                    `gorm:"-" json:"data_scope_department_ids,omitempty"`
	DataScopeDepartments   []RoleDataScopeDepartment `gorm:"foreignKey:RoleID" json:"-"`
	CreatedAt              time.Time                 `json:"created_at"`
	UpdatedAt              time.Time                 `json:"updated_at"`
	Users                  []User                    `gorm:"many2many:user_roles;" json:"users,omitempty"`
	Permissions            []Permission              `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
}

// RolePermission links roles to permissions.
type RolePermission struct {
	ID           uint `gorm:"primaryKey" json:"id"`
	RoleID       uint `gorm:"not null;index" json:"role_id"`
	PermissionID uint `gorm:"not null;index" json:"permission_id"`
}

// RoleDataScopeDepartment links roles to custom data-scope departments.
type RoleDataScopeDepartment struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RoleID       uint      `gorm:"not null;uniqueIndex:uk_role_data_scope_department;index" json:"role_id"`
	DepartmentID uint      `gorm:"not null;uniqueIndex:uk_role_data_scope_department;index" json:"department_id"`
	CreatedAt    time.Time `json:"created_at"`
}
