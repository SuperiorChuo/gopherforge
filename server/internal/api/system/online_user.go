package system

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	sharedapi "github.com/go-admin-kit/server/internal/api/shared"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/response"
	authsvc "github.com/go-admin-kit/server/internal/service/auth"
	"github.com/go-admin-kit/server/internal/service/system"
)

// OnlineUserAPI handles online user management endpoints.
type OnlineUserAPI struct {
	onlineUserService onlineUserReader
	roleLoader        onlineUserRoleLoader
}

type onlineUserListItem struct {
	UserID               uint      `json:"user_id"`
	Username             string    `json:"username"`
	Nickname             string    `json:"nickname"`
	IP                   string    `json:"ip" mask:"ip"`
	Location             string    `json:"location"`
	Browser              string    `json:"browser"`
	OS                   string    `json:"os"`
	LoginTime            time.Time `json:"login_time"`
	TokenID              string    `json:"token_id" mask:"token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at,omitempty"`
}

type onlineUserListResponse struct {
	List  []onlineUserListItem `json:"list"`
	Total int                  `json:"total"`
}

type onlineUserCountResponse struct {
	Count int64 `json:"count"`
}

type onlineUserReader interface {
	GetOnlineUsersContext(ctx context.Context) ([]system.OnlineUser, error)
	GetOnlineUserCountContext(ctx context.Context) (int64, error)
	ForceLogoutContext(ctx context.Context, tokenID string) error
}

type onlineUserRoleLoader interface {
	GetRoleCodesContext(ctx context.Context, userID uint) ([]string, error)
}

type authUserRoleLoader struct {
	userService authsvc.UserService
}

// NewOnlineUserAPI creates an OnlineUserAPI instance.
func NewOnlineUserAPI() *OnlineUserAPI {
	return &OnlineUserAPI{
		onlineUserService: &system.OnlineUserService{},
		roleLoader:        authUserRoleLoader{userService: authsvc.UserService{}},
	}
}

// GetOnlineUsers returns online users.
// @Summary Get online users
// @Tags Online User Management
// @Security BearerAuth
// @Success 200 {object} response.Response{data=[]onlineUserListItem}
// @Router /online-users [get]
func (a *OnlineUserAPI) GetOnlineUsers(c *gin.Context) {
	users, err := a.onlineUserService.GetOnlineUsersContext(c.Request.Context())
	if err != nil {
		logOnlineUserError("failed to get online users", err)
		response.InternalServerError(c, "failed to get online users")
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

	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not found in context")
		return
	}

	roleCodes, err := a.roleLoader.GetRoleCodesContext(c.Request.Context(), userID.(uint))
	if err != nil {
		logOnlineUserError("failed to get current user roles", err)
		response.InternalServerError(c, "failed to get online users")
		return
	}

	response.SuccessMasked(c, onlineUserListResponse{
		List:  list,
		Total: len(list),
	}, sharedapi.ShouldMask(userID.(uint), nil, roleCodes))
}

// GetOnlineUserCount returns the online user count.
// @Summary Get online user count
// @Tags Online User Management
// @Security BearerAuth
// @Success 200 {object} response.Response{data=int64}
// @Router /online-users/count [get]
func (a *OnlineUserAPI) GetOnlineUserCount(c *gin.Context) {
	count, err := a.onlineUserService.GetOnlineUserCountContext(c.Request.Context())
	if err != nil {
		logOnlineUserError("failed to get online user count", err)
		response.InternalServerError(c, "failed to get online user count")
		return
	}

	response.Success(c, onlineUserCountResponse{Count: count})
}

// ForceLogout revokes an online user's session.
// @Summary Force user logout
// @Tags Online User Management
// @Security BearerAuth
// @Param token_id path string true "Token ID"
// @Success 200 {object} response.Response
// @Router /online-users/{token_id} [delete]
func (a *OnlineUserAPI) ForceLogout(c *gin.Context) {
	tokenID := c.Param("token_id")
	if tokenID == "" {
		response.BadRequest(c, "token_id is required")
		return
	}

	if err := a.onlineUserService.ForceLogoutContext(c.Request.Context(), tokenID); err != nil {
		logOnlineUserError("failed to force logout", err)
		response.InternalServerError(c, "failed to force logout")
		return
	}

	response.SuccessWithMessage(c, "user forced offline successfully", nil)
}

func logOnlineUserError(message string, err error) {
	if logger.Logger == nil {
		return
	}
	logger.Error(message, logger.Err(err))
}

func (l authUserRoleLoader) GetRoleCodesContext(ctx context.Context, userID uint) ([]string, error) {
	user, err := l.userService.GetUserWithRolesContext(ctx, userID)
	if err != nil {
		return nil, err
	}

	roleCodes := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		roleCodes = append(roleCodes, role.Code)
	}
	return roleCodes, nil
}
