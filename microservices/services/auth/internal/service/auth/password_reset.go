package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/auth/internal/dao/auth"
	localmodel "github.com/go-admin-kit/services/auth/internal/model"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	"github.com/go-admin-kit/services/shared/pkg/mailer"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// resetTokenTTL bounds how long a forgot-password link stays valid.
const resetTokenTTL = 30 * time.Minute

var (
	// ErrResetTokenInvalid is the single error surfaced for any token problem
	// (unknown / used / expired) to avoid oracle enumeration.
	ErrResetTokenInvalid = errors.New("reset token is invalid")
)

// PasswordResetStore is the persistence surface the reset flow needs, so tests
// can substitute an in-memory implementation.
type PasswordResetStore interface {
	CreateContext(ctx context.Context, reset *localmodel.PasswordReset) error
	GetByTokenHashContext(ctx context.Context, tokenHash string) (*localmodel.PasswordReset, error)
	MarkUsedContext(ctx context.Context, id uint) error
	PruneExpiredContext(ctx context.Context, before time.Time) (int64, error)
}

// PasswordResetService implements the forgot-password email link flow,
// mirroring the invite-registration pattern: only a sha256(token) is stored,
// tokens are consumed atomically, and every failure collapses to one error.
type PasswordResetService struct {
	userDAO     auth.UserDAO
	resetDAO    PasswordResetStore
	emailReader runtimeconfig.EmailNotificationReader
	// sender, when non-nil, bypasses the reader-built SMTP sender (tests).
	sender mailer.Sender
	now    func() time.Time
}

func NewPasswordResetService(db *gorm.DB) *PasswordResetService {
	if db == nil {
		return &PasswordResetService{now: time.Now}
	}
	return &PasswordResetService{
		userDAO:     *auth.NewUserDAO(db),
		resetDAO:    auth.NewPasswordResetDAO(db),
		emailReader: runtimeconfig.DefaultEmailNotificationReader(),
		now:         time.Now,
	}
}

// ForgotPasswordContext issues a reset token and emails a link. It always
// returns success when the email is absent or unknown — only a rate-limit
// rejection surfaces, so responders cannot enumerate accounts.
func (s *PasswordResetService) ForgotPasswordContext(ctx context.Context, email string) error {
	if s == nil || s.resetDAO == nil {
		return nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	user, err := s.userDAO.GetUserByEmailContext(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if user == nil || user.ID == 0 || user.Status == 0 {
		return nil
	}

	token, err := generateResetToken()
	if err != nil {
		return err
	}
	reset := &localmodel.PasswordReset{
		UserID:    user.ID,
		TenantID:  user.TenantID,
		TokenHash: hashResetToken(token),
		ExpiresAt: s.now().UTC().Add(resetTokenTTL),
	}
	if err := s.resetDAO.CreateContext(ctx, reset); err != nil {
		return err
	}

	s.sendResetEmail(ctx, user, token)
	// Best-effort pruning keeps the table bounded without a background loop.
	s.prune(ctx)
	return nil
}

// ResetPasswordContext validates a token, consumes it atomically and sets the
// new password. Any invalid token collapses to ErrResetTokenInvalid.
func (s *PasswordResetService) ResetPasswordContext(ctx context.Context, token, newPassword string) error {
	if s == nil || s.resetDAO == nil {
		return ErrResetTokenInvalid
	}
	if strings.TrimSpace(token) == "" {
		return ErrResetTokenInvalid
	}
	reset, err := s.resetDAO.GetByTokenHashContext(ctx, hashResetToken(strings.TrimSpace(token)))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrResetTokenInvalid
		}
		return err
	}
	now := s.now().UTC()
	if reset.UsedAt != nil || now.After(reset.ExpiresAt) {
		return ErrResetTokenInvalid
	}
	// Atomic consumption: conditional update; only one concurrent use succeeds.
	if err := s.resetDAO.MarkUsedContext(ctx, reset.ID); err != nil {
		return ErrResetTokenInvalid
	}

	if err := validatePasswordStrength(newPassword); err != nil {
		return err
	}
	user, err := s.userDAO.GetUserByIDContext(ctx, reset.UserID)
	if err != nil {
		return ErrResetTokenInvalid
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("password hashing failed")
	}
	// History check: historyCount 1 blocks reusing the current password — the
	// important guard for a reset flow. Full history reuse-policy is enforced
	// on subsequent ChangePassword calls as before.
	return s.userDAO.UpdatePasswordWithHistoryContext(
		ctx, user.ID, user.Password, string(hashed), now, 1)
}

func (s *PasswordResetService) sendResetEmail(ctx context.Context, user *model.User, token string) {
	if s.emailReader == nil {
		return
	}
	policy := s.emailReader.EmailNotification(ctx)
	if !policy.Enabled {
		return
	}
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	sender := s.sender
	if sender == nil {
		sender = mailer.NewSMTPSender(mailer.SMTPConfig{
			Enabled:  policy.Enabled,
			SMTPHost: policy.SMTPHost,
			SMTPPort: policy.SMTPPort,
			Username: policy.Username,
			Password: policy.Password,
			Sender:   policy.Sender,
			UseTLS:   policy.UseTLS,
			StartTLS: policy.StartTLS,
		}, nil)
	}
	from := policy.Sender
	if from == "" {
		from = "no-reply@localhost"
	}
	link := fmt.Sprintf("/reset-password?token=%s", token)
	_ = sender.Send(ctx, mailer.Message{
		From:    from,
		To:      []string{user.Email},
		Subject: "重置你的 GopherForge 密码",
		Body:    resetEmailBody(link),
	})
}

func (s *PasswordResetService) prune(ctx context.Context) {
	if s.resetDAO == nil {
		return
	}
	cutoff := s.now().UTC().Add(-24 * time.Hour)
	_, _ = s.resetDAO.PruneExpiredContext(ctx, cutoff)
}

func generateResetToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}

func resetEmailBody(link string) string {
	return "你请求重置 GopherForge 密码。请打开下面的链接设置新密码（30 分钟内有效）：\n\n" +
		link + "\n\n如果这不是你的操作，请忽略这封邮件。"
}
