// Package flow 定义流程节点树 JSON Schema（与设计文档 §2.2 的 TypeScript
// interface 等价的 Go 承载）与发布时校验。
//
// M1 支持范围（发布校验强制）：
//   - 节点类型：start / approval / cc（condition 条件分支留 M3）
//   - 多人模式：AND（会签）/ OR（或签）（SEQ 依次留 M3）
//   - 拒绝走向：reject（back_to_start 退回发起人留 M2）
//   - 审批人规则：users / roles / self_select（dept_leader 部门主管留 M2）
//
// 结构上保留 condition / SEQ / dept_leader 等字段位，保证 JSON 前后兼容，
// 仅在发布校验时拒绝，后续里程碑放开无需改存储。
package flow

import (
	"encoding/json"
	"errors"
	"fmt"
)

// 节点类型。
const (
	TypeStart     = "start"
	TypeApproval  = "approval"
	TypeCc        = "cc"
	TypeCondition = "condition"
)

// 多人模式。
const (
	MultiAnd = "AND"
	MultiOr  = "OR"
	MultiSeq = "SEQ"
)

// 审批人规则类型。
const (
	RuleUsers      = "users"
	RuleRoles      = "roles"
	RuleDeptLeader = "dept_leader"
	RuleSelfSelect = "self_select"
)

// 空候选人兜底策略。
const (
	FallbackAutoPass = "auto_pass"
	FallbackToUsers  = "to_users"
	FallbackSuspend  = "suspend" // 缺省
)

// MaxNodes 链上节点数上限（防御异常定义）。
const MaxNodes = 200

// Schema 一条流程定义的节点树（definition.node_tree 的内容）。
type Schema struct {
	Version int   `json:"version"`
	Start   *Node `json:"start"`
}

// Node 节点：单链 + 条件分支的统一承载（按 Type 判别有效字段）。
type Node struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Next *Node  `json:"next,omitempty"`

	// --- start ---
	FormFields []string `json:"formFields,omitempty"`

	// --- approval ---
	Assignee      *AssigneeRule `json:"assignee,omitempty"`
	MultiMode     string        `json:"multiMode,omitempty"` // AND|OR|SEQ，缺省按 OR
	OnReject      string        `json:"onReject,omitempty"`  // reject|back_to_start，缺省按 reject
	TimeoutHours  int           `json:"timeoutHours,omitempty"`
	AllowBackPrev bool          `json:"allowBackPrev,omitempty"`

	// --- cc ---
	Targets *AssigneeRule `json:"targets,omitempty"`

	// --- condition（M3 启用，字段位保留）---
	Branches []Branch `json:"branches,omitempty"`
}

// Branch 条件分支（M3 启用）。
type Branch struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Expr json.RawMessage `json:"expr,omitempty"` // null=default 兜底分支
	Next *Node           `json:"next,omitempty"`
}

// AssigneeRule 审批人 / 抄送对象解析规则。
type AssigneeRule struct {
	Type            string   `json:"type"`
	UserIDs         []uint64 `json:"userIds,omitempty"`
	RoleIDs         []uint64 `json:"roleIds,omitempty"`
	DeptLeaderBase  string   `json:"deptLeaderBase,omitempty"` // M2
	DeptFormField   string   `json:"deptFormField,omitempty"`  // M2
	EmptyFallback   string   `json:"emptyFallback,omitempty"`  // 缺省 suspend
	FallbackUserIDs []uint64 `json:"fallbackUserIds,omitempty"`
}

// Parse 解析 node_tree JSON；仅要求结构可解析（草稿可存半成品），
// 完整业务校验见 Validate（发布时强制）。
func Parse(raw []byte) (*Schema, error) {
	if len(raw) == 0 {
		return nil, errors.New("节点树不能为空")
	}
	var s Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("节点树 JSON 解析失败: %w", err)
	}
	return &s, nil
}

// Nodes 返回主链节点切片（含 start，按链序）。M1 无分支，链即全部。
func Nodes(s *Schema) []*Node {
	var out []*Node
	for n := s.Start; n != nil && len(out) <= MaxNodes; n = n.Next {
		out = append(out, n)
	}
	return out
}

// NodeByID 按 id 在主链上定位节点；未找到返回 nil。
func NodeByID(s *Schema, id string) *Node {
	for _, n := range Nodes(s) {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// EffectiveMultiMode 节点生效的多人模式（缺省 OR）。
func (n *Node) EffectiveMultiMode() string {
	if n.MultiMode == "" {
		return MultiOr
	}
	return n.MultiMode
}

// EffectiveFallback 规则生效的空候选人兜底（缺省 suspend）。
func (r *AssigneeRule) EffectiveFallback() string {
	if r == nil || r.EmptyFallback == "" {
		return FallbackSuspend
	}
	return r.EmptyFallback
}

// Validate 发布时的整树校验（M1 约束集）。校验失败返回可读中文错误。
func Validate(s *Schema) error {
	if s == nil || s.Start == nil {
		return errors.New("缺少发起节点")
	}
	if s.Start.Type != TypeStart {
		return errors.New("链头必须是发起节点（type=start）")
	}
	seen := map[string]bool{}
	approvals := 0
	count := 0
	for n := s.Start; n != nil; n = n.Next {
		count++
		if count > MaxNodes {
			return fmt.Errorf("节点数超过上限 %d", MaxNodes)
		}
		if n.ID == "" {
			return errors.New("存在缺少 id 的节点")
		}
		if seen[n.ID] {
			return fmt.Errorf("节点 id 重复: %s", n.ID)
		}
		seen[n.ID] = true
		if n.Name == "" {
			return fmt.Errorf("节点 %s 缺少名称", n.ID)
		}
		switch n.Type {
		case TypeStart:
			if n != s.Start {
				return fmt.Errorf("发起节点只能出现在链头（节点 %s）", n.ID)
			}
		case TypeApproval:
			approvals++
			if err := validateApproval(n); err != nil {
				return err
			}
		case TypeCc:
			if n.Targets == nil {
				return fmt.Errorf("抄送节点 %s 缺少抄送对象", n.Name)
			}
			if n.Targets.Type == RuleSelfSelect {
				return fmt.Errorf("抄送节点 %s 不支持发起人自选规则", n.Name)
			}
			if err := validateRule(n.Name, n.Targets); err != nil {
				return err
			}
		case TypeCondition:
			return fmt.Errorf("条件分支节点（%s）将在 M3 支持", n.Name)
		default:
			return fmt.Errorf("节点 %s 类型未知: %s", n.ID, n.Type)
		}
	}
	if approvals == 0 {
		return errors.New("流程至少需要一个审批节点")
	}
	return nil
}

func validateApproval(n *Node) error {
	switch n.MultiMode {
	case "", MultiAnd, MultiOr:
	case MultiSeq:
		return fmt.Errorf("审批节点 %s：依次审批（SEQ）将在 M3 支持", n.Name)
	default:
		return fmt.Errorf("审批节点 %s：多人模式未知: %s", n.Name, n.MultiMode)
	}
	switch n.OnReject {
	case "", "reject":
	case "back_to_start":
		return fmt.Errorf("审批节点 %s：拒绝退回发起人（back_to_start）将在 M2 支持", n.Name)
	default:
		return fmt.Errorf("审批节点 %s：拒绝走向未知: %s", n.Name, n.OnReject)
	}
	if n.TimeoutHours < 0 {
		return fmt.Errorf("审批节点 %s：超时小时数不能为负", n.Name)
	}
	if n.Assignee == nil {
		return fmt.Errorf("审批节点 %s 缺少审批人规则", n.Name)
	}
	return validateRule(n.Name, n.Assignee)
}

func validateRule(nodeName string, r *AssigneeRule) error {
	switch r.Type {
	case RuleUsers:
		if len(r.UserIDs) == 0 {
			return fmt.Errorf("节点 %s：指定用户规则至少需要一个用户", nodeName)
		}
	case RuleRoles:
		if len(r.RoleIDs) == 0 {
			return fmt.Errorf("节点 %s：指定角色规则至少需要一个角色", nodeName)
		}
	case RuleSelfSelect:
		// 发起时校验 variables.selected_assignees 提供选人（引擎侧）
	case RuleDeptLeader:
		return fmt.Errorf("节点 %s：部门主管规则将在 M2 支持", nodeName)
	default:
		return fmt.Errorf("节点 %s：审批人规则类型未知: %s", nodeName, r.Type)
	}
	switch r.EmptyFallback {
	case "", FallbackAutoPass, FallbackSuspend:
	case FallbackToUsers:
		if len(r.FallbackUserIDs) == 0 {
			return fmt.Errorf("节点 %s：兜底转指定人至少需要一个用户", nodeName)
		}
	default:
		return fmt.Errorf("节点 %s：空候选人兜底策略未知: %s", nodeName, r.EmptyFallback)
	}
	return nil
}
