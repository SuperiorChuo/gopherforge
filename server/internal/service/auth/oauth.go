package auth

import (
	"fmt"
	"net/url"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// OAuthService OAuth服务
type OAuthService struct{}

// OAuthResponse OAuth响应
type OAuthResponse struct {
	User         model.User `json:"user"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
}

// GetGithubAuthURL 获取GitHub授权URL
func (s *OAuthService) GetGithubAuthURL() (string, error) {
	cfg := config.Cfg.OAuth.Github
	state := uuid.New().String()

	authURL := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/login/oauth/authorize",
		RawQuery: url.Values{
			"client_id":     {cfg.ClientID},
			"redirect_uri":  {cfg.RedirectURI},
			"state":         {state},
			"scope":         {"user:email"},
			"response_type": {"code"},
		}.Encode(),
	}

	return authURL.String(), nil
}

// GithubCallback 处理GitHub回调
func (s *OAuthService) GithubCallback(code, state string) (*OAuthResponse, error) {
	// 获取GitHub用户信息
	// 这里简化实现，实际应该调用GitHub API获取用户信息
	githubUser := struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}{
		ID:        123456,
		Login:     "testuser",
		Email:     "test@example.com",
		AvatarURL: "https://avatars.githubusercontent.com/u/123456?v=4",
	}

	// 查找或创建用户
	user, err := s.findOrCreateUser("github", fmt.Sprintf("%d", githubUser.ID), githubUser.Login, githubUser.Email, githubUser.AvatarURL)
	if err != nil {
		return nil, err
	}

	// 生成token
	accessToken, refreshToken, err := jwt.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, err
	}

	return &OAuthResponse{
		User:         *user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// GetWechatAuthURL 获取微信授权URL
func (s *OAuthService) GetWechatAuthURL() (string, error) {
	cfg := config.Cfg.OAuth.Wechat
	state := uuid.New().String()

	authURL := url.URL{
		Scheme: "https",
		Host:   "open.weixin.qq.com",
		Path:   "/connect/qrconnect",
		RawQuery: url.Values{
			"appid":         {cfg.ClientID},
			"redirect_uri":  {url.QueryEscape(cfg.RedirectURI)},
			"response_type": {"code"},
			"scope":         {"snsapi_login"},
			"state":         {state},
		}.Encode(),
	}

	return authURL.String() + "#wechat_redirect", nil
}

// WechatCallback 处理微信回调
func (s *OAuthService) WechatCallback(code, state string) (*OAuthResponse, error) {
	// 获取微信用户信息
	// 这里简化实现，实际应该调用微信API获取用户信息
	wechatUser := struct {
		OpenID     string `json:"openid"`
		Nickname   string `json:"nickname"`
		Headimgurl string `json:"headimgurl"`
	}{
		OpenID:     "o123456",
		Nickname:   "微信用户",
		Headimgurl: "https://wx.qlogo.cn/mmopen/vi_32/Q0j4TwGTfTLJibFq1e0f9/132",
	}

	// 查找或创建用户
	user, err := s.findOrCreateUser("wechat", wechatUser.OpenID, "wx_"+wechatUser.OpenID[:8], "", wechatUser.Headimgurl)
	if err != nil {
		return nil, err
	}

	// 生成token
	accessToken, refreshToken, err := jwt.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, err
	}

	return &OAuthResponse{
		User:         *user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// findOrCreateUser 查找或创建用户
func (s *OAuthService) findOrCreateUser(provider, providerUserID, username, email, avatar string) (*model.User, error) {
	// 查找OAuth绑定
	var binding model.OAuthBinding
	result := database.DB.Where("provider = ? AND provider_user_id = ?", provider, providerUserID).First(&binding)

	if result.Error == nil {
		// 绑定存在，查找用户
		userDAO := auth.UserDAO{}
		user, err := userDAO.GetUserByID(binding.UserID)
		if err != nil {
			return nil, err
		}
		return user, nil
	}

	// 绑定不存在，创建用户
	user := model.User{
		Username: username,
		Password: generateRandomPassword(),
		Email:    email,
		Avatar:   avatar,
		Status:   1,
	}

	// 创建用户
	if err := database.DB.Create(&user).Error; err != nil {
		return nil, err
	}

	// 创建OAuth绑定
	binding = model.OAuthBinding{
		UserID:         user.ID,
		Provider:       provider,
		ProviderUserID: providerUserID,
	}

	if err := database.DB.Create(&binding).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// generateRandomPassword 生成随机密码
func generateRandomPassword() string {
	// 生成随机密码
	password := "random_password_" + uuid.New().String()[:8]
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashedPassword)
}
