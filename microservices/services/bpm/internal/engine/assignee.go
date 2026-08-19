package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-admin-kit/services/bpm/internal/flow"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"gorm.io/gorm"
	"strconv"
	"strings"
)

// resolveRule 通过 identity owner API 解析角色或部门主管候选人，
// 并保持禁用用户及租户边界校验。
func (e *Engine) resolveRule(tx *gorm.DB, inst *model.ProcessInstance, rule *flow.AssigneeRule, nodeID string) ([]uint64, error) {
	if rule == nil {
		return nil, errors.New("节点缺少人员规则")
	}
	switch rule.Type {
	case flow.RuleUsers:
		return rule.UserIDs, nil
	case flow.RuleRoles:
		// Phase 2C：按角色解析审批人改走 identity owner API，不再直查 users/user_roles。
		return e.usersByRoles(context.Background(), rule.RoleIDs, inst.TenantID), nil
	case flow.RuleSelfSelect:
		vars := parseVars(inst.Variables)
		return vars.SelectedAssignees[nodeID], nil
	case flow.RuleDeptLeader:
		// M2：部门主管。基准部门二选一（缺省发起人部门），主管通过
		// identity owner API 解析；解析为空走节点 emptyFallback 三策略。
		var deptID uint64
		switch rule.DeptLeaderBase {
		case "", flow.DeptBaseInitiator:
			deptID = inst.InitiatorDept
			if deptID == 0 { // 发起时未落部门（历史实例等），补查一次
				deptID = e.lookupUserDept(inst.TenantID, inst.InitiatorID)
			}
		case flow.DeptBaseFormField:
			deptID = snapshotUint64(inst.FormSnapshot, rule.DeptFormField)
		default:
			return nil, fmt.Errorf("部门主管基准未知: %s", rule.DeptLeaderBase)
		}
		if deptID == 0 {
			return nil, nil
		}
		// Phase 2C：部门主管解析/校验改走 identity owner API，不再直查 departments/users。
		leaderID := e.deptLeader(context.Background(), deptID, inst.TenantID)
		if leaderID == 0 || !e.userInTenantActive(context.Background(), leaderID, inst.TenantID) {
			return nil, nil // 无主管/主管禁用或跨租户 → 解析为空（与 roles 规则同口径）
		}
		return []uint64{leaderID}, nil
	default:
		return nil, fmt.Errorf("审批人规则 %s 未支持", rule.Type)
	}
}

// lookupUserDept 通过 identity owner API 返回用户部门；查询失败时
// 静默返回 0，由 emptyFallback 兜底。
func (e *Engine) lookupUserDept(tenantID, userID uint64) uint64 {
	return e.userDepartment(context.Background(), userID, tenantID)
}

// snapshotUint64 从表单快照 JSON 中取 uint64 字段（数字或数字字符串）。
func snapshotUint64(raw []byte, field string) uint64 {
	if len(raw) == 0 || field == "" {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	switch v := m[field].(type) {
	case float64:
		if v > 0 {
			return uint64(v)
		}
	case string:
		if n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return 0
}
