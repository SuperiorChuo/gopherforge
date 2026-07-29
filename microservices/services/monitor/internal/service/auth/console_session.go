package auth

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	authDAO "github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/gorm"
)

var (
	ErrConsoleSessionInvalid = errors.New("console session is invalid")
	ErrConsoleSessionRevoked = errors.New("console session has been revoked")
	ErrConsoleSessionExpired = errors.New("console session has expired")
)

const (
	defaultConsoleSessionTouchInterval = 60 * time.Second
	envConsoleSessionTouchInterval     = "CONSOLE_SESSION_TOUCH_INTERVAL_SECONDS"
)

// ConsoleSessionService persists and validates web-console cookie sessions.
type ConsoleSessionService struct {
	dao *authDAO.ConsoleSessionDAO
}

// NewConsoleSessionServiceWithDB builds a ConsoleSessionService backed by an
// injected database handle.
func NewConsoleSessionServiceWithDB(db *gorm.DB) ConsoleSessionService {
	dao := authDAO.NewConsoleSessionDAO(db)
	return ConsoleSessionService{dao: &dao}
}

func (s ConsoleSessionService) sessionDAO() authDAO.ConsoleSessionDAO {
	if s.dao != nil {
		return *s.dao
	}
	return authDAO.ConsoleSessionDAO{}
}

func (s ConsoleSessionService) ValidateActiveSessionContext(ctx context.Context, sessionID, username string) (*model.ConsoleSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	sessionDAO := s.sessionDAO()
	if sessionID == "" || !sessionDAO.Ready() {
		return nil, ErrConsoleSessionInvalid
	}

	record, err := sessionDAO.GetBySessionIDContext(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConsoleSessionInvalid
		}
		return nil, err
	}
	if record.RevokedAt != nil {
		return nil, ErrConsoleSessionRevoked
	}
	if !record.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrConsoleSessionExpired
	}
	if trimmed := strings.TrimSpace(username); trimmed != "" && record.Username != trimmed {
		return nil, ErrConsoleSessionInvalid
	}

	now := time.Now().UTC()
	if err := sessionDAO.TouchContext(ctx, record.SessionID, now, ConsoleSessionTouchInterval()); err != nil {
		return nil, err
	}
	record.LastSeenAt = &now
	return record, nil
}

// ConsoleSessionTouchInterval is the minimum age of last_seen_at before a
// validation refreshes it. Configurable via CONSOLE_SESSION_TOUCH_INTERVAL_SECONDS;
// 0 restores a write on every request.
func ConsoleSessionTouchInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv(envConsoleSessionTouchInterval))
	if raw == "" {
		return defaultConsoleSessionTouchInterval
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return defaultConsoleSessionTouchInterval
	}
	return time.Duration(seconds) * time.Second
}
