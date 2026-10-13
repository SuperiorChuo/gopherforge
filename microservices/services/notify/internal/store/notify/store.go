package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/model"
	connpool "github.com/go-admin-kit/services/shared/pkg/connpool"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Store struct {
	db                    *gorm.DB
	defaultChannels       string
	identityAPIBase       string
	identityInternalToken string
	identityResolver      *grpcx.Resolver     // Phase 3：Consul 解析器（缓存，勿每调新建）
	identityPool          *grpcx.ConnPool     // Phase 4 收尾：identity gRPC 连接复用
	identityBreaker       *resilience.Breaker // Phase 4：identity gRPC 路径熔断器
}

// ErrUserNotInTenant 表示收件人不属于请求声明的租户（跨租户投递被拒）。
// InternalSend 批量 fan-out 场景据此跳过单个错误用户而不中断整批。
var ErrUserNotInTenant = errors.New("user not in tenant")

// SetIdentity 设置 identity owner API 配置（Phase 2C）。
func (s *Store) SetIdentity(apiBase, internalToken string) {
	s.identityAPIBase = apiBase
	s.identityInternalToken = internalToken
}

func Open(dsn string, defaultChannels string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	connpool.Apply(db)
	return NewWithDB(db, defaultChannels)
}

func NewWithDB(db *gorm.DB, defaultChannels string) (*Store, error) {
	return NewWithDBAndIdentity(db, defaultChannels, "", "")
}

// NewWithDBAndIdentity 构造 store，identity 配置供 Phase 2C owner API 调用（空则软读降级）。
func NewWithDBAndIdentity(db *gorm.DB, defaultChannels, identityAPIBase, identityInternalToken string) (*Store, error) {
	if strings.TrimSpace(defaultChannels) == "" {
		defaultChannels = "inbox"
	}
	// identityResolver 懒建缓存（失败留 nil，调用侧走软读降级）。
	var resolver *grpcx.Resolver
	if r, err := grpcx.NewResolver(""); err == nil {
		resolver = r
	}
	s := &Store{
		db: db, defaultChannels: defaultChannels,
		identityAPIBase: identityAPIBase, identityInternalToken: identityInternalToken,
		identityResolver: resolver,
		identityPool:     grpcx.NewConnPool(resolver, "identity-service"),
		identityBreaker:  resilience.NewBreaker(5, 10*time.Second),
	}
	if err := db.AutoMigrate(&model.Template{}, &model.Message{}, &model.DeliveryLog{}); err != nil {
		return nil, err
	}
	if err := s.ensureIndexes(); err != nil {
		return nil, err
	}
	if err := s.seedTemplates(); err != nil {
		return nil, err
	}
	s.backfillChannels()
	return s, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

// ensureIndexes 补 gorm tag 表达不了的组合索引（AutoMigrate 系服务，全部
// CREATE INDEX IF NOT EXISTS 幂等），并清掉被组合索引取代的旧单列索引。
// 仅 PG 执行（测试注入 sqlite）。原来 notify_messages 只有 6 个单列索引，
// 而热路径查询都是多列条件，PG 每次只能用其中一个。
func (s *Store) ensureIndexes() error {
	if s.db.Dialector.Name() != "postgres" {
		return nil
	}
	stmts := []string{
		// fan-out 去重：tenant+user+ref(+template_code) + 24h 时间窗，
		// 批量化后是 user_id IN (...) 的单次扫描，这条索引直接支撑它。
		`CREATE INDEX IF NOT EXISTS idx_notify_messages_dedupe ON notify_messages (tenant_id, user_id, ref_type, ref_id, created_at)`,
		// retention 清理按 created_at 扫全表。
		`CREATE INDEX IF NOT EXISTS idx_notify_messages_created_at ON notify_messages (created_at)`,
		// 站内信列表 / 未读数：tenant+user 过滤 + id DESC 排序分页。
		`CREATE INDEX IF NOT EXISTS idx_notify_messages_list ON notify_messages (tenant_id, user_id, id DESC)`,
		// 被上面组合索引取代的旧单列索引。
		`DROP INDEX IF EXISTS idx_notify_messages_tenant_id`,
		`DROP INDEX IF EXISTS idx_notify_messages_user_id`,
		`DROP INDEX IF EXISTS idx_notify_messages_ref_type`,
		`DROP INDEX IF EXISTS idx_notify_messages_ref_id`,
	}
	for _, stmt := range stmts {
		if err := s.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("ensure index (%s): %w", stmt, err)
		}
	}
	return nil
}

func (s *Store) seedTemplates() error {
	// Default: inbox only. Ops can set channels="inbox,email" (or +sms) in DB
	// once SMTP / SMS is configured — no surprise external spam.
	defaults := []model.Template{
		{
			TenantID: 1, Code: "ticket.overdue", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "工单超时：{{ticket_no}}",
			BodyTpl:  "工单「{{title}}」已超过截止时间，请尽快处理。",
			LinkTpl:  "/ticket?id={{ticket_id}}",
		},
		{
			TenantID: 1, Code: "ticket.assigned", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "工单指派：{{ticket_no}}",
			BodyTpl:  "您被指派处理工单「{{title}}」。",
			LinkTpl:  "/ticket?id={{ticket_id}}",
		},
		{
			TenantID: 1, Code: "im.transfer_human", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "IM 转人工待接入",
			BodyTpl:  "会话 {{public_id}} 已转人工（{{reason}}），请及时接入。",
			LinkTpl:  "/im/desk",
		},
		{
			TenantID: 1, Code: "ticket.comment", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "工单新评论：{{ticket_no}}",
			BodyTpl:  "工单「{{title}}」有新评论：{{snippet}}",
			LinkTpl:  "/ticket?id={{ticket_id}}",
		},
		{
			TenantID: 1, Code: "crm.followup_due", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "回访提醒：{{target_name}}",
			BodyTpl:  "约定跟进时间已到（{{target_type}} #{{target_id}}）：{{snippet}}",
			LinkTpl:  "{{link}}",
		},
		{
			TenantID: 1, Code: "crm.customer_recycled", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "客户已回收公海：{{name}}",
			BodyTpl:  "客户「{{name}}」因超期未跟进已自动回公海，可重新领取。",
			LinkTpl:  "/crm/customers?scope=pool",
		},
		{
			TenantID: 1, Code: "im.low_rating", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "访客低分评价 ★{{score}}",
			BodyTpl:  "会话 {{public_id}} 获评 {{score}} 星：{{snippet}}",
			LinkTpl:  "/im/low-ratings",
		},
		{
			TenantID: 1, Code: "ticket.sla_warning", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "工单即将超时：{{ticket_no}}",
			BodyTpl:  "工单「{{title}}」将在 {{hours_left}} 小时内到期，请尽快处理。",
			LinkTpl:  "/ticket?id={{ticket_id}}",
		},
		{
			TenantID: 1, Code: "visibility.geo_weekly", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "GEO 检测完成：提及率 {{rate}}%（{{delta}}）",
			BodyTpl:  "「{{brand}}」AI 提及检测：{{mentions}}/{{total}} 题提及{{highlight}}。优化建议已自动更新，点击查看。",
			LinkTpl:  "/visibility/geo",
		},
		// bpm 审批流（M2 补种；发送侧带直发标题兜底，模板缺失也不阻断）
		{
			TenantID: 1, Code: "bpm.task_assigned", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "新的审批待办：{{instance_title}}",
			BodyTpl:  "节点「{{node_name}}」等待你审批。",
			LinkTpl:  "/bpm/todo?taskId={{task_id}}",
		},
		{
			TenantID: 1, Code: "bpm.task_timeout", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "审批超时提醒：{{instance_title}}",
			BodyTpl:  "节点「{{node_name}}」的待办已超过 {{hours}} 小时未处理，请尽快审批。",
			LinkTpl:  "/bpm/todo?taskId={{task_id}}",
		},
		{
			TenantID: 1, Code: "bpm.cc", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "审批抄送：{{instance_title}}",
			BodyTpl:  "流程「{{instance_title}}」在节点「{{node_name}}」抄送给你。",
			LinkTpl:  "/bpm/instance/{{instance_id}}",
		},
		{
			TenantID: 1, Code: "bpm.result", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "审批结果：{{instance_title}}",
			BodyTpl:  "你发起的流程「{{instance_title}}」{{result_text}}。",
			LinkTpl:  "/bpm/instance/{{instance_id}}",
		},
		// Prometheus 告警（Alertmanager webhook → ops_alert 模板，vars: alertname/
		// status/severity/summary/description）。此前缺模板致告警站内信不落库。
		{
			TenantID: 1, Code: "ops_alert", Channel: "inbox", Channels: "inbox", Enabled: true,
			TitleTpl: "{{alertname}} 告警（{{severity}}）",
			BodyTpl:  "告警 {{status}}：{{summary}}",
			LinkTpl:  "/monitor/grafana",
		},
	}
	for _, t := range defaults {
		var n int64
		if err := s.db.Model(&model.Template{}).Where("tenant_id = ? AND code = ?", t.TenantID, t.Code).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			if err := s.db.Create(&t).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// backfillChannels copies legacy channel → channels when empty.
func (s *Store) backfillChannels() {
	_ = s.db.Exec(`UPDATE notify_templates SET channels = channel WHERE channels IS NULL OR channels = ''`).Error
}

func render(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

type SendInput struct {
	TenantID     uint64
	UserID       uint64
	TemplateCode string
	Type         string
	RefType      string
	RefID        string
	Vars         map[string]string
	// Direct fields when no template
	Title   string
	Content string
	Link    string
	// Channels override template (optional csv or list via API)
	Channels []string
}

type SendResult struct {
	Message  *model.Message `json:"message,omitempty"`
	Skipped  bool           `json:"skipped,omitempty"`
	Title    string         `json:"title,omitempty"`
	Content  string         `json:"content,omitempty"`
	Link     string         `json:"link,omitempty"`
	Code     string         `json:"code,omitempty"`
	Type     string         `json:"type,omitempty"`
	Channels []string       `json:"channels,omitempty"`
}

// Rendered is content + resolved channels after template apply.
type Rendered struct {
	Title    string
	Content  string
	Link     string
	Type     string
	Code     string
	Channels []string
}

func (s *Store) resolveTemplate(in SendInput) Rendered {
	title, content, link := in.Title, in.Content, in.Link
	code := strings.TrimSpace(in.TemplateCode)
	chs := in.Channels
	msgType := in.Type
	if code != "" {
		var tpl model.Template
		err := s.db.Where("tenant_id = ? AND code = ? AND enabled = ?", in.TenantID, code, true).First(&tpl).Error
		if err != nil {
			err = s.db.Where("tenant_id = 1 AND code = ? AND enabled = ?", code, true).First(&tpl).Error
		}
		if err == nil {
			vars := in.Vars
			if vars == nil {
				vars = map[string]string{}
			}
			title = render(tpl.TitleTpl, vars)
			content = render(tpl.BodyTpl, vars)
			if tpl.LinkTpl != "" {
				link = render(tpl.LinkTpl, vars)
			}
			if len(chs) == 0 {
				raw := tpl.Channels
				if raw == "" {
					raw = tpl.Channel
				}
				if raw == "" {
					raw = s.defaultChannels
				}
				chs = channel.ParseChannels(raw)
			}
		}
	}
	if len(chs) == 0 {
		chs = channel.ParseChannels(s.defaultChannels)
	}
	if msgType == "" {
		msgType = "notice"
	}
	return Rendered{Title: title, Content: content, Link: link, Type: msgType, Code: code, Channels: chs}
}

// userInTenant 校验用户属于该租户（共享 users 表，identity 所有，只读）。
// Phase 2C：改走 identity owner API（userContacts），不再直查共享 users 表。
// identity 不可达沿用软读惯例放行；查不到匹配 → false（跨租户投递被拒）。
func (s *Store) userInTenant(ctx context.Context, tenantID, userID uint64) bool {
	allowed, _ := s.userContacts(ctx, tenantID, []uint64{userID})
	return allowed[userID]
}

// Send creates inbox (if channel includes inbox) and returns rendered payload for fan-out.
// Dedupes recent same (user, template, ref) within 24h on inbox path.
// 收件人必须属于 in.TenantID，否则返回 ErrUserNotInTenant（防跨租户投递）。
func (s *Store) Send(ctx context.Context, in SendInput) (*SendResult, error) {
	if in.UserID == 0 {
		return nil, errors.New("user_id required")
	}
	if in.TenantID == 0 {
		in.TenantID = 1
	}
	if !s.userInTenant(ctx, in.TenantID, in.UserID) {
		return nil, ErrUserNotInTenant
	}
	r := s.resolveTemplate(in)
	if strings.TrimSpace(r.Title) == "" {
		return nil, errors.New("title required")
	}

	// dedupe (shared across channels for same template+ref)。无模板的直发
	// （title/content）同样按 ref 去重——运维告警/余额预警这类直发依赖它，
	// 否则 Alertmanager repeat_interval / 小时级扫描会刷屏。
	if in.RefID != "" && in.RefType != "" {
		since := time.Now().Add(-24 * time.Hour)
		q := s.db.Model(&model.Message{}).
			Where("tenant_id = ? AND user_id = ? AND ref_type = ? AND ref_id = ? AND created_at > ?",
				in.TenantID, in.UserID, in.RefType, in.RefID, since)
		if r.Code != "" {
			q = q.Where("template_code = ?", r.Code)
		}
		var n int64
		_ = q.Count(&n)
		if n > 0 {
			return &SendResult{
				Skipped: true, Channels: r.Channels,
				Title: r.Title, Content: r.Content, Link: r.Link, Code: r.Code, Type: r.Type,
			}, errors.New("duplicate suppressed")
		}
	}

	out := &SendResult{
		Channels: r.Channels,
		Title:    r.Title, Content: r.Content, Link: r.Link, Code: r.Code, Type: r.Type,
	}
	if channel.Has(r.Channels, "inbox") {
		m := &model.Message{
			TenantID:     in.TenantID,
			UserID:       in.UserID,
			TemplateCode: r.Code,
			Title:        r.Title,
			Content:      r.Content,
			Link:         r.Link,
			Type:         r.Type,
			RefType:      in.RefType,
			RefID:        in.RefID,
		}
		if err := s.db.Create(m).Error; err != nil {
			return nil, err
		}
		out.Message = m
	}
	return out, nil
}

// GetUserContact reads email/phone from identity owner API（Phase 2C，不再直查共享 users 表）。
// 带租户条件：用户不属于该租户时返回空联系方式（防跨租户取 email/phone 外发）。
func (s *Store) GetUserContact(ctx context.Context, tenantID, userID uint64) channel.Contact {
	c := channel.Contact{UserID: userID}
	if userID == 0 {
		return c
	}
	if tenantID == 0 {
		tenantID = 1
	}
	_, contacts := s.userContacts(ctx, tenantID, []uint64{userID})
	if ct, ok := contacts[userID]; ok {
		return ct
	}
	return c
}

// SaveDeliveryLogs 一次外发结果的日志（一条批量 INSERT，原为逐条 Create）。
func (s *Store) SaveDeliveryLogs(tenantID, userID, messageID uint64, code, refType, refID string, results []channel.Result) {
	s.SaveDeliveryLogsBatch(BuildDeliveryLogs(tenantID, userID, messageID, code, refType, refID, results))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type ListFilter struct {
	UnreadOnly bool
	Type       string
	Page       int
	PageSize   int
}

func (s *Store) ListMessages(tenantID, userID uint64, f ListFilter) ([]model.Message, int64, error) {
	q := s.db.Model(&model.Message{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if f.UnreadOnly {
		q = q.Where("read_at IS NULL")
	}
	if t := strings.TrimSpace(f.Type); t != "" {
		q = q.Where("type = ?", t)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, ps := f.Page, f.PageSize
	if page <= 0 {
		page = 1
	}
	if ps <= 0 || ps > 50 {
		ps = 20
	}
	var list []model.Message
	err := q.Order("id DESC").Offset((page - 1) * ps).Limit(ps).Find(&list).Error
	return list, total, err
}

func (s *Store) UnreadCount(tenantID, userID uint64) (int64, error) {
	var n int64
	err := s.db.Model(&model.Message{}).
		Where("tenant_id = ? AND user_id = ? AND read_at IS NULL", tenantID, userID).
		Count(&n).Error
	return n, err
}

func (s *Store) MarkRead(tenantID, userID, id uint64) error {
	now := time.Now()
	res := s.db.Model(&model.Message{}).
		Where("id = ? AND tenant_id = ? AND user_id = ? AND read_at IS NULL", id, tenantID, userID).
		Update("read_at", now)
	return res.Error
}

func (s *Store) MarkAllRead(tenantID, userID uint64) (int64, error) {
	now := time.Now()
	res := s.db.Model(&model.Message{}).
		Where("tenant_id = ? AND user_id = ? AND read_at IS NULL", tenantID, userID).
		Update("read_at", now)
	return res.RowsAffected, res.Error
}

func (s *Store) ListTemplates(tenantID uint64) ([]model.Template, error) {
	if tenantID == 0 {
		tenantID = 1
	}
	return fullread.All[model.Template](s.db.Where("tenant_id = ? OR tenant_id = 1", tenantID).Order("id ASC"))
}

func (s *Store) GetTemplate(id, tenantID uint64) (*model.Template, error) {
	if tenantID == 0 {
		tenantID = 1
	}
	var t model.Template
	// tenant template first, then system tenant=1
	err := s.db.Where("id = ? AND (tenant_id = ? OR tenant_id = 1)", id, tenantID).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// TemplateUpdate is a partial update for ops console.
type TemplateUpdate struct {
	TitleTpl *string
	BodyTpl  *string
	LinkTpl  *string
	Channels *string
	Enabled  *bool
}

func (s *Store) UpdateTemplate(ctx context.Context, id, tenantID uint64, in TemplateUpdate) (*model.Template, error) {
	t, err := s.GetTemplate(id, tenantID)
	if err != nil {
		return nil, err
	}
	// only allow editing same-tenant or system defaults (tenant 1)
	if t.TenantID != tenantID && t.TenantID != 1 {
		return nil, errors.New("template not found")
	}
	if in.TitleTpl != nil {
		v := strings.TrimSpace(*in.TitleTpl)
		if v == "" {
			return nil, errors.New("title_tpl required")
		}
		if len(v) > 256 {
			v = v[:256]
		}
		t.TitleTpl = v
	}
	if in.BodyTpl != nil {
		v := strings.TrimSpace(*in.BodyTpl)
		if v == "" {
			return nil, errors.New("body_tpl required")
		}
		t.BodyTpl = v
	}
	if in.LinkTpl != nil {
		t.LinkTpl = strings.TrimSpace(*in.LinkTpl)
		if len(t.LinkTpl) > 512 {
			t.LinkTpl = t.LinkTpl[:512]
		}
	}
	if in.Channels != nil {
		chs := channel.ParseChannels(*in.Channels)
		// only allow known channels
		allowed := map[string]bool{"inbox": true, "email": true, "sms": true, "voice": true}
		clean := make([]string, 0, len(chs))
		for _, c := range chs {
			if allowed[c] {
				clean = append(clean, c)
			}
		}
		if len(clean) == 0 {
			clean = []string{"inbox"}
		}
		t.Channels = strings.Join(clean, ",")
		t.Channel = clean[0]
	}
	if in.Enabled != nil {
		t.Enabled = *in.Enabled
	}
	if err := s.db.WithContext(ctx).Save(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

// PreviewTemplate renders title/body/link with vars (no send).
func (s *Store) PreviewTemplate(id, tenantID uint64, vars map[string]string) (title, body, link string, tpl *model.Template, err error) {
	tpl, err = s.GetTemplate(id, tenantID)
	if err != nil {
		return "", "", "", nil, err
	}
	if vars == nil {
		vars = map[string]string{}
	}
	title = render(tpl.TitleTpl, vars)
	body = render(tpl.BodyTpl, vars)
	link = render(tpl.LinkTpl, vars)
	return title, body, link, tpl, nil
}

// ListAllDeliveryLogs 租户内全部发送日志（管理视图，分页 + 筛选；邮件管理面，
// 路线图第 12 项）。channel/status 空=不过滤，keyword 匹配 target/template_code。
func (s *Store) ListAllDeliveryLogs(tenantID uint64, channelName, status, keyword string, page, pageSize int) ([]model.DeliveryLog, int64, error) {
	q := s.db.Model(&model.DeliveryLog{}).Where("tenant_id = ?", tenantID)
	if channelName != "" {
		q = q.Where("channel = ?", channelName)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("target LIKE ? OR template_code LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	var list []model.DeliveryLog
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (s *Store) ListDeliveryLogs(tenantID, userID uint64, limit int) ([]model.DeliveryLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := s.db.Model(&model.DeliveryLog{}).Where("tenant_id = ?", tenantID)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var list []model.DeliveryLog
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}
