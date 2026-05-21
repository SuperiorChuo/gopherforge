package common

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/ipinfo"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

const ipInfoLookupFailedMessage = "failed to lookup IP information"

// IPInfoAPI handles IP geolocation endpoints.
type IPInfoAPI struct {
	client *ipinfo.IPInfoClient
}

// NewIPInfoAPI creates an IPInfoAPI instance.
func NewIPInfoAPI() *IPInfoAPI {
	return &IPInfoAPI{
		client: ipinfo.GetClient(),
	}
}

// GetIPInfo returns IP geolocation details.
func (a *IPInfoAPI) GetIPInfo(c *gin.Context) {
	ip := c.Query("ip")
	if ip == "" {
		// Fall back to the client IP when no query parameter is provided.
		ip = c.ClientIP()
	}

	info, err := a.client.GetIPInfo(ip)
	if err != nil {
		logger.Warn("ip info lookup failed", logger.String("ip", ip), logger.Err(err))
		response.BadRequest(c, ipInfoLookupFailedMessage)
		return
	}

	response.Success(c, info)
}

// GetMyIPInfo returns geolocation details for the current client IP.
func (a *IPInfoAPI) GetMyIPInfo(c *gin.Context) {
	ip := c.ClientIP()

	info, err := a.client.GetIPInfo(ip)
	if err != nil {
		logger.Warn("ip info lookup failed", logger.String("ip", ip), logger.Err(err))
		response.BadRequest(c, ipInfoLookupFailedMessage)
		return
	}

	response.Success(c, gin.H{
		"ip":       ip,
		"location": ipinfo.GetLocationByIP(ip),
		"detail":   info,
	})
}

// GetIPLocation returns a simplified IP location.
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
