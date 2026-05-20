package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/go-admin-kit/server/internal/pkg/redis"
)

// CacheService provides Redis-backed cache operations.
type CacheService struct{}

// NewCacheService creates a CacheService instance.
func NewCacheService() *CacheService {
	return &CacheService{}
}

// Cache key templates.
const (
	KeyJWTBlacklist           = "jwt:blacklist:%s"
	KeyLoginCaptcha           = "login:captcha:%s"
	KeyUserInfo               = "user:info:%d"
	KeyUserPermissions        = "user:permissions:%d"
	KeyUserPermissionsPattern = "user:permissions:*"
)

// Cache expiration durations.
const (
	JWTBlacklistExpire    = 24 * time.Hour
	LoginCaptchaExpire    = 5 * time.Minute
	UserInfoExpire        = 1 * time.Hour
	UserPermissionsExpire = 1 * time.Hour
)

// AddJWTToBlacklist adds a JWT to the blacklist.
func (s *CacheService) AddJWTToBlacklist(token string, expire time.Duration) error {
	return s.AddJWTToBlacklistContext(context.Background(), token, expire)
}

func (s *CacheService) AddJWTToBlacklistContext(ctx context.Context, token string, expire time.Duration) error {
	tokenID, err := tokenIDFromJWT(token)
	if err != nil {
		return err
	}
	key := fmt.Sprintf(KeyJWTBlacklist, tokenID)
	return redis.Client.Set(ctx, key, "1", expire).Err()
}

// IsJWTInBlacklist reports whether a JWT is blacklisted.
func (s *CacheService) IsJWTInBlacklist(token string) bool {
	return s.IsJWTInBlacklistContext(context.Background(), token)
}

func (s *CacheService) IsJWTInBlacklistContext(ctx context.Context, token string) bool {
	tokenID, err := tokenIDFromJWT(token)
	if err != nil {
		return false
	}
	key := fmt.Sprintf(KeyJWTBlacklist, tokenID)
	result, err := redis.Client.Get(ctx, key).Result()
	return err == nil && result == "1"
}

// RemoveJWTFromBlacklist removes a JWT blacklist entry for short-lived tests or session cleanup.
func (s *CacheService) RemoveJWTFromBlacklist(token string) error {
	return s.RemoveJWTFromBlacklistContext(context.Background(), token)
}

func (s *CacheService) RemoveJWTFromBlacklistContext(ctx context.Context, token string) error {
	tokenID, err := tokenIDFromJWT(token)
	if err != nil {
		return err
	}
	key := fmt.Sprintf(KeyJWTBlacklist, tokenID)
	return redis.Client.Del(ctx, key).Err()
}

// AddTokenToBlacklistUntilExpiry blacklists a token until its expiry time.
func (s *CacheService) AddTokenToBlacklistUntilExpiry(token string, expiresAt time.Time) error {
	return s.AddTokenToBlacklistUntilExpiryContext(context.Background(), token, expiresAt)
}

func (s *CacheService) AddTokenToBlacklistUntilExpiryContext(ctx context.Context, token string, expiresAt time.Time) error {
	expire := time.Until(expiresAt)
	if expire <= 0 {
		return nil
	}
	return s.AddJWTToBlacklistContext(ctx, token, expire)
}

func tokenIDFromJWT(token string) (string, error) {
	tokenID, err := jwtpkg.TokenID(token)
	if err != nil {
		return "", jwtpkg.ErrInvalidToken
	}
	return tokenID, nil
}

// SetLoginCaptcha stores a login captcha.
func (s *CacheService) SetLoginCaptcha(key string, captcha string) error {
	return s.SetLoginCaptchaContext(context.Background(), key, captcha)
}

func (s *CacheService) SetLoginCaptchaContext(ctx context.Context, key string, captcha string) error {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return redis.Client.Set(ctx, cacheKey, captcha, LoginCaptchaExpire).Err()
}

// GetLoginCaptcha returns a login captcha.
func (s *CacheService) GetLoginCaptcha(key string) (string, error) {
	return s.GetLoginCaptchaContext(context.Background(), key)
}

func (s *CacheService) GetLoginCaptchaContext(ctx context.Context, key string) (string, error) {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return redis.Client.Get(ctx, cacheKey).Result()
}

// DelLoginCaptcha deletes a login captcha.
func (s *CacheService) DelLoginCaptcha(key string) error {
	return s.DelLoginCaptchaContext(context.Background(), key)
}

func (s *CacheService) DelLoginCaptchaContext(ctx context.Context, key string) error {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return redis.Client.Del(ctx, cacheKey).Err()
}

// SetUserInfo caches user information.
func (s *CacheService) SetUserInfo(user *model.User) error {
	return s.SetUserInfoContext(context.Background(), user)
}

func (s *CacheService) SetUserInfoContext(ctx context.Context, user *model.User) error {
	key := fmt.Sprintf(KeyUserInfo, user.ID)
	return redis.Client.Set(ctx, key, user, UserInfoExpire).Err()
}

// GetUserInfo returns cached user information.
func (s *CacheService) GetUserInfo(userID uint) (*model.User, error) {
	return s.GetUserInfoContext(context.Background(), userID)
}

func (s *CacheService) GetUserInfoContext(ctx context.Context, userID uint) (*model.User, error) {
	key := fmt.Sprintf(KeyUserInfo, userID)
	var user model.User
	err := redis.Client.Get(ctx, key).Scan(&user)
	return &user, err
}

// DelUserInfo deletes cached user information.
func (s *CacheService) DelUserInfo(userID uint) error {
	return s.DelUserInfoContext(context.Background(), userID)
}

func (s *CacheService) DelUserInfoContext(ctx context.Context, userID uint) error {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return redis.Client.Del(ctx, key).Err()
}

// SetUserPermissions caches user permissions.
func (s *CacheService) SetUserPermissions(userID uint, permissions []string) error {
	return s.SetUserPermissionsContext(context.Background(), userID, permissions)
}

func (s *CacheService) SetUserPermissionsContext(ctx context.Context, userID uint, permissions []string) error {
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

// GetUserPermissions returns cached user permissions.
func (s *CacheService) GetUserPermissions(userID uint) ([]string, error) {
	return s.GetUserPermissionsContext(context.Background(), userID)
}

func (s *CacheService) GetUserPermissionsContext(ctx context.Context, userID uint) ([]string, error) {
	key := fmt.Sprintf(KeyUserPermissions, userID)
	return redis.Client.SMembers(ctx, key).Result()
}

// DelUserPermissions deletes cached user permissions.
func (s *CacheService) DelUserPermissions(userID uint) error {
	return s.DelUserPermissionsContext(context.Background(), userID)
}

func (s *CacheService) DelUserPermissionsContext(ctx context.Context, userID uint) error {
	key := fmt.Sprintf(KeyUserPermissions, userID)
	return redis.Client.Del(ctx, key).Err()
}

// DelUserPermissionsBatch deletes user permission caches in bulk.
func (s *CacheService) DelUserPermissionsBatch(userIDs []uint) error {
	return s.DelUserPermissionsBatchContext(context.Background(), userIDs)
}

func (s *CacheService) DelUserPermissionsBatchContext(ctx context.Context, userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		keys = append(keys, fmt.Sprintf(KeyUserPermissions, userID))
	}
	return redis.Client.Del(ctx, keys...).Err()
}

// DelAllUserPermissions deletes all cached user permissions.
func (s *CacheService) DelAllUserPermissions() error {
	return s.DelAllUserPermissionsContext(context.Background())
}

func (s *CacheService) DelAllUserPermissionsContext(ctx context.Context) error {
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
