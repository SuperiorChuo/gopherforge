package auth

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/captcha"
	"github.com/go-admin-kit/server/internal/pkg/jwt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService 用户认证服务
type UserService struct {
	userDAO auth.UserDAO
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	User         model.User `json:"user"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UpdateProfileRequest 更新当前用户个人资料请求
type UpdateProfileRequest struct {
	Nickname *string `json:"nickname"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Avatar   *string `json:"avatar"`
}

var (
	// ErrUserNotFound 当前用户不存在
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailAlreadyExists 邮箱已被其他用户占用
	ErrEmailAlreadyExists = errors.New("email already exists")
	// ErrPhoneAlreadyExists 手机号已被其他用户占用
	ErrPhoneAlreadyExists = errors.New("phone already exists")
)

// ProfileValidationError 个人资料参数校验错误
type ProfileValidationError struct {
	Message string
}

func (e ProfileValidationError) Error() string {
	return e.Message
}

// Login 用户登录
func (s *UserService) Login(req LoginRequest) (*LoginResponse, error) {
	// 验证验证码
	if !captcha.CheckTextCaptcha(req.CaptchaID, req.CaptchaCode) {
		return nil, errors.New("验证码错误或已过期")
	}

	// 根据用户名获取用户
	return s.LoginPassword(req.Username, req.Password)
}

func (s *UserService) LoginPassword(username, password string) (*LoginResponse, error) {
	return s.LoginPasswordWithAccessTTL(username, password, 0)
}

func (s *UserService) LoginPasswordWithAccessTTL(username, password string, accessTTL time.Duration) (*LoginResponse, error) {
	user, err := s.userDAO.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("invalid username or password")
	}

	// 检查用户状态
	if user.Status != 1 {
		return nil, errors.New("user is disabled")
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid username or password")
	}

	if shouldMarkDefaultAdminPassword(user, password) {
		user.MustChangePassword = true
		_ = s.userDAO.UpdateUser(user)
	}

	// 生成token
	accessToken, refreshToken, err := jwt.GenerateTokenWithAccessTTL(user.ID, user.Username, accessTTL)
	if err != nil {
		return nil, err
	}

	// 重新获取用户完整信息（包含角色和权限）
	userWithRoles, err := s.userDAO.GetUserWithRolesAndPermissions(user.ID)
	if err == nil {
		user = userWithRoles
	}

	return &LoginResponse{
		User:         *user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Register 用户注册
func (s *UserService) Register(req RegisterRequest) (*model.User, error) {
	// 检查用户名是否已存在
	_, err := s.userDAO.GetUserByUsername(req.Username)
	if err == nil {
		return nil, errors.New("username already exists")
	}

	// 检查邮箱是否已存在
	_, err = s.userDAO.GetUserByEmail(req.Email)
	if err == nil {
		return nil, errors.New("email already exists")
	}

	// 验证密码强度
	if err := validatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	// 哈希密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 创建用户
	user := &model.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Email:    req.Email,
		Status:   1,
	}

	if err := s.userDAO.CreateUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserWithRoles 获取用户及其角色
func (s *UserService) GetUserWithRoles(id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRoles(id)
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(userID uint, req ChangePasswordRequest) error {
	user, err := s.userDAO.GetUserByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return errors.New("old password is incorrect")
	}

	// 验证新密码强度
	if err := validatePasswordStrength(req.NewPassword); err != nil {
		return err
	}

	// 哈希新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashedPassword)
	user.MustChangePassword = false
	return s.userDAO.UpdateUser(user)
}

// UpdateProfile 更新当前用户个人资料
func (s *UserService) UpdateProfile(userID uint, req UpdateProfileRequest) (*model.User, error) {
	user, err := s.userDAO.GetUserByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	updates := make(map[string]interface{})

	if req.Nickname != nil {
		nickname := strings.TrimSpace(*req.Nickname)
		if len([]rune(nickname)) > 50 {
			return nil, ProfileValidationError{Message: "nickname must be no more than 50 characters"}
		}
		updates["nickname"] = nickname
	}

	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if len(email) > 100 {
			return nil, ProfileValidationError{Message: "email must be no more than 100 characters"}
		}
		if email != "" {
			if err := validateProfileEmail(email); err != nil {
				return nil, err
			}
			if !strings.EqualFold(email, user.Email) {
				existing, err := s.userDAO.GetUserByEmail(email)
				if err == nil && existing.ID != userID {
					return nil, ErrEmailAlreadyExists
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, err
				}
			}
			updates["email"] = email
		} else {
			updates["email"] = nil
		}
	}

	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		if len(phone) > 20 {
			return nil, ProfileValidationError{Message: "phone must be no more than 20 characters"}
		}
		if phone != "" {
			if err := validateProfilePhone(phone); err != nil {
				return nil, err
			}
			if phone != user.Phone {
				existing, err := s.userDAO.GetUserByPhone(phone)
				if err == nil && existing.ID != userID {
					return nil, ErrPhoneAlreadyExists
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, err
				}
			}
			updates["phone"] = phone
		} else {
			updates["phone"] = nil
		}
	}

	if req.Avatar != nil {
		avatar := strings.TrimSpace(*req.Avatar)
		if len(avatar) > 255 {
			return nil, ProfileValidationError{Message: "avatar must be no more than 255 characters"}
		}
		updates["avatar"] = avatar
	}

	if len(updates) > 0 {
		if err := s.userDAO.UpdateUserProfile(userID, updates); err != nil {
			return nil, err
		}
	}

	return s.userDAO.GetUserWithRolesAndPermissions(userID)
}

func shouldMarkDefaultAdminPassword(user *model.User, plainPassword string) bool {
	defaultAdmin := config.Cfg.Security.DefaultAdmin
	if !defaultAdmin.ForceChangePassword {
		return false
	}
	username := defaultAdmin.DefaultUsername
	if username == "" {
		username = "admin"
	}
	return user.Username == username && !user.MustChangePassword && plainPassword == "admin123"
}

// GetUserWithRolesAndPermissions 获取用户及其完整的角色和权限
func (s *UserService) GetUserWithRolesAndPermissions(id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRolesAndPermissions(id)
}

// GetUserPermissions 获取用户的所有权限代码（去重）
func (s *UserService) GetUserPermissions(user *model.User) []string {
	permissionMap := make(map[string]bool)

	// 遍历用户的所有角色
	for _, role := range user.Roles {
		// 遍历角色的所有权限
		for _, perm := range role.Permissions {
			permissionMap[perm.Code] = true
		}
	}

	// 转换为数组
	permissions := make([]string, 0, len(permissionMap))
	for code := range permissionMap {
		permissions = append(permissions, code)
	}

	return permissions
}

// validatePasswordStrength 验证密码强度
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > 32 {
		return errors.New("password must be no more than 32 characters")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return errors.New("password must contain uppercase, lowercase and digit")
	}

	return nil
}

func validateProfileEmail(email string) error {
	if email == "" {
		return nil
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return ProfileValidationError{Message: "invalid email format"}
	}
	domain := email[at+1:]
	if !strings.Contains(domain, ".") {
		return ProfileValidationError{Message: "invalid email format"}
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return ProfileValidationError{Message: "invalid email format"}
		}
	}
	if strings.ContainsAny(email, " \t\r\n") {
		return ProfileValidationError{Message: "invalid email format"}
	}
	return nil
}

func validateProfilePhone(phone string) error {
	if phone == "" {
		return nil
	}

	digitCount := 0
	for i, r := range phone {
		switch {
		case unicode.IsDigit(r):
			digitCount++
		case r == '+' && i == 0:
		case r == '-' || r == ' ' || r == '(' || r == ')':
		default:
			return ProfileValidationError{Message: "invalid phone format"}
		}
	}

	if digitCount < 5 || digitCount > 20 {
		return ProfileValidationError{Message: "invalid phone format"}
	}

	return nil
}
