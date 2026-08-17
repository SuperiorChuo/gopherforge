package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// LoginSecurityAPI exposes admin endpoints for the login IP failure shield:
// listing currently blocked IPs and unblocking them.
type LoginSecurityAPI struct {
	redis *goredis.Client
}

func NewLoginSecurityAPI(client *goredis.Client) *LoginSecurityAPI {
	return &LoginSecurityAPI{redis: client}
}

// blockedIPScanCount is the page size used when iterating the block keys with
// SCAN. The blocked set is small in practice (a handful of abusive IPs).
const blockedIPScanCount = 100

// BlockedIPEntry is one currently-blocked IP and its remaining block time.
type BlockedIPEntry struct {
	IP         string `json:"ip"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

// ListBlockedIPs GET /login-security/blocked-ips returns every IP under the
// coarse failure shield (login_ip_fail:block:<ip>) with its remaining TTL.
func (a *LoginSecurityAPI) ListBlockedIPs(c *gin.Context) {
	client := a.redisClient()
	if client == nil {
		response.Error(c, http.StatusServiceUnavailable, "redis unavailable")
		return
	}
	ctx := c.Request.Context()
	entries := []BlockedIPEntry{}
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, "login_ip_fail:block:*", blockedIPScanCount).Result()
		if err != nil {
			response.Error(c, http.StatusServiceUnavailable, "failed to scan blocked IPs")
			return
		}
		for _, key := range keys {
			ip := strings.TrimPrefix(key, "login_ip_fail:block:")
			if ip == "" || ip == key {
				continue
			}
			ttl, err := client.TTL(ctx, key).Result()
			if err != nil || ttl < 0 {
				continue
			}
			entries = append(entries, BlockedIPEntry{IP: ip, TTLSeconds: int64(ttl.Seconds())})
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	response.Success(c, gin.H{"items": entries})
}

// UnblockIP DELETE /login-security/blocked-ips/:ip lifts the coarse shield for
// an IP and resets its failure counter so the network can log in again.
func (a *LoginSecurityAPI) UnblockIP(c *gin.Context) {
	client := a.redisClient()
	if client == nil {
		response.Error(c, http.StatusServiceUnavailable, "redis unavailable")
		return
	}
	ip := strings.TrimSpace(c.Param("ip"))
	if ip == "" {
		response.BadRequest(c, "ip required")
		return
	}
	ctx := c.Request.Context()
	blockKey := "login_ip_fail:block:" + ip
	countKey := "login_ip_fail:" + ip
	pipe := client.Pipeline()
	pipe.Del(ctx, blockKey)
	pipe.Del(ctx, countKey)
	if _, err := pipe.Exec(ctx); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "failed to unblock IP")
		return
	}
	response.Success(c, gin.H{"ip": ip})
}

func (a *LoginSecurityAPI) redisClient() *goredis.Client {
	if a != nil && a.redis != nil {
		return a.redis
	}
	return redisstore.Client
}
