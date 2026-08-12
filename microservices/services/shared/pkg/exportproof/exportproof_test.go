package exportproof

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestIssueConsumeIsSingleUseAndRawTokenIsNotStored(t *testing.T) {
	store, redisServer := newTestStore(t)
	binding := edgeCertificateBinding(7, 42)

	token, err := store.Issue(context.Background(), binding)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if !validToken(token) {
		t.Fatalf("Issue() token is malformed: %q", token)
	}
	keys := redisServer.Keys()
	if len(keys) != 1 || keys[0] != redisKey(token) || strings.Contains(keys[0], token) {
		t.Fatalf("Redis keys = %#v, want one SHA-256-derived key without raw token", keys)
	}
	value, err := redisServer.Get(keys[0])
	if err != nil {
		t.Fatalf("read proof binding: %v", err)
	}
	if strings.Contains(value, token) {
		t.Fatal("Redis proof payload contains the raw token")
	}

	if err := store.Consume(context.Background(), token, binding); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if err := store.Consume(context.Background(), token, binding); !errors.Is(err, ErrProofInvalid) {
		t.Fatalf("Consume(replay) error = %v, want ErrProofInvalid", err)
	}
}

func TestConsumeRejectsExpiredProof(t *testing.T) {
	store, redisServer := newTestStore(t)
	binding := edgeCertificateBinding(7, 42)
	token, err := store.Issue(context.Background(), binding)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	redisServer.FastForward(ProofTTL + time.Second)

	if err := store.Consume(context.Background(), token, binding); !errors.Is(err, ErrProofInvalid) {
		t.Fatalf("Consume(expired) error = %v, want ErrProofInvalid", err)
	}
}

func TestConsumeRejectsEveryBindingMismatchAndBurnsProof(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(Binding) Binding
	}{
		{name: "user", mutate: func(value Binding) Binding { value.UserID++; return value }},
		{name: "session", mutate: func(value Binding) Binding { value.SessionID = "another-session"; return value }},
		{name: "resource type", mutate: func(value Binding) Binding { value.ResourceType = "another_resource"; return value }},
		{name: "resource id", mutate: func(value Binding) Binding { value.ResourceID++; return value }},
		{name: "audience", mutate: func(value Binding) Binding { value.Audience = "another-export"; return value }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, _ := newTestStore(t)
			binding := edgeCertificateBinding(7, 42)
			token, err := store.Issue(context.Background(), binding)
			if err != nil {
				t.Fatalf("Issue() error = %v", err)
			}
			if err := store.Consume(context.Background(), token, tc.mutate(binding)); !errors.Is(err, ErrProofInvalid) {
				t.Fatalf("Consume(mismatch) error = %v, want ErrProofInvalid", err)
			}
			if err := store.Consume(context.Background(), token, binding); !errors.Is(err, ErrProofInvalid) {
				t.Fatalf("Consume(after mismatch) error = %v, want burned proof", err)
			}
		})
	}
}

func TestStoreFailsClosedWithoutRedisAndForMalformedInputs(t *testing.T) {
	binding := edgeCertificateBinding(7, 42)
	if _, err := NewStore(nil).Issue(context.Background(), binding); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("Issue(nil Redis) error = %v, want ErrStoreUnavailable", err)
	}
	if err := NewStore(nil).Consume(context.Background(), "token", binding); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("Consume(nil Redis) error = %v, want ErrStoreUnavailable", err)
	}

	store, _ := newTestStore(t)
	invalidBinding := binding
	invalidBinding.UserID = 0
	if _, err := store.Issue(context.Background(), invalidBinding); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("Issue(invalid binding) error = %v, want ErrInvalidBinding", err)
	}
	invalidBinding = binding
	invalidBinding.SessionID = ""
	if _, err := store.Issue(context.Background(), invalidBinding); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("Issue(missing session binding) error = %v, want ErrInvalidBinding", err)
	}
	if err := store.Consume(context.Background(), "not-a-proof", binding); !errors.Is(err, ErrProofInvalid) {
		t.Fatalf("Consume(malformed token) error = %v, want ErrProofInvalid", err)
	}

	var typedNil *goredis.Client
	if _, err := NewStore(typedNil).Issue(context.Background(), binding); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("Issue(typed nil Redis) error = %v, want ErrStoreUnavailable", err)
	}
}

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewStore(client), server
}

func edgeCertificateBinding(userID, certificateID uint64) Binding {
	return Binding{
		UserID:       userID,
		SessionID:    "session-123",
		ResourceType: ResourceTypeEdgeCertificate,
		ResourceID:   certificateID,
		Audience:     AudienceEdgeCertificateExport,
	}
}
