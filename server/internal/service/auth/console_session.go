package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	authDAO "github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/model"
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

type ConsoleSessionUser struct {
	ID                 uint     `json:"id"`
	Username           string   `json:"username"`
	DisplayName        string   `json:"display_name"`
	Role               string   `json:"role"`
	Roles              []string `json:"roles"`
	Permissions        []string `json:"permissions"`
	ActorType          string   `json:"actor_type"`
	ActorID            string   `json:"actor_id"`
	Nickname           string   `json:"nickname"`
	Avatar             string   `json:"avatar"`
	MustChangePassword bool     `json:"must_change_password"`
}

type ConsoleSessionResponse struct {
	Authenticated bool               `json:"authenticated"`
	AuthEnabled   bool               `json:"auth_enabled"`
	User          ConsoleSessionUser `json:"user"`
	ExpiresAt     string             `json:"expires_at"`
	TTLSec        int                `json:"ttl_sec"`
	AccessToken   string             `json:"access_token,omitempty"`
	RefreshToken  string             `json:"refresh_token,omitempty"`
}

func (s ConsoleSessionService) sessionDAO() authDAO.ConsoleSessionDAO {
	return authDAO.ConsoleSessionDAO{}
}

// Deprecated: use CreateFromTokenContext instead.
func (s ConsoleSessionService) CreateFromToken(token, clientIP, userAgent string) (*model.ConsoleSession, error) {
	return s.CreateFromTokenContext(context.Background(), token, clientIP, userAgent)
}

func (s ConsoleSessionService) CreateFromTokenContext(ctx context.Context, token, clientIP, userAgent string) (*model.ConsoleSession, error) {
	claims, err := jwtpkg.ParseToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	if claims.TokenType != jwtpkg.AccessTokenType || claims.ID == "" || claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return nil, ErrConsoleSessionInvalid
	}
	sessionDAO := s.sessionDAO()
	if !sessionDAO.Ready() {
		return nil, ErrConsoleSessionInvalid
	}

	now := time.Now().UTC()
	record := &model.ConsoleSession{
		SessionID:        claims.ID,
		Username:         claims.Username,
		IssuedAt:         claims.IssuedAt.UTC(),
		ExpiresAt:        claims.ExpiresAt.UTC(),
		LastSeenAt:       &now,
		ClientIPHash:     hashSummary(clientIP),
		UserAgentHash:    hashSummary(userAgent),
		UserAgentPreview: truncateRunes(strings.TrimSpace(userAgent), 255),
		CreatedAt:        now,
	}
	if err := sessionDAO.CreateContext(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// Deprecated: use ValidateActiveSessionContext instead.
func (s ConsoleSessionService) ValidateActiveSession(sessionID, username string) (*model.ConsoleSession, error) {
	return s.ValidateActiveSessionContext(context.Background(), sessionID, username)
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
	if err := sessionDAO.TouchContext(ctx, record.SessionID, now); err != nil {
		return nil, err
	}
	record.LastSeenAt = &now
	return record, nil
}

// Deprecated: use RevokeByTokenContext instead.
func (s ConsoleSessionService) RevokeByToken(token string) (*model.ConsoleSession, error) {
	return s.RevokeByTokenContext(context.Background(), token)
}

func (s ConsoleSessionService) RevokeByTokenContext(ctx context.Context, token string) (*model.ConsoleSession, error) {
	claims, err := jwtpkg.ParseToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	return s.RevokeBySessionIDContext(ctx, claims.ID)
}

// Deprecated: use RevokeBySessionIDContext instead.
func (s ConsoleSessionService) RevokeBySessionID(sessionID string) (*model.ConsoleSession, error) {
	return s.RevokeBySessionIDContext(context.Background(), sessionID)
}

func (s ConsoleSessionService) RevokeBySessionIDContext(ctx context.Context, sessionID string) (*model.ConsoleSession, error) {
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
	if record.RevokedAt == nil {
		now := time.Now().UTC()
		if err := sessionDAO.RevokeContext(ctx, record, now); err != nil {
			return nil, err
		}
		record.RevokedAt = &now
	}
	return record, nil
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

func BuildConsoleSession(user *model.User, permissions []string, accessToken, refreshToken string) ConsoleSessionResponse {
	expiresAt := time.Now().UTC().Add(time.Hour)
	if accessToken != "" {
		if claims, err := jwtpkg.ParseToken(accessToken); err == nil && claims.ExpiresAt != nil {
			expiresAt = claims.ExpiresAt.UTC()
		}
	}
	ttl := int(time.Until(expiresAt).Seconds())
	if ttl < 0 {
		ttl = 0
	}
	return ConsoleSessionResponse{
		Authenticated: true,
		AuthEnabled:   true,
		User:          buildConsoleSessionUser(user, permissions),
		ExpiresAt:     expiresAt.Format(time.RFC3339),
		TTLSec:        ttl,
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
	}
}

func buildConsoleSessionUser(user *model.User, permissions []string) ConsoleSessionUser {
	roles := ConsoleRoleCodes(user.Roles)
	role := "operator"
	if len(roles) > 0 {
		role = roles[0]
	}
	displayName := strings.TrimSpace(user.Nickname)
	if displayName == "" {
		displayName = user.Username
	}
	return ConsoleSessionUser{
		ID:                 user.ID,
		Username:           user.Username,
		DisplayName:        displayName,
		Role:               role,
		Roles:              roles,
		Permissions:        permissions,
		ActorType:          "operator",
		ActorID:            user.Username,
		Nickname:           user.Nickname,
		Avatar:             user.Avatar,
		MustChangePassword: user.MustChangePassword,
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
