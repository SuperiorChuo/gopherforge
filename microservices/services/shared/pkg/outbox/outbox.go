// Package outbox 事务性 Outbox：业务写与事件落库同一事务，worker 异步投递 NATS。
//
// 状态机：pending → sending → sent
//                    ↘ pending（退避重试）→ failed（超过 MaxAttempts）
//
// 用法：
//
//	// 业务事务内
//	outbox.Insert(tx, tenantID, "audit.log.create", payload)
//
//	// 投递进程（如 audit-service）
//	stop := outbox.StartWorker(ctx, db, publisher, outbox.Options{})
//	defer stop()
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusPending = "pending"
	StatusSending = "sending"
	StatusSent    = "sent"
	StatusFailed  = "failed"

	// TableName 逻辑表名；PG 上写 public.outbox_events 避免 search_path 落到业务 schema。
	TableName = "outbox_events"

	DefaultMaxAttempts  = 8
	DefaultPollInterval = 2 * time.Second
	DefaultBatchSize    = 32
	DefaultClaimLease   = 30 * time.Second
)

func tableRef(db *gorm.DB) string {
	if db != nil && isPostgres(db) {
		return "public." + TableName
	}
	return TableName
}

// Event 对应 outbox_events 行。
type Event struct {
	ID        uint64    `gorm:"primaryKey"`
	TenantID  uint64    `gorm:"column:tenant_id;not null;default:1"`
	Subject   string    `gorm:"column:subject;size:200;not null"`
	Payload   []byte    `gorm:"column:payload;type:bytea;not null"`
	Status    string    `gorm:"column:status;size:16;not null;default:pending"`
	Attempts  int       `gorm:"column:attempts;not null;default:0"`
	LastError string    `gorm:"column:last_error;size:500"`
	NextAt    time.Time `gorm:"column:next_at"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Event) TableName() string { return TableName }

// Publisher 投递抽象（NATS JetStream / 测试 mock）。
type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

// PublisherFunc 函数适配。
type PublisherFunc func(ctx context.Context, subject string, payload []byte) error

func (f PublisherFunc) Publish(ctx context.Context, subject string, payload []byte) error {
	return f(ctx, subject, payload)
}

var (
	txMode atomic.Bool // 开启后 audittrail 等走 Outbox 而非直发 NATS
)

// EnableTransactional 打开事务 Outbox 模式（通常由跑 worker 的进程在 NATS 就绪后调用）。
func EnableTransactional() { txMode.Store(true) }

// DisableTransactional 关闭（测试用）。
func DisableTransactional() { txMode.Store(false) }

// TransactionalEnabled 是否走 Outbox 写入。
func TransactionalEnabled() bool { return txMode.Load() }

// Insert 在已有事务内写入一条待投递事件。subject/payload 必填。
func Insert(tx *gorm.DB, tenantID uint64, subject string, payload []byte) error {
	if tx == nil {
		return errors.New("outbox: tx is nil")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return errors.New("outbox: subject is required")
	}
	if len(payload) == 0 {
		return errors.New("outbox: payload is required")
	}
	if tenantID == 0 {
		tenantID = 1
	}
	now := time.Now()
	ev := Event{
		TenantID:  tenantID,
		Subject:   subject,
		Payload:   payload,
		Status:    StatusPending,
		Attempts:  0,
		NextAt:    now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return tx.Table(tableRef(tx)).Create(&ev).Error
}

// EnsureTable AutoMigrate（测试 / 无 goose 场景）；生产以 000072 迁移为准。
func EnsureTable(db *gorm.DB) error {
	if db == nil {
		return errors.New("outbox: db is nil")
	}
	return db.Table(tableRef(db)).AutoMigrate(&Event{})
}

// TableReady 探测 outbox 表是否可写（迁移已执行）。
func TableReady(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	// 零行探测：表不存在则报错
	if err := db.Table(tableRef(db)).Limit(0).Find(&[]Event{}).Error; err != nil {
		return false
	}
	return true
}

// Options Worker 配置。
type Options struct {
	MaxAttempts  int
	PollInterval time.Duration
	BatchSize    int
	// Logger 可选；nil 用标准 log。
	Logger func(format string, args ...any)
}

func (o Options) withDefaults() Options {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = DefaultMaxAttempts
	}
	if o.PollInterval <= 0 {
		o.PollInterval = DefaultPollInterval
	}
	if o.BatchSize <= 0 {
		o.BatchSize = DefaultBatchSize
	}
	if o.Logger == nil {
		o.Logger = log.Printf
	}
	return o
}

// StartWorker 轮询 pending 事件并投递；返回 stop（可重复调用）。
func StartWorker(ctx context.Context, db *gorm.DB, pub Publisher, opt Options) (stop func()) {
	opt = opt.withDefaults()
	if db == nil || pub == nil {
		opt.Logger("outbox worker: db or publisher nil, not started")
		return func() {}
	}
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		opt.Logger("outbox worker: started (poll=%s batch=%d max_attempts=%d)",
			opt.PollInterval, opt.BatchSize, opt.MaxAttempts)
		ticker := time.NewTicker(opt.PollInterval)
		defer ticker.Stop()
		for {
			if err := ProcessOnce(runCtx, db, pub, opt); err != nil && !errors.Is(err, context.Canceled) {
				opt.Logger("outbox worker: process: %v", err)
			}
			select {
			case <-runCtx.Done():
				opt.Logger("outbox worker: stopped")
				return
			case <-ticker.C:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			wg.Wait()
		})
	}
}

// ProcessOnce 抢占一批 pending 并投递（供测试与 worker 调用）。
func ProcessOnce(ctx context.Context, db *gorm.DB, pub Publisher, opt Options) error {
	opt = opt.withDefaults()
	events, err := Claim(db, opt.BatchSize)
	if err != nil {
		return err
	}
	for i := range events {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ev := &events[i]
		if err := pub.Publish(ctx, ev.Subject, ev.Payload); err != nil {
			if markErr := MarkFailure(db, ev, err, opt.MaxAttempts); markErr != nil {
				opt.Logger("outbox: mark failure id=%d: %v (publish err: %v)", ev.ID, markErr, err)
			}
			continue
		}
		if err := MarkSent(db, ev.ID); err != nil {
			opt.Logger("outbox: mark sent id=%d: %v", ev.ID, err)
		}
	}
	return nil
}

// Claim 抢占最多 limit 条到期 pending 行，标为 sending 并 attempts+1。
// PostgreSQL 用 FOR UPDATE SKIP LOCKED；其它方言退化为事务内串行锁。
func Claim(db *gorm.DB, limit int) ([]Event, error) {
	if db == nil {
		return nil, errors.New("outbox: db is nil")
	}
	if limit <= 0 {
		limit = DefaultBatchSize
	}
	if isPostgres(db) {
		return claimPostgres(db, limit)
	}
	return claimGeneric(db, limit)
}

func isPostgres(db *gorm.DB) bool {
	name := strings.ToLower(db.Dialector.Name())
	return name == "postgres" || name == "postgresql"
}

func claimPostgres(db *gorm.DB, limit int) ([]Event, error) {
	var events []Event
	// 单语句 CTE：SKIP LOCKED 多副本安全
	sql := `
WITH cte AS (
  SELECT id FROM outbox_events
  WHERE status = ? AND next_at <= NOW()
  ORDER BY id
  FOR UPDATE SKIP LOCKED
  LIMIT ?
)
UPDATE outbox_events e
SET status = ?, attempts = e.attempts + 1, updated_at = NOW()
FROM cte
WHERE e.id = cte.id
RETURNING e.id, e.tenant_id, e.subject, e.payload, e.status, e.attempts, e.last_error, e.next_at, e.created_at, e.updated_at`
	if err := db.Raw(sql, StatusPending, limit, StatusSending).Scan(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func claimGeneric(db *gorm.DB, limit int) ([]Event, error) {
	var events []Event
	tbl := tableRef(db)
	err := db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Table(tbl).
			Where("status = ? AND next_at <= ?", StatusPending, now).
			Order("id ASC").
			Limit(limit).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Find(&events).Error; err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		ids := make([]uint64, len(events))
		for i := range events {
			ids[i] = events[i].ID
		}
		if err := tx.Table(tbl).Where("id IN ?", ids).Updates(map[string]any{
			"status":     StatusSending,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		// 回填 attempts
		for i := range events {
			events[i].Status = StatusSending
			events[i].Attempts++
		}
		return nil
	})
	return events, err
}

// MarkSent 标记投递成功。
func MarkSent(db *gorm.DB, id uint64) error {
	return db.Table(tableRef(db)).Where("id = ?", id).Updates(map[string]any{
		"status":     StatusSent,
		"last_error": "",
		"updated_at": time.Now(),
	}).Error
}

// MarkFailure 失败退避；超过 maxAttempts 进 failed。
func MarkFailure(db *gorm.DB, ev *Event, publishErr error, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	errMsg := ""
	if publishErr != nil {
		errMsg = publishErr.Error()
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
	}
	attempts := ev.Attempts
	if attempts < 1 {
		attempts = 1
	}
	status := StatusPending
	nextAt := time.Now().Add(backoff(attempts))
	if attempts >= maxAttempts {
		status = StatusFailed
		nextAt = time.Now()
	}
	return db.Table(tableRef(db)).Where("id = ?", ev.ID).Updates(map[string]any{
		"status":     status,
		"last_error": errMsg,
		"next_at":    nextAt,
		"updated_at": time.Now(),
	}).Error
}

func backoff(attempts int) time.Duration {
	// 2^min(attempts,8) 秒，上限 5 分钟
	shift := attempts
	if shift > 8 {
		shift = 8
	}
	d := time.Duration(1<<uint(shift)) * time.Second
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

// CountByStatus 运维/测试用。
func CountByStatus(db *gorm.DB, status string) (int64, error) {
	var n int64
	err := db.Table(tableRef(db)).Where("status = ?", status).Count(&n).Error
	return n, err
}

// String 便于日志。
func (e Event) String() string {
	return fmt.Sprintf("outbox#%d subject=%s status=%s attempts=%d", e.ID, e.Subject, e.Status, e.Attempts)
}
