package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/jwt"
	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	goredis "github.com/redis/go-redis/v9"
)

type OnlineUser struct {
	// TenantID 界定会话所属租户。本字段出现之前写入的会话会解码为 0，
	// 依据本代码库"0 表示默认"的约定，归入默认租户而不是直接隐藏。在
	// 多租户部署上，这会短暂地把升级前的会话展示给默认租户的运维人员；
	// 随着那些令牌过期，这个窗口会关闭。
	TenantID             uint      `json:"tenant_id"`
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

// OnlineUserRedisClient 是 OnlineUserService 使用的 Redis 命令子集。
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

// NewOnlineUserService 创建基于包级 Redis 客户端的服务。
func NewOnlineUserService() *OnlineUserService {
	return &OnlineUserService{}
}

// NewOnlineUserServiceWithClient 创建基于指定 Redis 客户端的服务。
func NewOnlineUserServiceWithClient(client OnlineUserRedisClient) *OnlineUserService {
	return &OnlineUserService{client: client}
}

const (
	onlineUserPrefix          = "online_user:"
	onlineUserIndexKey        = "online_users"
	onlineUserUserIndexPrefix = "online_users:user:"
	// 按租户的索引。控制台从此索引做列表与计数，从而租户运维人员
	// 永远不会看到其他租户的会话，计数也只需一次 ZCARD 而无需解码每个负载。
	onlineUserTenantIndexPrefix = "online_users:tenant:"
)

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
	// 索引键本身没有 TTL，成员只在某些可能对某用户永不执行的路径上按分数清理——
	// 单纯过期的会话会永久留下它的成员。这里 Redis 运行 volatile-lru 策略，
	// 而该策略从不淘汰没有 TTL 的键，因此泄漏是无界的。
	//
	// 使用 NX 再 GT，而非仅 GT：评估 GT 时 Redis 将没有 TTL 的键视为无限 TTL，
	// 因此 GT 无法安置首次过期。NX 负责设置，之后的 GT 只会延长，
	// 这样索引不会因为成员更长寿而被过早截断。
	pipe.ExpireNX(ctx, onlineUserUserIndexKey(user.UserID), expiration)
	pipe.ExpireGT(ctx, onlineUserUserIndexKey(user.UserID), expiration)
	pipe.ZAdd(ctx, onlineUserTenantIndexKey(tenant.Normalize(user.TenantID)), goredis.Z{
		Score:  score,
		Member: user.TokenID,
	})
	pipe.ExpireNX(ctx, onlineUserTenantIndexKey(tenant.Normalize(user.TenantID)), expiration)
	pipe.ExpireGT(ctx, onlineUserTenantIndexKey(tenant.Normalize(user.TenantID)), expiration)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *OnlineUserService) RemoveOnlineUserContext(ctx context.Context, tokenID string) error {
	var userIndexKey, tenantIndexKey string
	client := s.redisClient()
	if data, err := client.Get(ctx, onlineUserKey(tokenID)).Result(); err == nil {
		var user OnlineUser
		if json.Unmarshal([]byte(data), &user) == nil {
			userIndexKey = onlineUserUserIndexKey(user.UserID)
			tenantIndexKey = onlineUserTenantIndexKey(tenant.Normalize(user.TenantID))
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
	if tenantIndexKey != "" {
		pipe.ZRem(ctx, tenantIndexKey, tokenID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ErrOnlineUserForbidden 在会话属于其他租户时返回。
var ErrOnlineUserForbidden = errors.New("online user: session belongs to another tenant")

// GetOnlineUsersContext 只列出调用方租户的会话。全局索引是平台级的，
// 直接读取它会把所有租户的用户名、IP 与令牌 ID 展示给任何持有列表权限的
// 运维人员。
//
// 在租户索引出现之前创建的会话不在索引中，会一直隐藏到其令牌过期；
// 它们并未丢失，只是不展示。
func (s *OnlineUserService) GetOnlineUsersContext(ctx context.Context) ([]OnlineUser, error) {
	return s.getIndexedOnlineUsers(ctx, onlineUserTenantIndexKey(tenant.FromContextOrDefault(ctx)))
}

func (s *OnlineUserService) GetOnlineUserCountContext(ctx context.Context) (int64, error) {
	tenantIndexKey := onlineUserTenantIndexKey(tenant.FromContextOrDefault(ctx))
	if err := s.pruneExpiredIndex(ctx, tenantIndexKey); err != nil {
		return 0, err
	}
	return s.redisClient().ZCard(ctx, tenantIndexKey).Result()
}

// ForceLogoutContext 强制下线一个会话，若它属于其他租户则拒绝。
func (s *OnlineUserService) ForceLogoutContext(ctx context.Context, tokenID string) error {
	data, err := s.redisClient().Get(ctx, onlineUserKey(tokenID)).Result()
	var targetUserID uint
	if err == nil {
		var user OnlineUser
		if json.Unmarshal([]byte(data), &user) == nil {
			if tenant.Normalize(user.TenantID) != tenant.FromContextOrDefault(ctx) {
				return ErrOnlineUserForbidden
			}
			targetUserID = user.UserID
			s.revokeOnlineUserToken(ctx, user)
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

		s.revokeOnlineUserToken(ctx, user)
		pipe.Del(ctx, onlineUserKey(tokenID))
		pipe.ZRem(ctx, onlineUserIndexKey, tokenID)
		pipe.ZRem(ctx, userIndexKey, tokenID)
		pipe.ZRem(ctx, onlineUserTenantIndexKey(tenant.Normalize(user.TenantID)), tokenID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *OnlineUserService) revokeOnlineUserToken(ctx context.Context, user OnlineUser) {
	if user.TokenID != "" && !user.AccessTokenExpiresAt.IsZero() {
		if ttl := time.Until(user.AccessTokenExpiresAt); ttl > 0 {
			_ = jwt.BlacklistTokenIDContext(ctx, user.TokenID, ttl)
		}
	}
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

func (s *OnlineUserService) getIndexedOnlineUsers(ctx context.Context, indexKey string) ([]OnlineUser, error) {
	if err := s.pruneExpiredIndex(ctx, indexKey); err != nil {
		return nil, err
	}

	client := s.redisClient()
	tokenIDs, err := client.ZRange(ctx, indexKey, 0, -1).Result()
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
		_ = client.ZRem(ctx, indexKey, staleTokenIDs...).Err()
	}
	return users, nil
}

func onlineUserKey(tokenID string) string {
	return onlineUserPrefix + tokenID
}

func onlineUserTenantIndexKey(tenantID uint) string {
	return onlineUserTenantIndexPrefix + strconv.FormatUint(uint64(tenantID), 10)
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
	return s.pruneExpiredIndex(ctx, onlineUserIndexKey)
}

func (s *OnlineUserService) pruneExpiredIndex(ctx context.Context, indexKey string) error {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	return s.redisClient().ZRemRangeByScore(ctx, indexKey, "-inf", now).Err()
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
