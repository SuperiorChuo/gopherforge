package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/dao/auth"
	"github.com/go-admin-kit/services/auth/internal/model"
	"github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/auth/internal/pkg/captcha"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService handles user authentication.
type UserService struct {
	userDAO      auth.UserDAO
	policyReader runtimeconfig.SecurityPolicyReader
}

// NewUserServiceWithDB builds a UserService backed by an injected database handle.
func NewUserServiceWithDB(db *gorm.DB) UserService {
	return UserService{userDAO: *auth.NewUserDAO(db)}
}

// LoginRequest is the login request payload.
type LoginRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required"`
	// TenantCode selects SaaS tenant; empty means "default".
	TenantCode string `json:"tenant_code"`
}

// LoginResponse is the login response payload.
type LoginResponse struct {
	User            model.User `json:"user"`
	AccessToken     string     `json:"access_token"`
	RefreshToken    string     `json:"refresh_token"`
	RequiresTOTP    bool       `json:"requires_totp"`
	TOTPChallengeID string     `json:"totp_challenge_id,omitempty"`
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
	// ErrTenantDisabled indicates the actor's tenant was suspended.
	ErrTenantDisabled = errors.New("tenant is disabled")
	// ErrUserNotFound indicates the current user does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrOldPasswordIncorrect indicates the old password did not match.
	ErrOldPasswordIncorrect = errors.New("old password is incorrect")
	// ErrPasswordRecentlyUsed indicates the requested password appears in recent history.
	ErrPasswordRecentlyUsed = errors.New("new password was recently used")
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

func (s *UserService) LoginContext(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	if !captcha.CheckTextCaptchaContext(ctx, req.CaptchaID, req.CaptchaCode) {
		return nil, ErrInvalidCaptcha
	}

	return s.LoginPasswordWithTenantContext(ctx, req.Username, req.Password, req.TenantCode, 0)
}

func (s *UserService) LoginPasswordContext(ctx context.Context, username, password string) (*LoginResponse, error) {
	return s.LoginPasswordWithTenantContext(ctx, username, password, "", 0)
}

func (s *UserService) LoginPasswordWithAccessTTLContext(ctx context.Context, username, password string, accessTTL time.Duration) (*LoginResponse, error) {
	return s.LoginPasswordWithTenantContext(ctx, username, password, "", accessTTL)
}

// EnsureTenantActiveContext returns ErrTenantDisabled when the tenant is
// suspended (status != 1). Missing tenants row is treated as active for
// pre-migration compatibility. Used to gate refresh so disabled tenants lose
// access within one access-token TTL without a global session store.
func (s *UserService) EnsureTenantActiveContext(ctx context.Context, tenantID uint) error {
	tenant, err := s.userDAO.GetTenantByIDContext(ctx, tenantID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		// Pre-migration DBs without a tenants row: treat as active.
		return nil
	}
	if tenant.Status != 1 {
		return ErrTenantDisabled
	}
	return nil
}

func (s *UserService) LoginPasswordWithTenantContext(ctx context.Context, username, password, tenantCode string, accessTTL time.Duration) (*LoginResponse, error) {
	if tenantCode == "" {
		tenantCode = "default"
	}
	tenant, err := s.userDAO.GetTenantByCodeContext(ctx, tenantCode)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// Fallback: pre-migration DBs without tenants table still log into default user lookup
		if tenantCode != "default" {
			return nil, ErrInvalidCredentials
		}
		tenant = &model.Tenant{ID: 1, Code: "default", Status: 1}
	}

	user, err := s.userDAO.GetUserByTenantUsernameContext(ctx, tenant.ID, username)
	if err != nil {
		// Compatibility: if tenant_id column missing/unfilled, try global username for default tenant only
		if tenant.ID == 1 {
			user, err = s.userDAO.GetUserByUsernameLegacyContext(ctx, username)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, ErrInvalidCredentials
		}
	}

	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	policy := s.securityPolicy(ctx)
	passwordChangeRequired := shouldMarkDefaultAdminPassword(user, password) ||
		isPasswordExpired(user, time.Now(), policy.PasswordMaxAgeDays)
	if passwordChangeRequired && !user.MustChangePassword {
		if err := s.userDAO.MarkPasswordChangeRequiredContext(ctx, user.ID); err != nil {
			return nil, err
		}
		user.MustChangePassword = true
	}

	userWithRoles, err := s.userDAO.GetUserWithRolesAndPermissionsContext(ctx, user.ID)
	if err == nil {
		user = userWithRoles
	}
	if passwordChangeRequired {
		user.MustChangePassword = true
	}
	tenantID := user.TenantID
	if tenantID == 0 {
		tenantID = tenant.ID
	}
	if user.TOTPEnabled {
		challengeID, err := jwt.GenerateTOTPChallengeWithTenant(user.ID, user.Username, tenantID, 5*time.Minute)
		if err != nil {
			return nil, err
		}
		return &LoginResponse{
			User:            *user,
			RequiresTOTP:    true,
			TOTPChallengeID: challengeID,
		}, nil
	}

	accessToken, refreshToken, err := jwt.GenerateTokenWithTenantPlatformAndAccessTTL(
		user.ID, user.Username, tenantID, user.IsPlatformAdmin, accessTTL,
	)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		User:         *user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
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

	now := time.Now()
	user := &model.User{
		Username:          req.Username,
		Password:          string(hashedPassword),
		Email:             req.Email,
		Status:            1,
		PasswordChangedAt: &now,
	}

	if err := s.userDAO.CreateUserContext(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) GetUserWithRolesContext(ctx context.Context, id uint) (*model.User, error) {
	return s.userDAO.GetUserWithRolesContext(ctx, id)
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

	historyCount := s.securityPolicy(ctx).PasswordHistoryCount
	if err := s.ensurePasswordNotRecentlyUsed(ctx, user, req.NewPassword, historyCount); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	now := time.Now()
	previousHash := user.Password
	err = s.userDAO.UpdatePasswordWithHistoryContext(ctx, user.ID, previousHash, string(hashedPassword), now, historyCount)
	if err != nil {
		return mapPasswordUpdateError(err)
	}
	return nil
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

func hasDefaultAdminPassword(user *model.User) bool {
	defaultAdmin := config.Cfg.Security.DefaultAdmin
	if !defaultAdmin.ForceChangePassword || user == nil || user.MustChangePassword {
		return false
	}
	username := defaultAdmin.DefaultUsername
	if username == "" {
		username = "admin"
	}
	return user.Username == username && bcrypt.CompareHashAndPassword([]byte(user.Password), []byte("admin123")) == nil
}

func isPasswordExpired(user *model.User, now time.Time, maxAgeDays int) bool {
	if maxAgeDays <= 0 || user == nil {
		return false
	}
	if user.PasswordChangedAt == nil || user.PasswordChangedAt.IsZero() {
		return true
	}
	expiresAt := user.PasswordChangedAt.AddDate(0, 0, maxAgeDays)
	return !expiresAt.After(now)
}

func (s *UserService) ensurePasswordNotRecentlyUsed(ctx context.Context, user *model.User, newPassword string, historyCount int) error {
	if user == nil {
		return ErrUserNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(newPassword)) == nil {
		return ErrPasswordRecentlyUsed
	}
	if historyCount <= 0 {
		return nil
	}

	history, err := s.userDAO.ListRecentPasswordHistoryContext(ctx, user.ID, historyCount)
	if err != nil {
		return err
	}
	for _, item := range history {
		if bcrypt.CompareHashAndPassword([]byte(item.PasswordHash), []byte(newPassword)) == nil {
			return ErrPasswordRecentlyUsed
		}
	}
	return nil
}

func (s *UserService) securityPolicy(ctx context.Context) runtimeconfig.SecurityPolicy {
	if s != nil && s.policyReader != nil {
		return s.policyReader.SecurityPolicy(ctx)
	}
	return runtimeconfig.DefaultSecurityPolicyReader().SecurityPolicy(ctx)
}

func mapPasswordUpdateError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrOldPasswordIncorrect
	}
	return err
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

// ValidatePasswordStrength validates password strength.
func ValidatePasswordStrength(password string) error {
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

func validatePasswordStrength(password string) error {
	return ValidatePasswordStrength(password)
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
