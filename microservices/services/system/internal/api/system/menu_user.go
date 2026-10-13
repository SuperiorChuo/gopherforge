package system

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	system "github.com/go-admin-kit/services/system/internal/service/system"
)

// MenuAPI 处理用户菜单相关端点。
type MenuAPI struct {
	menuUserService system.MenuUserService
}

// NewMenuAPI 创建 MenuAPI 实例。
func NewMenuAPI() *MenuAPI {
	return &MenuAPI{
		menuUserService: system.MenuUserService{},
	}
}

// NewMenuAPIWithService 从注入的服务创建 MenuAPI 实例。
func NewMenuAPIWithService(menuUserService system.MenuUserService) *MenuAPI {
	return &MenuAPI{menuUserService: menuUserService}
}

// GetUserMenus 返回当前登录用户的菜单树。
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
