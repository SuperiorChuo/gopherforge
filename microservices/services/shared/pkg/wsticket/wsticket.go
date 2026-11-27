// Package wsticket issues and verifies short-lived, single-use WebSocket
// handshake tickets so browsers never place long-lived access JWTs in URL
// query strings (which leak into gateway access logs and browser history).
package wsticket

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// TokenType marks short-lived WebSocket handshake tickets so API bearer
	// parsers reject them if replayed against REST endpoints.
	TokenType = "ws_ticket"

	// DefaultTTL bounds ticket lifetime to one minute.
	DefaultTTL = time.Minute
)

var (
	ErrMissingSecret = errors.New("wsticket: signing secret is empty")
	ErrInvalidTicket = errors.New("wsticket: invalid ticket")
	ErrTicketUsed    = errors.New("wsticket: ticket already consumed")
)

// Claims holds the signed identity carried by a WebSocket handshake ticket.
type Claims struct {
	UserID    uint64 `json:"user_id"`
	Username  string `json:"username,omitempty"`
	TenantID  uint64 `json:"tenant_id"`
	TokenType string `json:"token_type"`
	Scope     string `json:"scope"`
	jwt.RegisteredClaims
}

// Mint signs a short-lived, scope-bound WebSocket handshake ticket.
func Mint(secret, scope string, userID, tenantID uint64, username string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", ErrMissingSecret
	}
	if userID == 0 || strings.TrimSpace(scope) == "" {
		return "", ErrInvalidTicket
	}
	if tenantID == 0 {
		tenantID = 1
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Username:  username,
		TenantID:  tenantID,
		TokenType: TokenType,
		Scope:     scope,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.NewString(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Store verifies WebSocket tickets and enforces in-process single-use semantics.
// Consumed ticket IDs are retained only until their expiry and pruned lazily.
type Store struct {
	mu   sync.Mutex
	used map[string]time.Time
	now  func() time.Time
}

// NewStore creates a single-use ticket verifier.
func NewStore() *Store {
	return &Store{
		used: make(map[string]time.Time),
		now:  time.Now,
	}
}

// Consume validates a signed ticket for expectedScope and marks its jti used.
func (s *Store) Consume(secret, expectedScope, raw string) (*Claims, error) {
	if secret == "" {
		return nil, ErrMissingSecret
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrInvalidTicket
	}
	tok, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, ErrInvalidTicket
	}
	c, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid || c.UserID == 0 || c.ID == "" || c.ExpiresAt == nil {
		return nil, ErrInvalidTicket
	}
	if c.TokenType != TokenType || c.Scope != expectedScope {
		return nil, ErrInvalidTicket
	}
	if c.TenantID == 0 {
		c.TenantID = 1
	}
	if s != nil {
		if err := s.markUsed(c.ID, c.ExpiresAt.Time); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (s *Store) markUsed(jti string, expiresAt time.Time) error {
	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used == nil {
		s.used = make(map[string]time.Time)
	}
	for id, exp := range s.used {
		if !exp.After(now) {
			delete(s.used, id)
		}
	}
	if exp, exists := s.used[jti]; exists && exp.After(now) {
		return ErrTicketUsed
	}
	s.used[jti] = expiresAt
	return nil
}
