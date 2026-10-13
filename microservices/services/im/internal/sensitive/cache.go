package sensitive

import (
	"sync"
	"time"

	"github.com/go-admin-kit/services/im/internal/model"
)

// WordSource 加载一个租户的启用词条（由 store.Store 实现）。
type WordSource interface {
	ListEnabledSensitiveWords(tenantID uint64) ([]model.SensitiveWord, error)
}

// cacheTTL 界定跨多实例的过期时间：本实例写入即失效，其它实例最迟
// TTL 后拿到新词库（与站点/技能组等配置的最终一致口径一致）。
const cacheTTL = 30 * time.Second

type cacheEntry struct {
	matcher *Matcher
	expires time.Time
}

// Cache 是基于 WordSource 的按租户匹配器缓存。
type Cache struct {
	mu      sync.RWMutex
	src     WordSource
	entries map[uint64]*cacheEntry
}

func NewCache(src WordSource) *Cache {
	return &Cache{src: src, entries: map[uint64]*cacheEntry{}}
}

// Matcher 返回租户匹配器；当缓存副本过期时从数据源重新加载。
// 数据源错误时降级为空匹配器（fail-open：消息
// 不因词库查询故障被误拦）。
func (c *Cache) Matcher(tenantID uint64) *Matcher {
	if c == nil {
		return nil
	}
	now := time.Now()
	c.mu.RLock()
	e := c.entries[tenantID]
	c.mu.RUnlock()
	if e != nil && now.Before(e.expires) {
		return e.matcher
	}
	words, err := c.src.ListEnabledSensitiveWords(tenantID)
	if err != nil {
		if e != nil {
			return e.matcher // 过期结果优于故障降级
		}
		return nil
	}
	entries := make([]Entry, 0, len(words))
	for _, w := range words {
		entries = append(entries, Entry{Word: w.Word, Action: w.Action})
	}
	m := NewMatcher(entries)
	c.mu.Lock()
	c.entries[tenantID] = &cacheEntry{matcher: m, expires: now.Add(cacheTTL)}
	c.mu.Unlock()
	return m
}

// Invalidate 删除租户缓存，使下一条消息立即重新加载
// （控制台写入后调用）。
func (c *Cache) Invalidate(tenantID uint64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, tenantID)
	c.mu.Unlock()
}

// Apply 运行租户匹配器；对无缓存的服务（测试）做 nil 安全处理。
func (c *Cache) Apply(tenantID uint64, text string) (string, bool, []Hit) {
	m := c.Matcher(tenantID)
	if m == nil {
		return text, false, nil
	}
	return m.Apply(text)
}
