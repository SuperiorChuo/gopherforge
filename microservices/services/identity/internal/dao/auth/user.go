package auth

import (
	sharedDAO "github.com/go-admin-kit/services/identity/internal/dao"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/authdao"
	"gorm.io/gorm"
)

// UserDAO keeps auth-specific user queries while reusing shared user persistence methods.
type UserDAO struct {
	sharedDAO.UserDAO
	authdao.AuthUserOps[localmodel.User]
}

func NewUserDAO(db *gorm.DB) *UserDAO {
	return &UserDAO{
		UserDAO:     *sharedDAO.NewUserDAO(db),
		AuthUserOps: *authdao.NewAuthUserOps[localmodel.User](db),
	}
}
