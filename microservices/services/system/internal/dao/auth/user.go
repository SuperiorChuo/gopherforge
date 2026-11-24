package auth

import (
	"github.com/go-admin-kit/services/shared/pkg/authdao"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	sharedDAO "github.com/go-admin-kit/services/system/internal/dao"
	"gorm.io/gorm"
)

// UserDAO 在复用共享用户持久化方法的同时，维护鉴权相关的用户查询。
type UserDAO struct {
	sharedDAO.UserDAO
	authdao.AuthUserOps[model.User]
}

func NewUserDAO(db *gorm.DB) *UserDAO {
	return &UserDAO{
		UserDAO:     *sharedDAO.NewUserDAO(db),
		AuthUserOps: *authdao.NewAuthUserOps[model.User](db),
	}
}
