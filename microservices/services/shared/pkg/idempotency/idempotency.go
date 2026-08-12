// Package idempotency 提供跨服务可复用的写路径幂等键：
// 同 (tenant_id, key) 在 TTL 窗口内只执行一次业务写，重试/HTTP 回退返回首次结果。
//
// 典型用法（在业务事务内）：
//
//	err := db.Transaction(func(tx *gorm.DB) error {
//	    id, reused, err := idempotency.Run(tx, "crm_idempotency_key", tenantID, key, hash,
//	        func(tx *gorm.DB) (uint64, error) {
//	            // 真正写业务，返回业务主键
//	            return followUpID, tx.Create(...).Error
//	        })
//	    ...
//	})
package idempotency

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// MaxKeyLength 幂等键最大长度（与 BPM 既有约定对齐）。
	MaxKeyLength = 128
	// DefaultTTL 键保留窗口：覆盖客户端最长重试/回退窗口。
	DefaultTTL = 24 * time.Hour
)

var (
	// ErrKeyReuse 同键不同请求哈希——防 key 复用错配。
	ErrKeyReuse = errors.New("幂等键已用于不同的请求")
	// ErrIncomplete 键已占位但业务结果尚未写入（并发窗口/崩溃半状态）。
	ErrIncomplete = errors.New("幂等记录尚未完成")
)

// Record 通用幂等行。各服务用 Table(name) 映射到自己的表（schema-per-service）。
type Record struct {
	ID          uint64    `gorm:"primaryKey"`
	TenantID    uint64    `gorm:"column:tenant_id;not null;uniqueIndex:ux_idem_tenant_key,priority:1"`
	Key         string    `gorm:"column:key;size:128;not null;uniqueIndex:ux_idem_tenant_key,priority:2"`
	RequestHash string    `gorm:"column:request_hash;size:64;not null"`
	ResultID    uint64    `gorm:"column:result_id;not null;default:0"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// NormalizeKey 去空白。
func NormalizeKey(key string) string {
	return strings.TrimSpace(key)
}

// NewKey 生成 32 字符 hex 幂等键（128-bit 随机）。
func NewKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("idempotency: 生成键失败: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// Hash 对任意可 JSON 序列化的请求体做 sha256 hex（稳定、与字段序相关——调用方应传结构体而非 map）。
func Hash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%v", v)))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Run 在已有事务 tx 内按幂等键执行 fn。
//
//   - key 为空：直接执行 fn（兼容旧调用方）
//   - key 首次：执行 fn，把返回的 resultID 写入记录
//   - key 命中且 hash 相同：返回首次 resultID，reused=true，不执行 fn
//   - key 命中但 hash 不同：返回 ErrKeyReuse
//   - key 命中但 result_id=0：返回 ErrIncomplete（并发占位未完成）
func Run(tx *gorm.DB, table string, tenantID uint64, key, requestHash string, fn func(tx *gorm.DB) (resultID uint64, err error)) (resultID uint64, reused bool, err error) {
	if tx == nil {
		return 0, false, errors.New("idempotency: tx is nil")
	}
	if table == "" {
		return 0, false, errors.New("idempotency: table is required")
	}
	if fn == nil {
		return 0, false, errors.New("idempotency: fn is required")
	}

	key = NormalizeKey(key)
	if key == "" {
		id, err := fn(tx)
		return id, false, err
	}
	if len(key) > MaxKeyLength {
		return 0, false, fmt.Errorf("幂等键长度不能超过 %d", MaxKeyLength)
	}
	requestHash = strings.TrimSpace(requestHash)
	if requestHash == "" {
		return 0, false, errors.New("idempotency: requestHash is required when key is set")
	}

	rec := Record{
		TenantID:    tenantID,
		Key:         key,
		RequestHash: requestHash,
	}
	result := tx.Table(table).Clauses(clause.OnConflict{DoNothing: true}).Create(&rec)
	if result.Error != nil {
		return 0, false, result.Error
	}
	if result.RowsAffected == 1 {
		id, err := fn(tx)
		if err != nil {
			return 0, false, err
		}
		if err := tx.Table(table).
			Where("tenant_id = ? AND key = ?", tenantID, key).
			Update("result_id", id).Error; err != nil {
			return 0, false, err
		}
		return id, false, nil
	}

	var existing Record
	if err := tx.Table(table).
		Where("tenant_id = ? AND key = ?", tenantID, key).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&existing).Error; err != nil {
		return 0, false, err
	}
	if existing.RequestHash != requestHash {
		return 0, false, ErrKeyReuse
	}
	if existing.ResultID == 0 {
		return 0, false, ErrIncomplete
	}
	return existing.ResultID, true, nil
}

// EnsureTable 用 GORM AutoMigrate 建表（测试与 AutoMigrate 型服务用）。
func EnsureTable(db *gorm.DB, table string) error {
	if db == nil || table == "" {
		return errors.New("idempotency: db and table required")
	}
	return db.Table(table).AutoMigrate(&Record{})
}

// PurgeOlderThan 删除超过 maxAge 的幂等记录，返回删除行数。
func PurgeOlderThan(db *gorm.DB, table string, maxAge time.Duration) (int64, error) {
	if db == nil || table == "" {
		return 0, errors.New("idempotency: db and table required")
	}
	if maxAge <= 0 {
		maxAge = DefaultTTL
	}
	cutoff := time.Now().Add(-maxAge)
	res := db.Table(table).Where("created_at < ?", cutoff).Delete(&Record{})
	return res.RowsAffected, res.Error
}
