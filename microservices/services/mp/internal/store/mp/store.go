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
	"gorm.io/gorm/clause"
)

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	connpool.Apply(db)
	return NewWithDB(db)
}

// NewWithDB wraps an existing gorm.DB (tests inject sqlite in-memory).
// AutoMigrate 逐阶段追加：M1 只有 MpAccount/MpMessage，M3 补 MpFan（粉丝列表）
// 与 MpMenu（最简，M4 才发布），后续 M5 再补 MpArticle。
func NewWithDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db}
	if err := migrateSchema(db); err != nil {
		return nil, err
	}
	return s, nil
}

func migrateSchema(db *gorm.DB) error {
	migrate := func(tx *gorm.DB) error {
		if err := deduplicateLegacyMenus(tx); err != nil {
			return err
		}
		if err := deduplicateLegacyInboundMessages(tx); err != nil {
			return err
		}
		return tx.AutoMigrate(
			&MpAccount{},
			&MpMessage{},
			&MpPublishResultJob{},
			&MpFanSyncJob{},
			&MpFan{},
			&MpMenu{},
			&MpArticle{},
		)
	}
	if db.Dialector.Name() != "postgres" {
		return migrate(db)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '30s'").Error; err != nil {
			return fmt.Errorf("set mp migration lock timeout: %w", err)
		}
		if err := tx.Exec("SET LOCAL statement_timeout = '2min'").Error; err != nil {
			return fmt.Errorf("set mp migration statement timeout: %w", err)
		}
		// Serialize new replicas, then block legacy writers while duplicate rows
		// are removed and unique indexes are created in the same transaction.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x4d504d34)).Error; err != nil {
			return fmt.Errorf("lock mp schema migration: %w", err)
		}
		if tx.Migrator().HasTable(&MpMenu{}) {
			if err := tx.Exec("LOCK TABLE mp_menus IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
				return fmt.Errorf("lock legacy menu rows: %w", err)
			}
		}
		if tx.Migrator().HasTable(&MpMessage{}) {
			if err := tx.Exec("LOCK TABLE mp_messages IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
				return fmt.Errorf("lock legacy message rows: %w", err)
			}
		}
		return migrate(tx)
	})
}

// deduplicateLegacyMenus repairs rows that the pre-M4 check-then-insert upsert
// could create concurrently. The newest draft wins before AutoMigrate adds the
// tenant/account unique index.
func deduplicateLegacyMenus(db *gorm.DB) error {
	if !db.Migrator().HasTable(&MpMenu{}) {
		return nil
	}
	type menuIdentity struct {
		ID        uint64
		TenantID  uint64
		AccountID uint64
	}
	var rows []menuIdentity
	if err := db.Table("mp_menus").
		Select("id, tenant_id, account_id").
		Order("tenant_id ASC, account_id ASC, updated_at DESC, id DESC").
		Scan(&rows).Error; err != nil {
		return fmt.Errorf("inspect legacy menu rows: %w", err)
	}
	type menuKey struct {
		tenantID  uint64
		accountID uint64
	}
	seen := make(map[menuKey]struct{}, len(rows))
	staleIDs := make([]uint64, 0)
	for _, row := range rows {
		key := menuKey{tenantID: row.TenantID, accountID: row.AccountID}
		if _, exists := seen[key]; exists {
			staleIDs = append(staleIDs, row.ID)
			continue
		}
		seen[key] = struct{}{}
	}
	if len(staleIDs) == 0 {
		return nil
	}
	if err := db.Where("id IN ?", staleIDs).Delete(&MpMenu{}).Error; err != nil {
		return fmt.Errorf("deduplicate legacy menu rows: %w", err)
	}
	return nil
}

// deduplicateLegacyInboundMessages retains the earliest archive row for each
// non-empty inbound MsgId before the partial unique index is created.
func deduplicateLegacyInboundMessages(db *gorm.DB) error {
	if !db.Migrator().HasTable(&MpMessage{}) {
		return nil
	}
	var sql string
	if db.Dialector.Name() == "postgres" {
		sql = `DELETE FROM mp_messages AS stale
			USING mp_messages AS kept
			WHERE stale.direction = 'in'
			  AND stale.msg_id <> ''
			  AND kept.direction = 'in'
			  AND stale.tenant_id = kept.tenant_id
			  AND stale.account_id = kept.account_id
			  AND stale.msg_id = kept.msg_id
			  AND stale.id > kept.id`
	} else {
		sql = `DELETE FROM mp_messages
			WHERE direction = 'in'
			  AND msg_id <> ''
			  AND id NOT IN (
				SELECT MIN(id) FROM mp_messages
				WHERE direction = 'in' AND msg_id <> ''
				GROUP BY tenant_id, account_id, msg_id
			  )`
	}
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("deduplicate legacy inbound messages: %w", err)
	}
	return nil
}

func (s *Store) DB() *gorm.DB { return s.db }

func tenantQ(db *gorm.DB, tenantID uint64) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

// Page 分页入参，clamp 兜底越界值（与 crm 同约定）。
type Page struct {
	Page     int
	PageSize int
}

func (p Page) clamp() (offset, limit int) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 || p.PageSize > 100 {
		p.PageSize = 20
	}
	return (p.Page - 1) * p.PageSize, p.PageSize
}

// EnsureTenant maps zero to the default tenant (id=1), matching im/cc semantics.
func EnsureTenant(id uint64) uint64 {
	if id == 0 {
		return 1
	}
	return id
}

// ---------- Accounts ----------

// GetAccountByAppID 回调路由用：按 appid 取账号（跨租户唯一，不带 tenant 过滤）。
func (s *Store) GetAccountByAppID(appID string) (*MpAccount, error) {
	var a MpAccount
	err := s.db.Where("app_id = ?", strings.TrimSpace(appID)).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) GetAccount(id, tenantID uint64) (*MpAccount, error) {
	var a MpAccount
	err := tenantQ(s.db, tenantID).First(&a, id).Error
	return &a, err
}

// 账号枚举校验：type 服务号/订阅号，encrypt_mode 明文/兼容/安全。
var (
	validAccountTypes    = map[string]bool{"service": true, "subscribe": true}
	validEncryptModes    = map[string]bool{"plain": true, "compat": true, "safe": true}
	validAccountStatuses = map[string]bool{"active": true, "inactive": true}
)

// ListAccounts 列出当前租户的账号，keyword 匹配 name/app_id。Token 已 json:"-"，
// 返回体不含密钥明文（SecretRef/AESKeyRef 是引用名可回显）。
func (s *Store) ListAccounts(tenantID uint64, keyword string, page, pageSize int) ([]MpAccount, int64, error) {
	q := tenantQ(s.db.Model(&MpAccount{}), tenantID)
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR app_id LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := Page{Page: page, PageSize: pageSize}.clamp()
	var list []MpAccount
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// CreateAccount 建账号：校验 app_id 非空 + type/encrypt_mode 合法；app_id 全局
// 唯一冲突转友好错误（回调按 appid 路由，跨租户唯一）。
func (s *Store) CreateAccount(ctx context.Context, a *MpAccount) error {
	a.AppID = strings.TrimSpace(a.AppID)
	if a.AppID == "" {
		return errors.New("app_id 必填")
	}
	a.TenantID = EnsureTenant(a.TenantID)
	if a.Type == "" {
		a.Type = "service"
	}
	if !validAccountTypes[a.Type] {
		return errors.New("无效的公众号类型（service|subscribe）")
	}
	if a.EncryptMode == "" {
		a.EncryptMode = "plain"
	}
	if !validEncryptModes[a.EncryptMode] {
		return errors.New("无效的加密模式（plain|compat|safe）")
	}
	if a.Status == "" {
		a.Status = "active"
	}
	if !validAccountStatuses[a.Status] {
		return errors.New("无效的账号状态（active|inactive）")
	}
	if err := s.db.WithContext(ctx).Create(a).Error; err != nil {
		if strings.Contains(err.Error(), "ux_mp_accounts_appid") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("AppID %s 已存在", a.AppID)
		}
		return err
	}
	return nil
}

// UpdateAccount 保存改动（app_id 不可改，由 handler 保证）。type/encrypt_mode
// 若被赋值需合法。
func (s *Store) UpdateAccount(ctx context.Context, a *MpAccount) error {
	if a.Type != "" && !validAccountTypes[a.Type] {
		return errors.New("无效的公众号类型（service|subscribe）")
	}
	if a.EncryptMode != "" && !validEncryptModes[a.EncryptMode] {
		return errors.New("无效的加密模式（plain|compat|safe）")
	}
	if a.Status != "" && !validAccountStatuses[a.Status] {
		return errors.New("无效的账号状态（active|inactive）")
	}
	return s.db.WithContext(ctx).Save(a).Error
}

// DeleteAccount 删账号（tenant 隔离，删不到返回 ErrNotFound）。
func (s *Store) DeleteAccount(ctx context.Context, id, tenantID uint64) error {
	res := tenantQ(s.db.WithContext(ctx), tenantID).Delete(&MpAccount{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- Messages ----------

// LogMessage 留档一条收/发消息。带 MsgID 的入站消息由数据库唯一索引原子去重。
func (s *Store) LogMessage(m *MpMessage) error {
	_, err := s.createMessage(m)
	return err
}

// ArchiveInboundMessage atomically archives an inbound message and reports
// whether this callback inserted a new row. A duplicate retry returns false.
func (s *Store) ArchiveInboundMessage(m *MpMessage) (bool, error) {
	if m == nil {
		return false, errors.New("inbound message is nil")
	}
	m.Direction = "in"
	return s.createMessage(m)
}

func (s *Store) createMessage(m *MpMessage) (bool, error) {
	if m == nil {
		return false, errors.New("message is nil")
	}
	m.TenantID = EnsureTenant(m.TenantID)
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	q := s.db
	if m.Direction == "in" && strings.TrimSpace(m.MsgID) != "" {
		q = q.Clauses(clause.OnConflict{DoNothing: true})
	}
	res := q.Create(m)
	return res.RowsAffected == 1, res.Error
}

// ListMessages 收发消息列表（tenant 隔离，按 id desc）。account_id/open_id/
// direction 皆为可选过滤。
func (s *Store) ListMessages(tenantID, accountID uint64, openID, direction string, page, pageSize int) ([]MpMessage, int64, error) {
	q := tenantQ(s.db.Model(&MpMessage{}), tenantID)
	if accountID > 0 {
		q = q.Where("account_id = ?", accountID)
	}
	if oid := strings.TrimSpace(openID); oid != "" {
		q = q.Where("open_id = ?", oid)
	}
	if d := strings.TrimSpace(direction); d != "" {
		q = q.Where("direction = ?", d)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := Page{Page: page, PageSize: pageSize}.clamp()
	var list []MpMessage
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// GetMessage 读取当前租户下的一条消息，供基于入站记录的客服回复使用。
func (s *Store) GetMessage(id, tenantID uint64) (*MpMessage, error) {
	var m MpMessage
	err := tenantQ(s.db, tenantID).First(&m, id).Error
	return &m, err
}

// HasRecentInboundMessage 判断指定会话是否仍处于客服回复窗口。
func (s *Store) HasRecentInboundMessage(tenantID, accountID uint64, openID string, since time.Time) (bool, error) {
	var count int64
	err := tenantQ(s.db.Model(&MpMessage{}), tenantID).
		Where("account_id = ? AND open_id = ? AND direction = 'in' AND created_at >= ?", accountID, openID, since).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}

// ---------- Fans ----------

// ListFans 粉丝列表（tenant 隔离，按 id desc）。keyword 匹配 nickname/open_id。
// 本阶段数据可能为空（M4 才从微信拉），接口先落地。
func (s *Store) ListFans(tenantID, accountID uint64, keyword string, page, pageSize int) ([]MpFan, int64, error) {
	q := tenantQ(s.db.Model(&MpFan{}), tenantID)
	if accountID > 0 {
		q = q.Where("account_id = ?", accountID)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("nickname LIKE ? OR open_id LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := Page{Page: page, PageSize: pageSize}.clamp()
	var list []MpFan
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// ---------- Articles (M5) ----------

// ListArticles 图文列表（tenant 隔离，可按 account_id/status 过滤，id desc）。
func (s *Store) ListArticles(tenantID, accountID uint64, status string, page, pageSize int) ([]MpArticle, int64, error) {
	q := tenantQ(s.db.Model(&MpArticle{}), tenantID)
	if accountID > 0 {
		q = q.Where("account_id = ?", accountID)
	}
	if st := strings.TrimSpace(status); st != "" {
		q = q.Where("status = ?", st)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := Page{Page: page, PageSize: pageSize}.clamp()
	var list []MpArticle
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func (s *Store) GetArticle(id, tenantID uint64) (*MpArticle, error) {
	var a MpArticle
	err := tenantQ(s.db, tenantID).First(&a, id).Error
	return &a, err
}

func (s *Store) CreateArticle(a *MpArticle) error {
	a.TenantID = EnsureTenant(a.TenantID)
	if a.Status == "" {
		a.Status = "draft_ai"
	}
	return s.db.Create(a).Error
}

func (s *Store) UpdateArticle(a *MpArticle) error { return s.db.Save(a).Error }

func (s *Store) DeleteArticle(id, tenantID uint64) error {
	res := tenantQ(s.db, tenantID).Delete(&MpArticle{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateArticleStatusByPublishID atomically settles only the article owned by
// the account whose callback signature was verified.
func (s *Store) UpdateArticleStatusByPublishID(tenantID, accountID uint64, publishID, status string) (bool, error) {
	res := tenantQ(s.db.Model(&MpArticle{}), tenantID).
		Where("account_id = ? AND publish_id = ?", accountID, publishID).
		Update("status", status)
	return res.RowsAffected > 0, res.Error
}

// UpsertPublishResultJob durably records the latest verified result for an
// account/publish pair. A repeated callback revives a pending/dead job.
func (s *Store) UpsertPublishResultJob(job *MpPublishResultJob) (*MpPublishResultJob, error) {
	if job == nil {
		return nil, errors.New("publish result job is nil")
	}
	job.TenantID = EnsureTenant(job.TenantID)
	job.Status = "pending"
	job.Attempts = 0
	job.NextAt = time.Now()
	job.LastError = ""
	job.SettledAt = nil
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "account_id"}, {Name: "publish_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"publish_status": job.PublishStatus,
			"status":         "pending",
			"attempts":       0,
			"next_at":        job.NextAt,
			"last_error":     "",
			"settled_at":     nil,
			"updated_at":     time.Now(),
		}),
	}).Create(job).Error; err != nil {
		return nil, err
	}
	var saved MpPublishResultJob
	err := s.db.Where(
		"tenant_id = ? AND account_id = ? AND publish_id = ?",
		job.TenantID, job.AccountID, job.PublishID,
	).First(&saved).Error
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// ClaimPublishResultJobs leases due jobs. PostgreSQL uses SKIP LOCKED so the
// two Swarm replicas never own the same retry concurrently.
func (s *Store) ClaimPublishResultJobs(limit int, lease time.Duration) ([]MpPublishResultJob, error) {
	if limit <= 0 {
		limit = 50
	}
	var claimed []MpPublishResultJob
	err := s.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("status = ? AND next_at <= ?", "pending", time.Now()).Order("id").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Find(&claimed).Error; err != nil {
			return err
		}
		leaseUntil := time.Now().Add(lease)
		for i := range claimed {
			if err := tx.Model(&MpPublishResultJob{}).
				Where("id = ? AND status = ?", claimed[i].ID, "pending").
				Update("next_at", leaseUntil).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return claimed, err
}

func (s *Store) FinishPublishResultJob(id uint64) error {
	now := time.Now()
	return s.db.Model(&MpPublishResultJob{}).Where("id = ?", id).Updates(map[string]any{
		"status": "settled", "settled_at": &now, "last_error": "",
	}).Error
}

func (s *Store) RetryPublishResultJob(id uint64, attempts, maxAttempts int, lastError string) error {
	if len(lastError) > 480 {
		lastError = lastError[:480]
	}
	updates := map[string]any{"attempts": attempts, "last_error": lastError}
	if attempts >= maxAttempts {
		updates["status"] = "dead"
	} else {
		shift := attempts
		if shift > 6 {
			shift = 6
		}
		updates["next_at"] = time.Now().Add(time.Duration(1<<uint(shift)) * time.Second)
	}
	return s.db.Model(&MpPublishResultJob{}).Where("id = ?", id).Updates(updates).Error
}

var ErrNotFound = errors.New("not found")

// ---------- Menu ----------

// UpsertMenu 原子保存某账号的自定义菜单（一租户一账号一条）。
func (s *Store) UpsertMenu(tenantID, accountID uint64, buttons string, published bool) error {
	m := MpMenu{
		TenantID: EnsureTenant(tenantID), AccountID: accountID,
		Buttons: buttons, Published: published,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "account_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"buttons", "published", "updated_at",
		}),
	}).Create(&m).Error
}

// GetMenu 取某账号的菜单记录（可能不存在）。
func (s *Store) GetMenu(tenantID, accountID uint64) (*MpMenu, error) {
	var m MpMenu
	err := tenantQ(s.db, tenantID).Where("account_id = ?", accountID).First(&m).Error
	return &m, err
}

// MarkMenuPublished marks only the exact draft that was sent to WeChat. It
// also persists the normalized JSON without overwriting a concurrent save.
func (s *Store) MarkMenuPublished(tenantID, accountID uint64, expectedButtons, normalizedButtons string) (bool, error) {
	res := tenantQ(s.db.Model(&MpMenu{}), tenantID).
		Where("account_id = ? AND buttons = ?", accountID, expectedButtons).
		Updates(map[string]any{"buttons": normalizedButtons, "published": true})
	return res.RowsAffected == 1, res.Error
}

// DeleteMenu 删除某账号的本地菜单草稿。无记录也视为成功。
func (s *Store) DeleteMenu(tenantID, accountID uint64) error {
	return tenantQ(s.db, tenantID).Where("account_id = ?", accountID).Delete(&MpMenu{}).Error
}

// SetFanSubscribed records subscribe/unsubscribe callbacks without overwriting
// profile fields previously fetched from WeChat. It also creates a minimal fan
// row when a callback arrives before the first full fan synchronization.
func (s *Store) SetFanSubscribed(tenantID, accountID uint64, openID string, subscribed bool, eventAt time.Time) error {
	if eventAt.IsZero() {
		eventAt = time.Now()
	}
	// WeChat CreateTime has second precision. Place callbacks at the end of
	// that second so they conservatively win over a sync snapshot captured in
	// the same second, regardless of which database write lands first. For two
	// opposite callbacks in the same second, unsubscribe wins because their
	// source ordering cannot be recovered and false is the safer state.
	eventAt = eventAt.Truncate(time.Second).Add(time.Second - time.Microsecond)
	now := time.Now()
	return s.db.Model(&MpFan{}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}, {Name: "open_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"subscribed": gorm.Expr(
				`CASE WHEN mp_fans.subscribe_event_at IS NULL
					OR mp_fans.subscribe_event_at < excluded.subscribe_event_at
					OR (mp_fans.subscribe_event_at = excluded.subscribe_event_at AND excluded.subscribed = false)
				THEN excluded.subscribed ELSE mp_fans.subscribed END`,
			),
			"subscribe_event_at": gorm.Expr(
				`CASE WHEN mp_fans.subscribe_event_at IS NULL
					OR mp_fans.subscribe_event_at < excluded.subscribe_event_at
					OR (mp_fans.subscribe_event_at = excluded.subscribe_event_at AND excluded.subscribed = false)
				THEN excluded.subscribe_event_at ELSE mp_fans.subscribe_event_at END`,
			),
			"tenant_id":  EnsureTenant(tenantID),
			"updated_at": now,
		}),
	}).Create(map[string]any{
		"tenant_id": EnsureTenant(tenantID), "account_id": accountID,
		"open_id": openID, "subscribed": subscribed, "subscribe_event_at": eventAt,
		"created_at": now, "updated_at": now,
	}).Error
}
