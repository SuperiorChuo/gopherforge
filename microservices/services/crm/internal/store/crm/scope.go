package store

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ---------- CRM 数据权限（data scope）----------
//
// 与 system 同库：读 user_roles → roles.data_scope 取用户最宽档；
// 部门档再解析 users.department_id + departments.parent_id 出可见部门树。
// 档位沿用系统角色定义：all > department_and_children > self。
// 公海（owner_user_id=0）不受档位限制，人人可见可领取。

type ScopeKind string

const (
	ScopeAll  ScopeKind = "all"
	ScopeDept ScopeKind = "department_and_children"
	ScopeSelf ScopeKind = "self"
)

// UserScope 一个用户在 CRM 里的可见范围。
type UserScope struct {
	Kind ScopeKind
	// Me 解析时的用户 ID（self 档过滤用）。
	Me uint64
	// VisibleUserIDs Dept 档时=部门树内所有成员 ID（含自己）；其他档为 nil。
	VisibleUserIDs []uint64
	// DeptIDs Dept 档时=可见部门树 ID（过滤名片部门池用）；其他档为 nil。
	DeptIDs []uint64
}

// scopeCache 5 分钟内存缓存：角色/部门变更最迟 5 分钟生效，避免每请求 3 次查询。
// key 含租户：同一 userID 换租户上下文不复用，避免跨租户串档。
type scopeKey struct {
	tenantID uint64
	userID   uint64
}

type scopeEntry struct {
	scope   UserScope
	expires time.Time
}

var (
	scopeMu    sync.Mutex
	scopeCache = map[scopeKey]scopeEntry{}
)

const scopeTTL = 5 * time.Minute

// ResolveScope 解析用户 data scope。platformAdmin（网关 X-Auth-Platform-Admin）
// 直接 all。查询失败时保守降级 self，不放大可见面。
func (s *Store) ResolveScope(ctx context.Context, tenantID, userID uint64, platformAdmin bool) UserScope {
	if platformAdmin {
		return UserScope{Kind: ScopeAll, Me: userID}
	}
	key := scopeKey{tenantID: tenantID, userID: userID}
	scopeMu.Lock()
	if e, ok := scopeCache[key]; ok && time.Now().Before(e.expires) {
		scopeMu.Unlock()
		return e.scope
	}
	scopeMu.Unlock()

	scope := s.resolveScopeUncached(ctx, tenantID, userID)
	scope.Me = userID

	scopeMu.Lock()
	scopeCache[key] = scopeEntry{scope: scope, expires: time.Now().Add(scopeTTL)}
	scopeMu.Unlock()
	return scope
}

// InvalidateScopeCache 清缓存（测试用）。
func InvalidateScopeCache() {
	scopeMu.Lock()
	scopeCache = map[scopeKey]scopeEntry{}
	scopeMu.Unlock()
}

func (s *Store) resolveScopeUncached(ctx context.Context, tenantID, userID uint64) UserScope {
	// Phase 2C：数据权限解析改走 identity owner API（roles/user_roles/departments 不再直查）。
	dataScope, deptID := s.userDataScope(ctx, userID, tenantID)
	switch dataScope {
	case "all":
		return UserScope{Kind: ScopeAll}
	case "dept", "department", "department_tree":
		// 部门档：identity 返回部门树内全部成员（含自身兜底）
		userIDs := s.deptMembers(ctx, deptID, tenantID)
		if deptID == 0 || len(userIDs) == 0 {
			return UserScope{Kind: ScopeSelf}
		}
		found := false
		for _, id := range userIDs {
			if id == userID {
				found = true
				break
			}
		}
		if !found {
			userIDs = append(userIDs, userID)
		}
		return UserScope{Kind: ScopeDept, VisibleUserIDs: userIDs, DeptIDs: []uint64{deptID}}
	default:
		return UserScope{Kind: ScopeSelf}
	}
}

// deptTree 迭代 BFS 展开子部门（departments 无递归 CTE 依赖，兼容 sqlite 测试）。
// CanTouchCustomer 写操作归属校验：all 任意；其余档要求是 owner 本人、
// 可见范围内成员的客户、或公海客户。
func (sc UserScope) CanTouchOwner(ownerUserID, me uint64) bool {
	if sc.Kind == ScopeAll || ownerUserID == 0 || ownerUserID == me {
		return true
	}
	if sc.Kind == ScopeDept {
		for _, id := range sc.VisibleUserIDs {
			if id == ownerUserID {
				return true
			}
		}
	}
	return false
}

// applyCustomerAccess 客户硬隔离：self=自己的+公海；dept=树内成员的+公海；
// all/零值=不限制。公海放行保证"人人可见可领取"语义。
func applyCustomerAccess(q *gorm.DB, a UserScope) *gorm.DB {
	switch a.Kind {
	case ScopeSelf:
		if a.Me > 0 {
			return q.Where("owner_user_id = ? OR owner_user_id = 0", a.Me)
		}
	case ScopeDept:
		if len(a.VisibleUserIDs) > 0 {
			return q.Where("owner_user_id IN ? OR owner_user_id = 0", a.VisibleUserIDs)
		}
	}
	return q
}

// applyLeadAccess 名片硬隔离：额外放行未分配（owner=0 dept=0）与可见部门池。
func applyLeadAccess(q *gorm.DB, a UserScope) *gorm.DB {
	switch a.Kind {
	case ScopeSelf:
		if a.Me > 0 {
			return q.Where("owner_user_id = ? OR owner_user_id = 0", a.Me)
		}
	case ScopeDept:
		if len(a.VisibleUserIDs) > 0 {
			if len(a.DeptIDs) > 0 {
				return q.Where("owner_user_id IN ? OR (owner_user_id = 0 AND (department_id = 0 OR department_id IN ?))",
					a.VisibleUserIDs, a.DeptIDs)
			}
			return q.Where("owner_user_id IN ? OR owner_user_id = 0", a.VisibleUserIDs)
		}
	}
	return q
}

// applyOwnerAccess 商机/合同硬隔离：按自身 owner_user_id 过滤（self=自己的；
// dept=树内成员的）；all/零值=不限制。商机/合同创建即有归属、无公海语义，
// 故不放行 owner=0。
func applyOwnerAccess(q *gorm.DB, a UserScope) *gorm.DB {
	switch a.Kind {
	case ScopeSelf:
		if a.Me > 0 {
			return q.Where("owner_user_id = ?", a.Me)
		}
	case ScopeDept:
		if len(a.VisibleUserIDs) > 0 {
			return q.Where("owner_user_id IN ?", a.VisibleUserIDs)
		}
	}
	return q
}
