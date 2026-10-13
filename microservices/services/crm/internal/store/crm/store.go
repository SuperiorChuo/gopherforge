// Package store persists CRM entities. AutoMigrate 实验线（与 im/cc 同约定），
// 全部查询强制 tenant_id 隔离；关键业务动作（转化/阶段推进/回款）走事务。
package store

import (
	"fmt"
	"github.com/go-admin-kit/services/crm/internal/dao"
	"github.com/go-admin-kit/services/crm/internal/model"
	connpool "github.com/go-admin-kit/services/shared/pkg/connpool"
	"github.com/go-admin-kit/services/shared/pkg/idempotency"
	"github.com/go-admin-kit/services/shared/pkg/identityclient"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"strings"
)

// crmIdempotencyTable 挂机写跟进幂等键表（schema 内，AutoMigrate 建）。
const crmIdempotencyTable = "crm_idempotency_key"

type Store struct {
	db       *gorm.DB
	tagDAO   *dao.TagDAO
	idClient *identityclient.Client
}

// SetIdentity 设置 identity owner API 客户端（Phase 3：gRPC 优先 + HTTP 回退）。
func (s *Store) SetIdentity(apiBase, internalToken string) {
	if c, err := identityclient.New(apiBase, internalToken); err == nil {
		s.idClient = c
	}
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
func NewWithDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db, tagDAO: dao.NewTagDAO(db)}
	if err := db.AutoMigrate(
		&model.Lead{}, &model.Customer{}, &model.Contact{},
		&model.Opportunity{}, &model.OpportunityStageLog{},
		&model.Contract{}, &model.Payment{}, &model.FollowUp{},
		&model.AssignLog{}, &model.PoolRule{}, &model.Tag{},
	); err != nil {
		return nil, err
	}
	// Phase 5：挂机写跟进幂等表（gRPC 超时 → HTTP 回退不双写）
	if err := idempotency.EnsureTable(db, crmIdempotencyTable); err != nil {
		return nil, fmt.Errorf("crm idempotency table: %w", err)
	}
	// 存量回款补数（BPM M2，设计文档 §8-Q4）：status 列加列前的历史回款未走
	// 审批，一律补 confirmed；列默认值已覆盖 postgres 加列场景，这里兜底
	// 空串/NULL（幂等，启动即跑，与 notify backfillChannels 同手法）。
	if err := db.Exec(
		`UPDATE crm_payments SET status = 'confirmed' WHERE status IS NULL OR status = ''`,
	).Error; err != nil {
		return nil, fmt.Errorf("回款状态补数失败: %w", err)
	}
	if err := s.ensureIndexes(); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureIndexes 补建 gorm tag 表达不了的组合索引（列表全部是
// tenant_id 过滤 + id DESC 分页，单列 tenant_id 索引扫完还要排序）。
// 全部 IF NOT EXISTS 幂等；仅 PG 执行（测试注入 sqlite，DESC 索引对其无意义）。
func (s *Store) ensureIndexes() error {
	if s.db.Dialector.Name() != "postgres" {
		return nil
	}
	for _, stmt := range indexStatements() {
		if err := s.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure index (%s): %w", stmt, err)
		}
	}
	return nil
}

// indexStatements 组合索引 DDL 清单（测试直接跑两遍验幂等）。
func indexStatements() []string {
	return []string{
		// 名片列表：租户 + 分页；带 status 过滤的一档单独走复合索引
		`CREATE INDEX IF NOT EXISTS idx_crm_leads_tenant_id ON crm_leads (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_crm_leads_tenant_status_id ON crm_leads (tenant_id, status, id DESC)`,
		// 客户列表 + 公海视图（owner_user_id=0 / >0 是最常用的过滤维度）
		`CREATE INDEX IF NOT EXISTS idx_crm_customers_tenant_id ON crm_customers (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_crm_customers_tenant_owner_id ON crm_customers (tenant_id, owner_user_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_crm_customers_tenant_phone_norm ON crm_customers (tenant_id, RIGHT(REGEXP_REPLACE(COALESCE(phone, ''), '[^0-9]', '', 'g'), 11))`,
		`CREATE INDEX IF NOT EXISTS idx_crm_customers_tenant_name_norm ON crm_customers (tenant_id, REPLACE(TRIM(COALESCE(name, '')), ' ', ''))`,
		// 商机列表 / 漏斗分组：stage 是唯一的强过滤列
		`CREATE INDEX IF NOT EXISTS idx_crm_opportunities_tenant_id ON crm_opportunities (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_crm_opportunities_tenant_stage_id ON crm_opportunities (tenant_id, stage, id DESC)`,
		// 客户卡片/保护判定按 customer_id 分组聚合
		`CREATE INDEX IF NOT EXISTS idx_crm_opportunities_tenant_customer ON crm_opportunities (tenant_id, customer_id, stage)`,
		// 合同列表 + 客户维度聚合
		`CREATE INDEX IF NOT EXISTS idx_crm_contracts_tenant_id ON crm_contracts (tenant_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_crm_contracts_tenant_status_id ON crm_contracts (tenant_id, status, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_crm_contracts_tenant_customer ON crm_contracts (tenant_id, customer_id, status)`,
		// 合同列表的回款进度：一次 contract_id IN (...) 分组聚合
		`CREATE INDEX IF NOT EXISTS idx_crm_payments_contract_status ON crm_payments (contract_id, status)`,
		// 公海实时态「今日已回收」与归属流转记录
		`CREATE INDEX IF NOT EXISTS idx_crm_assign_logs_tenant_type_created ON crm_assign_logs (tenant_id, target_type, action, created_at DESC)`,
	}
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) TagDAO() *dao.TagDAO { return s.tagDAO }

func tenantQ(db *gorm.DB, tenantID uint64) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(vals ...uint64) uint64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}
