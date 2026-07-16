package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/go-admin-kit/services/auth/internal/config"
	"github.com/go-admin-kit/services/auth/internal/pkg/jwt"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const totpRecoveryCodeCount = 8

type VerifyTOTPLoginRequest struct {
	ChallengeID string `json:"challenge_id" binding:"required"`
	Code        string `json:"code" binding:"required"`
}

type TOTPSetupResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otp_auth_url"`
}

type TOTPSetupRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
}

type TOTPVerifyRequest struct {
	Code            string `json:"code" binding:"required"`
	CurrentPassword string `json:"current_password" binding:"required"`
}

type TOTPRecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

var (
	ErrTOTPRequired       = errors.New("totp verification required")
	ErrTOTPInvalid        = errors.New("totp code is invalid")
	ErrTOTPNotConfigured  = errors.New("totp is not configured")
	ErrTOTPAlreadyEnabled = errors.New("totp is already enabled")
)

func (s *UserService) GenerateTOTPSetupContext(ctx context.Context, userID uint, req TOTPSetupRequest) (*TOTPSetupResponse, error) {
	user, err := s.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if user.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}
	if err := verifyCurrentPasswordHash(user.Password, req.CurrentPassword); err != nil {
		return nil, err
	}

	issuer := strings.TrimSpace(config.Cfg.App.Name)
	if issuer == "" {
		issuer = "go-admin-kit"
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: user.Username,
	})
	if err != nil {
		return nil, err
	}
	if err := s.userDAO.UpdateTOTPSetupContext(ctx, userID, key.Secret()); err != nil {
		return nil, err
	}

	return &TOTPSetupResponse{
		Secret:     key.Secret(),
		OTPAuthURL: key.URL(),
	}, nil
}

func (s *UserService) EnableTOTPContext(ctx context.Context, userID uint, req TOTPVerifyRequest) (*TOTPRecoveryCodesResponse, error) {
	user, err := s.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if user.TOTPSecret == "" {
		return nil, ErrTOTPNotConfigured
	}
	if err := verifyCurrentPasswordHash(user.Password, req.CurrentPassword); err != nil {
		return nil, err
	}
	if !validateTOTPCode(req.Code, user.TOTPSecret) {
		return nil, ErrTOTPInvalid
	}

	codes, hashes, err := generateTOTPRecoveryCodes(totpRecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if err := s.userDAO.EnableTOTPWithRecoveryCodesContext(ctx, userID, hashes, time.Now()); err != nil {
		return nil, err
	}
	return &TOTPRecoveryCodesResponse{RecoveryCodes: codes}, nil
}

func (s *UserService) DisableTOTPContext(ctx context.Context, userID uint, req TOTPVerifyRequest) error {
	user, err := s.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	if !user.TOTPEnabled || user.TOTPSecret == "" {
		return ErrTOTPNotConfigured
	}
	if err := verifyCurrentPasswordHash(user.Password, req.CurrentPassword); err != nil {
		return err
	}
	if !validateTOTPCode(req.Code, user.TOTPSecret) {
		return ErrTOTPInvalid
	}
	return s.userDAO.DisableTOTPContext(ctx, userID)
}

func (s *UserService) RegenerateTOTPRecoveryCodesContext(ctx context.Context, userID uint, req TOTPVerifyRequest) (*TOTPRecoveryCodesResponse, error) {
	user, err := s.userDAO.GetUserByIDContext(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if !user.TOTPEnabled || user.TOTPSecret == "" {
		return nil, ErrTOTPNotConfigured
	}
	if err := verifyCurrentPasswordHash(user.Password, req.CurrentPassword); err != nil {
		return nil, err
	}
	if !validateTOTPCode(req.Code, user.TOTPSecret) {
		return nil, ErrTOTPInvalid
	}

	codes, hashes, err := generateTOTPRecoveryCodes(totpRecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if err := s.userDAO.ReplaceTOTPRecoveryCodesContext(ctx, userID, hashes, time.Now()); err != nil {
		return nil, err
	}
	return &TOTPRecoveryCodesResponse{RecoveryCodes: codes}, nil
}

func (s *UserService) VerifyTOTPLoginContext(ctx context.Context, req VerifyTOTPLoginRequest) (*LoginResponse, error) {
	return s.VerifyTOTPLoginWithAccessTTLContext(ctx, req, 0)
}

func (s *UserService) VerifyTOTPLoginWithAccessTTLContext(ctx context.Context, req VerifyTOTPLoginRequest, accessTTL time.Duration) (*LoginResponse, error) {
	claims, err := jwt.ParseTOTPChallenge(strings.TrimSpace(req.ChallengeID))
	if err != nil {
		return nil, err
	}

	user, err := s.userDAO.GetUserByIDContext(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}
	if !user.TOTPEnabled || user.TOTPSecret == "" {
		return nil, ErrTOTPNotConfigured
	}
	if !validateTOTPCode(req.Code, user.TOTPSecret) {
		accepted, err := s.consumeTOTPRecoveryCodeContext(ctx, user.ID, req.Code)
		if err != nil {
			return nil, err
		}
		if !accepted {
			return nil, ErrTOTPInvalid
		}
	}
	if err := consumeTOTPChallengeContext(ctx, claims); err != nil {
		return nil, err
	}

	if userWithRoles, err := s.userDAO.GetUserWithRolesAndPermissionsContext(ctx, user.ID); err == nil {
		user = userWithRoles
	}

	tenantID := user.TenantID
	if tenantID == 0 {
		tenantID = jwt.NormalizeTenantID(claims.TenantID)
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

func validateTOTPCode(code, secret string) bool {
	code = strings.TrimSpace(code)
	if code == "" || secret == "" {
		return false
	}
	return totp.Validate(code, secret)
}

func verifyCurrentPasswordHash(passwordHash, currentPassword string) error {
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(currentPassword)) != nil {
		return ErrOldPasswordIncorrect
	}
	return nil
}

func generateTOTPRecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 {
		return nil, nil, nil
	}

	codes := make([]string, 0, count)
	hashes := make([]string, 0, count)
	for len(codes) < count {
		code, err := generateTOTPRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(normalizeTOTPRecoveryCode(code)), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, string(hash))
	}
	return codes, hashes, nil
}

func generateTOTPRecoveryCode() (string, error) {
	raw := make([]byte, 9)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	return encoded[:5] + "-" + encoded[5:10] + "-" + encoded[10:15], nil
}

func normalizeTOTPRecoveryCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, strings.TrimSpace(code))
}

func isTOTPRecoveryCodeFormat(normalized string) bool {
	if len(normalized) != 15 {
		return false
	}
	for _, r := range normalized {
		if r < 'A' || r > 'Z' {
			if r < '2' || r > '7' {
				return false
			}
		}
	}
	return true
}

func (s *UserService) consumeTOTPRecoveryCodeContext(ctx context.Context, userID uint, code string) (bool, error) {
	normalized := normalizeTOTPRecoveryCode(code)
	if !isTOTPRecoveryCodeFormat(normalized) {
		return false, nil
	}

	codes, err := s.userDAO.ListUnusedTOTPRecoveryCodesContext(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, item := range codes {
		if bcrypt.CompareHashAndPassword([]byte(item.CodeHash), []byte(normalized)) != nil {
			continue
		}
		if err := s.userDAO.MarkTOTPRecoveryCodeUsedContext(ctx, userID, item.ID, time.Now()); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func consumeTOTPChallengeContext(ctx context.Context, claims *jwt.Claims) error {
	if claims == nil || claims.ID == "" || claims.ExpiresAt == nil {
		return jwt.ErrInvalidToken
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return jwt.ErrExpiredToken
	}
	consumed, err := jwt.ConsumeTokenID(ctx, claims.ID, ttl)
	if err != nil {
		return err
	}
	if !consumed {
		return jwt.ErrRevokedToken
	}
	return nil
}
