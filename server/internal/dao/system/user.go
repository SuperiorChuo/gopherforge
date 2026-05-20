package system

import (
	"gorm.io/gorm"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/pagination"
)

// UserDAO 用户数据访问对象（管理相关）
type UserDAO struct{}

// GetUserByID 根据ID获取用户
func (d *UserDAO) GetUserByID(id uint) (*model.User, error) {
	var user model.User
	result := database.DB.First(&user, id)
	return &user, result.Error
}

// GetUserWithRoles 获取用户及其角色
func (d *UserDAO) GetUserWithRoles(id uint) (*model.User, error) {
	var user model.User
	result := database.DB.Preload("Roles").First(&user, id)
	return &user, result.Error
}

// GetUserList 获取用户列表（分页）
func (d *UserDAO) GetUserList(req pagination.PageRequest, keyword string, status *int8, dataScope authz.UserDataScope) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := database.DB.Model(&model.User{})
	query = authz.ApplyUserEntityScope(query, dataScope, "id", "department_id")

	// 关键词搜索
	if keyword != "" {
		query = query.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ? OR phone LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 状态筛选
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	result := query.Scopes(pagination.Paginate(req)).
		Preload("Roles").
		Order("created_at DESC").
		Find(&users)

	return users, total, result.Error
}

// UpdateUser 更新用户
func (d *UserDAO) UpdateUser(user *model.User) error {
	return database.DB.Save(user).Error
}

// DeleteUser 删除用户（软删除）
func (d *UserDAO) DeleteUser(id uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// 先删除关联关系
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		// 再删除用户
		if err := tx.Delete(&model.User{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}

// UpdateUserStatus 更新用户状态
func (d *UserDAO) UpdateUserStatus(id uint, status int8) error {
	return database.DB.Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
}

// AssignRoles 分配角色给用户
func (d *UserDAO) AssignRoles(userID uint, roleIDs []uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// 先删除原有角色
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}

		if len(roleIDs) == 0 {
			return nil
		}

		// 批量添加新角色
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
		return nil
	})
}

// GetUserByEmail 根据邮箱获取用户
func (d *UserDAO) GetUserByEmail(email string) (*model.User, error) {
	var user model.User
	result := database.DB.Where("email = ?", email).First(&user)
	return &user, result.Error
}

// GetUserByUsername 根据用户名获取用户
func (d *UserDAO) GetUserByUsername(username string) (*model.User, error) {
	var user model.User
	result := database.DB.Where("username = ?", username).First(&user)
	return &user, result.Error
}

// CreateUser 创建用户
func (d *UserDAO) CreateUser(user *model.User) error {
	return database.DB.Create(user).Error
}
