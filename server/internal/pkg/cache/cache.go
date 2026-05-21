package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/go-admin-kit/server/internal/model"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

// RedisClient is the Redis command subset used by CacheService.
type RedisClient interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) *goredis.StatusCmd
	Get(ctx context.Context, key string) *goredis.StringCmd
	Del(ctx context.Context, keys ...string) *goredis.IntCmd
	SMembers(ctx context.Context, key string) *goredis.StringSliceCmd
	TxPipeline() goredis.Pipeliner
}

// CacheService provides Redis-backed cache operations.
type CacheService struct {
	client RedisClient
}

// NewCacheService creates a CacheService instance.
func NewCacheService() *CacheService {
	return &CacheService{}
}

// NewCacheServiceWithClient creates a CacheService backed by the provided Redis client.
func NewCacheServiceWithClient(client RedisClient) *CacheService {
	return &CacheService{client: client}
}

// Cache key templates.
const (
	KeyJWTBlacklist         = "jwt:blacklist:%s"
	KeyLoginCaptcha         = "login:captcha:%s"
	KeyUserInfo             = "user:info:%d"
	KeyUserPermissions      = "user:permissions:%d"
	KeyUserPermissionsIndex = "user:permissions:index"
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
	return s.redisClient().Set(ctx, key, "1", expire).Err()
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
	result, err := s.redisClient().Get(ctx, key).Result()
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
	return s.redisClient().Del(ctx, key).Err()
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
	return s.redisClient().Set(ctx, cacheKey, captcha, LoginCaptchaExpire).Err()
}

// GetLoginCaptcha returns a login captcha.
func (s *CacheService) GetLoginCaptcha(key string) (string, error) {
	return s.GetLoginCaptchaContext(context.Background(), key)
}

func (s *CacheService) GetLoginCaptchaContext(ctx context.Context, key string) (string, error) {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return s.redisClient().Get(ctx, cacheKey).Result()
}

// DelLoginCaptcha deletes a login captcha.
func (s *CacheService) DelLoginCaptcha(key string) error {
	return s.DelLoginCaptchaContext(context.Background(), key)
}

func (s *CacheService) DelLoginCaptchaContext(ctx context.Context, key string) error {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return s.redisClient().Del(ctx, cacheKey).Err()
}

// SetUserInfo caches user information.
func (s *CacheService) SetUserInfo(user *model.User) error {
	return s.SetUserInfoContext(context.Background(), user)
}

func (s *CacheService) SetUserInfoContext(ctx context.Context, user *model.User) error {
	key := fmt.Sprintf(KeyUserInfo, user.ID)
	return s.redisClient().Set(ctx, key, user, UserInfoExpire).Err()
}

// GetUserInfo returns cached user information.
func (s *CacheService) GetUserInfo(userID uint) (*model.User, error) {
	return s.GetUserInfoContext(context.Background(), userID)
}

func (s *CacheService) GetUserInfoContext(ctx context.Context, userID uint) (*model.User, error) {
	key := fmt.Sprintf(KeyUserInfo, userID)
	var user model.User
	err := s.redisClient().Get(ctx, key).Scan(&user)
	return &user, err
}

// DelUserInfo deletes cached user information.
func (s *CacheService) DelUserInfo(userID uint) error {
	return s.DelUserInfoContext(context.Background(), userID)
}

func (s *CacheService) DelUserInfoContext(ctx context.Context, userID uint) error {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return s.redisClient().Del(ctx, key).Err()
}

// SetUserPermissions caches user permissions.
func (s *CacheService) SetUserPermissions(userID uint, permissions []string) error {
	return s.SetUserPermissionsContext(context.Background(), userID, permissions)
}

func (s *CacheService) SetUserPermissionsContext(ctx context.Context, userID uint, permissions []string) error {
	key := fmt.Sprintf(KeyUserPermissions, userID)
	pipe := s.redisClient().TxPipeline()
	pipe.Del(ctx, key)
	if len(permissions) > 0 {
		pipe.SAdd(ctx, key, stringsToAny(permissions)...)
		pipe.Expire(ctx, key, UserPermissionsExpire)
		pipe.SAdd(ctx, KeyUserPermissionsIndex, key)
	} else {
		pipe.SRem(ctx, KeyUserPermissionsIndex, key)
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
	return s.redisClient().SMembers(ctx, key).Result()
}

// DelUserPermissions deletes cached user permissions.
func (s *CacheService) DelUserPermissions(userID uint) error {
	return s.DelUserPermissionsContext(context.Background(), userID)
}

func (s *CacheService) DelUserPermissionsContext(ctx context.Context, userID uint) error {
	key := fmt.Sprintf(KeyUserPermissions, userID)
	pipe := s.redisClient().TxPipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, KeyUserPermissionsIndex, key)
	_, err := pipe.Exec(ctx)
	return err
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
	pipe := s.redisClient().TxPipeline()
	pipe.Del(ctx, keys...)
	pipe.SRem(ctx, KeyUserPermissionsIndex, stringsToAny(keys)...)
	_, err := pipe.Exec(ctx)
	return err
}

// DelAllUserPermissions deletes all cached user permissions.
func (s *CacheService) DelAllUserPermissions() error {
	return s.DelAllUserPermissionsContext(context.Background())
}

func (s *CacheService) DelAllUserPermissionsContext(ctx context.Context) error {
	keys, err := s.redisClient().SMembers(ctx, KeyUserPermissionsIndex).Result()
	if err != nil {
		return err
	}

	pipe := s.redisClient().TxPipeline()
	if len(keys) > 0 {
		pipe.Del(ctx, keys...)
	}
	pipe.Del(ctx, KeyUserPermissionsIndex)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *CacheService) redisClient() RedisClient {
	if s != nil && s.client != nil {
		return s.client
	}
	return redisstore.Client
}

func stringsToAny(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}
