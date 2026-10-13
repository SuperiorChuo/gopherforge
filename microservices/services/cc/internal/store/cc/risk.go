package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CCRiskPolicy 租户外呼风控策略：每租户一行。数值 0 / 空字符串 = 该规则关闭。
type CCRiskPolicy struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;uniqueIndex" json:"tenant_id"`
	// OutboundStart / OutboundEnd 允许外呼的本地时段（Asia/Shanghai，HH:mm）。
	// 都空 = 不限时段；仅填一侧视为未配置。Start>End 视为隔夜窗（如 22:00-06:00）。
	OutboundStart string `gorm:"size:8" json:"outbound_start"`
	OutboundEnd   string `gorm:"size:8" json:"outbound_end"`
	// OutboundWeekdays ISO 星期 1=周一 … 7=周日，逗号分隔；空 = 每天。
	OutboundWeekdays string `gorm:"size:32" json:"outbound_weekdays"`
	// 频次：0=不限。被叫/分机按滚动 1 小时，租户按滚动 1 分钟。
	MaxCallsPerNumberPerHour int `json:"max_calls_per_number_per_hour"`
	MaxCallsPerAgentPerHour  int `json:"max_calls_per_agent_per_hour"`
	MaxCallsPerTenantPerMin  int `json:"max_calls_per_tenant_per_min"`
	// BlockedPrefixes 禁止外呼的被叫前缀，逗号分隔（如 "00,166"）。
	BlockedPrefixes string `gorm:"size:256" json:"blocked_prefixes"`
	// MaxRatePerMin 命中费率超过该值（分/分钟）则拒呼；0=不限。
	MaxRatePerMin int64     `json:"max_rate_per_min"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (CCRiskPolicy) TableName() string { return "cc_risk_policies" }

// CCRiskListEntry 租户外呼黑/白名单。白名单命中后跳过其余外呼规则。
type CCRiskListEntry struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;uniqueIndex:ux_cc_risk_lists_tenant_number_type" json:"tenant_id"`
	Number   string `gorm:"size:32;not null;uniqueIndex:ux_cc_risk_lists_tenant_number_type" json:"number"`
	// Match exact=全号相等，prefix=被叫以此号码开头。
	Match string `gorm:"size:8;not null" json:"match"`
	// ListType black / white
	ListType  string    `gorm:"size:8;not null;uniqueIndex:ux_cc_risk_lists_tenant_number_type" json:"list_type"`
	Remark    string    `gorm:"size:128" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}

func (CCRiskListEntry) TableName() string { return "cc_risk_list_entries" }

// CCRiskAttempt 一次已放行的外呼尝试（频次计数事实源）。不审计：量比配置表高一个数量级。
type CCRiskAttempt struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;index" json:"tenant_id"`
	FromExt   string    `gorm:"size:32;index" json:"from_ext"`
	Callee    string    `gorm:"size:64;index" json:"callee"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (CCRiskAttempt) TableName() string { return "cc_risk_attempts" }

func (s *Store) GetOrCreatePolicy(ctx context.Context, tenantID uint64) (*CCRiskPolicy, error) {
	tenantID = EnsureTenant(tenantID)
	var p CCRiskPolicy
	err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&p).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		p = CCRiskPolicy{TenantID: tenantID}
		if cerr := s.db.WithContext(ctx).Create(&p).Error; cerr != nil && !isUniqueViolation(cerr) {
			return nil, cerr
		}
		if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&p).Error; err != nil {
			return nil, err
		}
		return &p, nil
	case err != nil:
		return nil, err
	}
	return &p, nil
}

func (s *Store) SavePolicy(ctx context.Context, p *CCRiskPolicy) (*CCRiskPolicy, error) {
	if p == nil {
		return nil, errors.New("policy required")
	}
	existing, err := s.GetOrCreatePolicy(ctx, p.TenantID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&CCRiskPolicy{}).Where("id = ?", existing.ID).Updates(map[string]any{
		"outbound_start":                strings.TrimSpace(p.OutboundStart),
		"outbound_end":                  strings.TrimSpace(p.OutboundEnd),
		"outbound_weekdays":             strings.TrimSpace(p.OutboundWeekdays),
		"max_calls_per_number_per_hour": p.MaxCallsPerNumberPerHour,
		"max_calls_per_agent_per_hour":  p.MaxCallsPerAgentPerHour,
		"max_calls_per_tenant_per_min":  p.MaxCallsPerTenantPerMin,
		"blocked_prefixes":              strings.TrimSpace(p.BlockedPrefixes),
		"max_rate_per_min":              p.MaxRatePerMin,
		"updated_at":                    time.Now(),
	}).Error; err != nil {
		return nil, err
	}
	var out CCRiskPolicy
	if err := s.db.WithContext(ctx).First(&out, existing.ID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

const maxRiskListRows = 500

func (s *Store) ListRiskLists(ctx context.Context, tenantID uint64) ([]CCRiskListEntry, error) {
	var list []CCRiskListEntry
	err := s.db.WithContext(ctx).
		Where("tenant_id = ?", EnsureTenant(tenantID)).
		Order("id DESC").
		Limit(maxRiskListRows).
		Find(&list).Error
	return list, err
}

func (s *Store) CreateRiskList(ctx context.Context, e *CCRiskListEntry) (*CCRiskListEntry, error) {
	if e == nil {
		return nil, errors.New("entry required")
	}
	e.TenantID = EnsureTenant(e.TenantID)
	e.Number = strings.TrimSpace(e.Number)
	e.Match = strings.TrimSpace(e.Match)
	e.ListType = strings.TrimSpace(e.ListType)
	e.Remark = strings.TrimSpace(e.Remark)
	if e.Number == "" {
		return nil, errors.New("number required")
	}
	if e.Match != "exact" && e.Match != "prefix" {
		return nil, errors.New("match must be exact or prefix")
	}
	if e.ListType != "black" && e.ListType != "white" {
		return nil, errors.New("list_type must be black or white")
	}
	if err := s.db.WithContext(ctx).Create(e).Error; err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Store) DeleteRiskList(ctx context.Context, tenantID, id uint64) (bool, error) {
	if id == 0 {
		return false, nil
	}
	res := s.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", EnsureTenant(tenantID), id).
		Delete(&CCRiskListEntry{})
	return res.RowsAffected > 0, res.Error
}

func (s *Store) RecordRiskAttempt(ctx context.Context, tenantID uint64, fromExt, callee string, at time.Time) error {
	if at.IsZero() {
		at = time.Now()
	}
	return s.db.WithContext(ctx).Create(&CCRiskAttempt{
		TenantID:  EnsureTenant(tenantID),
		FromExt:   strings.TrimSpace(fromExt),
		Callee:    strings.TrimSpace(callee),
		CreatedAt: at,
	}).Error
}

func (s *Store) CountOutboundAttempts(ctx context.Context, tenantID uint64, fromExt, callee string, now time.Time) (OutboundRiskCounts, error) {
	tenantID = EnsureTenant(tenantID)
	var out OutboundRiskCounts
	hourAgo := now.Add(-time.Hour)
	minAgo := now.Add(-time.Minute)
	count := func(conds ...any) (int, error) {
		q := s.db.WithContext(ctx).Model(&CCRiskAttempt{}).Where("tenant_id = ?", tenantID)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		var n int64
		if err := q.Count(&n).Error; err != nil {
			return 0, err
		}
		return int(n), nil
	}
	var err error
	if callee != "" {
		out.NumberLastHour, err = count("callee = ? AND created_at >= ?", callee, hourAgo)
		if err != nil {
			return out, err
		}
	}
	if fromExt != "" {
		out.AgentLastHour, err = count("from_ext = ? AND created_at >= ?", fromExt, hourAgo)
		if err != nil {
			return out, err
		}
	}
	out.TenantLastMin, err = count("created_at >= ?", minAgo)
	return out, err
}

func (s *Store) PurgeRiskAttempts(ctx context.Context, before time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Where("created_at < ?", before).Delete(&CCRiskAttempt{})
	return res.RowsAffected, res.Error
}
