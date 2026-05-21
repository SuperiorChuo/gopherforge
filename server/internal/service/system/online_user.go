package system

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/pkg/jwt"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

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
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at,omitempty"`
}

// OnlineUserRedisClient is the Redis command subset used by OnlineUserService.
type OnlineUserRedisClient interface {
	Get(ctx context.Context, key string) *goredis.StringCmd
	ZRange(ctx context.Context, key string, start, stop int64) *goredis.StringSliceCmd
	MGet(ctx context.Context, keys ...string) *goredis.SliceCmd
	Exists(ctx context.Context, keys ...string) *goredis.IntCmd
	ZRem(ctx context.Context, key string, members ...any) *goredis.IntCmd
	ZCard(ctx context.Context, key string) *goredis.IntCmd
	ZRemRangeByScore(ctx context.Context, key, min, max string) *goredis.IntCmd
	TxPipeline() goredis.Pipeliner
}

type OnlineUserService struct {
	client OnlineUserRedisClient
}

// NewOnlineUserService creates a service backed by the package Redis client.
func NewOnlineUserService() *OnlineUserService {
	return &OnlineUserService{}
}

// NewOnlineUserServiceWithClient creates a service backed by the provided Redis client.
func NewOnlineUserServiceWithClient(client OnlineUserRedisClient) *OnlineUserService {
	return &OnlineUserService{client: client}
}

const (
	onlineUserPrefix          = "online_user:"
	onlineUserIndexKey        = "online_users"
	onlineUserUserIndexPrefix = "online_users:user:"
)

// Deprecated: use SetOnlineUserContext instead.
func (s *OnlineUserService) SetOnlineUser(user OnlineUser, expiration time.Duration) error {
	return s.SetOnlineUserContext(context.Background(), user, expiration)
}

func (s *OnlineUserService) SetOnlineUserContext(ctx context.Context, user OnlineUser, expiration time.Duration) error {
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	score := onlineUserExpiryScore(expiration, user.AccessTokenExpiresAt)
	pipe := s.redisClient().TxPipeline()
	pipe.Set(ctx, onlineUserKey(user.TokenID), data, expiration)
	pipe.ZAdd(ctx, onlineUserIndexKey, goredis.Z{
		Score:  score,
		Member: user.TokenID,
	})
	pipe.ZAdd(ctx, onlineUserUserIndexKey(user.UserID), goredis.Z{
		Score:  score,
		Member: user.TokenID,
	})
	_, err = pipe.Exec(ctx)
	return err
}

// Deprecated: use RemoveOnlineUserContext instead.
func (s *OnlineUserService) RemoveOnlineUser(tokenID string) error {
	return s.RemoveOnlineUserContext(context.Background(), tokenID)
}

func (s *OnlineUserService) RemoveOnlineUserContext(ctx context.Context, tokenID string) error {
	var userIndexKey string
	client := s.redisClient()
	if data, err := client.Get(ctx, onlineUserKey(tokenID)).Result(); err == nil {
		var user OnlineUser
		if json.Unmarshal([]byte(data), &user) == nil {
			userIndexKey = onlineUserUserIndexKey(user.UserID)
		}
	} else if err != goredis.Nil {
		return err
	}

	pipe := client.TxPipeline()
	pipe.Del(ctx, onlineUserKey(tokenID))
	pipe.ZRem(ctx, onlineUserIndexKey, tokenID)
	if userIndexKey != "" {
		pipe.ZRem(ctx, userIndexKey, tokenID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Deprecated: use GetOnlineUsersContext instead.
func (s *OnlineUserService) GetOnlineUsers() ([]OnlineUser, error) {
	return s.GetOnlineUsersContext(context.Background())
}

func (s *OnlineUserService) GetOnlineUsersContext(ctx context.Context) ([]OnlineUser, error) {
	return s.getIndexedOnlineUsers(ctx)
}

// Deprecated: use GetOnlineUserCountContext instead.
func (s *OnlineUserService) GetOnlineUserCount() (int64, error) {
	return s.GetOnlineUserCountContext(context.Background())
}

func (s *OnlineUserService) GetOnlineUserCountContext(ctx context.Context) (int64, error) {
	if err := s.pruneExpiredOnlineUsers(ctx); err != nil {
		return 0, err
	}
	return s.countIndexedOnlineUsersContext(ctx)
}

// Deprecated: use ForceLogoutContext instead.
func (s *OnlineUserService) ForceLogout(tokenID string) error {
	return s.ForceLogoutContext(context.Background(), tokenID)
}

func (s *OnlineUserService) ForceLogoutContext(ctx context.Context, tokenID string) error {
	data, err := s.redisClient().Get(ctx, onlineUserKey(tokenID)).Result()
	var targetUserID uint
	if err == nil {
		var user OnlineUser
		if json.Unmarshal([]byte(data), &user) == nil {
			targetUserID = user.UserID
			s.revokeOnlineUserToken(user)
		}
	}
	if targetUserID != 0 {
		_ = s.revokeUserOnlineTokensContext(ctx, targetUserID)
	}
	return s.RemoveOnlineUserContext(ctx, tokenID)
}

func (s *OnlineUserService) revokeUserOnlineTokensContext(ctx context.Context, userID uint) error {
	userIndexKey := onlineUserUserIndexKey(userID)
	if err := s.pruneExpiredUserOnlineUsers(ctx, userID); err != nil {
		return err
	}

	client := s.redisClient()
	tokenIDs, err := client.ZRange(ctx, userIndexKey, 0, -1).Result()
	if err != nil {
		return err
	}
	if len(tokenIDs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		keys = append(keys, onlineUserKey(tokenID))
	}
	values, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		return err
	}

	pipe := client.TxPipeline()
	for i, value := range values {
		tokenID := tokenIDs[i]
		if value == nil {
			pipe.ZRem(ctx, onlineUserIndexKey, tokenID)
			pipe.ZRem(ctx, userIndexKey, tokenID)
			continue
		}

		data, ok := value.(string)
		if !ok {
			data = fmt.Sprint(value)
		}

		var user OnlineUser
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			pipe.ZRem(ctx, onlineUserIndexKey, tokenID)
			pipe.ZRem(ctx, userIndexKey, tokenID)
			continue
		}
		if user.TokenID == "" {
			user.TokenID = tokenID
		}
		if user.UserID != userID {
			pipe.ZRem(ctx, userIndexKey, tokenID)
			continue
		}

		s.revokeOnlineUserToken(user)
		pipe.Del(ctx, onlineUserKey(tokenID))
		pipe.ZRem(ctx, onlineUserIndexKey, tokenID)
		pipe.ZRem(ctx, userIndexKey, tokenID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *OnlineUserService) revokeOnlineUserToken(user OnlineUser) {
	if user.TokenID != "" && !user.AccessTokenExpiresAt.IsZero() {
		if ttl := time.Until(user.AccessTokenExpiresAt); ttl > 0 {
			_ = jwt.BlacklistTokenID(user.TokenID, ttl)
		}
	}
}

// Deprecated: use IsUserOnlineContext instead.
func (s *OnlineUserService) IsUserOnline(tokenID string) bool {
	return s.IsUserOnlineContext(context.Background(), tokenID)
}

func (s *OnlineUserService) IsUserOnlineContext(ctx context.Context, tokenID string) bool {
	client := s.redisClient()
	exists, err := client.Exists(ctx, onlineUserKey(tokenID)).Result()
	if err != nil {
		return false
	}
	if exists > 0 {
		return true
	}
	_ = client.ZRem(ctx, onlineUserIndexKey, tokenID).Err()
	return false
}

func (s *OnlineUserService) getIndexedOnlineUsers(ctx context.Context) ([]OnlineUser, error) {
	if err := s.pruneExpiredOnlineUsers(ctx); err != nil {
		return nil, err
	}

	client := s.redisClient()
	tokenIDs, err := client.ZRange(ctx, onlineUserIndexKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		keys = append(keys, onlineUserKey(tokenID))
	}
	values, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	users := make([]OnlineUser, 0, len(values))
	staleTokenIDs := make([]any, 0)
	for i, value := range values {
		if value == nil {
			staleTokenIDs = append(staleTokenIDs, tokenIDs[i])
			continue
		}

		data, ok := value.(string)
		if !ok {
			data = fmt.Sprint(value)
		}

		var user OnlineUser
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			staleTokenIDs = append(staleTokenIDs, tokenIDs[i])
			continue
		}
		if user.TokenID == "" {
			user.TokenID = tokenIDs[i]
		}
		users = append(users, user)
	}
	if len(staleTokenIDs) > 0 {
		_ = client.ZRem(ctx, onlineUserIndexKey, staleTokenIDs...).Err()
	}
	return users, nil
}

func (s *OnlineUserService) countIndexedOnlineUsersContext(ctx context.Context) (int64, error) {
	return s.redisClient().ZCard(ctx, onlineUserIndexKey).Result()
}

func onlineUserKey(tokenID string) string {
	return onlineUserPrefix + tokenID
}

func onlineUserUserIndexKey(userID uint) string {
	return onlineUserUserIndexPrefix + strconv.FormatUint(uint64(userID), 10)
}

func onlineUserExpiryScore(expiration time.Duration, expiresAt time.Time) float64 {
	if !expiresAt.IsZero() {
		return float64(expiresAt.Unix())
	}
	if expiration <= 0 {
		return float64(time.Now().Unix())
	}
	return float64(time.Now().Add(expiration).Unix())
}

func (s *OnlineUserService) pruneExpiredOnlineUsers(ctx context.Context) error {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	return s.redisClient().ZRemRangeByScore(ctx, onlineUserIndexKey, "-inf", now).Err()
}

func (s *OnlineUserService) pruneExpiredUserOnlineUsers(ctx context.Context, userID uint) error {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	return s.redisClient().ZRemRangeByScore(ctx, onlineUserUserIndexKey(userID), "-inf", now).Err()
}

func (s *OnlineUserService) redisClient() OnlineUserRedisClient {
	if s != nil && s.client != nil {
		return s.client
	}
	return redisstore.Client
}

func ParseUserAgent(userAgent string) (browser, os string) {
	ua := strings.ToLower(userAgent)

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
		browser = "Unknown Browser"
	}

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
		os = "Unknown OS"
	}

	return browser, os
}
