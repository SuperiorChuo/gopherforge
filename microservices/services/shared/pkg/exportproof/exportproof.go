// Package exportproof issues short-lived, single-use proofs for exporting
// sensitive resources. Raw proofs are never stored in Redis: keys contain only
// their SHA-256 digest and values bind the proof to one actor and resource.
package exportproof

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	ProofTTL = 2 * time.Minute

	ResourceTypeEdgeCertificate   = "edge_certificate"
	AudienceEdgeCertificateExport = "edge-cert-private-key-export"

	redisKeyPrefix = "exportproof:v1:"
	tokenBytes     = 32
)

var (
	ErrStoreUnavailable = errors.New("exportproof: store unavailable")
	ErrInvalidBinding   = errors.New("exportproof: invalid binding")
	ErrProofInvalid     = errors.New("exportproof: proof is invalid or expired")
	ErrProofCollision   = errors.New("exportproof: proof collision")
)

// Binding is the complete authorization context captured after step-up. A
// proof is valid only when every field matches the consuming operation.
type Binding struct {
	UserID       uint64 `json:"user_id"`
	SessionID    string `json:"session_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   uint64 `json:"resource_id"`
	Audience     string `json:"audience"`
}

// RedisClient is the atomic Redis subset needed by Store.
type RedisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *goredis.BoolCmd
	GetDel(ctx context.Context, key string) *goredis.StringCmd
}

// Store persists proof bindings in Redis.
type Store struct {
	client RedisClient
	random io.Reader
}

// NewStore creates a proof store. A nil client is accepted at construction so
// missing runtime dependencies can fail closed when Issue or Consume is called.
func NewStore(client RedisClient) *Store {
	if redisClientIsNil(client) {
		client = nil
	}
	return &Store{client: client, random: rand.Reader}
}

// Issue creates a random, two-minute proof bound to one export operation.
func (s *Store) Issue(ctx context.Context, binding Binding) (string, error) {
	if s == nil || s.client == nil {
		return "", ErrStoreUnavailable
	}
	if !validBinding(binding) {
		return "", ErrInvalidBinding
	}
	payload, err := json.Marshal(binding)
	if err != nil {
		return "", ErrInvalidBinding
	}

	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return "", ErrStoreUnavailable
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	stored, err := s.client.SetNX(ctx, redisKey(token), payload, ProofTTL).Result()
	if err != nil {
		return "", ErrStoreUnavailable
	}
	if !stored {
		return "", ErrProofCollision
	}
	return token, nil
}

// Consume atomically deletes a proof before validating its operation binding.
// Consequently a mismatched attempt cannot probe and then replay the proof.
func (s *Store) Consume(ctx context.Context, token string, expected Binding) error {
	if s == nil || s.client == nil {
		return ErrStoreUnavailable
	}
	if !validBinding(expected) || !validToken(token) {
		return ErrProofInvalid
	}

	data, err := s.client.GetDel(ctx, redisKey(token)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return ErrProofInvalid
	}
	if err != nil {
		return ErrStoreUnavailable
	}

	var actual Binding
	if err := json.Unmarshal(data, &actual); err != nil || actual != expected || !validBinding(actual) {
		return ErrProofInvalid
	}
	return nil
}

func redisKey(token string) string {
	digest := sha256.Sum256([]byte(token))
	return redisKeyPrefix + hex.EncodeToString(digest[:])
}

func validBinding(binding Binding) bool {
	return binding.UserID > 0 && binding.ResourceID > 0 &&
		validOpaqueID(binding.SessionID) &&
		validLabel(binding.ResourceType) && validLabel(binding.Audience)
}

func validOpaqueID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 256 {
		return false
	}
	for _, char := range value {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}

func validLabel(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == ':' {
			continue
		}
		return false
	}
	return true
}

func validToken(token string) bool {
	if token == "" || token != strings.TrimSpace(token) {
		return false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(raw) == tokenBytes
}

func redisClientIsNil(client RedisClient) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
