package system

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/mask"
	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/system/internal/pkg/tenant"
	"gorm.io/gorm"
)

type SettingService struct {
	settingDAO         systemdao.SettingDAO
	tenantSettingDAO   systemdao.TenantSettingDAO
	runtimeInvalidator runtimeconfig.SecurityPolicyInvalidator
	emailInvalidator   runtimeconfig.EmailNotificationInvalidator
}

// NewSettingServiceWithDB builds a SettingService backed by an injected
// database handle. Invalidators keep their default implementations.
func NewSettingServiceWithDB(db *gorm.DB) SettingService {
	return SettingService{
		settingDAO:       *systemdao.NewSettingDAO(db),
		tenantSettingDAO: *systemdao.NewTenantSettingDAO(db),
	}
}

const runtimeConfigInvalidationTimeout = 2 * time.Second

type UpsertSettingRequest struct {
	SettingKey string         `json:"setting_key"`
	ValueJSON  map[string]any `json:"value_json" binding:"required"`
}

type BatchUpsertSettingsRequest struct {
	Settings []UpsertSettingRequest `json:"settings" binding:"required"`
}

var (
	ErrSystemSettingNotFound   = errors.New("system setting not found")
	ErrInvalidSystemSettingKey = errors.New("invalid system setting key")
	// ErrProtectedSystemSettingKey guards machine-managed secret settings (e.g.
	// the OIDC signing private key) from being read back, overwritten, or
	// deleted through the generic settings API.
	ErrProtectedSystemSettingKey = errors.New("system setting is protected and cannot be accessed via this API")
)

var systemSettingKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

// protectedSettingKeys are auto-generated secrets that must never be exposed or
// mutated through the generic settings CRUD API. The OIDC signing key is an RSA
// private key managed by auth-service; leaking it lets an attacker forge
// id_tokens for any user, and deleting/overwriting it breaks id_token
// verification across replicas. Keep this list in sync with the owning service.
var protectedSettingKeys = map[string]bool{
	"oidc.signing_key": true,
}

func isProtectedSettingKey(key string) bool {
	return protectedSettingKeys[key]
}

// maskedSettingValue is returned in place of a protected setting's real value on
// list reads, so its existence is visible but its secret contents are not.
var maskedSettingValue = map[string]any{"protected": true}

func (s *SettingService) ListSettingsContext(ctx context.Context, group string) ([]model.SystemSetting, error) {
	settings, err := s.settingDAO.ListContext(ctx, group)
	if err != nil {
		return nil, err
	}
	for i := range settings {
		if isProtectedSettingKey(settings[i].SettingKey) {
			settings[i].ValueJSON = maskedSettingValue
			continue
		}
		settings[i].ValueJSON = maskSettingValue(settings[i].ValueJSON)
	}
	return settings, nil
}

func (s *SettingService) GetSettingContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	if !isValidSystemSettingKey(key) {
		return nil, ErrInvalidSystemSettingKey
	}
	if isProtectedSettingKey(key) {
		return nil, ErrProtectedSystemSettingKey
	}
	setting, err := s.settingDAO.GetByKeyContext(ctx, key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSystemSettingNotFound
		}
		return nil, err
	}
	setting.ValueJSON = maskSettingValue(setting.ValueJSON)
	return setting, nil
}

func (s *SettingService) UpsertSettingContext(ctx context.Context, req UpsertSettingRequest) (*model.SystemSetting, error) {
	if !isValidSystemSettingKey(req.SettingKey) {
		return nil, ErrInvalidSystemSettingKey
	}
	if isProtectedSettingKey(req.SettingKey) {
		return nil, ErrProtectedSystemSettingKey
	}
	if req.ValueJSON == nil {
		req.ValueJSON = map[string]any{}
	}
	valueJSON, err := s.restoreRedactedSettingValueContext(ctx, req.SettingKey, req.ValueJSON)
	if err != nil {
		return nil, err
	}
	setting := &model.SystemSetting{
		SettingKey: req.SettingKey,
		ValueJSON:  valueJSON,
	}
	if err := s.settingDAO.UpsertContext(ctx, setting); err != nil {
		return nil, err
	}
	s.refreshRuntimeConfigIfNeeded(ctx, req.SettingKey)
	return maskedSetting(setting), nil
}

func (s *SettingService) BatchUpsertSettingsContext(ctx context.Context, req BatchUpsertSettingsRequest) ([]model.SystemSetting, error) {
	settings := make([]model.SystemSetting, 0, len(req.Settings))
	for _, item := range req.Settings {
		if !isValidSystemSettingKey(item.SettingKey) {
			return nil, ErrInvalidSystemSettingKey
		}
		if isProtectedSettingKey(item.SettingKey) {
			return nil, ErrProtectedSystemSettingKey
		}
		if item.ValueJSON == nil {
			item.ValueJSON = map[string]any{}
		}
		valueJSON, err := s.restoreRedactedSettingValueContext(ctx, item.SettingKey, item.ValueJSON)
		if err != nil {
			return nil, err
		}
		settings = append(settings, model.SystemSetting{
			SettingKey: item.SettingKey,
			ValueJSON:  valueJSON,
		})
	}
	if err := s.settingDAO.BatchUpsertContext(ctx, settings); err != nil {
		return nil, err
	}
	for _, setting := range settings {
		s.refreshRuntimeConfigIfNeeded(ctx, setting.SettingKey)
	}
	for i := range settings {
		settings[i].ValueJSON = maskSettingValue(settings[i].ValueJSON)
	}
	return settings, nil
}

func (s *SettingService) restoreRedactedSettingValueContext(ctx context.Context, key string, incoming map[string]any) (map[string]any, error) {
	if !mask.ContainsRedactedSensitiveValue(incoming) {
		return incoming, nil
	}
	stored := map[string]any{}
	existing, err := s.settingDAO.GetByKeyContext(ctx, key)
	if err == nil {
		stored = existing.ValueJSON
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	restored, _ := mask.RestoreRedactedSensitiveValues(incoming, stored).(map[string]any)
	return restored, nil
}

func maskedSetting(setting *model.SystemSetting) *model.SystemSetting {
	if setting == nil {
		return nil
	}
	masked := *setting
	masked.ValueJSON = maskSettingValue(setting.ValueJSON)
	return &masked
}

func maskSettingValue(value map[string]any) map[string]any {
	masked, _ := mask.MaskSensitiveValue(value).(map[string]any)
	return masked
}

func (s *SettingService) DeleteSettingContext(ctx context.Context, key string) error {
	if !isValidSystemSettingKey(key) {
		return ErrInvalidSystemSettingKey
	}
	if isProtectedSettingKey(key) {
		return ErrProtectedSystemSettingKey
	}
	if err := s.settingDAO.DeleteContext(ctx, key); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSystemSettingNotFound
		}
		return err
	}
	s.refreshRuntimeConfigIfNeeded(ctx, key)
	return nil
}

// ---------- 租户级配置覆盖（tenant_settings） ----------

var (
	ErrTenantSettingNotConfigurable = errors.New("该配置不属于租户可配范围，仅平台可配")
	ErrTenantSettingNotFound        = errors.New("tenant setting not found")
	ErrTenantContextRequired        = errors.New("tenant context required")
)

// tenantConfigurableKeys 租户管理员可配置的键白名单。与三个请求路径消费方
// （ai.provider / notification.email / weather.provider）一一对应；平台机密
// （oidc.signing_key、security.policy）不进租户维度。
var tenantConfigurableKeys = map[string]bool{
	"ai.provider":        true,
	"notification.email": true,
	"weather.provider":   true,
}

func isTenantConfigurableKey(key string) bool {
	return tenantConfigurableKeys[key]
}

// ListTenantSettingsContext 返回当前租户的全部覆盖行（value 已脱敏）。
func (s *SettingService) ListTenantSettingsContext(ctx context.Context) ([]model.TenantSetting, error) {
	tid := tenant.FromContext(ctx)
	if tid == 0 {
		return nil, ErrTenantContextRequired
	}
	settings, err := s.tenantSettingDAO.ListContext(ctx, tid)
	if err != nil {
		return nil, err
	}
	for i := range settings {
		settings[i].ValueJSON = maskSettingValue(settings[i].ValueJSON)
	}
	return settings, nil
}

// UpsertTenantSettingContext 写当前租户对某键的覆盖。白名单外拒绝。
func (s *SettingService) UpsertTenantSettingContext(ctx context.Context, key string, value map[string]any) (*model.TenantSetting, error) {
	if !isValidSystemSettingKey(key) {
		return nil, ErrInvalidSystemSettingKey
	}
	if !isTenantConfigurableKey(key) {
		return nil, ErrTenantSettingNotConfigurable
	}
	tid := tenant.FromContext(ctx)
	if tid == 0 {
		return nil, ErrTenantContextRequired
	}
	if value == nil {
		value = map[string]any{}
	}
	valueJSON, err := s.restoreRedactedTenantSettingValueContext(ctx, tid, key, value)
	if err != nil {
		return nil, err
	}
	setting := &model.TenantSetting{TenantID: tid, SettingKey: key, ValueJSON: valueJSON}
	if err := s.tenantSettingDAO.UpsertContext(ctx, setting); err != nil {
		return nil, err
	}
	masked := *setting
	masked.ValueJSON = maskSettingValue(setting.ValueJSON)
	return &masked, nil
}

// DeleteTenantSettingContext 删除当前租户对某键的覆盖，之后回落平台默认。
func (s *SettingService) DeleteTenantSettingContext(ctx context.Context, key string) error {
	if !isValidSystemSettingKey(key) {
		return ErrInvalidSystemSettingKey
	}
	if !isTenantConfigurableKey(key) {
		return ErrTenantSettingNotConfigurable
	}
	tid := tenant.FromContext(ctx)
	if tid == 0 {
		return ErrTenantContextRequired
	}
	if err := s.tenantSettingDAO.DeleteContext(ctx, tid, key); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTenantSettingNotFound
		}
		return err
	}
	return nil
}

// restoreRedactedTenantSettingValueContext 与平台版同语义，但读的是租户自己的覆盖行。
func (s *SettingService) restoreRedactedTenantSettingValueContext(ctx context.Context, tid uint, key string, incoming map[string]any) (map[string]any, error) {
	if !mask.ContainsRedactedSensitiveValue(incoming) {
		return incoming, nil
	}
	stored := map[string]any{}
	existing, err := s.tenantSettingDAO.GetByKeyContext(ctx, tid, key)
	if err == nil {
		stored = existing.ValueJSON
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	restored, _ := mask.RestoreRedactedSensitiveValues(incoming, stored).(map[string]any)
	return restored, nil
}

func (s *SettingService) refreshRuntimeConfigIfNeeded(ctx context.Context, key string) {
	switch key {
	case runtimeconfig.SecurityPolicySettingKey:
		invalidator := s.runtimeInvalidator
		if invalidator == nil {
			invalidator = runtimeconfig.DefaultSecurityPolicyReader()
		}
		refreshCtx, cancel := runtimeConfigInvalidationContext(ctx)
		defer cancel()
		_ = invalidator.Refresh(refreshCtx)
		_ = runtimeconfig.PublishInvalidation(refreshCtx, key)
	case runtimeconfig.EmailNotificationSettingKey:
		invalidator := s.emailInvalidator
		if invalidator == nil {
			invalidator = runtimeconfig.DefaultEmailNotificationReader()
		}
		refreshCtx, cancel := runtimeConfigInvalidationContext(ctx)
		defer cancel()
		_ = invalidator.Refresh(refreshCtx)
		_ = runtimeconfig.PublishInvalidation(refreshCtx, key)
	case runtimeconfig.AIProviderSettingKey:
		// AI 配置由 ai-service 消费，这里只负责把变更广播出去。
		refreshCtx, cancel := runtimeConfigInvalidationContext(ctx)
		defer cancel()
		_ = runtimeconfig.PublishInvalidation(refreshCtx, key)
	case runtimeconfig.WeatherProviderSettingKey:
		// 天气配置由本服务消费：先刷本进程缓存，再广播给其他副本。
		refreshCtx, cancel := runtimeConfigInvalidationContext(ctx)
		defer cancel()
		_ = runtimeconfig.DefaultWeatherReader().Refresh(refreshCtx)
		_ = runtimeconfig.PublishInvalidation(refreshCtx, key)
	}
}

func runtimeConfigInvalidationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, runtimeConfigInvalidationTimeout)
}

func isValidSystemSettingKey(key string) bool {
	return systemSettingKeyPattern.MatchString(key)
}
