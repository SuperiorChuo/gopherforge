package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	localmodel "github.com/go-admin-kit/services/system/internal/model"
	goredis "github.com/redis/go-redis/v9"
)

// Dictionary data cache.
//
// Dictionaries are near-static reference data that every console page reads
// (a page load asks for 5~15 codes), so the read path used to be pure query
// traffic: /dicts/all cost 1+N queries and /dicts?codes=a,b,c cost two per
// code. Caching the *resolved* payload turns a warm page load into zero
// database queries.
//
// Key design follows the role-code cache (see cache.go): one key per logical
// answer plus a Redis set acting as an index, so invalidation is an SMEMBERS +
// DEL of exactly the keys we wrote — no SCAN, no KEYS, no stale survivors.
//
// Tenancy: dict_types / dict_items carry no tenant_id today (see
// monitor/migrations/000001), so the rows are platform-global. Keys are still
// tenant-scoped so that adding tenant_id later cannot silently serve one
// tenant's dictionary to another; the flip side is that a write in any tenant
// must drop every tenant's entries, which is what DelAllDictDataContext does
// via the index.
const (
	// KeyDictData holds one code's resolved items: tenant id, dict code.
	KeyDictData = "dict:data:%d:%s"
	// KeyDictAll holds the whole code→items map for a tenant.
	KeyDictAll = "dict:all:%d"
	// KeyDictIndex is the set of every live dict cache key.
	KeyDictIndex = "dict:index"
)

const (
	// DictDataDefaultExpire bounds staleness for any path that somehow bypasses
	// explicit invalidation. Dictionaries change by hand, rarely.
	DictDataDefaultExpire = 1 * time.Hour
	// dictDataMaxExpire caps operator misconfiguration.
	dictDataMaxExpire = 24 * time.Hour
	// EnvDictCacheTTL overrides the TTL in seconds. 0 disables the cache and
	// restores direct database reads.
	EnvDictCacheTTL = "DICT_CACHE_TTL_SECONDS"
)

// DictEntry is one code's cached answer. Found distinguishes "this code has no
// active items" (Found=true, empty Items) from "no such dict type" (Found=false)
// so a bogus code is a cache hit too instead of re-querying on every request.
type DictEntry struct {
	Found bool                  `json:"found"`
	Items []localmodel.DictItem `json:"items"`
}

// DictDataExpire resolves the configured dictionary cache TTL.
func DictDataExpire() time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvDictCacheTTL))
	if raw == "" {
		return DictDataDefaultExpire
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return DictDataDefaultExpire
	}
	expire := time.Duration(seconds) * time.Second
	if expire > dictDataMaxExpire {
		return dictDataMaxExpire
	}
	return expire
}

// DictCacheEnabled reports whether dictionary reads are cached.
func DictCacheEnabled() bool {
	return DictDataExpire() > 0
}

func dictDataKey(tenantID uint, code string) string {
	return fmt.Sprintf(KeyDictData, tenantID, code)
}

func dictAllKey(tenantID uint) string {
	return fmt.Sprintf(KeyDictAll, tenantID)
}

// dictRedisClient returns a usable client or nil. The package-global handle is
// a *goredis.Client, so before Redis is initialized redisClient() hands back a
// typed nil wrapped in a non-nil interface — a plain `client == nil` check
// misses it and the first pipeline call panics. Dictionary reads run on paths
// that may have no Redis at all (tests, Redis-less deployments), so they must
// degrade to direct database reads rather than crash.
func dictRedisClient(s *CacheService) RedisClient {
	client := s.redisClient()
	if client == nil {
		return nil
	}
	if typed, ok := client.(*goredis.Client); ok && typed == nil {
		return nil
	}
	return client
}

// GetDictCodesContext reads the cached entries for codes in a single round
// trip. The returned map only contains codes that hit; misses are simply
// absent, so the caller queries just those.
func (s *CacheService) GetDictCodesContext(ctx context.Context, tenantID uint, codes []string) map[string]DictEntry {
	if len(codes) == 0 || !DictCacheEnabled() {
		return nil
	}
	client := dictRedisClient(s)
	if client == nil {
		return nil
	}

	pipe := client.TxPipeline()
	cmds := make(map[string]*goredis.StringCmd, len(codes))
	for _, code := range codes {
		if _, seen := cmds[code]; seen {
			continue
		}
		cmds[code] = pipe.Get(ctx, dictDataKey(tenantID, code))
	}
	// Exec reports the first miss as an error; per-command results are still
	// populated, so inspect each command instead of bailing out here.
	_, _ = pipe.Exec(ctx)

	hits := make(map[string]DictEntry, len(cmds))
	for code, cmd := range cmds {
		raw, err := cmd.Result()
		if err != nil {
			continue
		}
		var entry DictEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			continue
		}
		hits[code] = entry
	}
	return hits
}

// GetDictCodeContext reads one code's cached entry.
func (s *CacheService) GetDictCodeContext(ctx context.Context, tenantID uint, code string) (DictEntry, bool) {
	hits := s.GetDictCodesContext(ctx, tenantID, []string{code})
	entry, ok := hits[code]
	return entry, ok
}

// SetDictCodesContext caches resolved entries for the given codes and records
// their keys in the invalidation index.
func (s *CacheService) SetDictCodesContext(ctx context.Context, tenantID uint, entries map[string]DictEntry) error {
	expire := DictDataExpire()
	if expire <= 0 || len(entries) == 0 {
		return nil
	}
	client := dictRedisClient(s)
	if client == nil {
		return ErrCacheUnavailable
	}

	pipe := client.TxPipeline()
	keys := make([]any, 0, len(entries))
	for code, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		key := dictDataKey(tenantID, code)
		pipe.Set(ctx, key, payload, expire)
		keys = append(keys, key)
	}
	pipe.SAdd(ctx, KeyDictIndex, keys...)
	// The index outlives its members so it can never expire mid-flight and
	// strand keys that invalidation would then miss.
	pipe.Expire(ctx, KeyDictIndex, expire+dictDataMaxExpire)
	_, err := pipe.Exec(ctx)
	return err
}

// GetAllDictDataContext reads the cached code→items map for a tenant.
func (s *CacheService) GetAllDictDataContext(ctx context.Context, tenantID uint) (map[string][]localmodel.DictItem, bool) {
	if !DictCacheEnabled() {
		return nil, false
	}
	client := dictRedisClient(s)
	if client == nil {
		return nil, false
	}
	raw, err := client.Get(ctx, dictAllKey(tenantID)).Result()
	if err != nil {
		return nil, false
	}
	var data map[string][]localmodel.DictItem
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, false
	}
	return data, true
}

// SetAllDictDataContext caches the whole dictionary payload for a tenant.
func (s *CacheService) SetAllDictDataContext(ctx context.Context, tenantID uint, data map[string][]localmodel.DictItem) error {
	expire := DictDataExpire()
	if expire <= 0 {
		return nil
	}
	client := dictRedisClient(s)
	if client == nil {
		return ErrCacheUnavailable
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	key := dictAllKey(tenantID)
	pipe := client.TxPipeline()
	pipe.Set(ctx, key, payload, expire)
	pipe.SAdd(ctx, KeyDictIndex, key)
	pipe.Expire(ctx, KeyDictIndex, expire+dictDataMaxExpire)
	_, err = pipe.Exec(ctx)
	return err
}

// DelAllDictDataContext drops every cached dictionary payload across all
// tenants. Called from every dict write path (type and item create / update /
// delete) — dictionary writes are rare and hand-driven, so blowing the whole
// namespace away is both cheap and the only variant that cannot leave a stale
// entry behind: one item edit changes its code's entry, the tenant's /dicts/all
// blob, and (on a status flip) which codes exist at all.
func (s *CacheService) DelAllDictDataContext(ctx context.Context) error {
	client := dictRedisClient(s)
	if client == nil {
		return nil
	}
	keys, err := client.SMembers(ctx, KeyDictIndex).Result()
	if err != nil {
		return err
	}

	pipe := client.TxPipeline()
	if len(keys) > 0 {
		pipe.Del(ctx, keys...)
	}
	pipe.Del(ctx, KeyDictIndex)
	_, err = pipe.Exec(ctx)
	return err
}
