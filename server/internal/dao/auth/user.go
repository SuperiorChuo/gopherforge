package auth

import (
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

// UserDAO 用户数据访问对象（认证相关）
type UserDAO struct{}

// GetUserByUsername 根据用户名获取用户
func (d *UserDAO) GetUserByUsername(username string) (*model.User, error) {
	var user model.User
	result := database.DB.Where("username = ?", username).First(&user)
	return &user, result.Error
}

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

// CreateUser 创建用户
func (d *UserDAO) CreateUser(user *model.User) error {
	return database.DB.Create(user).Error
}

// UpdateUser 更新用户
func (d *UserDAO) UpdateUser(user *model.User) error {
	return database.DB.Save(user).Error
}

// UpdateUserProfile 更新个人资料白名单字段
func (d *UserDAO) UpdateUserProfile(id uint, updates map[string]interface{}) error {
	return database.DB.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// GetUserByEmail 根据邮箱获取用户
func (d *UserDAO) GetUserByEmail(email string) (*model.User, error) {
	var user model.User
	result := database.DB.Where("email = ?", email).First(&user)
	return &user, result.Error
}

// GetUserByPhone 根据手机号获取用户
func (d *UserDAO) GetUserByPhone(phone string) (*model.User, error) {
	var user model.User
	result := database.DB.Where("phone = ?", phone).First(&user)
	return &user, result.Error
}

// GetUserWithRolesAndPermissions 获取用户及其角色和权限
func (d *UserDAO) GetUserWithRolesAndPermissions(id uint) (*model.User, error) {
	var user model.User
	result := database.DB.
		Preload("Roles.Permissions"). // 预加载角色的权限
		First(&user, id)
	return &user, result.Error
}
