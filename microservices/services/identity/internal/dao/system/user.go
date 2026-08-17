package system

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/go-admin-kit/services/shared/pkg/tenant"

	sharedDAO "github.com/go-admin-kit/services/identity/internal/dao"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	model "github.com/go-admin-kit/services/shared/pkg/model"

	"github.com/go-admin-kit/services/shared/pkg/pagination"
)

// UserDAO keeps system-management user queries while reusing shared user persistence methods.
type UserDAO struct {
	sharedDAO.UserDAO
	db *gorm.DB
}

func NewUserDAO(db *gorm.DB) *UserDAO {
	shared := sharedDAO.NewUserDAO(db)
	return &UserDAO{
		UserDAO: *shared,
		db:      db,
	}
}

func (d *UserDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *UserDAO) GetUserListContext(ctx context.Context, req pagination.PageRequest, keyword string, status *int8, dataScope authz.UserDataScope) ([]localmodel.User, int64, error) {
	var users []localmodel.User
	var total int64

	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).Model(&localmodel.User{})

	// Multi-tenant isolation (explicit; GORM tenant plugin also applies when registered).
	if tid := tenant.FromContext(ctx); tid > 0 {
		query = query.Where("users.tenant_id = ?", tid)
	}

	if keyword != "" {
		query = query.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ? OR phone LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.Scopes(pagination.Paginate(req)).
		Preload("Roles").
		Preload("Posts").
		Order("created_at DESC").
		Find(&users)

	return users, total, result.Error
}

// ExportUserCursor is the keyset position of the last exported row.
// Zero value means "start from the newest row".
type ExportUserCursor struct {
	CreatedAt time.Time
	ID        uint
}

// Valid reports whether the cursor points at a real row.
func (c ExportUserCursor) Valid() bool {
	return c.ID > 0
}

// exportUserColumns are the columns the export writes. Selecting explicitly
// keeps password hashes and TOTP secrets out of the result set entirely.
var exportUserColumns = []string{
	"users.id", "users.username", "users.nickname", "users.email",
	"users.phone", "users.department_id", "users.status", "users.created_at",
}

// ExportUsersPageContext returns one keyset page of the user list for export,
// in the same (created_at DESC, id DESC) order the list endpoint shows.
//
// Three things differ from GetUserListContext on purpose:
//
//   - Keyset instead of OFFSET. The old export walked 20 pages with LIMIT 500
//     OFFSET n*500, so the last page made Postgres scan and discard 9500 rows.
//     The (created_at, id) row-value comparison rides idx_users_tenant_created
//     and costs the same on page 20 as on page 1.
//   - No COUNT. The loop ran one full-table COUNT per page; the cursor tells
//     the caller when it is done (a short page means the end).
//   - No Preload("Roles") / Preload("Posts") and an explicit column list. The
//     export sheet writes ID / 用户名 / 昵称 / 邮箱 / 手机号 / 部门 / 状态 /
//     创建时间 (see api/system/user_excel.go) — no role or post column exists,
//     so those two extra queries per page fetched rows nobody read.
//
// Ties on created_at are broken by id, so the composite cursor cannot skip or
// repeat a row — which plain OFFSET over a non-unique sort key could.
func (d *UserDAO) ExportUsersPageContext(
	ctx context.Context,
	cursor ExportUserCursor,
	limit int,
	keyword string,
	status *int8,
	dataScope authz.UserDataScope,
) ([]localmodel.User, error) {
	if limit <= 0 {
		limit = 500
	}

	query := d.dbWithContext(authz.EnableDataScope(ctx, dataScope)).
		Model(&localmodel.User{}).
		Select(exportUserColumns)

	// Multi-tenant isolation (explicit; GORM tenant plugin also applies when registered).
	if tid := tenant.FromContext(ctx); tid > 0 {
		query = query.Where("users.tenant_id = ?", tid)
	}

	if keyword != "" {
		query = query.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ? OR phone LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if cursor.Valid() {
		// Row-value comparison: strictly "older than the last row we emitted".
		query = query.Where("(users.created_at, users.id) < (?, ?)", cursor.CreatedAt, cursor.ID)
	}

	var users []localmodel.User
	err := query.
		Order("users.created_at DESC, users.id DESC").
		Limit(limit).
		Find(&users).Error
	return users, err
}

// GetUserWithRolesPostsContext loads a user together with roles and posts.
func (d *UserDAO) GetUserWithRolesPostsContext(ctx context.Context, id uint) (*localmodel.User, error) {
	var user localmodel.User
	result := d.dbWithContext(ctx).Preload("Roles").Preload("Posts").First(&user, id)
	return &user, result.Error
}

func (d *UserDAO) DeleteUserContext(ctx context.Context, id uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&localmodel.UserPost{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&localmodel.User{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}

func (d *UserDAO) UpdateUserStatusContext(ctx context.Context, id uint, status int8) error {
	return d.dbWithContext(ctx).Model(&localmodel.User{}).Where("id = ?", id).Update("status", status).Error
}

func (d *UserDAO) AssignRolesContext(ctx context.Context, userID uint, roleIDs []uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before := make([]uint, 0)
		if err := tx.Model(&model.UserRole{}).Where("user_id = ?", userID).Pluck("role_id", &before).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}

		if len(roleIDs) > 0 {
			userRoles := make([]model.UserRole, 0, len(roleIDs))
			for _, roleID := range roleIDs {
				userRoles = append(userRoles, model.UserRole{
					UserID: userID,
					RoleID: roleID,
				})
			}

			if err := tx.Create(&userRoles).Error; err != nil {
				return err
			}
		}
		return audittrail.RecordAssociation(ctx, tx, audittrail.RecordAssociationRequest{
			TargetType: "user_roles",
			TargetID:   fmt.Sprint(userID),
			Action:     "update",
			Before:     map[string]any{"role_ids": append([]uint{}, before...)},
			After:      map[string]any{"role_ids": append([]uint{}, roleIDs...)},
			Summary:    fmt.Sprintf("update user %d roles", userID),
		})
	})
}

// AssertRolesInTenantContext ensures every role id belongs to tenantID.
func (d *UserDAO) AssertRolesInTenantContext(ctx context.Context, roleIDs []uint, tenantID uint) error {
	if len(roleIDs) == 0 {
		return nil
	}
	if tenantID == 0 {
		tenantID = 1
	}
	seen := make(map[uint]struct{}, len(roleIDs))
	uniq := make([]uint, 0, len(roleIDs))
	for _, id := range roleIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return nil
	}
	var count int64
	if err := d.dbWithContext(ctx).Model(&model.Role{}).
		Where("tenant_id = ? AND id IN ?", tenantID, uniq).
		Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(uniq) {
		return ErrRoleNotInTenant
	}
	return nil
}

// AssignPostsContext replaces the post assignment of a user.
func (d *UserDAO) AssignPostsContext(ctx context.Context, userID uint, postIDs []uint) error {
	return d.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before := make([]uint, 0)
		if err := tx.Model(&localmodel.UserPost{}).Where("user_id = ?", userID).Pluck("post_id", &before).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&localmodel.UserPost{}).Error; err != nil {
			return err
		}

		if len(postIDs) > 0 {
			userPosts := make([]localmodel.UserPost, 0, len(postIDs))
			for _, postID := range postIDs {
				userPosts = append(userPosts, localmodel.UserPost{
					UserID: userID,
					PostID: postID,
				})
			}

			if err := tx.Create(&userPosts).Error; err != nil {
				return err
			}
		}
		return audittrail.RecordAssociation(ctx, tx, audittrail.RecordAssociationRequest{
			TargetType: "user_posts",
			TargetID:   fmt.Sprint(userID),
			Action:     "update",
			Before:     map[string]any{"post_ids": append([]uint{}, before...)},
			After:      map[string]any{"post_ids": append([]uint{}, postIDs...)},
			Summary:    fmt.Sprintf("update user %d posts", userID),
		})
	})
}

// AssertPostsInTenantContext ensures every post id belongs to tenantID.
func (d *UserDAO) AssertPostsInTenantContext(ctx context.Context, postIDs []uint, tenantID uint) error {
	if len(postIDs) == 0 {
		return nil
	}
	if tenantID == 0 {
		tenantID = 1
	}
	seen := make(map[uint]struct{}, len(postIDs))
	uniq := make([]uint, 0, len(postIDs))
	for _, id := range postIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return nil
	}
	var count int64
	if err := d.dbWithContext(ctx).Model(&localmodel.Post{}).
		Where("tenant_id = ? AND id IN ?", tenantID, uniq).
		Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(uniq) {
		return ErrPostNotInTenant
	}
	return nil
}

// AssertDepartmentInTenantContext ensures department belongs to tenant.
func (d *UserDAO) AssertDepartmentInTenantContext(ctx context.Context, departmentID, tenantID uint) error {
	if departmentID == 0 {
		return nil
	}
	if tenantID == 0 {
		tenantID = 1
	}
	var count int64
	if err := d.dbWithContext(ctx).Model(&localmodel.Department{}).
		Where("id = ? AND tenant_id = ?", departmentID, tenantID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrDepartmentNotInTenant
	}
	return nil
}

// Tenant boundary errors (M2).
var (
	ErrRoleNotInTenant       = errors.New("role does not belong to current tenant")
	ErrDepartmentNotInTenant = errors.New("department does not belong to current tenant")
	ErrPostNotInTenant       = errors.New("post does not belong to current tenant")
)
