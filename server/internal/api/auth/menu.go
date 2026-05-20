package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// MenuAPI handles user menu endpoints.
type MenuAPI struct {
	menuUserService system.MenuUserService
}

// NewMenuAPI creates a MenuAPI instance.
func NewMenuAPI() *MenuAPI {
	return &MenuAPI{
		menuUserService: system.MenuUserService{},
	}
}

// GetUserMenus returns the authenticated user's menu tree.
func (a *MenuAPI) GetUserMenus(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	menus, err := a.menuUserService.GetUserMenuTreeContext(c.Request.Context(), userID.(uint))
	if err != nil {
		internalServerError(c, "failed to get user menus", err)
		return
	}

	response.Success(c, menus)
}
