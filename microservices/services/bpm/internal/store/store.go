// Package store 持久化 bpm 五张表。AutoMigrate 自管表，
// 全部查询强制 tenant_id 隔离；防重的部分唯一索引在建表后补建
// （AutoMigrate 不支持 WHERE 索引）。引擎推进事务见 internal/engine。
package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/bpm/internal/flow"
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
	if err := flow.Validate(sc); err != nil {
		return nil, err
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

// ListDone 我的已办（approved/rejected 历史，按处理时间倒序）。
func (s *Store) ListDone(tenantID, me uint64, p Page) ([]TaskView, int64, error) {
	q := s.taskViewQ(tenantID).
		Where("bpm_task.assignee_id = ? AND bpm_task.status IN ?",
			me, []string{model.TaskApproved, model.TaskRejected})
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

// CanView 实例可见性（M1 从简）：发起人 ∪ 任务参与者 ∪ 被抄送人。
// 平台管理员放行由 handler 层判断（X-Auth-Platform-Admin）。
func (s *Store) CanView(inst *model.ProcessInstance, userID uint64) bool {
	if inst.InitiatorID == userID {
		return true
	}
	var cnt int64
	s.db.Model(&model.Task{}).
		Where("instance_id = ? AND assignee_id = ?", inst.ID, userID).Count(&cnt)
	if cnt > 0 {
		return true
	}
	s.db.Model(&model.CcRecord{}).
		Where("instance_id = ? AND user_id = ?", inst.ID, userID).Count(&cnt)
	return cnt > 0
}
