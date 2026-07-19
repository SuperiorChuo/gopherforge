package auth

import (
	"context"

	"github.com/go-admin-kit/services/system/internal/dao"
	"github.com/go-admin-kit/services/system/internal/model"
	"gorm.io/gorm"
)

// UserService is the slim slice of the auth-domain user service this service
// needs: the online-user API loads the current user's roles to decide whether
// token IDs may be shown unmasked.
type UserService struct {
	userDAO dao.UserDAO
}

// NewUserServiceWithDB builds a UserService backed by an injected database
// handle.
func NewUserServiceWithDB(db *gorm.DB) UserService {
	return UserService{userDAO: *dao.NewUserDAO(db)}
}

// GetUserWithRolesContext loads a user with roles preloaded.
func (s *UserService) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRolesContext(ctx, id)
}
