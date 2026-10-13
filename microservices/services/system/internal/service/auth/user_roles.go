package auth

import (
	"context"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/system/internal/dao"
	"gorm.io/gorm"
)

// UserService 是本服务所需的鉴权域用户服务的一个精简切片：
// 在线用户 API 加载当前用户的角色，以决定令牌 ID 是否可以脱敏展示。
type UserService struct {
	userDAO dao.UserDAO
}

// NewUserServiceWithDB 构建一个基于注入的数据库句柄的 UserService。
func NewUserServiceWithDB(db *gorm.DB) UserService {
	return UserService{userDAO: *dao.NewUserDAO(db)}
}

// GetUserWithRolesContext 加载一个预加载了角色的用户。
func (s *UserService) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRolesContext(ctx, id)
}
