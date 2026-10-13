// Package store persists call records pushed by freeswitch-cc webhooks.
// Tables are AutoMigrated like im-service (experimental line); they move to
// goose when the CC feature graduates.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	connpool "github.com/go-admin-kit/services/shared/pkg/connpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Call mirrors freeswitch-cc's cc_calls shape plus tenant scoping. call.* 事件
// 按 call_id 幂等 upsert：乱序/重放的 webhook 不会产生重复行。
type Call struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	TenantID  uint64 `gorm:"not null;default:1;index" json:"tenant_id"`
	CallID    string `gorm:"size:64;uniqueIndex" json:"call_id"`
	Direction string `gorm:"size:16" json:"direction"`
	Caller    string `gorm:"size:64;index" json:"caller"`
	Callee    string `gorm:"size:64" json:"callee"`
	AgentExt  string `gorm:"size:32;index" json:"agent_ext"`
	// AgentUserID 由 webhook 侧按 cc_agents 绑定归因（分机 → 用户），可空。
	AgentUserID *uint64 `gorm:"index" json:"agent_user_id,omitempty"`
	Queue       string  `gorm:"size:64" json:"queue"`
	Status      string  `gorm:"size:32;index" json:"status"`
	HangupCause string  `gorm:"size:64" json:"hangup_cause"`
	// HangupClass 信令原因归类：ok/empty/busy/noanswer/unavailable/rejected/local/other。
	HangupClass string `gorm:"size:16" json:"hangup_class"`
	DurationSec int    `json:"duration_sec"`
	Recording   string `gorm:"size:512" json:"recording"`
	// CollectedDigits 是 IVR 收号（play_and_get_digits 存 cc_collected）随 CDR
	// variables 带出的按键串，可空。
	CollectedDigits string     `gorm:"size:64" json:"collected_digits"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	AnsweredAt      *time.Time `json:"answered_at,omitempty"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (Call) TableName() string { return "cc_calls" }

// WebhookLog keeps the raw envelope for troubleshooting and replay audits.
type WebhookLog struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Event     string    `gorm:"size:64;index" json:"event"`
	CallID    string    `gorm:"size:64;index" json:"call_id"`
	Body      string    `gorm:"type:text" json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func (WebhookLog) TableName() string { return "cc_webhook_logs" }

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	connpool.Apply(db)
	s := &Store{db: db}
	if err := db.AutoMigrate(&Call{}, &WebhookLog{}, &Agent{}, &Contact{},
		&CCRate{}, &CCBillingAccount{}, &CCCallCharge{}, &CCAccountTxn{}, &CCBill{},
		&CCTranscript{}, &CCQcKeyword{}, &CCQcResult{}, &CCTranscribeJob{},
		&CCRiskPolicy{}, &CCRiskListEntry{}, &CCRiskAttempt{}, &CCVoiceNotify{}); err != nil {
		return nil, err
	}
	if err := s.EnsureIndexes(); err != nil {
		return nil, err
	}
	return s, nil
}

// EnsureIndexes 维护 gorm tag 表达不了的复合/降序索引（口径同 im-service）：
// 列表查询清一色 WHERE tenant_id=? [AND status=?] ORDER BY id DESC，单列
// tenant_id 索引只能过滤不能供序，PG 仍要排序全部命中行。全部 IF NOT EXISTS
// 幂等，仅 PG 执行（单测注入 sqlite 时直接跳过）。
func (s *Store) EnsureIndexes() error {
	if s.db.Dialector.Name() != "postgres" {
		return nil
	}
	for _, stmt := range ensureIndexStatements() {
		if err := s.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure index (%s): %w", stmt, err)
		}
	}
	return nil
}

func ensureIndexStatements() []string {
	return []string{
		// 话单列表：tenant + id DESC 供序；带 status 过滤的走三列索引
		`CREATE INDEX IF NOT EXISTS idx_cc_calls_tenant_id_desc ON cc_calls (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_calls_tenant_status_id ON cc_calls (tenant_id, status, id DESC)`,
		// 报表按时间范围拉行（started_at 为空回落 created_at）
		`CREATE INDEX IF NOT EXISTS idx_cc_calls_tenant_started ON cc_calls (tenant_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_calls_tenant_created ON cc_calls (tenant_id, created_at)`,
		// 话费流水列表 / 账单明细导出（tenant + created_at 范围 + id 序）
		`CREATE INDEX IF NOT EXISTS idx_cc_call_charges_tenant_id_desc ON cc_call_charges (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_call_charges_tenant_created ON cc_call_charges (tenant_id, created_at)`,
		// 资金流水列表
		`CREATE INDEX IF NOT EXISTS idx_cc_account_txns_tenant_id_desc ON cc_account_txns (tenant_id, id DESC)`,
		// 质检结果列表
		`CREATE INDEX IF NOT EXISTS idx_cc_qc_results_tenant_id_desc ON cc_qc_results (tenant_id, id DESC)`,
		// 转写任务：worker 取 pending（status + id ASC）、前端按 tenant+call 查
		`CREATE INDEX IF NOT EXISTS idx_cc_transcribe_jobs_status_id ON cc_transcribe_jobs (status, id)`,
		// 余额预警扫描：可用额 ≤ 阈值且未冻结
		`CREATE INDEX IF NOT EXISTS idx_cc_billing_accounts_status_tenant ON cc_billing_accounts (status, tenant_id)`,
		// webhook 日志按时间清理
		`CREATE INDEX IF NOT EXISTS idx_cc_webhook_logs_created ON cc_webhook_logs (created_at)`,
		// 外呼风控：名单列表、尝试频次三个滚动窗口
		`CREATE INDEX IF NOT EXISTS idx_cc_risk_lists_tenant_id_desc ON cc_risk_list_entries (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_risk_attempts_tenant_created ON cc_risk_attempts (tenant_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_risk_attempts_tenant_callee_created ON cc_risk_attempts (tenant_id, callee, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_risk_attempts_tenant_ext_created ON cc_risk_attempts (tenant_id, from_ext, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_voice_notifies_tenant_id_desc ON cc_voice_notifies (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_cc_voice_notifies_status_id ON cc_voice_notifies (status, id)`,
		// 被上面复合索引取代的单列索引
		`DROP INDEX IF EXISTS idx_cc_calls_tenant_id`,
		`DROP INDEX IF EXISTS idx_cc_calls_status`,
		`DROP INDEX IF EXISTS idx_cc_call_charges_tenant_id`,
		`DROP INDEX IF EXISTS idx_cc_call_charges_created_at`,
		`DROP INDEX IF EXISTS idx_cc_account_txns_tenant_id`,
		`DROP INDEX IF EXISTS idx_cc_qc_results_tenant_id`,
		`DROP INDEX IF EXISTS idx_cc_transcribe_jobs_status`,
	}
}

// NewWithDB wraps an existing connection (tests use in-memory sqlite).
func NewWithDB(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) DB() *gorm.DB { return s.db }

// ApplyEvent merges an event into the call row without regressing state:
// a late call.ringing must not clear answered/ended fields already written.
func (s *Store) ApplyEvent(ctx context.Context, event string, in *Call) (*Call, error) {
	var existing Call
	err := s.db.WithContext(ctx).Where("call_id = ?", in.CallID).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if in.HangupCause != "" && in.HangupClass == "" {
			in.HangupClass = ClassifyHangup(in.HangupCause)
		}
		return in, s.db.WithContext(ctx).Create(in).Error
	case err != nil:
		return nil, err
	}

	updates := map[string]any{"updated_at": time.Now()}
	setIf := func(col string, v string) {
		if v != "" {
			updates[col] = v
		}
	}
	setIf("direction", in.Direction)
	setIf("caller", in.Caller)
	setIf("callee", in.Callee)
	setIf("agent_ext", in.AgentExt)
	if in.AgentUserID != nil && existing.AgentUserID == nil {
		updates["agent_user_id"] = in.AgentUserID
	}
	// 租户归属只前进不后退：incoming 明确（>1）而 existing 仍是默认值（1）时更新。
	if in.TenantID > 1 && existing.TenantID == 1 {
		updates["tenant_id"] = in.TenantID
	}
	setIf("queue", in.Queue)
	setIf("recording", in.Recording)
	setIf("hangup_cause", in.HangupCause)
	if in.HangupClass != "" {
		updates["hangup_class"] = in.HangupClass
	} else if in.HangupCause != "" && existing.HangupClass == "" {
		// 早媒体提示音分类比后续本端 CALL_REJECTED 更精确，不覆盖。
		updates["hangup_class"] = ClassifyHangup(in.HangupCause)
	}
	setIf("collected_digits", in.CollectedDigits)
	if in.DurationSec > 0 {
		updates["duration_sec"] = in.DurationSec
	}
	if in.StartedAt != nil && existing.StartedAt == nil {
		updates["started_at"] = in.StartedAt
	}
	if in.AnsweredAt != nil && existing.AnsweredAt == nil {
		updates["answered_at"] = in.AnsweredAt
	}
	if in.EndedAt != nil && existing.EndedAt == nil {
		updates["ended_at"] = in.EndedAt
	}
	// 状态只前进不后退：ringing < answered < hangup；recording_ready 不覆盖 hangup
	if statusRank(in.Status) > statusRank(existing.Status) {
		updates["status"] = in.Status
	}
	if err := s.db.WithContext(ctx).Model(&Call{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	var out Call
	if err := s.db.WithContext(ctx).First(&out, existing.ID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func statusRank(status string) int {
	switch status {
	case "ringing":
		return 1
	case "answered":
		return 2
	case "hangup":
		return 3
	default:
		return 0
	}
}

func (s *Store) LogWebhook(ctx context.Context, event, callID, body string) error {
	return s.db.WithContext(ctx).Create(&WebhookLog{Event: event, CallID: callID, Body: body}).Error
}

// PurgeWebhookLogs 删除 before 之前的原始事件留档（cc_webhook_logs 只进不出会
// 成为库里涨得最快的表；话单事实已在 cc_calls，留档只服务近期排障）。
func (s *Store) PurgeWebhookLogs(ctx context.Context, before time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Where("created_at < ?", before).Delete(&WebhookLog{})
	return res.RowsAffected, res.Error
}

// GetCall 按 call_id 查话单（转写质检拉录音用）。未命中返回 gorm.ErrRecordNotFound。
func (s *Store) GetCall(ctx context.Context, tenantID uint64, callID string) (*Call, error) {
	var call Call
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND call_id = ?", tenantID, callID).First(&call).Error; err != nil {
		return nil, err
	}
	return &call, nil
}

// FindCallByRecording 按录音文件名查本租户话单（recording 存完整路径，按等值或
// “/文件名”结尾匹配），录音下载的归属校验用。file 里的 LIKE 通配符（_）已转义。
// 未命中返回 gorm.ErrRecordNotFound。
func (s *Store) FindCallByRecording(ctx context.Context, tenantID uint64, file string) (*Call, error) {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(file)
	var call Call
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND (recording = ? OR recording LIKE ? ESCAPE '\\')", tenantID, file, "%/"+escaped).
		First(&call).Error; err != nil {
		return nil, err
	}
	return &call, nil
}

type ListCallsQuery struct {
	TenantID uint64
	Status   string
	Caller   string
	AgentExt string
	// Number 号码双向匹配（caller 或 callee），CRM 客户通话记录用。
	Number   string
	Page     int
	PageSize int
}

func (s *Store) ListCalls(ctx context.Context, q ListCallsQuery) ([]Call, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 20
	}
	db := s.db.WithContext(ctx).Model(&Call{}).Where("tenant_id = ?", q.TenantID)
	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}
	if q.Caller != "" {
		db = db.Where("caller LIKE ?", "%"+q.Caller+"%")
	}
	if q.AgentExt != "" {
		db = db.Where("agent_ext = ?", q.AgentExt)
	}
	if q.Number != "" {
		like := "%" + q.Number + "%"
		db = db.Where("caller LIKE ? OR callee LIKE ?", like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Call
	err := db.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&list).Error
	return list, total, err
}

// EnsureTenant maps zero to the default tenant, matching im-service semantics.
func EnsureTenant(id uint64) uint64 {
	if id == 0 {
		return 1
	}
	return id
}

// ---- 录音转写质检 ----

// CCTranscript 录音 ASR 转写结果（按 call_id 唯一）。Segments 存整段 JSON
// （分段/时间戳，可空），Text 是纯文本；Status pending|done|failed。
type CCTranscript struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	CallID    string    `gorm:"size:64;uniqueIndex" json:"call_id"`
	Text      string    `gorm:"type:text" json:"text"`
	Segments  string    `gorm:"type:jsonb" json:"segments,omitempty"`
	Status    string    `gorm:"size:16" json:"status"`
	Error     string    `gorm:"size:512" json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (CCTranscript) TableName() string { return "cc_transcripts" }

// CCQcKeyword 质检关键词库。Type "forbidden"（违禁词，命中扣分）或
// "required"（必说话术，缺失扣分）。Word 唯一。
type CCQcKeyword struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	Word      string    `gorm:"size:128;uniqueIndex" json:"word"`
	Type      string    `gorm:"size:16" json:"type"`
	Remark    string    `gorm:"size:256" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}

func (CCQcKeyword) TableName() string { return "cc_qc_keywords" }

// CCQcResult 一通电话的质检结论（按 call_id 唯一）。Hits 存命中明细 JSON
// 数组 [{word,type,count}]。
type CCQcResult struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;default:1;index" json:"tenant_id"`
	CallID    string    `gorm:"size:64;uniqueIndex" json:"call_id"`
	Score     int       `json:"score"`
	Hits      string    `gorm:"type:jsonb" json:"hits"`
	CreatedAt time.Time `json:"created_at"`
}

func (CCQcResult) TableName() string { return "cc_qc_results" }

// UpsertTranscript 按 call_id 幂等写入转写（重跑覆盖旧结果）。
func (s *Store) UpsertTranscript(ctx context.Context, t *CCTranscript) error {
	var existing CCTranscript
	err := s.db.WithContext(ctx).Where("call_id = ?", t.CallID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.WithContext(ctx).Create(t).Error
	}
	if err != nil {
		return err
	}
	t.ID = existing.ID
	t.CreatedAt = existing.CreatedAt
	return s.db.WithContext(ctx).Save(t).Error
}

// GetTranscript 单查转写（未命中返回 gorm.ErrRecordNotFound）。
func (s *Store) GetTranscript(ctx context.Context, tenantID uint64, callID string) (*CCTranscript, error) {
	var t CCTranscript
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND call_id = ?", tenantID, callID).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ListKeywords 列关键词库（按类型、创建时间）。
func (s *Store) ListKeywords(ctx context.Context, tenantID uint64) ([]CCQcKeyword, error) {
	var list []CCQcKeyword
	err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("type ASC, id DESC").Limit(500).Find(&list).Error
	return list, err
}

func (s *Store) CreateKeyword(ctx context.Context, k *CCQcKeyword) error {
	return s.db.WithContext(ctx).Create(k).Error
}

func (s *Store) DeleteKeyword(ctx context.Context, tenantID, id uint64) error {
	res := s.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&CCQcKeyword{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpsertQcResult 按 call_id 幂等写入质检结论。
func (s *Store) UpsertQcResult(ctx context.Context, r *CCQcResult) error {
	var existing CCQcResult
	err := s.db.WithContext(ctx).Where("call_id = ?", r.CallID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.WithContext(ctx).Create(r).Error
	}
	if err != nil {
		return err
	}
	r.ID = existing.ID
	r.CreatedAt = existing.CreatedAt
	return s.db.WithContext(ctx).Save(r).Error
}

// GetQcResult 单查质检结论（异步任务完成后回传得分/命中用）。
// 未命中返回 gorm.ErrRecordNotFound。
func (s *Store) GetQcResult(ctx context.Context, tenantID uint64, callID string) (*CCQcResult, error) {
	var r CCQcResult
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND call_id = ?", EnsureTenant(tenantID), callID).First(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// QcResultRow 联表带出对应通话的基本字段，供质检结果列表展示。
type QcResultRow struct {
	CCQcResult
	Caller   string `json:"caller"`
	Callee   string `json:"callee"`
	AgentExt string `json:"agent_ext"`
}

// ListQcResults 分页列质检结果，联 cc_calls 带主叫/被叫/坐席。
func (s *Store) ListQcResults(ctx context.Context, tenantID uint64, page, pageSize int) ([]QcResultRow, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	base := s.db.WithContext(ctx).Model(&CCQcResult{}).Where("cc_qc_results.tenant_id = ?", tenantID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []QcResultRow
	err := s.db.WithContext(ctx).
		Table("cc_qc_results").
		Select("cc_qc_results.*, cc_calls.caller AS caller, cc_calls.callee AS callee, cc_calls.agent_ext AS agent_ext").
		Joins("LEFT JOIN cc_calls ON cc_calls.call_id = cc_qc_results.call_id").
		Where("cc_qc_results.tenant_id = ?", tenantID).
		Order("cc_qc_results.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error
	return rows, total, err
}
