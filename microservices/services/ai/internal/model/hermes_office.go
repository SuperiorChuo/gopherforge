package localmodel

import "time"

// 3D 办公室地图的数据模型。工位只保存布局与员工绑定，设备状态在查询时
// 从 hermes_devices 关联得到，不在此冗余存储。
//
// 两个结构体都带 TenantID，因此会被 internal/pkg/tenant 的 schema 驱动插件
// 自动加上租户过滤（见 tenant.Plugin）。

type HermesOfficeFloor struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint   `gorm:"not null;index" json:"tenant_id"`
	Name     string `gorm:"size:80;not null" json:"name"`
	Sort     int    `gorm:"not null" json:"sort"`
	// 平面尺寸（米）：3D 里 Width→X 轴、Depth→Z 轴
	Width     float64   `gorm:"not null" json:"width"`
	Depth     float64   `gorm:"not null" json:"depth"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (HermesOfficeFloor) TableName() string { return "hermes_office_floors" }

type HermesOfficeSeat struct {
	ID       uint64  `gorm:"primaryKey" json:"id"`
	TenantID uint    `gorm:"not null;index" json:"tenant_id"`
	FloorID  uint64  `gorm:"not null;index" json:"floor_id"`
	Label    string  `gorm:"size:40;not null" json:"label"`
	X        float64 `gorm:"not null" json:"x"`
	// 平面第二轴，3D 里映射到 z
	Y float64 `gorm:"not null" json:"y"`
	// 朝向（度），约束在 [0, 360)
	Rotation float64 `gorm:"not null" json:"rotation"`
	// 0 = 空位
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (HermesOfficeSeat) TableName() string { return "hermes_office_seats" }
