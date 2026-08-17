package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Console cookie-session validation cache.
//
// The cookie branch of AuthMiddleware used to spend three SELECTs and one UPDATE
// per authenticated request (session row, user row, preloaded roles, last_seen_at
// touch). This cache holds the *outcome* of that validation for a few tens of
// seconds so a burst of console requests costs nothing.
//
// Two properties keep it from widening access:
//
//   - It stores identity only (user id + username), never roles or permissions.
//     Role and permission decisions keep flowing through the role/permission
//     caches with their existing invalidation, so a cache hit here can never
//     grant a privilege the user no longer has.
//   - The key is the SHA-256 of the session id, so the raw cookie/session token
//     never lands in a Redis key name (keys show up in SCAN, MONITOR and slowlog).
const (
	KeyConsoleSession          = "console:session:%s"
	KeyConsoleSessionUserIndex = "console:session:user:%d"
	KeyConsoleSessionIndex     = "console:session:index"
)

const (
	// ConsoleSessionDefaultExpire is deliberately short: it bounds every window
	// this cache can open, including paths that have no explicit invalidation.
	ConsoleSessionDefaultExpire = 30 * time.Second
	// consoleSessionMaxExpire caps operator misconfiguration.
	consoleSessionMaxExpire = 5 * time.Minute
	// EnvConsoleSessionCacheTTL overrides the TTL in seconds. 0 disables the
	// cache entirely and restores the previous per-request validation.
	EnvConsoleSessionCacheTTL = "CONSOLE_SESSION_CACHE_TTL_SECONDS"
)

// ConsoleSessionIdentity is the validated console identity kept in Redis.
type ConsoleSessionIdentity struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	TenantID uint   `json:"tenant_id,omitempty"`
}

// ConsoleSessionExpire resolves the configured cache TTL.
func ConsoleSessionExpire() time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvConsoleSessionCacheTTL))
	if raw == "" {
		return ConsoleSessionDefaultExpire
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return ConsoleSessionDefaultExpire
	}
	expire := time.Duration(seconds) * time.Second
	if expire > consoleSessionMaxExpire {
		return consoleSessionMaxExpire
	}
	return expire
}

// ConsoleSessionCacheEnabled reports whether validation results are cached.
func ConsoleSessionCacheEnabled() bool {
	return ConsoleSessionExpire() > 0
}

func consoleSessionKey(sessionID string) (string, bool) {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(trimmed))
	return fmt.Sprintf(KeyConsoleSession, hex.EncodeToString(sum[:])), true
}

// SetConsoleSessionContext caches a validated console session.
func (s *CacheService) SetConsoleSessionContext(ctx context.Context, sessionID string, identity ConsoleSessionIdentity) error {
	expire := ConsoleSessionExpire()
	if expire <= 0 || identity.UserID == 0 {
		return nil
	}
	key, ok := consoleSessionKey(sessionID)
	if !ok {
		return nil
	}
	client := s.redisClient()
	if client == nil {
		return ErrCacheUnavailable
	}
	payload, err := json.Marshal(identity)
	if err != nil {
		return err
	}

	userIndexKey := fmt.Sprintf(KeyConsoleSessionUserIndex, identity.UserID)
	pipe := client.TxPipeline()
	pipe.Set(ctx, key, payload, expire)
	// The per-user and global indexes let the existing permission/role
	// invalidation channel drop live sessions too. Their TTL is generous
	// relative to the entry TTL so an index never expires before its members.
	pipe.SAdd(ctx, userIndexKey, key)
	pipe.Expire(ctx, userIndexKey, expire+consoleSessionMaxExpire)
	pipe.SAdd(ctx, KeyConsoleSessionIndex, key)
	pipe.Expire(ctx, KeyConsoleSessionIndex, expire+consoleSessionMaxExpire)
	_, err = pipe.Exec(ctx)
	return err
}

// GetConsoleSessionContext returns the cached identity for a session id.
func (s *CacheService) GetConsoleSessionContext(ctx context.Context, sessionID string) (ConsoleSessionIdentity, bool) {
	var identity ConsoleSessionIdentity
	if ConsoleSessionExpire() <= 0 {
		return identity, false
	}
	key, ok := consoleSessionKey(sessionID)
	if !ok {
		return identity, false
	}
	client := s.redisClient()
	if client == nil {
		return identity, false
	}
	raw, err := client.Get(ctx, key).Result()
	if err != nil {
		return identity, false
	}
	if err := json.Unmarshal([]byte(raw), &identity); err != nil || identity.UserID == 0 {
		return ConsoleSessionIdentity{}, false
	}
	return identity, true
}

// DelConsoleSessionContext drops one cached session. Called on logout/revoke.
func (s *CacheService) DelConsoleSessionContext(ctx context.Context, sessionID string) error {
	key, ok := consoleSessionKey(sessionID)
	if !ok {
		return nil
	}
	client := s.redisClient()
	if client == nil {
		return nil
	}
	pipe := client.TxPipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, KeyConsoleSessionIndex, key)
	_, err := pipe.Exec(ctx)
	return err
}

// DelConsoleSessionsForUsersContext drops every cached session of the given
// users. Wired into the same invalidation points as the permission/role caches.
func (s *CacheService) DelConsoleSessionsForUsersContext(ctx context.Context, userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}
	client := s.redisClient()
	if client == nil {
		return nil
	}

	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		userIndexKey := fmt.Sprintf(KeyConsoleSessionUserIndex, userID)
		keys, err := client.SMembers(ctx, userIndexKey).Result()
		if err != nil {
			return err
		}
		pipe := client.TxPipeline()
		if len(keys) > 0 {
			pipe.Del(ctx, keys...)
			pipe.SRem(ctx, KeyConsoleSessionIndex, stringsToAny(keys)...)
		}
		pipe.Del(ctx, userIndexKey)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// DelAllConsoleSessionsContext drops every cached console session.
func (s *CacheService) DelAllConsoleSessionsContext(ctx context.Context) error {
	client := s.redisClient()
	if client == nil {
		return nil
	}
	keys, err := client.SMembers(ctx, KeyConsoleSessionIndex).Result()
	if err != nil {
		return err
	}

	pipe := client.TxPipeline()
	if len(keys) > 0 {
		pipe.Del(ctx, keys...)
	}
	pipe.Del(ctx, KeyConsoleSessionIndex)
	_, err = pipe.Exec(ctx)
	return err
}
