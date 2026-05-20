package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// MenuAPI 用户菜单API
type MenuAPI struct {
	menuUserService system.MenuUserService
}

// NewMenuAPI 创建MenuAPI实例
func NewMenuAPI() *MenuAPI {
	return &MenuAPI{
		menuUserService: system.MenuUserService{},
	}
}

// GetUserMenus 获取当前用户的菜单树（用于动态路由）
func (a *MenuAPI) GetUserMenus(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	menus, err := a.menuUserService.GetUserMenuTree(userID.(uint))
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, menus)
}
