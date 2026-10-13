// Package weather 代理高德（Amap）的 IP 定位与实时天气 API，
// 供看板顶部组件使用。API key 存于 weather.provider 的 system_settings
// 配置行（控制台管理、热重载）；响应在进程内缓存，
// 因为天气的新鲜度以数十分钟为计量单位。
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

// SettingKey 是本包消费的 system_settings 配置行。
const SettingKey = "weather.provider"

// ErrNotConfigured 在未配置任何 API key 时返回；前端遇到该错误会静默隐藏组件。
var ErrNotConfigured = errors.New("weather provider is not configured")

// Settings 对应 weather.provider 配置行的结构。
type Settings struct {
	// AmapKey 是高德开放平台 Web 服务 key（IP 定位与天气共用）。
	AmapKey string
	// DefaultCity 是 IP 定位失败时使用的 adcode 或城市名
	// （例如出口 IP 无法定位的内网部署）。
	DefaultCity string
	// CacheMinutes 控制按城市划分的天气缓存 TTL；0 表示 30。
	CacheMinutes int
}

func (s Settings) Configured() bool { return strings.TrimSpace(s.AmapKey) != "" }

// ApplySetting 把非空的数据库字段叠加到默认值之上，与 ai.provider
// 使用的 runtimeconfig 约定保持一致。
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

// Live 是返回给看板的数据载荷。
type Live struct {
	City        string `json:"city"`
	Adcode      string `json:"adcode"`
	Weather     string `json:"weather"`
	Temperature string `json:"temperature"`
	Humidity    string `json:"humidity"`
	WindDir     string `json:"wind_dir"`
	WindPower   string `json:"wind_power"`
	ReportTime  string `json:"report_time"`
	// TempHigh / TempLow 来自高德预报当天的 daytemp / nighttemp；
	// 取 max/min 作为展示用高低，拉取失败时留空，前端降级隐藏。
	TempHigh string `json:"temp_high,omitempty"`
	TempLow  string `json:"temp_low,omitempty"`
}

// SettingsReader 提供当前 provider 设置（数据库优先于环境变量）。
type SettingsReader interface {
	WeatherSettings(ctx context.Context) Settings
}

// Service 将客户端 IP 解析为城市，并返回缓存的实时天气。
type Service struct {
	reader     SettingsReader
	httpClient *http.Client
	baseURL    string

	mu    sync.Mutex
	cache map[string]cacheEntry
	// ipCache 单独记录 ip→adcode：IP 归属基本不变，
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

// NewServiceForTest 注入自定义 HTTP client 与 base URL。
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

// LiveByIP 将 clientIP 解析为城市（失败时回退到 DefaultCity），
// 并返回已缓存或新拉取的实时天气。
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

// locate 通过高德 IP 定位把客户端 IP 映射为 adcode。私有与
// 回环地址会跳过 API 调用：API 看到的出口 IP 是服务器的，
// 而非浏览器的，因此 DefaultCity 才是诚实的答案。
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
	live := &Live{
		City:        cityName,
		Adcode:      l.Adcode,
		Weather:     l.Weather,
		Temperature: l.Temperature,
		Humidity:    l.Humidity,
		WindDir:     l.WindDir,
		WindPower:   l.WindPower,
		ReportTime:  l.ReportTime,
	}
	// 预报高低是装饰字段：失败不影响主实况
	if high, low, ok := s.fetchTodayRange(ctx, key, city); ok {
		live.TempHigh = high
		live.TempLow = low
	}
	return live, nil
}

// fetchTodayRange 拉取高德预报（extensions=all）当天的 daytemp/nighttemp。
func (s *Service) fetchTodayRange(ctx context.Context, key, city string) (high, low string, ok bool) {
	q := url.Values{"key": {key}, "city": {city}, "extensions": {"all"}, "output": {"JSON"}}
	var resp struct {
		Status    string `json:"status"`
		Forecasts []struct {
			Casts []struct {
				DayTemp   string `json:"daytemp"`
				NightTemp string `json:"nighttemp"`
			} `json:"casts"`
		} `json:"forecasts"`
	}
	if err := s.getJSON(ctx, "/v3/weather/weatherInfo?"+q.Encode(), &resp); err != nil {
		return "", "", false
	}
	if resp.Status != "1" || len(resp.Forecasts) == 0 || len(resp.Forecasts[0].Casts) == 0 {
		return "", "", false
	}
	c := resp.Forecasts[0].Casts[0]
	if c.DayTemp == "" || c.NightTemp == "" {
		return "", "", false
	}
	// day/night 未必 day>night（高原昼夜反差），统一成 高/低
	if dayN, err1 := parseTemp(c.DayTemp); err1 == nil {
		if nightN, err2 := parseTemp(c.NightTemp); err2 == nil {
			if dayN >= nightN {
				return c.DayTemp, c.NightTemp, true
			}
			return c.NightTemp, c.DayTemp, true
		}
	}
	return c.DayTemp, c.NightTemp, true
}

func parseTemp(s string) (float64, error) {
	var n float64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%f", &n)
	return n, err
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

// flexString 兼容高德对同一字段返回 "440300" 或 [] 的情况。
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
