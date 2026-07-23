package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// OAuth2 authorization codes live in Redis: short-lived and single-use. Storing
// them out of Postgres keeps the one-time GetDel semantics simple and avoids a
// cleanup job for a high-churn, ephemeral artifact.
const (
	KeyOAuth2Code    = "oauth2:code:%s"
	OAuth2CodeExpire = 10 * time.Minute
)

var ErrOAuth2CodeNotFound = errors.New("oauth2 authorization code not found")

// OAuth2CodePayload is the data bound to an authorization code. The code binds
// client_id, redirect_uri, user, scopes and the PKCE challenge so the token
// exchange can re-verify every one of them.
type OAuth2CodePayload struct {
	ClientID            string   `json:"client_id"`
	RedirectURI         string   `json:"redirect_uri"`
	UserID              uint     `json:"user_id"`
	Username            string   `json:"username"`
	TenantID            uint     `json:"tenant_id"`
	Scopes              []string `json:"scopes"`
	CodeChallenge       string   `json:"code_challenge"`
	CodeChallengeMethod string   `json:"code_challenge_method"`
	Nonce               string   `json:"nonce,omitempty"`
}

func (s *CacheService) StoreOAuth2CodeContext(ctx context.Context, code string, payload OAuth2CodePayload) error {
	client := s.redisClient()
	if client == nil {
		return ErrCacheUnavailable
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	key := fmt.Sprintf(KeyOAuth2Code, code)
	stored, err := client.SetNX(ctx, key, data, OAuth2CodeExpire).Result()
	if err != nil {
		return err
	}
	if !stored {
		return errors.New("oauth2 authorization code collision")
	}
	return nil
}

// ConsumeOAuth2CodeContext atomically fetches and deletes the code (single use).
func (s *CacheService) ConsumeOAuth2CodeContext(ctx context.Context, code string) (*OAuth2CodePayload, error) {
	client := s.redisClient()
	if client == nil {
		return nil, ErrCacheUnavailable
	}
	key := fmt.Sprintf(KeyOAuth2Code, code)
	data, err := client.GetDel(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, ErrOAuth2CodeNotFound
	}
	if err != nil {
		return nil, err
	}
	var payload OAuth2CodePayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
