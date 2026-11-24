package dao

import (
	"github.com/go-admin-kit/services/shared/pkg/authdao"
	"gorm.io/gorm"
)

type UserDAO = authdao.BaseUserDAO

func NewUserDAO(db *gorm.DB) *UserDAO {
	return authdao.NewBaseUserDAO(db)
}
