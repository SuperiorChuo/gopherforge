package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

type StartInput struct {
	TenantID      uint64
	DefinitionKey string
	Title         string
	BizType       string
	BizID         string
	FormSnapshot  []byte
	Variables     []byte
	InitiatorID   uint64
	InitiatorDept uint64
	// IdempotencyKey is generated once by the logical caller and reused across
	// gRPC and HTTP fallback attempts. Empty keeps the legacy non-idempotent
	// behavior for generic/user-facing starts.
	IdempotencyKey string
}

const maxIdempotencyKeyLength = 128

func normalizeIdempotencyKey(key string) string {
	return strings.TrimSpace(key)
}

func startRequestHash(in StartInput) string {
	payload := struct {
		TenantID      uint64 `json:"tenant_id"`
		DefinitionKey string `json:"definition_key"`
		Title         string `json:"title"`
		BizType       string `json:"biz_type"`
		BizID         string `json:"biz_id"`
		FormSnapshot  []byte `json:"form_snapshot"`
		Variables     []byte `json:"variables"`
		InitiatorID   uint64 `json:"initiator_id"`
		InitiatorDept uint64 `json:"initiator_dept"`
	}{
		TenantID: in.TenantID, DefinitionKey: strings.TrimSpace(in.DefinitionKey),
		Title: strings.TrimSpace(in.Title), BizType: strings.TrimSpace(in.BizType),
		BizID: strings.TrimSpace(in.BizID), FormSnapshot: in.FormSnapshot,
		Variables: in.Variables, InitiatorID: in.InitiatorID, InitiatorDept: in.InitiatorDept,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// loadOrReserveIdempotency inserts the key inside the same transaction as the
// instance. ON CONFLICT waits for a concurrent transaction on PostgreSQL; the
// losing caller then reads the committed result and returns it without replaying
// engine effects.
func loadOrReserveIdempotency(tx *gorm.DB, tenantID uint64, key, requestHash string) (*model.IdempotencyRecord, bool, error) {
	if key == "" {
		return nil, false, nil
	}
	record := &model.IdempotencyRecord{
		TenantID: tenantID, Key: key, RequestHash: requestHash,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return record, true, nil
	}
	var existing model.IdempotencyRecord
	if err := tx.Where("tenant_id = ? AND key = ?", tenantID, key).
		Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing).Error; err != nil {
		return nil, false, err
	}
	if existing.RequestHash != requestHash {
		return nil, false, ErrIdempotencyKeyReuse
	}
	return &existing, false, nil
}

// Start 发起流程：冻结 active 定义版本 → 建实例 → 从发起节点推进。
func (e *Engine) Start(in StartInput) (*Effects, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, errors.New("标题不能为空")
	}
	if strings.TrimSpace(in.BizType) == "" || strings.TrimSpace(in.BizID) == "" {
		return nil, errors.New("biz_type / biz_id 不能为空")
	}
	if in.InitiatorID == 0 {
		return nil, errors.New("发起人不能为空")
	}
	key := normalizeIdempotencyKey(in.IdempotencyKey)
	if len(key) > maxIdempotencyKeyLength {
		return nil, fmt.Errorf("幂等键长度不能超过 %d", maxIdempotencyKeyLength)
	}
	in.IdempotencyKey = key
	var def model.ProcessDefinition
	err := e.db.Where("tenant_id = ? AND key = ? AND status = ?",
		in.TenantID, strings.TrimSpace(in.DefinitionKey), model.DefActive).
		First(&def).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoActiveDefinition
		}
		return nil, err
	}
	sc, err := flow.Parse(def.NodeTree)
	if err != nil {
		return nil, err
	}
	// 发起前置校验：所有 self_select 审批节点必须已提供选人（发起时报错
	// 比运行中挂起更友好）。
	vars := parseVars(in.Variables)
	for _, n := range flow.Nodes(sc) {
		if n.Type == flow.TypeApproval && n.Assignee != nil &&
			n.Assignee.Type == flow.RuleSelfSelect &&
			len(dedupe(vars.SelectedAssignees[n.ID])) == 0 {
			return nil, fmt.Errorf("节点「%s」需要发起人选择审批人（variables.selected_assignees[%q]）", n.Name, n.ID)
		}
	}

	// 发起人部门兜底：调用方未传时直查同库 users.department_id（与 roles
	// 规则同一条"同库直读 identity 表"路径；查不到保持 0，dept_leader 规则
	// 届时走 emptyFallback）。
	if in.InitiatorDept == 0 {
		in.InitiatorDept = e.lookupUserDept(in.TenantID, in.InitiatorID)
	}
	requestHash := startRequestHash(in)

	eff := &Effects{}
	err = e.db.Transaction(func(tx *gorm.DB) error {
		if key != "" {
			record, created, err := loadOrReserveIdempotency(tx, in.TenantID, key, requestHash)
			if err != nil {
				return err
			}
			if !created {
				if record.InstanceID == 0 {
					return ErrIdempotencyRecord
				}
				var existing model.ProcessInstance
				if err := tx.First(&existing, record.InstanceID).Error; err != nil {
					return fmt.Errorf("幂等记录对应的审批实例不存在: %w", err)
				}
				eff.Instance = &existing
				return nil
			}
		}
		// 应用层预查在途（可读报错）；部分唯一索引兜底并发窗口
		var cnt int64
		if err := tx.Model(&model.ProcessInstance{}).
			Where("tenant_id = ? AND biz_type = ? AND biz_id = ? AND status = ?",
				in.TenantID, in.BizType, in.BizID, model.InstRunning).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return ErrDuplicateRunning
		}
		form := in.FormSnapshot
		if len(form) == 0 {
			form = []byte("{}")
		}
		inst := &model.ProcessInstance{
			TenantID:      in.TenantID,
			DefinitionID:  def.ID,
			DefinitionKey: def.Key,
			Title:         title,
			BizType:       strings.TrimSpace(in.BizType),
			BizID:         strings.TrimSpace(in.BizID),
			Status:        model.InstRunning,
			CurrentNodeID: sc.Start.ID,
			FormSnapshot:  model.JSONB(form),
			Variables:     model.JSONB(in.Variables),
			InitiatorID:   in.InitiatorID,
			InitiatorDept: in.InitiatorDept,
		}
		if err := tx.Create(inst).Error; err != nil {
			return err
		}
		if key != "" {
			if err := tx.Model(&model.IdempotencyRecord{}).
				Where("tenant_id = ? AND key = ?", in.TenantID, key).
				Update("instance_id", inst.ID).Error; err != nil {
				return err
			}
		}
		eff.Instance = inst
		writeLog(tx, inst, sc.Start.ID, 0, model.ActionSubmit, in.InitiatorID,
			map[string]any{"title": title})
		return e.advanceFrom(tx, inst, sc, sc.Start, eff)
	})
	if err != nil {
		return nil, err
	}
	return eff, nil
}
