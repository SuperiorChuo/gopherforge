// Package flow 定义流程节点树 JSON Schema（与设计文档 §2.2 的 TypeScript
// interface 等价的 Go 承载）与发布时校验。
//
// M3 支持范围（发布校验强制）：
//   - 节点类型：start / approval / cc / condition（排他条件分支，允许嵌套）
//   - 多人模式：AND（会签）/ OR（或签）/ SEQ（依次）
//   - 拒绝走向：reject / back_to_start
//   - 审批人规则：users / roles / self_select / dept_leader
//
// 结构模型：单链 + 条件分支。condition 节点的每个分支挂一条子链，
// 子链走完自动"汇合"回 condition 的 next（执行后继统一由 Successor 计算）。
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

// dept_leader 规则的部门基准（M2）。
const (
	DeptBaseInitiator = "initiator"  // 缺省：以发起人所在部门取主管
	DeptBaseFormField = "form_field" // 从发起表单快照的指定字段取部门 id
)

// 拒绝走向。
const (
	OnRejectReject      = "reject"        // 缺省：节点拒绝 → 实例终态 rejected
	OnRejectBackToStart = "back_to_start" // M2：节点拒绝 → 退回发起人重新提交
)

// MaxNodes 整树节点数上限（含分支子链，防御异常定义）。
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
	// FieldPerms 表单字段权限（M1 仅 "hidden"；键须在定义的 form_schema 内，
	// 发布时经 form.ValidateFieldPerms 校验）——该节点任务详情按此过滤快照
	FieldPerms map[string]string `json:"fieldPerms,omitempty"`

	// --- cc ---
	Targets *AssigneeRule `json:"targets,omitempty"`

	// --- condition（M3 启用）---
	Branches []Branch `json:"branches,omitempty"`
}

// Branch 条件分支（M3）：expr=null 为 default 兜底分支（数组末尾唯一一个）。
type Branch struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Expr json.RawMessage `json:"expr,omitempty"` // null=default 兜底分支
	Next *Node           `json:"next,omitempty"` // 分支子链头；空=直通汇合点
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

// Nodes 返回整树节点切片（含分支子链，深度优先：主链序，condition 节点
// 之后紧跟其各分支子链）。总量受 MaxNodes 截断保护。
func Nodes(s *Schema) []*Node {
	var out []*Node
	Walk(s, func(n *Node) bool {
		out = append(out, n)
		return len(out) <= MaxNodes
	})
	return out
}

// Walk 深度优先遍历整树（主链 + 条件分支子链）；fn 返回 false 时终止。
func Walk(s *Schema, fn func(n *Node) bool) {
	if s == nil {
		return
	}
	walkChain(s.Start, fn, 0)
}

// walkChain 遍历一条链；depth 防御异常嵌套（JSON 树无环，纯保险）。
func walkChain(head *Node, fn func(n *Node) bool, depth int) bool {
	if depth > 32 {
		return false
	}
	for n := head; n != nil; n = n.Next {
		if !fn(n) {
			return false
		}
		if n.Type == TypeCondition {
			for i := range n.Branches {
				if !walkChain(n.Branches[i].Next, fn, depth+1) {
					return false
				}
			}
		}
	}
	return true
}

// NodeByID 在整树上定位节点（含分支子链）；未找到返回 nil。
func NodeByID(s *Schema, id string) *Node {
	var hit *Node
	Walk(s, func(n *Node) bool {
		if n.ID == id {
			hit = n
			return false
		}
		return true
	})
	return hit
}

// Successor 返回节点在整树上的"执行后继"：分支子链走完自动汇合回所属
// condition 的 next（该 next 为空则继续向外层汇合）。节点不存在返回 nil；
// 返回 nil 亦表示到达链尾（实例应终态 approved）。
func Successor(s *Schema, id string) *Node {
	if s == nil {
		return nil
	}
	n, _ := succIn(s.Start, id, nil)
	return n
}

// succIn 在 chain 中找 id 的后继；joins 为由内到外的汇合目标栈
//（元素可为 nil，表示该层 condition 之后直接续接更外层的汇合点）。
func succIn(chain *Node, id string, joins []*Node) (*Node, bool) {
	for n := chain; n != nil; n = n.Next {
		if n.ID == id {
			if n.Next != nil {
				return n.Next, true
			}
			return firstJoin(joins), true
		}
		if n.Type == TypeCondition {
			inner := append([]*Node{n.Next}, joins...)
			for i := range n.Branches {
				if got, found := succIn(n.Branches[i].Next, id, inner); found {
					return got, true
				}
			}
		}
	}
	return nil, false
}

func firstJoin(joins []*Node) *Node {
	for _, j := range joins {
		if j != nil {
			return j
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

// Validate 发布时的整树校验（M3 约束集）。校验失败返回可读中文错误。
func Validate(s *Schema) error {
	if s == nil || s.Start == nil {
		return errors.New("缺少发起节点")
	}
	if s.Start.Type != TypeStart {
		return errors.New("链头必须是发起节点（type=start）")
	}
	v := &validator{
		seen:       map[string]bool{},
		formFields: s.Start.FormFields,
	}
	if err := v.chain(s.Start, true); err != nil {
		return err
	}
	if v.approvals == 0 {
		return errors.New("流程至少需要一个审批节点")
	}
	return nil
}

type validator struct {
	seen       map[string]bool
	formFields []string
	approvals  int
	count      int
}

// chain 校验一条链；topLevel 仅主链为 true（start 只允许出现在主链头）。
func (v *validator) chain(head *Node, topLevel bool) error {
	for n := head; n != nil; n = n.Next {
		v.count++
		if v.count > MaxNodes {
			return fmt.Errorf("节点数超过上限 %d", MaxNodes)
		}
		if n.ID == "" {
			return errors.New("存在缺少 id 的节点")
		}
		if v.seen[n.ID] {
			return fmt.Errorf("节点 id 重复: %s", n.ID)
		}
		v.seen[n.ID] = true
		if n.Name == "" {
			return fmt.Errorf("节点 %s 缺少名称", n.ID)
		}
		switch n.Type {
		case TypeStart:
			if !topLevel || n != head || v.count != 1 {
				return fmt.Errorf("发起节点只能出现在链头（节点 %s）", n.ID)
			}
		case TypeApproval:
			v.approvals++
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
			if err := v.condition(n); err != nil {
				return err
			}
		default:
			return fmt.Errorf("节点 %s 类型未知: %s", n.ID, n.Type)
		}
	}
	return nil
}

// condition 校验条件分支节点：分支 ≥2、default 有且仅有一个且在末尾、
// 表达式字段已声明、分支子链递归校验。
func (v *validator) condition(n *Node) error {
	if len(n.Branches) < 2 {
		return fmt.Errorf("条件分支节点 %s 至少需要 2 个分支", n.Name)
	}
	defaults := 0
	branchSeen := map[string]bool{}
	for i := range n.Branches {
		b := &n.Branches[i]
		if b.ID == "" {
			return fmt.Errorf("条件分支节点 %s 存在缺少 id 的分支", n.Name)
		}
		if branchSeen[b.ID] {
			return fmt.Errorf("条件分支节点 %s 分支 id 重复: %s", n.Name, b.ID)
		}
		branchSeen[b.ID] = true
		if b.Name == "" {
			return fmt.Errorf("条件分支节点 %s 存在缺少名称的分支", n.Name)
		}
		expr, err := ParseExpr(b.Expr)
		if err != nil {
			return fmt.Errorf("分支「%s」: %w", b.Name, err)
		}
		if expr == nil {
			defaults++
			if i != len(n.Branches)-1 {
				return fmt.Errorf("条件分支节点 %s 的默认分支「%s」必须位于末尾", n.Name, b.Name)
			}
		} else {
			if err := ValidateExpr(expr, v.formFields); err != nil {
				return fmt.Errorf("分支「%s」: %w", b.Name, err)
			}
		}
		if err := v.chain(b.Next, false); err != nil {
			return err
		}
	}
	if defaults != 1 {
		return fmt.Errorf("条件分支节点 %s 必须有且仅有一个默认（兜底）分支", n.Name)
	}
	return nil
}

func validateApproval(n *Node) error {
	switch n.MultiMode {
	case "", MultiAnd, MultiOr, MultiSeq:
	default:
		return fmt.Errorf("审批节点 %s：多人模式未知: %s", n.Name, n.MultiMode)
	}
	switch n.OnReject {
	case "", OnRejectReject, OnRejectBackToStart:
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
		switch r.DeptLeaderBase {
		case "", DeptBaseInitiator:
			// 缺省以发起人部门为基准
		case DeptBaseFormField:
			if r.DeptFormField == "" {
				return fmt.Errorf("节点 %s：部门主管规则按表单字段取部门时需指定字段名（deptFormField）", nodeName)
			}
		default:
			return fmt.Errorf("节点 %s：部门主管基准未知: %s", nodeName, r.DeptLeaderBase)
		}
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
