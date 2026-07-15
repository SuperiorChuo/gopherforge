package authjwt

import (
	"testing"
	"time"
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
