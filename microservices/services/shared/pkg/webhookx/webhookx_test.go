package webhookx

import (
	"context"
	"errors"
	"testing"
)

func TestValidateEndpointRejectsUnsafeDestinations(t *testing.T) {
	for _, raw := range []string{
		"http://example.com/hook",
		"https://127.0.0.1/hook",
		"https://10.0.0.1/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://user:pass@example.com/hook",
	} {
		if _, err := ValidateEndpoint(context.Background(), raw, Policy{}); err == nil {
			t.Errorf("ValidateEndpoint(%q) accepted unsafe endpoint", raw)
		}
	}
}

func TestValidateEndpointAllowsExplicitLocalTestPolicy(t *testing.T) {
	u, err := ValidateEndpoint(context.Background(), "http://127.0.0.1:8080/hook", Policy{AllowHTTP: true, AllowPrivate: true})
	if err != nil || u.String() != "http://127.0.0.1:8080/hook" {
		t.Fatalf("ValidateEndpoint() = %v, %v", u, err)
	}
}

func TestSignatureRoundTripAndTamper(t *testing.T) {
	secret := []byte("secret")
	body := []byte(`{"event_id":"evt_1"}`)
	sig := Signature(secret, 1710000000, body)
	if !Verify(secret, 1710000000, body, sig) {
		t.Fatal("signature did not verify")
	}
	if Verify(secret, 1710000000, []byte(`{"event_id":"evt_2"}`), sig) {
		t.Fatal("tampered body verified")
	}
	if errors.Is(nil, ErrUnsafeEndpoint) {
		t.Fatal("unreachable guard")
	}
}
