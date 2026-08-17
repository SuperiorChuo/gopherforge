package runtimeconfig

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/database"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/pkg/weather"
	"gorm.io/gorm"
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
	store       WeatherStore
	tenantStore TenantSettingStore
	ttl         time.Duration

	mu        sync.RWMutex
	settings  weather.Settings
	expiresAt time.Time
	loaded    bool
}

func NewCachedWeatherReader(store WeatherStore, ttl time.Duration) *CachedWeatherReader {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CachedWeatherReader{store: store, tenantStore: defaultTenantSettingStore{}, ttl: ttl}
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
	var settings weather.Settings
	if r == nil {
		return settings
	}
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		settings = r.settings
		r.mu.RUnlock()
		return r.applyTenantOverride(ctx, settings)
	}
	r.mu.RUnlock()

	if err := r.Refresh(ctx); err != nil {
		r.mu.RLock()
		settings = r.settings
		loaded := r.loaded
		r.mu.RUnlock()
		if !loaded {
			return weather.Settings{}
		}
	} else {
		r.mu.RLock()
		settings = r.settings
		r.mu.RUnlock()
	}
	return r.applyTenantOverride(ctx, settings)
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
	if ctx == nil {
		ctx = context.Background()
	}

	var settings weather.Settings
	var err error
	if r.store != nil {
		var setting *model.SystemSetting
		setting, err = r.store.GetByKeyContext(ctx, WeatherProviderSettingKey)
		switch {
		case err == nil && setting != nil:
			settings = weather.ApplySetting(settings, setting.ValueJSON)
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = nil
		}
	}

	if err == nil {
		r.mu.Lock()
		r.settings = settings
		r.expiresAt = time.Now().Add(r.ttl)
		r.loaded = true
		r.mu.Unlock()
	}
	return err
}
