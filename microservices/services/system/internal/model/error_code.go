package model

import "time"

// ErrorCode 错误码管理表：错误码 → 对外文案的映射，支持控制台在线修改。
// 各服务通过 30s TTL 缓存读取（见 internal/pkg/runtimeconfig/error_code.go），
// 修改后无需重启即可热生效。
type ErrorCode struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Code 错误码标识，与 shared/pkg/response 的 ErrorCode 常量对齐（如 DICT_TYPE_NOT_FOUND）
	Code string `gorm:"size:128;not null;uniqueIndex" json:"code"`
	// Message 对外展示文案（命中时覆盖代码里的默认文案）
	Message string `gorm:"size:512;not null" json:"message"`
	// Memo 内部备注（排查提示、负责人等，不对外返回）
	Memo string `gorm:"size:255" json:"memo"`
	// Scope 来源标记：哪个服务/模块产生该错误码（如 system / auth / global）
	Scope     string    `gorm:"size:64;not null;default:global" json:"scope"`
	Status    int8      `gorm:"default:1" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ErrorCode) TableName() string {
	return "error_codes"
}
