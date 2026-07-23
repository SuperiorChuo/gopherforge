package system

import (
	"context"
	"errors"
	"regexp"
	"time"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	"gorm.io/gorm"
)

type SettingService struct {
	settingDAO         systemdao.SettingDAO
	runtimeInvalidator runtimeconfig.SecurityPolicyInvalidator
	emailInvalidator   runtimeconfig.EmailNotificationInvalidator
}

// NewSettingServiceWithDB builds a SettingService backed by an injected
// database handle. Invalidators keep their default implementations.
func NewSettingServiceWithDB(db *gorm.DB) SettingService {
	return SettingService{settingDAO: *systemdao.NewSettingDAO(db)}
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
		}
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
	setting := &model.SystemSetting{
		SettingKey: req.SettingKey,
		ValueJSON:  req.ValueJSON,
	}
	if err := s.settingDAO.UpsertContext(ctx, setting); err != nil {
		return nil, err
	}
	s.refreshRuntimeConfigIfNeeded(ctx, req.SettingKey)
	return setting, nil
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
		settings = append(settings, model.SystemSetting{
			SettingKey: item.SettingKey,
			ValueJSON:  item.ValueJSON,
		})
	}
	if err := s.settingDAO.BatchUpsertContext(ctx, settings); err != nil {
		return nil, err
	}
	for _, setting := range settings {
		s.refreshRuntimeConfigIfNeeded(ctx, setting.SettingKey)
	}
	return settings, nil
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
		// 配置变更只负责广播失效，由对应消费方按需刷新。
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
