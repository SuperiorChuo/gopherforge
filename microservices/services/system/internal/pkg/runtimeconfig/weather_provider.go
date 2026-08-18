package runtimeconfig

import (
	"context"
	"sync"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/database"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	sharedruntimeconfig "github.com/go-admin-kit/services/shared/pkg/runtimeconfig"
	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/pkg/weather"
)

// WeatherProviderSettingKey is the system_settings row holding the Amap key
// and defaults for the dashboard weather chip.
const WeatherProviderSettingKey = weather.SettingKey

type WeatherStore interface {
	GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error)
}

// CachedWeatherReader layers the weather.provider setting row over empty
// defaults with a short TTL, refreshed instantly via the invalidation channel.
type CachedWeatherReader struct {
	reader      *sharedruntimeconfig.CachedSettingReader[weather.Settings]
	tenantStore TenantSettingStore
}

func NewCachedWeatherReader(store WeatherStore, ttl time.Duration) *CachedWeatherReader {
	return &CachedWeatherReader{
		reader: sharedruntimeconfig.NewCachedSettingReader(
			store, WeatherProviderSettingKey, ttl,
			func() weather.Settings { return weather.Settings{} }, weather.ApplySetting,
		),
		tenantStore: defaultTenantSettingStore{},
	}
}

var (
	defaultWeatherOnce   sync.Once
	defaultWeatherReader *CachedWeatherReader
)

func DefaultWeatherReader() *CachedWeatherReader {
	defaultWeatherOnce.Do(func() {
		defaultWeatherReader = NewCachedWeatherReader(defaultWeatherStore{}, 30*time.Second)
	})
	return defaultWeatherReader
}

type defaultWeatherStore struct{}

func (defaultWeatherStore) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	if database.DB == nil {
		return nil, ErrStoreUnavailable
	}
	// system 的 SettingDAO 不回退全局连接，必须显式传 database.DB
	return systemdao.NewSettingDAO(database.DB).GetByKeyContext(ctx, key)
}

// WeatherSettings implements weather.SettingsReader.
func (r *CachedWeatherReader) WeatherSettings(ctx context.Context) weather.Settings {
	if r == nil {
		return weather.Settings{}
	}
	return r.applyTenantOverride(ctx, r.reader.Value(ctx))
}

// applyTenantOverride 显式租户上下文命中 tenant_settings 覆盖时应用之；
// 后台/无租户上下文维持平台默认。
func (r *CachedWeatherReader) applyTenantOverride(ctx context.Context, settings weather.Settings) weather.Settings {
	if r == nil || r.tenantStore == nil {
		return settings
	}
	if override := tenantOverride(ctx, r.tenantStore, WeatherProviderSettingKey); override != nil {
		settings = weather.ApplySetting(settings, override.ValueJSON)
	}
	return settings
}

func (r *CachedWeatherReader) Refresh(ctx context.Context) error {
	if r == nil {
		return nil
	}
	return r.reader.Refresh(ctx)
}
