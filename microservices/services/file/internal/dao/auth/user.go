package auth

import (
	sharedDAO "github.com/go-admin-kit/services/file/internal/dao"
	"github.com/go-admin-kit/services/shared/pkg/authdao"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// UserDAO keeps auth-specific user queries while reusing shared user persistence methods.
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
