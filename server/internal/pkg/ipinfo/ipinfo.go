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

// IPInfo IP归属地信息
type IPInfo struct {
	Status      string  `json:"status"`      // success 或 fail
	Message     string  `json:"message"`     // 错误信息（当 status 为 fail 时）
	Country     string  `json:"country"`     // 国家
	CountryCode string  `json:"countryCode"` // 国家代码
	Region      string  `json:"region"`      // 地区代码
	RegionName  string  `json:"regionName"`  // 地区名称
	City        string  `json:"city"`        // 城市
	Zip         string  `json:"zip"`         // 邮编
	Lat         float64 `json:"lat"`         // 纬度
	Lon         float64 `json:"lon"`         // 经度
	Timezone    string  `json:"timezone"`    // 时区
	ISP         string  `json:"isp"`         // ISP 服务商
	Org         string  `json:"org"`         // 组织
	AS          string  `json:"as"`          // AS 号
	Query       string  `json:"query"`       // 查询的 IP
}

// IPInfoClient IP 归属地查询客户端
type IPInfoClient struct {
	httpClient *http.Client
	cache      sync.Map // 简单的内存缓存
	cacheTTL   time.Duration
}

// cacheItem 缓存项
type cacheItem struct {
	info      *IPInfo
	expiredAt time.Time
}

var (
	defaultClient *IPInfoClient
	once          sync.Once
)

// GetClient 获取默认客户端
func GetClient() *IPInfoClient {
	once.Do(func() {
		defaultClient = NewIPInfoClient(5*time.Second, 1*time.Hour)
	})
	return defaultClient
}

// NewIPInfoClient 创建新的客户端
func NewIPInfoClient(timeout, cacheTTL time.Duration) *IPInfoClient {
	return &IPInfoClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cacheTTL: cacheTTL,
	}
}

// GetIPInfo 获取 IP 归属地信息
// 使用 ip-api.com 免费 API
// 文档：https://ip-api.com/docs/api:json
func (c *IPInfoClient) GetIPInfo(ip string) (*IPInfo, error) {
	// 检查是否是内网 IP
	if isPrivateIP(ip) {
		return &IPInfo{
			Status:  "success",
			Country: "内网",
			City:    "内网",
			Query:   ip,
		}, nil
	}

	// 检查缓存
	if item, ok := c.cache.Load(ip); ok {
		cached := item.(*cacheItem)
		if time.Now().Before(cached.expiredAt) {
			return cached.info, nil
		}
		// 缓存过期，删除
		c.cache.Delete(ip)
	}

	// 调用 API
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// ip-api.com API 端点
	// 注意：免费版不支持 HTTPS，限制 45 次/分钟
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,region,regionName,city,zip,lat,lon,timezone,isp,org,as,query&lang=zh-CN", ip)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Warn("IP 信息查询失败", logger.String("IP", ip), logger.Err(err))
		return nil, fmt.Errorf("IP 信息查询失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查速率限制
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

	// 存入缓存
	c.cache.Store(ip, &cacheItem{
		info:      &info,
		expiredAt: time.Now().Add(c.cacheTTL),
	})

	return &info, nil
}

// GetLocation 获取 IP 归属地（简化版，只返回位置字符串）
func (c *IPInfoClient) GetLocation(ip string) string {
	info, err := c.GetIPInfo(ip)
	if err != nil {
		return ""
	}
	return formatLocation(info)
}

// GetLocationAsync 异步获取 IP 归属地
func (c *IPInfoClient) GetLocationAsync(ip string, callback func(location string)) {
	go func() {
		location := c.GetLocation(ip)
		if callback != nil {
			callback(location)
		}
	}()
}

// formatLocation 格式化位置信息
func formatLocation(info *IPInfo) string {
	if info == nil {
		return ""
	}

	if info.Country == "内网" {
		return "内网"
	}

	// 中国地址格式：国家 省份 城市
	if info.CountryCode == "CN" {
		if info.RegionName != "" && info.City != "" {
			if info.RegionName == info.City {
				return info.City // 直辖市
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

	// 国外地址格式：国家 城市
	if info.City != "" {
		return info.Country + " " + info.City
	}
	return info.Country
}

// isPrivateIP 检查是否是内网 IP
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

// GetIPInfoByQuery 快捷方法：使用默认客户端查询
func GetIPInfoByQuery(ip string) (*IPInfo, error) {
	return GetClient().GetIPInfo(ip)
}

// GetLocationByIP 快捷方法：使用默认客户端获取位置
func GetLocationByIP(ip string) string {
	return GetClient().GetLocation(ip)
}

// ClearCache 清空缓存
func (c *IPInfoClient) ClearCache() {
	c.cache.Range(func(key, value interface{}) bool {
		c.cache.Delete(key)
		return true
	})
}
