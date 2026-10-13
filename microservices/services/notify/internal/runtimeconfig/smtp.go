// Package runtimeconfig 提供 notify 的 DB 热配置读取（照 ai 服务
// ai_provider.go 范式的精简版）：system_settings 行叠加 env 兜底 + TTL 缓存，
// 控制台设置页改配置后 30s 内热生效，无需重启。
package runtimeconfig

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/go-admin-kit/services/notify/internal/channel"
	"gorm.io/gorm"
)

// SMTPSettingKey system_settings 行键：控制台「通知设置 → 邮件发信（SMTP）」
// 卡片写入（前缀 notification. 对齐设置页既有分组）。
// value_json 字段：host / port / user / password / from / enabled(bool)。
const SMTPSettingKey = "notification.smtp"

// SMTPReader system_settings 直读（同库）+ TTL 缓存 + env 兜底。
type SMTPReader struct {
	db  *gorm.DB
	env channel.SMTPConfig
	ttl time.Duration

	mu      sync.RWMutex
	cached  channel.SMTPConfig
	expires time.Time
	loaded  bool
}

func NewSMTPReader(db *gorm.DB, env channel.SMTPConfig, ttl time.Duration) *SMTPReader {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &SMTPReader{db: db, env: env, ttl: ttl}
}

// settingRow 只映射需要的列，避免引入 system 服务的 model。
type settingRow struct {
	ValueJSON map[string]any `gorm:"column:value_json;type:json;serializer:json"`
}

// Current 返回叠加后的 SMTP 配置：DB 非空字段覆盖 env（enabled=false 显式
// 关闭通道）。读库失败回退上次缓存 / env——发信路径不因配置读取失败中断。
func (r *SMTPReader) Current(ctx context.Context) channel.SMTPConfig {
	if r == nil {
		return channel.SMTPConfig{}
	}
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expires) {
		cfg := r.cached
		r.mu.RUnlock()
		return cfg
	}
	r.mu.RUnlock()

	cfg := r.env
	var row settingRow
	err := r.db.WithContext(ctx).Table("system_settings").
		Select("value_json").Where("setting_key = ?", SMTPSettingKey).
		Take(&row).Error
	switch {
	case err == nil:
		cfg = applySMTPSetting(cfg, row.ValueJSON)
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 无 DB 配置：纯 env
	default:
		// 读库异常：有旧缓存用旧缓存（不刷新过期时间，下次重试）
		r.mu.RLock()
		if r.loaded {
			cached := r.cached
			r.mu.RUnlock()
			return cached
		}
		r.mu.RUnlock()
		return cfg
	}

	r.mu.Lock()
	r.cached = cfg
	r.expires = now.Add(r.ttl)
	r.loaded = true
	r.mu.Unlock()
	return cfg
}

func applySMTPSetting(cfg channel.SMTPConfig, v map[string]any) channel.SMTPConfig {
	if v == nil {
		return cfg
	}
	cfg.Host = strSetting(v, "host", cfg.Host)
	cfg.Port = strSetting(v, "port", cfg.Port)
	cfg.User = strSetting(v, "user", cfg.User)
	cfg.Password = strSetting(v, "password", cfg.Password)
	cfg.From = strSetting(v, "from", cfg.From)
	if b, isBool := v["enabled"].(bool); isBool && !b {
		cfg.Host = "" // 空 Host 即禁用（与 SMTPEmail.Enabled 判定一致）
	}
	return cfg
}

func strSetting(v map[string]any, key, fallback string) string {
	if raw, isStr := v[key].(string); isStr {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}
