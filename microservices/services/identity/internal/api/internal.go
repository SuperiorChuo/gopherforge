package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/go-admin-kit/services/identity/internal/config"
)

// InternalAPI 服务间内部只读 API（Phase 2C：owner 服务暴露，X-Internal-Token 鉴权）。
// 供 notify/visibility/crm/bpm 等消费方调 identity 读用户/部门，替代直接查共享 users 表。
type InternalAPI struct {
	db *gorm.DB
}

// NewInternalAPI 创建内部 API。
func NewInternalAPI(db *gorm.DB) *InternalAPI { return &InternalAPI{db: db} }

// requireInternal 校验 X-Internal-Token；失败已写响应，返回 false。
func (a *InternalAPI) requireInternal(c *gin.Context) bool {
	token := config.Cfg.App.InternalToken
	if token == "" {
		failInternal(c, http.StatusServiceUnavailable, "identity internal token not configured")
		return false
	}
	if c.GetHeader("X-Internal-Token") != token {
		failInternal(c, http.StatusUnauthorized, "invalid internal token")
		return false
	}
	return true
}

func failInternal(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"code": code, "message": msg})
}

// BatchUserContacts 批量取用户联系方式 + 租户归属（notify 收件人校验/取 email/phone）。
// 返回 contacts: {userID: {email, phone}}——出现在 contacts 即视为该租户内有效用户。
func (a *InternalAPI) BatchUserContacts(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	var req struct {
		TenantID uint64   `json:"tenant_id"`
		IDs      []uint64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		failInternal(c, http.StatusBadRequest, "invalid request")
		return
	}

	var rows []struct {
		ID    uint64
		Email string
		Phone string
	}
	if err := a.db.Table("users").Select("id, email, phone").
		Where("id IN ? AND tenant_id = ? AND status = 1", req.IDs, req.TenantID).
		Scan(&rows).Error; err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}

	type contact struct {
		Email string `json:"email"`
		Phone string `json:"phone"`
	}
	out := gin.H{"contacts": map[string]contact{}}
	contacts := out["contacts"].(map[string]contact)
	for _, r := range rows {
		contacts[fmt.Sprintf("%d", r.ID)] = contact{Email: r.Email, Phone: r.Phone}
	}
	c.JSON(http.StatusOK, out)
}

// BatchUserDisplayNames 批量取用户显示名（COALESCE(nickname, username)），crm Owner 显示名等用。
func (a *InternalAPI) BatchUserDisplayNames(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	var req struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		failInternal(c, http.StatusBadRequest, "invalid request")
		return
	}
	// 显示名不按租户过滤：ids 由调用方（crm 等）从自己租户记录里取，id 全局唯一。
	var rows []struct {
		ID       uint64
		Nickname string
		Username string
	}
	if err := a.db.Table("users").Select("id, nickname, username").
		Where("id IN ?", req.IDs).
		Scan(&rows).Error; err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	out := gin.H{"names": map[string]string{}}
	names := out["names"].(map[string]string)
	for _, r := range rows {
		n := r.Nickname
		if n == "" {
			n = r.Username
		}
		names[fmt.Sprintf("%d", r.ID)] = n
	}
	c.JSON(http.StatusOK, out)
}

// DepartmentInfo 返回部门信息（id/name/leader_user_id），crm 分配目标校验 + bpm 部门主管用。
// 非本租户部门返回 404。
func (a *InternalAPI) DepartmentInfo(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	id := c.Query("id")
	tenantID := c.Query("tenant_id")
	if id == "" || tenantID == "" {
		failInternal(c, http.StatusBadRequest, "id and tenant_id required")
		return
	}
	var row struct {
		ID           uint64
		Name         string
		LeaderUserID uint64
	}
	err := a.db.Table("departments").Select("id, name, leader_user_id").
		Where("id = ? AND tenant_id = ?", id, tenantID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		failInternal(c, http.StatusNotFound, "department not found")
		return
	}
	if err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "name": row.Name, "leader_user_id": row.LeaderUserID})
}

// UserDepartment 返回用户所属部门（bpm 发起人部门兜底）。非本租户/不存在返回 0。
func (a *InternalAPI) UserDepartment(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	id := c.Query("id")
	tenantID := c.Query("tenant_id")
	if id == "" || tenantID == "" {
		failInternal(c, http.StatusBadRequest, "id and tenant_id required")
		return
	}
	var deptID uint64
	err := a.db.Table("users").Select("COALESCE(department_id,0)").
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Limit(1).Scan(&deptID).Error
	if err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"department_id": deptID})
}

// UsersByRoles 返回租户内启用用户中拥有指定角色的用户 id（bpm 按角色解析审批人）。
func (a *InternalAPI) UsersByRoles(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	var req struct {
		TenantID uint64   `json:"tenant_id"`
		RoleIDs  []uint64 `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.RoleIDs) == 0 {
		failInternal(c, http.StatusBadRequest, "invalid request")
		return
	}
	var ids []uint64
	if err := a.db.Table("users").
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Where("user_roles.role_id IN ?", req.RoleIDs).
		Where("users.status = 1 AND users.tenant_id = ?", req.TenantID).
		Order("users.id ASC").
		Distinct().
		Pluck("users.id", &ids).Error; err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ids": ids})
}

// UserDataScope 返回用户数据权限（roles.data_scope 最宽值）+ 部门（crm 数据权限解析）。
func (a *InternalAPI) UserDataScope(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	id := c.Query("id")
	tenantID := c.Query("tenant_id")
	if id == "" || tenantID == "" {
		failInternal(c, http.StatusBadRequest, "id and tenant_id required")
		return
	}
	// 用户全部角色的 data_scope，取最宽（与 crm/bpm 既有惯例一致）。
	var scopes []string
	if err := a.db.Table("roles").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ? AND roles.tenant_id = ?", id, tenantID).
		Pluck("roles.data_scope", &scopes).Error; err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	dataScope := "self"
	for _, sc := range scopes {
		if sc == "all" {
			dataScope = "all"
			break
		}
		if sc == "dept" || sc == "department" || sc == "department_tree" {
			dataScope = "dept"
		}
	}
	// 部门（dept 档兜底，与 crm 逻辑一致）。
	var deptID uint64
	_ = a.db.Table("users").Select("COALESCE(department_id,0)").
		Where("id = ? AND tenant_id = ?", id, tenantID).Limit(1).Scan(&deptID).Error
	c.JSON(http.StatusOK, gin.H{"data_scope": dataScope, "department_id": deptID})
}

// DepartmentMembers 返回部门及其子部门树内全部成员 id（crm dept 档数据权限）。
func (a *InternalAPI) DepartmentMembers(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	deptID := c.Query("department_id")
	tenantID := c.Query("tenant_id")
	if deptID == "" || tenantID == "" {
		failInternal(c, http.StatusBadRequest, "department_id and tenant_id required")
		return
	}
	// 递归取部门树（含自身），再取全部成员。用 WITH RECURSIVE 一次查全。
	var ids []uint64
	err := a.db.Raw(`
		WITH RECURSIVE tree AS (
			SELECT id FROM departments WHERE id = ? AND tenant_id = ?
			UNION ALL
			SELECT d.id FROM departments d JOIN tree t ON d.parent_id = t.id WHERE d.tenant_id = ?
		)
		SELECT DISTINCT u.id FROM users u JOIN tree ON u.department_id = tree.id
		WHERE u.tenant_id = ? AND u.status = 1 ORDER BY u.id ASC`,
		deptID, tenantID, tenantID, tenantID).Scan(&ids).Error
	if err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ids": ids})
}

// Org 返回租户组织架构（部门 + 启用成员），crm 分配选择器用。
func (a *InternalAPI) Org(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		failInternal(c, http.StatusBadRequest, "tenant_id required")
		return
	}
	var depts []map[string]any
	if err := a.db.Table("departments").Select("id, name").
		Where("status = 1 AND tenant_id = ?", tenantID).Order("sort ASC, id ASC").
		Scan(&depts).Error; err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	var users []map[string]any
	if err := a.db.Table("users").
		Select("id, COALESCE(NULLIF(nickname,''), username) AS name, department_id").
		Where("status = 1 AND tenant_id = ?", tenantID).Order("id ASC").
		Scan(&users).Error; err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"depts": depts, "users": users})
}

// AdminUserIDs 返回租户内启用+平台管理员的用户 id（visibility GEO 周报收件人）。
func (a *InternalAPI) AdminUserIDs(c *gin.Context) {
	if !a.requireInternal(c) {
		return
	}
	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		failInternal(c, http.StatusBadRequest, "tenant_id required")
		return
	}
	var ids []uint64
	if err := a.db.Table("users").
		Where("tenant_id = ? AND status = 1 AND is_platform_admin = true", tenantID).
		Pluck("id", &ids).Error; err != nil {
		failInternal(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ids": ids})
}
