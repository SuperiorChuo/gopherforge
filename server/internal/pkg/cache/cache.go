package cache

import (
	"context"
	"errors"
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
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *goredis.BoolCmd
	Get(ctx context.Context, key string) *goredis.StringCmd
	GetDel(ctx context.Context, key string) *goredis.StringCmd
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
	KeyOAuthState           = "oauth:state:%s"
	KeyUserInfo             = "user:info:%d"
	KeyUserPermissions      = "user:permissions:%d"
	KeyUserPermissionsIndex = "user:permissions:index"
)

// Cache expiration durations.
const (
	JWTBlacklistExpire    = 24 * time.Hour
	LoginCaptchaExpire    = 5 * time.Minute
	OAuthStateExpire      = 10 * time.Minute
	UserInfoExpire        = 1 * time.Hour
	UserPermissionsExpire = 1 * time.Hour
)

var (
	ErrOAuthStateNotFound      = errors.New("oauth state not found")
	ErrOAuthStateAlreadyExists = errors.New("oauth state already exists")
	ErrCacheUnavailable        = errors.New("redis cache unavailable")
)

func (s *CacheService) AddJWTToBlacklistContext(ctx context.Context, token string, expire time.Duration) error {
	tokenID, err := tokenIDFromJWT(token)
	if err != nil {
		return err
	}
	key := fmt.Sprintf(KeyJWTBlacklist, tokenID)
	return s.redisClient().Set(ctx, key, "1", expire).Err()
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

func (s *CacheService) RemoveJWTFromBlacklistContext(ctx context.Context, token string) error {
	tokenID, err := tokenIDFromJWT(token)
	if err != nil {
		return err
	}
	key := fmt.Sprintf(KeyJWTBlacklist, tokenID)
	return s.redisClient().Del(ctx, key).Err()
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

func (s *CacheService) SetLoginCaptchaContext(ctx context.Context, key string, captcha string) error {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return s.redisClient().Set(ctx, cacheKey, captcha, LoginCaptchaExpire).Err()
}

func (s *CacheService) GetLoginCaptchaContext(ctx context.Context, key string) (string, error) {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return s.redisClient().Get(ctx, cacheKey).Result()
}

func (s *CacheService) DelLoginCaptchaContext(ctx context.Context, key string) error {
	cacheKey := fmt.Sprintf(KeyLoginCaptcha, key)
	return s.redisClient().Del(ctx, cacheKey).Err()
}

func (s *CacheService) SetOAuthStateContext(ctx context.Context, state, verifier string, expire time.Duration) error {
	cacheKey := fmt.Sprintf(KeyOAuthState, state)
	if expire <= 0 {
		expire = OAuthStateExpire
	}
	client := s.redisClient()
	if client == nil {
		return ErrCacheUnavailable
	}
	stored, err := client.SetNX(ctx, cacheKey, verifier, expire).Result()
	if err != nil {
		return err
	}
	if !stored {
		return ErrOAuthStateAlreadyExists
	}
	return nil
}

func (s *CacheService) StoreOAuthStateContext(ctx context.Context, state, verifier string, expire time.Duration) error {
	return s.SetOAuthStateContext(ctx, state, verifier, expire)
}

func (s *CacheService) ConsumeOAuthStateContext(ctx context.Context, state string) (string, error) {
	cacheKey := fmt.Sprintf(KeyOAuthState, state)
	client := s.redisClient()
	if client == nil {
		return "", ErrCacheUnavailable
	}
	verifier, err := client.GetDel(ctx, cacheKey).Result()
	if errors.Is(err, goredis.Nil) {
		return "", ErrOAuthStateNotFound
	}
	return verifier, err
}

func (s *CacheService) SetUserInfoContext(ctx context.Context, user *model.User) error {
	key := fmt.Sprintf(KeyUserInfo, user.ID)
	return s.redisClient().Set(ctx, key, user, UserInfoExpire).Err()
}

func (s *CacheService) GetUserInfoContext(ctx context.Context, userID uint) (*model.User, error) {
	key := fmt.Sprintf(KeyUserInfo, userID)
	var user model.User
	err := s.redisClient().Get(ctx, key).Scan(&user)
	return &user, err
}

func (s *CacheService) DelUserInfoContext(ctx context.Context, userID uint) error {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return s.redisClient().Del(ctx, key).Err()
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

func (s *CacheService) GetUserPermissionsContext(ctx context.Context, userID uint) ([]string, error) {
	key := fmt.Sprintf(KeyUserPermissions, userID)
	return s.redisClient().SMembers(ctx, key).Result()
}

func (s *CacheService) DelUserPermissionsContext(ctx context.Context, userID uint) error {
	key := fmt.Sprintf(KeyUserPermissions, userID)
	pipe := s.redisClient().TxPipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, KeyUserPermissionsIndex, key)
	_, err := pipe.Exec(ctx)
	return err
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
