package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
	"gorm.io/gorm"
)

var (
	ErrConsoleSessionInvalid = errors.New("console session is invalid")
	ErrConsoleSessionRevoked = errors.New("console session has been revoked")
	ErrConsoleSessionExpired = errors.New("console session has expired")
)

// ConsoleSessionService persists and validates web-console cookie sessions.
type ConsoleSessionService struct{}

func (s ConsoleSessionService) CreateFromToken(token, clientIP, userAgent string) (*model.ConsoleSession, error) {
	claims, err := jwtpkg.ParseToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	if claims.TokenType != jwtpkg.AccessTokenType || claims.ID == "" || claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return nil, ErrConsoleSessionInvalid
	}
	if database.DB == nil {
		return nil, ErrConsoleSessionInvalid
	}

	now := time.Now().UTC()
	record := &model.ConsoleSession{
		SessionID:        claims.ID,
		Username:         claims.Username,
		IssuedAt:         claims.IssuedAt.Time.UTC(),
		ExpiresAt:        claims.ExpiresAt.Time.UTC(),
		LastSeenAt:       &now,
		ClientIPHash:     hashSummary(clientIP),
		UserAgentHash:    hashSummary(userAgent),
		UserAgentPreview: truncateRunes(strings.TrimSpace(userAgent), 255),
		CreatedAt:        now,
	}
	if err := database.DB.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (s ConsoleSessionService) ValidateActiveSession(sessionID, username string) (*model.ConsoleSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || database.DB == nil {
		return nil, ErrConsoleSessionInvalid
	}

	var record model.ConsoleSession
	if err := database.DB.First(&record, "session_id = ?", sessionID).Error; err != nil {
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
	if err := database.DB.Model(&model.ConsoleSession{}).
		Where("session_id = ?", record.SessionID).
		Update("last_seen_at", now).
		Error; err != nil {
		return nil, err
	}
	record.LastSeenAt = &now
	return &record, nil
}

func (s ConsoleSessionService) RevokeByToken(token string) (*model.ConsoleSession, error) {
	claims, err := jwtpkg.ParseToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	return s.RevokeBySessionID(claims.ID)
}

func (s ConsoleSessionService) RevokeBySessionID(sessionID string) (*model.ConsoleSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || database.DB == nil {
		return nil, ErrConsoleSessionInvalid
	}

	var record model.ConsoleSession
	if err := database.DB.First(&record, "session_id = ?", sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConsoleSessionInvalid
		}
		return nil, err
	}
	if record.RevokedAt == nil {
		now := time.Now().UTC()
		if err := database.DB.Model(&record).Update("revoked_at", now).Error; err != nil {
			return nil, err
		}
		record.RevokedAt = &now
	}
	return &record, nil
}

func ConsoleSessionSnapshot(record *model.ConsoleSession) map[string]any {
	if record == nil {
		return nil
	}
	return map[string]any{
		"session_id": record.SessionID,
		"username":   record.Username,
		"issued_at":  record.IssuedAt,
		"expires_at": record.ExpiresAt,
		"revoked_at": record.RevokedAt,
	}
}

func hashSummary(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}
