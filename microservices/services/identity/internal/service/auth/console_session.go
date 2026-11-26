package auth

import (
	"context"
	"time"

	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

const (
	defaultConsoleSessionTouchInterval = consoleauth.DefaultConsoleSessionTouchInterval
	envConsoleSessionTouchInterval     = consoleauth.EnvConsoleSessionTouchInterval
)

var (
	ErrConsoleSessionInvalid = consoleauth.ErrConsoleSessionInvalid
	ErrConsoleSessionRevoked = consoleauth.ErrConsoleSessionRevoked
	ErrConsoleSessionExpired = consoleauth.ErrConsoleSessionExpired
)

type (
	ConsoleSessionService  = consoleauth.ConsoleSessionService
	ConsoleSessionUser     = consoleauth.ConsoleSessionUser
	ConsoleSessionResponse = consoleauth.ConsoleSessionResponse
)

func NewConsoleSessionServiceWithDB(db *gorm.DB) ConsoleSessionService {
	return consoleauth.NewConsoleSessionServiceWithDB(db)
}

func ConsoleSessionTouchInterval() time.Duration {
	return consoleauth.ConsoleSessionTouchInterval()
}

func ConsoleSessionSnapshot(record *model.ConsoleSession) map[string]any {
	return consoleauth.ConsoleSessionSnapshot(record)
}

func BuildConsoleSession(ctx context.Context, user *localmodel.User, permissions []string, accessToken, refreshToken string) ConsoleSessionResponse {
	return consoleauth.BuildConsoleSessionFromInput(ctx, consoleauth.ConsoleSessionUserInput{
		ID:                 user.ID,
		Username:           user.Username,
		Nickname:           user.Nickname,
		Avatar:             user.Avatar,
		MustChangePassword: user.MustChangePassword,
		Roles:              user.Roles,
	}, permissions, accessToken, refreshToken)
}

func hashSummary(value string) string {
	return consoleauth.HashSummary(value)
}

func truncateRunes(value string, maxRunes int) string {
	return consoleauth.TruncateRunes(value, maxRunes)
}
