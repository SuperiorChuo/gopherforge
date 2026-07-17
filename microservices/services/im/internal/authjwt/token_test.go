package authjwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGuestTokenRoundTrip(t *testing.T) {
	secret := "test-secret-at-least-32-characters!!"
	tok, err := MintGuest(secret, 9, 1, "gk", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseGuest(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.VisitorID != 9 || c.SiteID != 1 || c.GuestKey != "gk" {
		t.Fatalf("claims %#v", c)
	}
}

func TestGuestTokenRejections(t *testing.T) {
	secret := "test-secret-at-least-32-characters!!"

	t.Run("expired", func(t *testing.T) {
		tok, err := MintGuest(secret, 9, 1, "gk", -time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseGuest(secret, tok); err == nil {
			t.Fatal("expired guest token accepted")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		tok, err := MintGuest("another-secret-32-characters-long!!!", 9, 1, "gk", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseGuest(secret, tok); err == nil {
			t.Fatal("token signed with wrong secret accepted")
		}
	})

	t.Run("tampered payload", func(t *testing.T) {
		tok, err := MintGuest(secret, 9, 1, "gk", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseGuest(secret, tok[:len(tok)-3]+"xxx"); err == nil {
			t.Fatal("tampered guest token accepted")
		}
	})

	t.Run("agent token is not a guest", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &AgentClaims{
			UserID:   42,
			Username: "agent",
			TenantID: 1,
		}).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseGuest(secret, tok); err == nil {
			t.Fatal("agent token accepted as guest (role check missing)")
		}
	})
}

func TestAgentRejectsGuestToken(t *testing.T) {
	secret := "test-secret-at-least-32-characters!!"
	tok, err := MintGuest(secret, 9, 1, "gk", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// guest claims carry no user_id → must not be usable as agent token
	if _, err := ParseAgent(secret, tok); err == nil {
		t.Fatal("guest token accepted as agent")
	}
}

func TestNormalizeTenantID(t *testing.T) {
	if got := NormalizeTenantID(0); got != 1 {
		t.Fatalf("zero -> %d", got)
	}
	if got := NormalizeTenantID(7); got != 7 {
		t.Fatalf("7 -> %d", got)
	}
}

func TestParseAgentNormalizesTenant(t *testing.T) {
	secret := "test-secret-at-least-32-characters!!"
	// mint with zero tenant_id → parser should normalize to 1
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &AgentClaims{
		UserID:   42,
		Username: "agent",
		TenantID: 0,
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseAgent(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != 42 || c.TenantID != 1 {
		t.Fatalf("claims %#v", c)
	}

	tok2, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &AgentClaims{
		UserID:   42,
		Username: "agent",
		TenantID: 3,
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	c2, err := ParseAgent(secret, tok2)
	if err != nil {
		t.Fatal(err)
	}
	if c2.TenantID != 3 {
		t.Fatalf("tenant %#v", c2)
	}
}
