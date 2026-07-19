package runtimeconfig

import (
	"context"
	"errors"
	"sync"
	"time"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/database"
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
	store WeatherStore
	ttl   time.Duration

	mu        sync.RWMutex
	settings  weather.Settings
	expiresAt time.Time
	loaded    bool
}

func NewCachedWeatherReader(store WeatherStore, ttl time.Duration) *CachedWeatherReader {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CachedWeatherReader{store: store, ttl: ttl}
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
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		settings := r.settings
		r.mu.RUnlock()
		return settings
	}
	r.mu.RUnlock()

	if err := r.Refresh(ctx); err != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if r.loaded {
			return r.settings
		}
		return weather.Settings{}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings
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
