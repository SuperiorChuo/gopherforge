package system

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// OnlineUserAPI 在线用户管理 API
type OnlineUserAPI struct {
	onlineUserService system.OnlineUserService
}

type onlineUserListItem struct {
	UserID               uint      `json:"user_id"`
	Username             string    `json:"username"`
	Nickname             string    `json:"nickname"`
	IP                   string    `json:"ip"`
	Location             string    `json:"location"`
	Browser              string    `json:"browser"`
	OS                   string    `json:"os"`
	LoginTime            time.Time `json:"login_time"`
	TokenID              string    `json:"token_id"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at,omitempty"`
}

// NewOnlineUserAPI 创建 OnlineUserAPI 实例
func NewOnlineUserAPI() *OnlineUserAPI {
	return &OnlineUserAPI{
		onlineUserService: system.OnlineUserService{},
	}
}

// GetOnlineUsers 获取在线用户列表
// @Summary 获取在线用户列表
// @Tags 在线用户管理
// @Security BearerAuth
// @Success 200 {object} response.Response{data=[]onlineUserListItem}
// @Router /online-users [get]
func (a *OnlineUserAPI) GetOnlineUsers(c *gin.Context) {
	users, err := a.onlineUserService.GetOnlineUsers()
	if err != nil {
		response.InternalServerError(c, "获取在线用户列表失败: "+err.Error())
		return
	}

	if users == nil {
		users = []system.OnlineUser{}
	}
	list := make([]onlineUserListItem, 0, len(users))
	for _, user := range users {
		list = append(list, onlineUserListItem{
			UserID:               user.UserID,
			Username:             user.Username,
			Nickname:             user.Nickname,
			IP:                   user.IP,
			Location:             user.Location,
			Browser:              user.Browser,
			OS:                   user.OS,
			LoginTime:            user.LoginTime,
			TokenID:              user.TokenID,
			AccessTokenExpiresAt: user.AccessTokenExpiresAt,
		})
	}

	response.Success(c, gin.H{
		"list":  list,
		"total": len(list),
	})
}

// GetOnlineUserCount 获取在线用户数量
// @Summary 获取在线用户数量
// @Tags 在线用户管理
// @Security BearerAuth
// @Success 200 {object} response.Response{data=int64}
// @Router /online-users/count [get]
func (a *OnlineUserAPI) GetOnlineUserCount(c *gin.Context) {
	count, err := a.onlineUserService.GetOnlineUserCount()
	if err != nil {
		response.InternalServerError(c, "获取在线用户数量失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"count": count,
	})
}

// ForceLogout 强制用户下线
// @Summary 强制用户下线
// @Tags 在线用户管理
// @Security BearerAuth
// @Param token_id path string true "Token ID"
// @Success 200 {object} response.Response
// @Router /online-users/{token_id} [delete]
func (a *OnlineUserAPI) ForceLogout(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		response.BadRequest(c, "token_id 不能为空")
		return
	}

	if err := a.onlineUserService.ForceLogout(tokenID); err != nil {
		response.InternalServerError(c, "强制下线失败: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "用户已强制下线", nil)
}
