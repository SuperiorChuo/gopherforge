package system

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/redis"
)

// OnlineUser 在线用户信息
type OnlineUser struct {
	UserID               uint      `json:"user_id"`
	Username             string    `json:"username"`
	Nickname             string    `json:"nickname"`
	IP                   string    `json:"ip"`
	Location             string    `json:"location"`
	Browser              string    `json:"browser"`
	OS                   string    `json:"os"`
	LoginTime            time.Time `json:"login_time"`
	TokenID              string    `json:"token_id"`
	AccessToken          string    `json:"access_token,omitempty"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at,omitempty"`
}

// OnlineUserService 在线用户服务
type OnlineUserService struct{}

const onlineUserPrefix = "online_user:"

// SetOnlineUser 设置用户在线状态
func (s *OnlineUserService) SetOnlineUser(user OnlineUser, expiration time.Duration) error {
	ctx := context.Background()
	key := onlineUserPrefix + user.TokenID

	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return redis.Client.Set(ctx, key, data, expiration).Err()
}

// RemoveOnlineUser 移除用户在线状态（强制下线）
func (s *OnlineUserService) RemoveOnlineUser(tokenID string) error {
	ctx := context.Background()
	key := onlineUserPrefix + tokenID
	return redis.Client.Del(ctx, key).Err()
}

// GetOnlineUsers 获取所有在线用户
func (s *OnlineUserService) GetOnlineUsers() ([]OnlineUser, error) {
	ctx := context.Background()

	// 使用 SCAN 遍历所有在线用户 key
	var cursor uint64
	var users []OnlineUser

	for {
		keys, nextCursor, err := redis.Client.Scan(ctx, cursor, onlineUserPrefix+"*", 100).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			data, err := redis.Client.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			var user OnlineUser
			if err := json.Unmarshal([]byte(data), &user); err != nil {
				continue
			}
			users = append(users, user)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return users, nil
}

// GetOnlineUserCount 获取在线用户数量
func (s *OnlineUserService) GetOnlineUserCount() (int64, error) {
	ctx := context.Background()

	var count int64
	var cursor uint64

	for {
		keys, nextCursor, err := redis.Client.Scan(ctx, cursor, onlineUserPrefix+"*", 100).Result()
		if err != nil {
			return 0, err
		}

		count += int64(len(keys))

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return count, nil
}

// ForceLogout 强制用户下线
func (s *OnlineUserService) ForceLogout(tokenID string) error {
	ctx := context.Background()
	key := onlineUserPrefix + tokenID

	data, err := redis.Client.Get(ctx, key).Result()
	var targetUserID uint
	if err == nil {
		var user OnlineUser
		if json.Unmarshal([]byte(data), &user) == nil {
			targetUserID = user.UserID
			s.revokeOnlineUserToken(user)
		}
	}
	if targetUserID != 0 {
		_ = s.revokeUserOnlineTokens(targetUserID)
	}
	return s.RemoveOnlineUser(tokenID)
}

func (s *OnlineUserService) revokeUserOnlineTokens(userID uint) error {
	ctx := context.Background()
	var cursor uint64

	for {
		keys, nextCursor, err := redis.Client.Scan(ctx, cursor, onlineUserPrefix+"*", 100).Result()
		if err != nil {
			return err
		}

		for _, key := range keys {
			data, err := redis.Client.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			var user OnlineUser
			if json.Unmarshal([]byte(data), &user) != nil || user.UserID != userID {
				continue
			}
			s.revokeOnlineUserToken(user)
			_ = redis.Client.Del(ctx, key).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}

func (s *OnlineUserService) revokeOnlineUserToken(user OnlineUser) {
	if user.AccessToken == "" {
		return
	}
	if claims, err := jwt.ParseToken(user.AccessToken); err == nil {
		_ = jwt.RevokeToken(user.AccessToken, claims)
	} else if !user.AccessTokenExpiresAt.IsZero() {
		if ttl := time.Until(user.AccessTokenExpiresAt); ttl > 0 {
			_ = jwt.BlacklistToken(user.AccessToken, ttl)
		}
	}
}

// IsUserOnline 检查用户是否在线
func (s *OnlineUserService) IsUserOnline(tokenID string) bool {
	ctx := context.Background()
	key := onlineUserPrefix + tokenID

	exists, err := redis.Client.Exists(ctx, key).Result()
	if err != nil {
		return false
	}

	return exists > 0
}

// ParseUserAgent 解析 User-Agent 获取浏览器和操作系统信息
func ParseUserAgent(userAgent string) (browser, os string) {
	ua := strings.ToLower(userAgent)

	// 解析浏览器
	switch {
	case strings.Contains(ua, "chrome") && !strings.Contains(ua, "edge"):
		browser = "Chrome"
	case strings.Contains(ua, "firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome"):
		browser = "Safari"
	case strings.Contains(ua, "edge"):
		browser = "Edge"
	case strings.Contains(ua, "opera") || strings.Contains(ua, "opr"):
		browser = "Opera"
	case strings.Contains(ua, "msie") || strings.Contains(ua, "trident"):
		browser = "IE"
	default:
		browser = "未知浏览器"
	}

	// 解析操作系统
	switch {
	case strings.Contains(ua, "windows"):
		os = "Windows"
	case strings.Contains(ua, "mac os") || strings.Contains(ua, "macos"):
		os = "macOS"
	case strings.Contains(ua, "linux"):
		os = "Linux"
	case strings.Contains(ua, "android"):
		os = "Android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		os = "iOS"
	default:
		os = "未知系统"
	}

	return browser, os
}
