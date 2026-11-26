package auth

import (
	"time"

	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
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

type ConsoleSessionService = consoleauth.ConsoleSessionService

func NewConsoleSessionServiceWithDB(db *gorm.DB) ConsoleSessionService {
	return consoleauth.NewConsoleSessionServiceWithDB(db)
}

func ConsoleSessionTouchInterval() time.Duration {
	return consoleauth.ConsoleSessionTouchInterval()
}
