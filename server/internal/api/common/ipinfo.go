package common

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/ipinfo"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

// IPInfoAPI IP 信息查询 API
type IPInfoAPI struct {
	client *ipinfo.IPInfoClient
}

// NewIPInfoAPI 创建 IPInfoAPI 实例
func NewIPInfoAPI() *IPInfoAPI {
	return &IPInfoAPI{
		client: ipinfo.GetClient(),
	}
}

// GetIPInfo 获取 IP 归属地信息
func (a *IPInfoAPI) GetIPInfo(c *gin.Context) {
	ip := c.Query("ip")
	if ip == "" {
		// 如果没有提供 IP，使用客户端 IP
		ip = c.ClientIP()
	}

	info, err := a.client.GetIPInfo(ip)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, info)
}

// GetMyIPInfo 获取当前客户端 IP 归属地信息
func (a *IPInfoAPI) GetMyIPInfo(c *gin.Context) {
	ip := c.ClientIP()

	info, err := a.client.GetIPInfo(ip)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"ip":       ip,
		"location": ipinfo.GetLocationByIP(ip),
		"detail":   info,
	})
}

// GetIPLocation 获取 IP 归属地（简化版）
func (a *IPInfoAPI) GetIPLocation(c *gin.Context) {
	ip := c.Query("ip")
	if ip == "" {
		ip = c.ClientIP()
	}

	location := a.client.GetLocation(ip)
	response.Success(c, gin.H{
		"ip":       ip,
		"location": location,
	})
}
