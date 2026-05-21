package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/go-admin-kit/server/internal/config"
	authDAO "github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type oauthBindingStore interface {
	GetByProviderUserContext(ctx context.Context, provider, providerUserID string) (*model.OAuthBinding, error)
	CreateContext(ctx context.Context, binding *model.OAuthBinding) error
}

type oauthUserStore interface {
	GetUserByIDContext(ctx context.Context, id uint) (*model.User, error)
	CreateUserContext(ctx context.Context, user *model.User) error
}

type OAuthService struct {
	bindingDAO oauthBindingStore
	userDAO    oauthUserStore
}

type OAuthResponse struct {
	User         model.User `json:"user"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
}

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

// Deprecated: use GithubCallbackContext instead.
func (s *OAuthService) GithubCallback(code, state string) (*OAuthResponse, error) {
	return s.GithubCallbackContext(context.Background(), code, state)
}

func (s *OAuthService) GithubCallbackContext(ctx context.Context, code, state string) (*OAuthResponse, error) {
	githubUser := struct {
		ID        int
		Login     string
		Email     string
		AvatarURL string
	}{
		ID:        123456,
		Login:     "testuser",
		Email:     "test@example.com",
		AvatarURL: "https://avatars.githubusercontent.com/u/123456?v=4",
	}

	user, err := s.findOrCreateUserContext(ctx, "github", fmt.Sprintf("%d", githubUser.ID), githubUser.Login, githubUser.Email, githubUser.AvatarURL)
	if err != nil {
		return nil, err
	}

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

// Deprecated: use WechatCallbackContext instead.
func (s *OAuthService) WechatCallback(code, state string) (*OAuthResponse, error) {
	return s.WechatCallbackContext(context.Background(), code, state)
}

func (s *OAuthService) WechatCallbackContext(ctx context.Context, code, state string) (*OAuthResponse, error) {
	wechatUser := struct {
		OpenID     string
		Nickname   string
		Headimgurl string
	}{
		OpenID:     "o123456",
		Nickname:   "wechat_user",
		Headimgurl: "https://wx.qlogo.cn/mmopen/vi_32/Q0j4TwGTfTLJibFq1e0f9/132",
	}

	user, err := s.findOrCreateUserContext(ctx, "wechat", wechatUser.OpenID, "wx_"+wechatUser.OpenID[:8], "", wechatUser.Headimgurl)
	if err != nil {
		return nil, err
	}

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

func (s *OAuthService) findOrCreateUserContext(ctx context.Context, provider, providerUserID, username, email, avatar string) (*model.User, error) {
	bindingDAO := s.bindingStore()
	binding, err := bindingDAO.GetByProviderUserContext(ctx, provider, providerUserID)
	if err == nil {
		userDAO := s.userStore()
		user, err := userDAO.GetUserByIDContext(ctx, binding.UserID)
		if err != nil {
			return nil, err
		}
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	user := model.User{
		Username: username,
		Password: generateRandomPassword(),
		Email:    email,
		Avatar:   avatar,
		Status:   1,
	}

	userDAO := s.userStore()
	if err := userDAO.CreateUserContext(ctx, &user); err != nil {
		return nil, err
	}

	binding = &model.OAuthBinding{
		UserID:         user.ID,
		Provider:       provider,
		ProviderUserID: providerUserID,
	}
	if err := bindingDAO.CreateContext(ctx, binding); err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *OAuthService) bindingStore() oauthBindingStore {
	if s != nil && s.bindingDAO != nil {
		return s.bindingDAO
	}
	return authDAO.OAuthBindingDAO{}
}

func (s *OAuthService) userStore() oauthUserStore {
	if s != nil && s.userDAO != nil {
		return s.userDAO
	}
	return authDAO.NewUserDAO(nil)
}

func generateRandomPassword() string {
	password := "random_password_" + uuid.New().String()[:8]
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashedPassword)
}
