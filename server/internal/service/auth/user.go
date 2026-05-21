package auth

import (
	"context"
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

// UserService handles user authentication.
type UserService struct {
	userDAO auth.UserDAO
}

// LoginRequest is the login request payload.
type LoginRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required"`
}

// LoginResponse is the login response payload.
type LoginResponse struct {
	User         model.User `json:"user"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
}

// RegisterRequest is the registration request payload.
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
}

// ChangePasswordRequest is the password change request payload.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UpdateProfileRequest updates the current user's profile.
type UpdateProfileRequest struct {
	Nickname *string `json:"nickname"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Avatar   *string `json:"avatar"`
}

var (
	// ErrInvalidCaptcha indicates an invalid or expired captcha.
	ErrInvalidCaptcha = errors.New("captcha is invalid or expired")
	// ErrInvalidCredentials indicates an invalid username/password pair.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrUserDisabled indicates a disabled user account.
	ErrUserDisabled = errors.New("user is disabled")
	// ErrUserNotFound indicates the current user does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrOldPasswordIncorrect indicates the old password did not match.
	ErrOldPasswordIncorrect = errors.New("old password is incorrect")
	// ErrUsernameAlreadyExists indicates the username is already registered.
	ErrUsernameAlreadyExists = errors.New("username already exists")
	// ErrEmailAlreadyExists indicates the email is used by another user.
	ErrEmailAlreadyExists = errors.New("email already exists")
	// ErrPhoneAlreadyExists indicates the phone number is used by another user.
	ErrPhoneAlreadyExists = errors.New("phone already exists")
)

// ProfileValidationError reports invalid profile input.
type ProfileValidationError struct {
	Message string
}

func (e ProfileValidationError) Error() string {
	return e.Message
}

// PasswordValidationError reports invalid password input.
type PasswordValidationError struct {
	Message string
}

func (e PasswordValidationError) Error() string {
	return e.Message
}

// Login authenticates a user.
// Deprecated: use LoginContext instead.
func (s *UserService) Login(req LoginRequest) (*LoginResponse, error) {
	return s.LoginContext(context.Background(), req)
}

func (s *UserService) LoginContext(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	if !captcha.CheckTextCaptchaContext(ctx, req.CaptchaID, req.CaptchaCode) {
		return nil, ErrInvalidCaptcha
	}

	return s.LoginPasswordContext(ctx, req.Username, req.Password)
}

// Deprecated: use LoginPasswordContext instead.
func (s *UserService) LoginPassword(username, password string) (*LoginResponse, error) {
	return s.LoginPasswordContext(context.Background(), username, password)
}

func (s *UserService) LoginPasswordContext(ctx context.Context, username, password string) (*LoginResponse, error) {
	return s.LoginPasswordWithAccessTTLContext(ctx, username, password, 0)
}

// Deprecated: use LoginPasswordWithAccessTTLContext instead.
func (s *UserService) LoginPasswordWithAccessTTL(username, password string, accessTTL time.Duration) (*LoginResponse, error) {
	return s.LoginPasswordWithAccessTTLContext(context.Background(), username, password, accessTTL)
}

func (s *UserService) LoginPasswordWithAccessTTLContext(ctx context.Context, username, password string, accessTTL time.Duration) (*LoginResponse, error) {
	user, err := s.userDAO.GetUserByUsernameContext(ctx, username)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, ErrInvalidCredentials
	}

	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if shouldMarkDefaultAdminPassword(user, password) {
		user.MustChangePassword = true
		_ = s.userDAO.UpdateUserContext(ctx, user)
	}

	accessToken, refreshToken, err := jwt.GenerateTokenWithAccessTTL(user.ID, user.Username, accessTTL)
	if err != nil {
		return nil, err
	}

	userWithRoles, err := s.userDAO.GetUserWithRolesAndPermissionsContext(ctx, user.ID)
	if err == nil {
		user = userWithRoles
	}

	return &LoginResponse{
		User:         *user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Register creates a user account.
// Deprecated: use RegisterContext instead.
func (s *UserService) Register(req RegisterRequest) (*model.User, error) {
	return s.RegisterContext(context.Background(), req)
}

func (s *UserService) RegisterContext(ctx context.Context, req RegisterRequest) (*model.User, error) {
	_, err := s.userDAO.GetUserByUsernameContext(ctx, req.Username)
	if err == nil {
		return nil, ErrUsernameAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	_, err = s.userDAO.GetUserByEmailContext(ctx, req.Email)
	if err == nil {
		return nil, ErrEmailAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := validatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Email:    req.Email,
		Status:   1,
	}

	if err := s.userDAO.CreateUserContext(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserWithRoles returns a user with roles.
// Deprecated: use GetUserWithRolesContext instead.
func (s *UserService) GetUserWithRoles(id uint) (*model.User, error) {
	return s.GetUserWithRolesContext(context.Background(), id)
}

func (s *UserService) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRolesContext(ctx, id)
}

// ChangePassword changes a user's password.
// Deprecated: use ChangePasswordContext instead.
func (s *UserService) ChangePassword(userID uint, req ChangePasswordRequest) error {
	return s.ChangePasswordContext(context.Background(), userID, req)
}

func (s *UserService) ChangePasswordContext(ctx context.Context, userID uint, req ChangePasswordRequest) error {
	user, err := s.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return ErrOldPasswordIncorrect
	}

	if err := validatePasswordStrength(req.NewPassword); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashedPassword)
	user.MustChangePassword = false
	return s.userDAO.UpdateUserContext(ctx, user)
}

// UpdateProfile updates the current user's profile.
// Deprecated: use UpdateProfileContext instead.
func (s *UserService) UpdateProfile(userID uint, req UpdateProfileRequest) (*model.User, error) {
	return s.UpdateProfileContext(context.Background(), userID, req)
}

func (s *UserService) UpdateProfileContext(ctx context.Context, userID uint, req UpdateProfileRequest) (*model.User, error) {
	user, err := s.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	updates := make(map[string]any)

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
				existing, err := s.userDAO.GetUserByEmailContext(ctx, email)
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
				existing, err := s.userDAO.GetUserByPhoneContext(ctx, phone)
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
		if err := s.userDAO.UpdateUserProfileContext(ctx, userID, updates); err != nil {
			return nil, err
		}
	}

	return s.userDAO.GetUserWithRolesAndPermissionsContext(ctx, userID)
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

// GetUserWithRolesAndPermissions returns a user with roles and permissions.
// Deprecated: use GetUserWithRolesAndPermissionsContext instead.
func (s *UserService) GetUserWithRolesAndPermissions(id uint) (*model.User, error) {
	return s.GetUserWithRolesAndPermissionsContext(context.Background(), id)
}

func (s *UserService) GetUserWithRolesAndPermissionsContext(ctx context.Context, id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRolesAndPermissionsContext(ctx, id)
}

// GetUserPermissions returns a deduplicated list of permission codes.
func (s *UserService) GetUserPermissions(user *model.User) []string {
	permissionMap := make(map[string]bool)

	for _, role := range user.Roles {
		for _, perm := range role.Permissions {
			permissionMap[perm.Code] = true
		}
	}

	permissions := make([]string, 0, len(permissionMap))
	for code := range permissionMap {
		permissions = append(permissions, code)
	}

	return permissions
}

// validatePasswordStrength validates password strength.
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return PasswordValidationError{Message: "password must be at least 8 characters"}
	}
	if len(password) > 32 {
		return PasswordValidationError{Message: "password must be no more than 32 characters"}
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
		return PasswordValidationError{Message: "password must contain uppercase, lowercase and digit"}
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
