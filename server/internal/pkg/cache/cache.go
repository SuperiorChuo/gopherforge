package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/redis"
)

// CacheService 缓存服务
type CacheService struct{}

// NewCacheService 创建CacheService实例
func NewCacheService() *CacheService {
	return &CacheService{}
}

// 缓存键常量
const (
	KeyJWTBlacklist           = "jwt:blacklist:%s"
	KeyLoginCaptcha           = "login:captcha:%s"
	KeyUserInfo               = "user:info:%d"
	KeyUserPermissions        = "user:permissions:%d"
	KeyUserPermissionsPattern = "user:permissions:*"
)

// 缓存过期时间
const (
	JWTBlacklistExpire    = 24 * time.Hour
	LoginCaptchaExpire    = 5 * time.Minute
	UserInfoExpire        = 1 * time.Hour
	UserPermissionsExpire = 1 * time.Hour
)

// AddJWTToBlacklist 将JWT加入黑名单
func (s *CacheService) AddJWTToBlacklist(token string, expire time.Duration) error {
	ctx := context.Background()
	key := fmt.Sprintf(KeyJWTBlacklist, token)
	return redis.Client.Set(ctx, key, "1", expire).Err()
}

// IsJWTInBlacklist 检查JWT是否在黑名单中
func (s *CacheService) IsJWTInBlacklist(token string) bool {
	ctx := context.Background()
	key := fmt.Sprintf(KeyJWTBlacklist, token)
	result, err := redis.Client.Get(ctx, key).Result()
	return err == nil && result == "1"
}

// RemoveJWTFromBlacklist 删除JWT黑名单记录，主要用于清理短期测试或会话残留。
func (s *CacheService) RemoveJWTFromBlacklist(token string) error {
	ctx := context.Background()
	key := fmt.Sprintf(KeyJWTBlacklist, token)
	return redis.Client.Del(ctx, key).Err()
}

// AddTokenToBlacklistUntilExpiry 按token剩余有效期加入黑名单。
func (s *CacheService) AddTokenToBlacklistUntilExpiry(token string, expiresAt time.Time) error {
	expire := time.Until(expiresAt)
	if expire <= 0 {
		return nil
	}
	return s.AddJWTToBlacklist(token, expire)
}

// SetLoginCaptcha 设置登录验证码
func (s *CacheService) SetLoginCaptcha(key string, captcha string) error {
	ctx := context.Background()
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return redis.Client.Set(ctx, cacheKey, captcha, LoginCaptchaExpire).Err()
}

// GetLoginCaptcha 获取登录验证码
func (s *CacheService) GetLoginCaptcha(key string) (string, error) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return redis.Client.Get(ctx, cacheKey).Result()
}

// DelLoginCaptcha 删除登录验证码
func (s *CacheService) DelLoginCaptcha(key string) error {
	ctx := context.Background()
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return redis.Client.Del(ctx, cacheKey).Err()
}

// SetUserInfo 缓存用户信息
func (s *CacheService) SetUserInfo(user *model.User) error {
	ctx := context.Background()
	key := fmt.Sprintf(KeyUserInfo, user.ID)
	return redis.Client.Set(ctx, key, user, UserInfoExpire).Err()
}

// GetUserInfo 获取缓存的用户信息
func (s *CacheService) GetUserInfo(userID uint) (*model.User, error) {
	ctx := context.Background()
	key := fmt.Sprintf(KeyUserInfo, userID)
	var user model.User
	err := redis.Client.Get(ctx, key).Scan(&user)
	return &user, err
}

// DelUserInfo 删除缓存的用户信息
func (s *CacheService) DelUserInfo(userID uint) error {
	ctx := context.Background()
	key := fmt.Sprintf(KeyUserInfo, userID)
	return redis.Client.Del(ctx, key).Err()
}

// SetUserPermissions 缓存用户权限
func (s *CacheService) SetUserPermissions(userID uint, permissions []string) error {
	ctx := context.Background()
	key := fmt.Sprintf(KeyUserPermissions, userID)
	pipe := redis.Client.TxPipeline()
	pipe.Del(ctx, key)
	if len(permissions) > 0 {
		pipe.SAdd(ctx, key, permissions)
		pipe.Expire(ctx, key, UserPermissionsExpire)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// GetUserPermissions 获取缓存的用户权限
func (s *CacheService) GetUserPermissions(userID uint) ([]string, error) {
	ctx := context.Background()
	key := fmt.Sprintf(KeyUserPermissions, userID)
	return redis.Client.SMembers(ctx, key).Result()
}

// DelUserPermissions 删除缓存的用户权限
func (s *CacheService) DelUserPermissions(userID uint) error {
	ctx := context.Background()
	key := fmt.Sprintf(KeyUserPermissions, userID)
	return redis.Client.Del(ctx, key).Err()
}

// DelUserPermissionsBatch 批量删除用户权限缓存
func (s *CacheService) DelUserPermissionsBatch(userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}

	ctx := context.Background()
	keys := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		keys = append(keys, fmt.Sprintf(KeyUserPermissions, userID))
	}
	return redis.Client.Del(ctx, keys...).Err()
}

// DelAllUserPermissions 删除所有用户权限缓存
func (s *CacheService) DelAllUserPermissions() error {
	ctx := context.Background()
	var cursor uint64

	for {
		keys, nextCursor, err := redis.Client.Scan(ctx, cursor, KeyUserPermissionsPattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := redis.Client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if nextCursor == 0 {
			return nil
		}
		cursor = nextCursor
	}
}
