package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// 字典数据缓存。
//
// 字典是近乎静态的参考数据，每个控制台页面都会读取（一次页面加载会请求 5~15 个编码），
// 因此读取路径原本就是纯查询流量：/dicts/all 一次 1+N 查询，
// /dicts?codes=a,b,c 每个编码两次查询。
// 缓存*解析后*的载荷，可以把一次热页面加载变成零数据库查询，
// 也就是让热路径完全绕开数据库。
//
// 键设计沿用角色编码缓存（见 cache.go）：每个逻辑答案一个键，
// 外加一个充当索引的 Redis set，因此失效就是对已写入的键做一次精确的
// SMEMBERS + DEL——没有 SCAN、没有 KEYS、不会残留过期键。
//
// 租户隔离：dict_types / dict_items 目前不携带 tenant_id（见 monitor/migrations/000001），
// 所以这些行是平台全局的。
// 键仍然按租户隔离，这样以后新增 tenant_id 时，
// 不会悄悄地把一个租户的字典提供给另一个租户；
// 反面是任何一个租户的写入都必须清掉所有租户的条目，
// 这正是 DelAllDictDataContext 借助索引完成的事。
const (
	// KeyDictData 保存单个编码解析后的条目：租户 id、字典编码。
	KeyDictData = "dict:data:%d:%s"
	// KeyDictAll 保存一个租户完整的编码→条目映射。
	KeyDictAll = "dict:all:%d"
	// KeyDictIndex 是所有存活字典缓存键的集合。
	KeyDictIndex = "dict:index"
)

const (
	// DictDataDefaultExpire 为任何绕过显式失效的路径限定数据的新鲜度上限。
	// 字典是手工修改的，很少变动。
	DictDataDefaultExpire = 1 * time.Hour
	// dictDataMaxExpire 用于限制运维配置错误。
	dictDataMaxExpire = 24 * time.Hour
	// EnvDictCacheTTL 以秒为单位覆盖 TTL。设为 0 会禁用缓存，
	// 恢复直接读取数据库。
	EnvDictCacheTTL = "DICT_CACHE_TTL_SECONDS"
)

// DictEntry 是单个编码的缓存答案。Found 用于区分"该编码没有有效的条目"
// （Found=true，Items 为空）和"不存在该字典类型"（Found=false），
// 这样即使编码错误也会命中缓存，而不是每次请求都重新查询数据库。
// DictItem is the cached dictionary value payload. It mirrors the system
// model JSON shape so existing Redis entries stay compatible.
type DictItem struct {
	ID         uint   `json:"id"`
	DictTypeID uint   `json:"dict_type_id"`
	Label      string `json:"label"`
	Value      string `json:"value"`
	Sort       int    `json:"sort"`
	Status     int8   `json:"status"`
	Remark     string `json:"remark"`
}

type DictEntry struct {
	Found bool       `json:"found"`
	Items []DictItem `json:"items"`
}

// DictDataExpire 解析配置的字典缓存 TTL。
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

// DictCacheEnabled 返回字典读取是否启用缓存。
func DictCacheEnabled() bool {
	return DictDataExpire() > 0
}

func dictDataKey(tenantID uint, code string) string {
	return fmt.Sprintf(KeyDictData, tenantID, code)
}

func dictAllKey(tenantID uint) string {
	return fmt.Sprintf(KeyDictAll, tenantID)
}

// dictRedisClient 返回一个可用的 client，否则返回 nil。包级全局句柄是
// *goredis.Client 类型，因此在 Redis 初始化之前，redisClient() 返回的是
// 包装在非 nil interface 中的类型化 nil——普通的 `client == nil` 判断
// 会漏掉它，导致第一次 pipeline 调用 panic。字典读取可能运行在完全没有 Redis 的
// 路径上（测试、无 Redis 的部署），因此必须降级为直接读取数据库而不是崩溃。
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

// GetDictCodesContext 在单次往返中读取多个编码的缓存条目。
// 返回的 map 只包含命中的编码；未命中的编码直接缺失，
// 因此调用方只需要查询这些缺失的编码。
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
	// Exec 会把第一个未命中当作错误返回；但每个命令的结果仍然会被填充，
	// 所以这里逐个检查命令而不是直接退出。
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

// GetDictCodeContext 读取单个编码的缓存条目。
func (s *CacheService) GetDictCodeContext(ctx context.Context, tenantID uint, code string) (DictEntry, bool) {
	hits := s.GetDictCodesContext(ctx, tenantID, []string{code})
	entry, ok := hits[code]
	return entry, ok
}

// SetDictCodesContext 缓存给定编码解析后的条目，并把它们的键
// 记录到失效索引中。
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
	// 索引的存活时间比它的成员更长，因此绝不可能中途过期，
	// 也不会留下失效逻辑遗漏的孤立键。
	pipe.Expire(ctx, KeyDictIndex, expire+dictDataMaxExpire)
	_, err := pipe.Exec(ctx)
	return err
}

// GetAllDictDataContext 读取一个租户缓存的编码→条目映射。
func (s *CacheService) GetAllDictDataContext(ctx context.Context, tenantID uint) (map[string][]DictItem, bool) {
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
	var data map[string][]DictItem
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, false
	}
	return data, true
}

// SetAllDictDataContext 缓存一个租户的完整字典载荷。
func (s *CacheService) SetAllDictDataContext(ctx context.Context, tenantID uint, data map[string][]DictItem) error {
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

// DelAllDictDataContext 清掉所有租户的全部缓存字典载荷。
// 它在每个字典写入路径上被调用（类型和条目的新增 / 更新 / 删除）——
// 字典写入既罕见又由人工驱动，因此把整个命名空间清空既廉价，
// 也是唯一不会留下过期条目的方案：
// 一次条目编辑会改变其编码的条目、租户的 /dicts/all 数据块，
// 以及（状态翻转时）哪些编码存在。
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
