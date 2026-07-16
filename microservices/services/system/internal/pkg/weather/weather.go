// Package weather proxies the Amap (高德) IP-location and live-weather APIs
// for the dashboard hero chip. The API key lives in the weather.provider
// system_settings row (console-managed, hot-reloaded); responses are cached
// in-process because weather freshness is measured in tens of minutes.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SettingKey is the system_settings row consumed by this package.
const SettingKey = "weather.provider"

// ErrNotConfigured is returned when no API key is configured; the frontend
// hides the chip silently on this error.
var ErrNotConfigured = errors.New("weather provider is not configured")

// Settings mirrors the weather.provider setting row.
type Settings struct {
	// AmapKey is the 高德开放平台 Web 服务 key（IP 定位与天气共用）.
	AmapKey string
	// DefaultCity is an adcode or city name used when IP location fails
	// (e.g. intranet deployments whose egress IP cannot be located).
	DefaultCity string
	// CacheMinutes controls the per-city weather cache TTL; 0 means 30.
	CacheMinutes int
}

func (s Settings) Configured() bool { return strings.TrimSpace(s.AmapKey) != "" }

// ApplySetting layers non-empty DB fields over defaults, mirroring the
// runtimeconfig convention used by ai.provider.
func ApplySetting(settings Settings, value map[string]any) Settings {
	if value == nil {
		return settings
	}
	if raw, ok := value["amap_key"].(string); ok {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			settings.AmapKey = trimmed
		}
	}
	if raw, ok := value["default_city"].(string); ok {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			settings.DefaultCity = trimmed
		}
	}
	if raw, ok := value["cache_minutes"].(float64); ok && raw > 0 {
		settings.CacheMinutes = int(raw)
	}
	return settings
}

// Live is the payload returned to the dashboard.
type Live struct {
	City        string `json:"city"`
	Adcode      string `json:"adcode"`
	Weather     string `json:"weather"`
	Temperature string `json:"temperature"`
	Humidity    string `json:"humidity"`
	WindDir     string `json:"wind_dir"`
	WindPower   string `json:"wind_power"`
	ReportTime  string `json:"report_time"`
}

// SettingsReader yields the current provider settings (DB over env).
type SettingsReader interface {
	WeatherSettings(ctx context.Context) Settings
}

// Service resolves a client IP to a city and returns cached live weather.
type Service struct {
	reader     SettingsReader
	httpClient *http.Client
	baseURL    string

	mu    sync.Mutex
	cache map[string]cacheEntry
	// ipCache remembers ip→adcode separately: IP 归属基本不变，
	// 定位接口没必要跟着天气一起过期。
	ipCache map[string]ipCacheEntry
}

type cacheEntry struct {
	live      Live
	expiresAt time.Time
}

type ipCacheEntry struct {
	adcode    string
	city      string
	expiresAt time.Time
}

const (
	defaultCacheTTL = 30 * time.Minute
	ipCacheTTL      = 12 * time.Hour
	amapBaseURL     = "https://restapi.amap.com"
)

func NewService(reader SettingsReader) *Service {
	return &Service{
		reader:     reader,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    amapBaseURL,
		cache:      make(map[string]cacheEntry),
		ipCache:    make(map[string]ipCacheEntry),
	}
}

// NewServiceForTest wires a custom HTTP client and base URL.
func NewServiceForTest(reader SettingsReader, client *http.Client, baseURL string) *Service {
	s := NewService(reader)
	if client != nil {
		s.httpClient = client
	}
	if baseURL != "" {
		s.baseURL = baseURL
	}
	return s
}

// LiveByIP resolves clientIP to a city (falling back to DefaultCity) and
// returns the cached or freshly fetched live weather.
func (s *Service) LiveByIP(ctx context.Context, clientIP string) (*Live, error) {
	settings := s.reader.WeatherSettings(ctx)
	if !settings.Configured() {
		return nil, ErrNotConfigured
	}

	city := s.locate(ctx, settings, clientIP)
	if city == "" {
		city = strings.TrimSpace(settings.DefaultCity)
	}
	if city == "" {
		return nil, fmt.Errorf("weather: no city resolved for ip %q and no default_city configured", clientIP)
	}

	ttl := defaultCacheTTL
	if settings.CacheMinutes > 0 {
		ttl = time.Duration(settings.CacheMinutes) * time.Minute
	}

	s.mu.Lock()
	if entry, ok := s.cache[city]; ok && time.Now().Before(entry.expiresAt) {
		live := entry.live
		s.mu.Unlock()
		return &live, nil
	}
	s.mu.Unlock()

	live, err := s.fetchLive(ctx, settings.AmapKey, city)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[city] = cacheEntry{live: *live, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
	return live, nil
}

// locate maps the client IP to an adcode via 高德 IP 定位. Private and
// loopback addresses skip the API call: the egress IP the API would see is
// the server's, not the browser's, so DefaultCity is the honest answer.
func (s *Service) locate(ctx context.Context, settings Settings, clientIP string) string {
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() {
		return ""
	}

	key := ip.String()
	s.mu.Lock()
	if entry, ok := s.ipCache[key]; ok && time.Now().Before(entry.expiresAt) {
		s.mu.Unlock()
		return entry.adcode
	}
	s.mu.Unlock()

	q := url.Values{"key": {settings.AmapKey}, "ip": {key}, "output": {"JSON"}}
	var resp struct {
		Status string          `json:"status"`
		Adcode json.RawMessage `json:"adcode"`
		City   json.RawMessage `json:"city"`
	}
	if err := s.getJSON(ctx, "/v3/ip?"+q.Encode(), &resp); err != nil {
		return ""
	}
	// 高德对定位失败的 IP 返回空数组而非空串，字段类型不稳，容错解析
	adcode := flexString(resp.Adcode)
	if resp.Status != "1" || adcode == "" {
		return ""
	}

	s.mu.Lock()
	s.ipCache[key] = ipCacheEntry{adcode: adcode, city: flexString(resp.City), expiresAt: time.Now().Add(ipCacheTTL)}
	s.mu.Unlock()
	return adcode
}

func (s *Service) fetchLive(ctx context.Context, key, city string) (*Live, error) {
	q := url.Values{"key": {key}, "city": {city}, "extensions": {"base"}, "output": {"JSON"}}
	var resp struct {
		Status string `json:"status"`
		Info   string `json:"info"`
		Lives  []struct {
			Province    string `json:"province"`
			City        string `json:"city"`
			Adcode      string `json:"adcode"`
			Weather     string `json:"weather"`
			Temperature string `json:"temperature"`
			Humidity    string `json:"humidity"`
			WindDir     string `json:"winddirection"`
			WindPower   string `json:"windpower"`
			ReportTime  string `json:"reporttime"`
		} `json:"lives"`
	}
	if err := s.getJSON(ctx, "/v3/weather/weatherInfo?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	if resp.Status != "1" || len(resp.Lives) == 0 {
		return nil, fmt.Errorf("weather: amap responded status=%s info=%s", resp.Status, resp.Info)
	}

	l := resp.Lives[0]
	cityName := l.City
	// 直辖市 city 与 province 同名，避免"北京市 北京市"式重复
	if l.Province != "" && l.Province != l.City {
		cityName = l.City
	}
	return &Live{
		City:        cityName,
		Adcode:      l.Adcode,
		Weather:     l.Weather,
		Temperature: l.Temperature,
		Humidity:    l.Humidity,
		WindDir:     l.WindDir,
		WindPower:   l.WindPower,
		ReportTime:  l.ReportTime,
	}, nil
}

func (s *Service) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("weather: upstream status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// flexString tolerates 高德 returning either "440300" or [] for the same field.
func flexString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}
