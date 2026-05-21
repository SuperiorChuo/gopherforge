package ipinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/go-admin-kit/server/internal/pkg/logger"
)

// IPInfo is an IP geolocation response.
type IPInfo struct {
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"region"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	Zip         string  `json:"zip"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	AS          string  `json:"as"`
	Query       string  `json:"query"`
}

// IPInfoClient queries and caches IP geolocation data.
type IPInfoClient struct {
	httpClient *http.Client
	cache      sync.Map
	cacheTTL   time.Duration
}

// cacheItem stores one cached IP response.
type cacheItem struct {
	info      *IPInfo
	expiredAt time.Time
}

var (
	defaultClient *IPInfoClient
	once          sync.Once
)

// GetClient returns the default client.
func GetClient() *IPInfoClient {
	once.Do(func() {
		defaultClient = NewIPInfoClient(5*time.Second, 1*time.Hour)
	})
	return defaultClient
}

// NewIPInfoClient creates a new client.
func NewIPInfoClient(timeout, cacheTTL time.Duration) *IPInfoClient {
	return &IPInfoClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cacheTTL: cacheTTL,
	}
}

// GetIPInfo returns IP geolocation details.
// It uses the free ip-api.com JSON API: https://ip-api.com/docs/api:json
// Deprecated: use GetIPInfoContext instead.
func (c *IPInfoClient) GetIPInfo(ip string) (*IPInfo, error) {
	return c.GetIPInfoContext(context.Background(), ip)
}

// GetIPInfoContext returns IP geolocation details using the caller context.
// It uses the free ip-api.com JSON API: https://ip-api.com/docs/api:json
func (c *IPInfoClient) GetIPInfoContext(ctx context.Context, ip string) (*IPInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if isPrivateIP(ip) {
		return &IPInfo{
			Status:  "success",
			Country: "Private Network",
			City:    "Private Network",
			Query:   ip,
		}, nil
	}

	if item, ok := c.cache.Load(ip); ok {
		cached := item.(*cacheItem)
		if time.Now().Before(cached.expiredAt) {
			return cached.info, nil
		}
		c.cache.Delete(ip)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// The free endpoint does not support HTTPS and is limited to 45 requests per minute.
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,region,regionName,city,zip,lat,lon,timezone,isp,org,as,query&lang=en", ip)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if logger.Logger != nil {
			logger.Warn("ip geolocation lookup failed", logger.String("ip", ip), logger.Err(err))
		}
		return nil, fmt.Errorf("ip geolocation lookup failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	var info IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if info.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", info.Message)
	}

	c.cache.Store(ip, &cacheItem{
		info:      &info,
		expiredAt: time.Now().Add(c.cacheTTL),
	})

	return &info, nil
}

// GetLocation returns a simplified location string.
// Deprecated: use GetLocationContext instead.
func (c *IPInfoClient) GetLocation(ip string) string {
	return c.GetLocationContext(context.Background(), ip)
}

// GetLocationContext returns a simplified location string using the caller context.
func (c *IPInfoClient) GetLocationContext(ctx context.Context, ip string) string {
	info, err := c.GetIPInfoContext(ctx, ip)
	if err != nil {
		return ""
	}
	return formatLocation(info)
}

// GetLocationAsync returns a simplified location string asynchronously.
func (c *IPInfoClient) GetLocationAsync(ip string, callback func(location string)) {
	go func() {
		location := c.GetLocation(ip)
		if callback != nil {
			callback(location)
		}
	}()
}

// formatLocation formats geolocation details.
func formatLocation(info *IPInfo) string {
	if info == nil {
		return ""
	}

	if info.Country == "Private Network" {
		return "Private Network"
	}

	if info.CountryCode == "CN" {
		if info.RegionName != "" && info.City != "" {
			if info.RegionName == info.City {
				return info.City
			}
			return info.RegionName + " " + info.City
		}
		if info.RegionName != "" {
			return info.RegionName
		}
		if info.City != "" {
			return info.City
		}
		return info.Country
	}

	if info.City != "" {
		return info.Country + " " + info.City
	}
	return info.Country
}

// isPrivateIP reports whether an IP belongs to a private or local range.
func isPrivateIP(ip string) bool {
	privateIPPatterns := []string{
		`^127\.`,                         // 127.x.x.x
		`^10\.`,                          // 10.x.x.x
		`^172\.(1[6-9]|2[0-9]|3[0-1])\.`, // 172.16.x.x - 172.31.x.x
		`^192\.168\.`,                    // 192.168.x.x
		`^::1$`,                          // IPv6 localhost
		`^localhost$`,                    // localhost
		`^0\.0\.0\.0$`,                   // 0.0.0.0
		`^fe80:`,                         // IPv6 link-local
		`^fc00:`,                         // IPv6 unique local
		`^fd`,                            // IPv6 unique local
	}

	for _, pattern := range privateIPPatterns {
		if matched, _ := regexp.MatchString(pattern, ip); matched {
			return true
		}
	}

	return false
}

// GetIPInfoByQuery queries with the default client.
func GetIPInfoByQuery(ip string) (*IPInfo, error) {
	return GetClient().GetIPInfo(ip)
}

// GetIPInfoByQueryContext queries with the default client using the caller context.
func GetIPInfoByQueryContext(ctx context.Context, ip string) (*IPInfo, error) {
	return GetClient().GetIPInfoContext(ctx, ip)
}

// GetLocationByIP returns a simplified location with the default client.
func GetLocationByIP(ip string) string {
	return GetClient().GetLocation(ip)
}

// GetLocationByIPContext returns a simplified location with the default client using the caller context.
func GetLocationByIPContext(ctx context.Context, ip string) string {
	return GetClient().GetLocationContext(ctx, ip)
}

// ClearCache clears cached geolocation responses.
func (c *IPInfoClient) ClearCache() {
	c.cache.Range(func(key, value any) bool {
		c.cache.Delete(key)
		return true
	})
}
