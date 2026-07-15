package authjwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleGuest = "im_guest"
	RoleAgent = "im_agent" // only used when we mint; agent access tokens use user_id claims from auth-service
)

type GuestClaims struct {
	Role      string `json:"role"`
	VisitorID uint64 `json:"visitor_id"`
	SiteID    uint64 `json:"site_id"`
	GuestKey  string `json:"guest_key"`
	jwt.RegisteredClaims
}

type AgentClaims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func MintGuest(secret string, visitorID, siteID uint64, guestKey string, ttl time.Duration) (string, error) {
	claims := GuestClaims{
		Role:      RoleGuest,
		VisitorID: visitorID,
		SiteID:    siteID,
		GuestKey:  guestKey,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   fmt.Sprintf("guest:%d", visitorID),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func ParseGuest(secret, token string) (*GuestClaims, error) {
	t, err := jwt.ParseWithClaims(token, &GuestClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := t.Claims.(*GuestClaims)
	if !ok || !t.Valid || c.Role != RoleGuest || c.VisitorID == 0 {
		return nil, errors.New("invalid guest token")
	}
	return c, nil
}

// ParseAgent parses auth-service access tokens (user_id claim).
func ParseAgent(secret, token string) (*AgentClaims, error) {
	t, err := jwt.ParseWithClaims(token, &AgentClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := t.Claims.(*AgentClaims)
	if !ok || !t.Valid || c.UserID == 0 {
		return nil, errors.New("invalid agent token")
	}
	return c, nil
}
