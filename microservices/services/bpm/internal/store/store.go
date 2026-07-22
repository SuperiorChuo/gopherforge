// Package store 持久化 bpm 五张表。AutoMigrate 自管表（轻量服务同
// 约定），全部查询强制 tenant_id 隔离；防重的部分唯一索引在建表后补建
// （AutoMigrate 不支持 WHERE 索引）。引擎推进事务见 internal/engine。
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/form"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	ErrKeyExists     = errors.New("流程 key 已存在，请在既有定义上发新版本")
	ErrNotDraft      = errors.New("仅草稿版本可编辑")
	ErrNotActive     = errors.New("仅已发布版本可停用")
	ErrNoActive      = errors.New("该流程没有已发布版本")
	ErrEmptyKey      = errors.New("流程 key 不能为空")
	ErrEmptyName     = errors.New("流程名称不能为空")
	ErrBadPublishSrc = errors.New("仅草稿版本可发布")
)

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return NewWithDB(db)
}

// NewWithDB 包装既有 gorm.DB（测试注入 sqlite 内存库）。
func NewWithDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db}
	if err := db.AutoMigrate(
		&model.ProcessDefinition{}, &model.ProcessInstance{},
		&model.Task{}, &model.CcRecord{}, &model.ProcessLog{},
	); err != nil {
		return nil, err
	}
	// 部分唯一索引：同一业务对象同时至多一条在途实例（DB 层防重复发起）。
	// postgres 与 sqlite 语法一致，测试环境同样生效。
	if err := db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_bpm_inst_biz_running
		 ON bpm_process_instance (tenant_id, biz_type, biz_id)
		 WHERE status = 'running'`,
	).Error; err != nil {
		return nil, fmt.Errorf("补建部分唯一索引失败: %w", err)
	}
	return s, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

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

// ---------- 流程定义 ----------

type CreateDefinitionInput struct {
	Key        string
	Name       string
	BizType    string
	NodeTree   []byte
	FormSchema []byte
	Remark     string
	CreatedBy  uint64
}

// CreateDefinition 新建定义（version=1, status=draft）。node_tree 只做结构
// 解析校验（可存半成品草稿），完整校验在发布时。
func (s *Store) CreateDefinition(tenantID uint64, in CreateDefinitionInput) (*model.ProcessDefinition, error) {
	key := strings.TrimSpace(in.Key)
	if key == "" || len(key) > 64 {
		return nil, ErrEmptyKey
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if _, err := flow.Parse(in.NodeTree); err != nil {
		return nil, err
	}
	var cnt int64
	if err := tenantQ(s.db.Model(&model.ProcessDefinition{}), tenantID).
		Where("key = ?", key).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt > 0 {
		return nil, ErrKeyExists
	}
	d := &model.ProcessDefinition{
		TenantID: tenantID, Key: key, Name: name, Version: 1,
		Status: model.DefDraft, NodeTree: model.JSONB(in.NodeTree),
		FormSchema: model.JSONB(in.FormSchema), BizType: strings.TrimSpace(in.BizType),
		Remark: strings.TrimSpace(in.Remark), CreatedBy: in.CreatedBy,
	}
	if err := s.db.Create(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Store) GetDefinition(id, tenantID uint64) (*model.ProcessDefinition, error) {
	var d model.ProcessDefinition
	if err := tenantQ(s.db, tenantID).Where("id = ?", id).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

type UpdateDefinitionInput struct {
	Name       *string
	BizType    *string
	NodeTree   []byte
	FormSchema []byte
	Remark     *string
}

// UpdateDefinition 修改草稿版本（active 版本不可改，需另存新版本）。
func (s *Store) UpdateDefinition(id, tenantID uint64, in UpdateDefinitionInput) (*model.ProcessDefinition, error) {
	d, err := s.GetDefinition(id, tenantID)
	if err != nil {
		return nil, err
	}
	if d.Status != model.DefDraft {
		return nil, ErrNotDraft
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, ErrEmptyName
		}
		d.Name = name
	}
	if in.BizType != nil {
		d.BizType = strings.TrimSpace(*in.BizType)
	}
	if in.Remark != nil {
		d.Remark = strings.TrimSpace(*in.Remark)
	}
	if len(in.NodeTree) > 0 {
		if _, err := flow.Parse(in.NodeTree); err != nil {
			return nil, err
		}
		d.NodeTree = model.JSONB(in.NodeTree)
	}
	if len(in.FormSchema) > 0 {
		d.FormSchema = model.JSONB(in.FormSchema)
	}
	if err := s.db.Save(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

// Publish 发布：整树校验通过后本版本置 active，同 key 旧 active 置 archived。
// 携带 form_schema（流程表单模式）时一并校验表单结构与节点字段权限，并以
// Schema keys 覆盖 start.formFields（条件求值字段声明与表单同源）。
func (s *Store) Publish(id, tenantID uint64) (*model.ProcessDefinition, error) {
	d, err := s.GetDefinition(id, tenantID)
	if err != nil {
		return nil, err
	}
	if d.Status != model.DefDraft {
		return nil, ErrBadPublishSrc
	}
	sc, err := flow.Parse(d.NodeTree)
	if err != nil {
		return nil, err
	}
	fs, err := form.Parse(d.FormSchema)
	if err != nil {
		return nil, err
	}
	if fs != nil {
		if err := fs.Validate(); err != nil {
			return nil, err
		}
		if sc.Start != nil {
			sc.Start.FormFields = fs.Keys()
		}
	}
	var permErr error
	flow.Walk(sc, func(n *flow.Node) bool {
		if n.Type == flow.TypeApproval && len(n.FieldPerms) > 0 {
			if err := fs.ValidateFieldPerms(n.Name, n.FieldPerms); err != nil {
				permErr = err
				return false
			}
		}
		return true
	})
	if permErr != nil {
		return nil, permErr
	}
	if err := flow.Validate(sc); err != nil {
		return nil, err
	}
	if fs != nil { // formFields 覆盖后回写 node_tree
		raw, err := json.Marshal(sc)
		if err != nil {
			return nil, err
		}
		d.NodeTree = model.JSONB(raw)
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tenantQ(tx.Model(&model.ProcessDefinition{}), tenantID).
			Where("key = ? AND status = ? AND id <> ?", d.Key, model.DefActive, d.ID).
			Update("status", model.DefArchived).Error; err != nil {
			return err
		}
		d.Status = model.DefActive
		return tx.Save(d).Error
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// NewVersion 以某版本为底复制出新 draft（version = 同 key 最大版本 + 1）。
func (s *Store) NewVersion(id, tenantID, byUserID uint64) (*model.ProcessDefinition, error) {
	src, err := s.GetDefinition(id, tenantID)
	if err != nil {
		return nil, err
	}
	var maxVer int
	if err := tenantQ(s.db.Model(&model.ProcessDefinition{}), tenantID).
		Where("key = ?", src.Key).
		Select("COALESCE(MAX(version),0)").Scan(&maxVer).Error; err != nil {
		return nil, err
	}
	d := &model.ProcessDefinition{
		TenantID: tenantID, Key: src.Key, Name: src.Name, Version: maxVer + 1,
		Status: model.DefDraft, NodeTree: src.NodeTree, FormSchema: src.FormSchema,
		BizType: src.BizType, Remark: src.Remark, CreatedBy: byUserID,
	}
	if err := s.db.Create(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

// Suspend 停用 active 版本（不再允许新发起，在途实例不受影响）。
func (s *Store) Suspend(id, tenantID uint64) (*model.ProcessDefinition, error) {
	d, err := s.GetDefinition(id, tenantID)
	if err != nil {
		return nil, err
	}
	if d.Status != model.DefActive {
		return nil, ErrNotActive
	}
	d.Status = model.DefSuspended
	if err := s.db.Save(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

// ListStartable 可发起流程（active 且携带 form_schema 的"流程表单"定义，
// 通用发起页用；登录即可访问，不设权限码——发起权是普适权）。
func (s *Store) ListStartable(tenantID uint64) ([]model.ProcessDefinition, error) {
	var list []model.ProcessDefinition
	err := tenantQ(s.db, tenantID).
		Where("status = ?", model.DefActive).
		Order("id DESC").Limit(100).Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.ProcessDefinition, 0, len(list))
	for _, d := range list {
		if fs, err := form.Parse(d.FormSchema); err == nil && fs != nil {
			out = append(out, d)
		}
	}
	return out, nil
}

// ActiveByKey 按 key 取当前 active 版本。
func (s *Store) ActiveByKey(key string, tenantID uint64) (*model.ProcessDefinition, error) {
	var d model.ProcessDefinition
	err := tenantQ(s.db, tenantID).
		Where("key = ? AND status = ?", strings.TrimSpace(key), model.DefActive).
		First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoActive
		}
		return nil, err
	}
	return &d, nil
}

// DefinitionRow 定义列表行：按 key 聚合的最新版本 + active 版本信息。
type DefinitionRow struct {
	model.ProcessDefinition
	ActiveVersion int    `json:"active_version"` // 0=无 active 版本
	ActiveID      uint64 `json:"active_id"`
}

type DefinitionFilter struct {
	Keyword string
	BizType string
	Page
}

// ListDefinitions 定义列表：每个 key 显示最新版本行，附 active 版本号。
func (s *Store) ListDefinitions(tenantID uint64, f DefinitionFilter) ([]DefinitionRow, int64, error) {
	base := tenantQ(s.db.Model(&model.ProcessDefinition{}), tenantID).
		Where(`version = (SELECT MAX(d2.version) FROM bpm_process_definition d2
			WHERE d2.tenant_id = bpm_process_definition.tenant_id
			  AND d2.key = bpm_process_definition.key)`)
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		base = base.Where("key LIKE ? OR name LIKE ?", like, like)
	}
	if bt := strings.TrimSpace(f.BizType); bt != "" {
		base = base.Where("biz_type = ?", bt)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := f.Page.clamp()
	var latest []model.ProcessDefinition
	if err := base.Order("id DESC").Offset(offset).Limit(limit).Find(&latest).Error; err != nil {
		return nil, 0, err
	}
	// 批量取各 key 的 active 版本
	keys := make([]string, 0, len(latest))
	for _, d := range latest {
		keys = append(keys, d.Key)
	}
	activeByKey := map[string]model.ProcessDefinition{}
	if len(keys) > 0 {
		var actives []model.ProcessDefinition
		if err := tenantQ(s.db, tenantID).
			Where("key IN ? AND status = ?", keys, model.DefActive).
			Find(&actives).Error; err != nil {
			return nil, 0, err
		}
		for _, a := range actives {
			activeByKey[a.Key] = a
		}
	}
	out := make([]DefinitionRow, 0, len(latest))
	for _, d := range latest {
		row := DefinitionRow{ProcessDefinition: d}
		if a, hit := activeByKey[d.Key]; hit {
			row.ActiveVersion = a.Version
			row.ActiveID = a.ID
		}
		out = append(out, row)
	}
	return out, total, nil
}

// ---------- 实例 / 任务 / 日志查询（引擎写路径见 internal/engine） ----------

func (s *Store) GetInstance(id, tenantID uint64) (*model.ProcessInstance, error) {
	var inst model.ProcessInstance
	if err := tenantQ(s.db, tenantID).Where("id = ?", id).First(&inst).Error; err != nil {
		return nil, err
	}
	return &inst, nil
}

// ListMyInstances 我发起的实例。
func (s *Store) ListMyInstances(tenantID, me uint64, status string, p Page) ([]model.ProcessInstance, int64, error) {
	q := tenantQ(s.db.Model(&model.ProcessInstance{}), tenantID).
		Where("initiator_id = ?", me)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := p.clamp()
	var list []model.ProcessInstance
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// ListAllInstances 租户内全部实例（M3 管理视图，平台管理员用）。
func (s *Store) ListAllInstances(tenantID uint64, status, keyword string, p Page) ([]model.ProcessInstance, int64, error) {
	q := tenantQ(s.db.Model(&model.ProcessInstance{}), tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR definition_key LIKE ? OR biz_id LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := p.clamp()
	var list []model.ProcessInstance
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

// FindByBiz 按业务对象反查实例（在途 + 历史，按 id 倒序）。
func (s *Store) FindByBiz(tenantID uint64, bizType, bizID string) ([]model.ProcessInstance, error) {
	var list []model.ProcessInstance
	err := tenantQ(s.db, tenantID).
		Where("biz_type = ? AND biz_id = ?", bizType, bizID).
		Order("id DESC").Limit(50).Find(&list).Error
	return list, err
}

// TaskView 任务列表行：任务 + 所属实例摘要（待办/已办共用）。
type TaskView struct {
	ID             uint64     `json:"id"`
	InstanceID     uint64     `json:"instance_id"`
	NodeID         string     `json:"node_id"`
	NodeName       string     `json:"node_name"`
	Round          int        `json:"round"`
	AssigneeID     uint64     `json:"assignee_id"`
	MultiMode      string     `json:"multi_mode"`
	Status         string     `json:"status"`
	Comment        string     `json:"comment,omitempty"`
	TimeoutAt      *time.Time `json:"timeout_at,omitempty"`
	ActedAt        *time.Time `json:"acted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	InstanceTitle  string     `json:"instance_title"`
	InstanceStatus string     `json:"instance_status"`
	BizType        string     `json:"biz_type"`
	BizID          string     `json:"biz_id"`
	InitiatorID    uint64     `json:"initiator_id"`
}

func (s *Store) taskViewQ(tenantID uint64) *gorm.DB {
	return s.db.Table("bpm_task").
		Select(`bpm_task.id, bpm_task.instance_id, bpm_task.node_id, bpm_task.node_name,
			bpm_task.round, bpm_task.assignee_id, bpm_task.multi_mode, bpm_task.status,
			bpm_task.comment, bpm_task.timeout_at, bpm_task.acted_at, bpm_task.created_at,
			i.title AS instance_title, i.status AS instance_status,
			i.biz_type, i.biz_id, i.initiator_id`).
		Joins("JOIN bpm_process_instance i ON i.id = bpm_task.instance_id").
		Where("bpm_task.tenant_id = ?", tenantID)
}

// ListTodo 我的待办（pending 任务，按到达时间正序）。
func (s *Store) ListTodo(tenantID, me uint64, p Page) ([]TaskView, int64, error) {
	q := s.taskViewQ(tenantID).
		Where("bpm_task.assignee_id = ? AND bpm_task.status = ?", me, model.TaskPending)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := p.clamp()
	var list []TaskView
	err := q.Order("bpm_task.id ASC").Offset(offset).Limit(limit).Scan(&list).Error
	return list, total, err
}

// ListDone 我的已办：本人处理过的（approved/rejected，以及带意见退回的
// returned——acted_at 非空区分"我退回的"与"被连带置 returned 的"），加上
// 转办出去的任务（origin_assignee=me，M2 验收：转办后原人已办可见）。
func (s *Store) ListDone(tenantID, me uint64, p Page) ([]TaskView, int64, error) {
	q := s.taskViewQ(tenantID).
		Where(`(bpm_task.assignee_id = ? AND (bpm_task.status IN ?
				OR (bpm_task.status = ? AND bpm_task.acted_at IS NOT NULL)))
			OR (bpm_task.origin_assignee = ? AND bpm_task.assignee_id <> ?)`,
			me, []string{model.TaskApproved, model.TaskRejected},
			model.TaskReturned, me, me)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := p.clamp()
	var list []TaskView
	err := q.Order("bpm_task.id DESC").Offset(offset).Limit(limit).Scan(&list).Error
	return list, total, err
}

func (s *Store) GetTask(id, tenantID uint64) (*model.Task, error) {
	var t model.Task
	if err := tenantQ(s.db, tenantID).Where("id = ?", id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ListInstanceTasks 实例的全部任务（流转图标注用，按创建正序）。
func (s *Store) ListInstanceTasks(instanceID, tenantID uint64) ([]model.Task, error) {
	var list []model.Task
	err := tenantQ(s.db, tenantID).
		Where("instance_id = ?", instanceID).
		Order("id ASC").Find(&list).Error
	return list, err
}

// ListInstanceLogs 实例时间线（按时间正序）。
func (s *Store) ListInstanceLogs(instanceID, tenantID uint64) ([]model.ProcessLog, error) {
	var list []model.ProcessLog
	err := tenantQ(s.db, tenantID).
		Where("instance_id = ?", instanceID).
		Order("id ASC").Find(&list).Error
	return list, err
}

// ListInstanceCc 实例的抄送记录。
func (s *Store) ListInstanceCc(instanceID, tenantID uint64) ([]model.CcRecord, error) {
	var list []model.CcRecord
	err := tenantQ(s.db, tenantID).
		Where("instance_id = ?", instanceID).
		Order("id ASC").Find(&list).Error
	return list, err
}

// InstanceSchema 解析实例冻结版本的节点树（任务详情动作列表用）。
func (s *Store) InstanceSchema(inst *model.ProcessInstance) (*flow.Schema, error) {
	var def model.ProcessDefinition
	if err := s.db.Where("id = ?", inst.DefinitionID).First(&def).Error; err != nil {
		return nil, err
	}
	return flow.Parse(def.NodeTree)
}

// ---------- 抄送（M2） ----------

// CcRow 抄送箱列表行（契约字段勿改：与前端 /bpm/cc/my 对齐）。
type CcRow struct {
	ID            uint64     `json:"id"`
	InstanceID    uint64     `json:"instance_id"`
	InstanceTitle string     `json:"instance_title"`
	NodeName      string     `json:"node_name"`
	InitiatorID   uint64     `json:"initiator_id"`
	ReadAt        *time.Time `json:"read_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ListMyCc 抄送我的列表（按抄送时间倒序；unreadOnly 只看未读）。
func (s *Store) ListMyCc(tenantID, me uint64, unreadOnly bool, p Page) ([]CcRow, int64, error) {
	q := s.db.Table("bpm_cc_record").
		Select(`bpm_cc_record.id, bpm_cc_record.instance_id, bpm_cc_record.node_name,
			bpm_cc_record.read_at, bpm_cc_record.created_at,
			i.title AS instance_title, i.initiator_id`).
		Joins("JOIN bpm_process_instance i ON i.id = bpm_cc_record.instance_id").
		Where("bpm_cc_record.tenant_id = ? AND bpm_cc_record.user_id = ?", tenantID, me)
	if unreadOnly {
		q = q.Where("bpm_cc_record.read_at IS NULL")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := p.clamp()
	var list []CcRow
	err := q.Order("bpm_cc_record.id DESC").Offset(offset).Limit(limit).Scan(&list).Error
	return list, total, err
}

var ErrNotCcOwner = errors.New("仅本人可标记该抄送已读")

// MarkCcRead 标记抄送已读（仅本人；幂等——已读重复调用直接成功）。
func (s *Store) MarkCcRead(id, tenantID, me uint64) error {
	var rec model.CcRecord
	if err := tenantQ(s.db, tenantID).Where("id = ?", id).First(&rec).Error; err != nil {
		return err
	}
	if rec.UserID != me {
		return ErrNotCcOwner
	}
	if rec.ReadAt != nil {
		return nil // 幂等
	}
	return s.db.Model(&model.CcRecord{}).
		Where("id = ? AND read_at IS NULL", id).
		Update("read_at", time.Now()).Error
}

// ---------- 超时提醒扫描（M2 ticker） ----------

// TimeoutDueRow 到点未提醒的待办（附实例摘要，发 bpm.task_timeout 用）。
type TimeoutDueRow struct {
	ID            uint64
	TenantID      uint64
	InstanceID    uint64
	NodeID        string
	NodeName      string
	AssigneeID    uint64
	TimeoutAt     time.Time
	CreatedAt     time.Time
	InstanceTitle string
}

// ListTimeoutDue 扫描 pending 且 timeout_at 已到、尚未提醒过的任务
//（跨租户系统扫描；命中 ix_bpm_task_timeout 部分索引）。
func (s *Store) ListTimeoutDue(limit int) ([]TimeoutDueRow, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []TimeoutDueRow
	err := s.db.Table("bpm_task").
		Select(`bpm_task.id, bpm_task.tenant_id, bpm_task.instance_id, bpm_task.node_id,
			bpm_task.node_name, bpm_task.assignee_id, bpm_task.timeout_at, bpm_task.created_at,
			i.title AS instance_title`).
		Joins("JOIN bpm_process_instance i ON i.id = bpm_task.instance_id").
		Where("bpm_task.status = ? AND bpm_task.timeout_at IS NOT NULL AND bpm_task.timeout_at <= ? AND bpm_task.reminded_at IS NULL",
			model.TaskPending, time.Now()).
		Where("i.status = ?", model.InstRunning).
		Order("bpm_task.timeout_at ASC").Limit(limit).Scan(&list).Error
	return list, err
}

// MarkTaskReminded 回填 reminded_at 并写 timeout_remind 日志（operator=0）。
// 条件更新防重：并发/重复扫描下仅第一次返回 true，保证"只提醒一次"。
func (s *Store) MarkTaskReminded(row TimeoutDueRow, hours int) (bool, error) {
	reminded := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Task{}).
			Where("id = ? AND status = ? AND reminded_at IS NULL", row.ID, model.TaskPending).
			Update("reminded_at", time.Now())
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // 已被提醒/已处理
		}
		reminded = true
		detail, _ := json.Marshal(map[string]any{
			"assignee_id": row.AssigneeID, "hours": hours,
		})
		// 日志失败不阻断（与引擎 writeLog 同理念）
		_ = tx.Create(&model.ProcessLog{
			TenantID: row.TenantID, InstanceID: row.InstanceID, NodeID: row.NodeID,
			TaskID: row.ID, Action: model.ActionTimeoutRemind, OperatorID: 0,
			Detail: model.JSONB(detail),
		}).Error
		return nil
	})
	return reminded, err
}

// ListUserTaskNodeIDs 查看者在实例内出过任务的节点（去重）；实例详情按
// 这些节点的 fieldPerms 并集过滤隐藏字段（表单构建器 M1）。
func (s *Store) ListUserTaskNodeIDs(instanceID, tenantID, userID uint64) []string {
	var ids []string
	_ = s.db.Model(&model.Task{}).
		Where("instance_id = ? AND tenant_id = ? AND (assignee_id = ? OR origin_assignee = ?)",
			instanceID, tenantID, userID, userID).
		Distinct().Pluck("node_id", &ids).Error
	return ids
}

// HasPrevApprovalTask 实例是否存在当前节点之外的历史审批任务（排除 start
// 重提任务）；taskActions 的 return_prev 可用性探测（M3，执行路径口径）。
func (s *Store) HasPrevApprovalTask(instanceID, tenantID uint64, currentNodeID, startNodeID string) bool {
	var cnt int64
	s.db.Model(&model.Task{}).
		Where("instance_id = ? AND tenant_id = ? AND node_id NOT IN ?",
			instanceID, tenantID, []string{currentNodeID, startNodeID}).
		Count(&cnt)
	return cnt > 0
}

// ---------- 审批统计（收官项，管理视图） ----------

// StatsTrendItem 单日发起数。
type StatsTrendItem struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int64  `json:"count"`
}

// DefStatsItem 按定义聚合：发起量 / 通过率 / 平均时长。
type DefStatsItem struct {
	DefinitionKey string  `json:"definition_key"`
	Name          string  `json:"name"`
	Total         int64   `json:"total"`
	Approved      int64   `json:"approved"`
	Rejected      int64   `json:"rejected"`
	Running       int64   `json:"running"`
	AvgHours      float64 `json:"avg_hours"` // 已结束实例平均时长（小时）
}

// NodeStatsItem 节点处理时长（瓶颈定位）。
type NodeStatsItem struct {
	NodeName string  `json:"node_name"`
	Acted    int64   `json:"acted"`
	AvgHours float64 `json:"avg_hours"`
}

// BpmStats 审批统计总览。
type BpmStats struct {
	StatusCounts    map[string]int64 `json:"status_counts"`
	Trend           []StatsTrendItem `json:"trend"`
	Definitions     []DefStatsItem   `json:"definitions"`
	NodeBottlenecks []NodeStatsItem  `json:"node_bottlenecks"`
}

// Stats 聚合审批统计：状态分布 / 近 30 天发起趋势 / 按定义通过率与均时长 /
// 节点瓶颈。为保 sqlite 测试可移植，日期分桶与时长均在 Go 端计算，取数
// 均有上限（趋势 5000 / 时长样本 1000/2000），大库下是近似值而非全量。
func (s *Store) Stats(tenantID uint64) (*BpmStats, error) {
	out := &BpmStats{StatusCounts: map[string]int64{}}

	// 状态分布
	type statusRow struct {
		Status string
		Cnt    int64
	}
	var statusRows []statusRow
	if err := tenantQ(s.db.Model(&model.ProcessInstance{}), tenantID).
		Select("status, COUNT(*) AS cnt").Group("status").Scan(&statusRows).Error; err != nil {
		return nil, err
	}
	for _, r := range statusRows {
		out.StatusCounts[r.Status] = r.Cnt
	}

	// 近 30 天发起趋势（Go 端按日分桶）
	since := time.Now().AddDate(0, 0, -29)
	dayStart := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location())
	var createdAts []time.Time
	if err := tenantQ(s.db.Model(&model.ProcessInstance{}), tenantID).
		Where("created_at >= ?", dayStart).Limit(5000).
		Pluck("created_at", &createdAts).Error; err != nil {
		return nil, err
	}
	byDay := map[string]int64{}
	for _, ts := range createdAts {
		byDay[ts.Format("2006-01-02")]++
	}
	for d := 0; d < 30; d++ {
		day := dayStart.AddDate(0, 0, d).Format("2006-01-02")
		out.Trend = append(out.Trend, StatsTrendItem{Date: day, Count: byDay[day]})
	}

	// 按定义聚合：状态计数（SQL 分组）+ 平均时长（最近 1000 条已结束，Go 端算）
	type defRow struct {
		DefinitionKey string
		Status        string
		Cnt           int64
	}
	var defRows []defRow
	if err := tenantQ(s.db.Model(&model.ProcessInstance{}), tenantID).
		Select("definition_key, status, COUNT(*) AS cnt").
		Group("definition_key").Group("status").Scan(&defRows).Error; err != nil {
		return nil, err
	}
	defItems := map[string]*DefStatsItem{}
	for _, r := range defRows {
		item := defItems[r.DefinitionKey]
		if item == nil {
			item = &DefStatsItem{DefinitionKey: r.DefinitionKey}
			defItems[r.DefinitionKey] = item
		}
		item.Total += r.Cnt
		switch r.Status {
		case model.InstApproved:
			item.Approved += r.Cnt
		case model.InstRejected:
			item.Rejected += r.Cnt
		case model.InstRunning, model.InstSuspended:
			item.Running += r.Cnt
		}
	}
	type durRow struct {
		DefinitionKey string
		CreatedAt     time.Time
		FinishedAt    *time.Time
	}
	var durRows []durRow
	if err := tenantQ(s.db.Model(&model.ProcessInstance{}), tenantID).
		Where("finished_at IS NOT NULL").
		Select("definition_key, created_at, finished_at").
		Order("id DESC").Limit(1000).Scan(&durRows).Error; err != nil {
		return nil, err
	}
	durSum := map[string]float64{}
	durCnt := map[string]int64{}
	for _, r := range durRows {
		if r.FinishedAt == nil {
			continue
		}
		durSum[r.DefinitionKey] += r.FinishedAt.Sub(r.CreatedAt).Hours()
		durCnt[r.DefinitionKey]++
	}
	// 定义名映射（每 key 最新版本名）
	var defs []model.ProcessDefinition
	_ = tenantQ(s.db, tenantID).Order("id ASC").Limit(500).
		Select("key, name").Find(&defs).Error
	nameByKey := map[string]string{}
	for _, d := range defs {
		nameByKey[d.Key] = d.Name // 后写覆盖 → 留下最新版本名
	}
	for key, item := range defItems {
		if durCnt[key] > 0 {
			item.AvgHours = round1(durSum[key] / float64(durCnt[key]))
		}
		item.Name = nameByKey[key]
		out.Definitions = append(out.Definitions, *item)
	}
	sort.Slice(out.Definitions, func(i, j int) bool {
		return out.Definitions[i].Total > out.Definitions[j].Total
	})

	// 节点瓶颈：最近 2000 条已处理任务的平均等待时长（创建→处理）
	type taskRow struct {
		NodeName  string
		CreatedAt time.Time
		ActedAt   *time.Time
	}
	var taskRows []taskRow
	if err := tenantQ(s.db.Model(&model.Task{}), tenantID).
		Where("acted_at IS NOT NULL").
		Select("node_name, created_at, acted_at").
		Order("id DESC").Limit(2000).Scan(&taskRows).Error; err != nil {
		return nil, err
	}
	nodeSum := map[string]float64{}
	nodeCnt := map[string]int64{}
	for _, r := range taskRows {
		if r.ActedAt == nil {
			continue
		}
		nodeSum[r.NodeName] += r.ActedAt.Sub(r.CreatedAt).Hours()
		nodeCnt[r.NodeName]++
	}
	for name, cnt := range nodeCnt {
		out.NodeBottlenecks = append(out.NodeBottlenecks, NodeStatsItem{
			NodeName: name, Acted: cnt, AvgHours: round1(nodeSum[name] / float64(cnt)),
		})
	}
	sort.Slice(out.NodeBottlenecks, func(i, j int) bool {
		return out.NodeBottlenecks[i].AvgHours > out.NodeBottlenecks[j].AvgHours
	})
	if len(out.NodeBottlenecks) > 10 {
		out.NodeBottlenecks = out.NodeBottlenecks[:10]
	}
	return out, nil
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// CanView 实例可见性（M1 从简）：发起人 ∪ 任务参与者（含转办转出人，M2）
// ∪ 被抄送人。平台管理员放行由 handler 层判断（X-Auth-Platform-Admin）。
func (s *Store) CanView(inst *model.ProcessInstance, userID uint64) bool {
	if inst.InitiatorID == userID {
		return true
	}
	var cnt int64
	s.db.Model(&model.Task{}).
		Where("instance_id = ? AND (assignee_id = ? OR origin_assignee = ?)",
			inst.ID, userID, userID).Count(&cnt)
	if cnt > 0 {
		return true
	}
	s.db.Model(&model.CcRecord{}).
		Where("instance_id = ? AND user_id = ?", inst.ID, userID).Count(&cnt)
	return cnt > 0
}
