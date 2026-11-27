package wsticket

import (
	"errors"
	"testing"
	"time"
)

func TestMintAndSingleUseConsume(t *testing.T) {
	secret := "test-ws-ticket-secret-at-least-32-chars"
	store := NewStore()

	ticket, err := Mint(secret, "cc_ws", 42, 3, "alice", time.Minute)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}

	// Wrong scope must be rejected without consuming the ticket for the real scope.
	if _, err := store.Consume(secret, "im_ws", ticket); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("wrong scope error = %v, want ErrInvalidTicket", err)
	}

	claims, err := store.Consume(secret, "cc_ws", ticket)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if claims.UserID != 42 || claims.TenantID != 3 || claims.Username != "alice" || claims.TokenType != TokenType {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	// Second use of the same ticket must fail.
	if _, err := store.Consume(secret, "cc_ws", ticket); !errors.Is(err, ErrTicketUsed) {
		t.Fatalf("replay error = %v, want ErrTicketUsed", err)
	}
}

func TestConsumeRejectsExpiredAndPrunes(t *testing.T) {
	secret := "test-ws-ticket-secret-at-least-32-chars"
	now := time.Now()
	store := NewStore()
	store.now = func() time.Time { return now }

	t1, err := Mint(secret, "im_ws", 7, 1, "bob", time.Minute)
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	if _, err := store.Consume(secret, "im_ws", t1); err != nil {
		t.Fatalf("Consume(t1) error = %v", err)
	}
	if len(store.used) != 1 {
		t.Fatalf("used len = %d, want 1", len(store.used))
	}

	now = now.Add(2 * time.Minute)
	t2, err := Mint(secret, "im_ws", 8, 1, "carol", time.Minute)
	if err != nil {
		t.Fatalf("Mint(t2) error = %v", err)
	}
	if _, err := store.Consume(secret, "im_ws", t2); err != nil {
		t.Fatalf("Consume(t2) error = %v", err)
	}
	if len(store.used) != 1 {
		t.Fatalf("expired entry was not pruned, used len = %d", len(store.used))
	}
}
