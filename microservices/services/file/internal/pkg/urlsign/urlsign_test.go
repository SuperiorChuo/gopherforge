package urlsign

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSignerVerifyValidSignature(t *testing.T) {
	signer := New("test-secret", 10*time.Minute)
	now := time.Unix(1_700_000_000, 0)

	expires, signature, err := signer.Sign("2024/01/02/abcdef.png", now)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	if err := signer.Verify("2024/01/02/abcdef.png", expires, signature, now.Add(9*time.Minute)); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Leading slash and redundant segments must not break verification.
	if err := signer.Verify("/2024/01/02/abcdef.png", expires, signature, now); err != nil {
		t.Fatalf("verify with leading slash failed: %v", err)
	}
}

func TestSignerVerifyExpiredSignature(t *testing.T) {
	signer := New("test-secret", time.Minute)
	now := time.Unix(1_700_000_000, 0)

	expires, signature, err := signer.Sign("2024/01/02/abcdef.png", now)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	err = signer.Verify("2024/01/02/abcdef.png", expires, signature, now.Add(2*time.Minute))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("verify error = %v, want ErrExpired", err)
	}
}

func TestSignerVerifyTamperedSignature(t *testing.T) {
	signer := New("test-secret", time.Minute)
	now := time.Unix(1_700_000_000, 0)

	expires, signature, err := signer.Sign("2024/01/02/abcdef.png", now)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	// Tampered key: signature does not cover another object.
	if err := signer.Verify("2024/01/02/other.png", expires, signature, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered key error = %v, want ErrInvalid", err)
	}

	// Tampered expiry: extending validity must invalidate the signature.
	if err := signer.Verify("2024/01/02/abcdef.png", "9999999999", signature, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered expiry error = %v, want ErrInvalid", err)
	}

	// Tampered signature bytes.
	bad := "0" + signature[1:]
	if bad == signature {
		bad = "1" + signature[1:]
	}
	if err := signer.Verify("2024/01/02/abcdef.png", expires, bad, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered signature error = %v, want ErrInvalid", err)
	}

	// Missing parameters.
	if err := signer.Verify("2024/01/02/abcdef.png", "", "", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing params error = %v, want ErrInvalid", err)
	}
}

func TestSignerWithoutSecretIsDisabled(t *testing.T) {
	signer := New("  ", time.Minute)
	if signer.Enabled() {
		t.Fatal("signer with blank secret should be disabled")
	}
	if _, _, err := signer.Sign("a.png", time.Now()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("sign error = %v, want ErrNotConfigured", err)
	}
	if err := signer.Verify("a.png", "1", "sig", time.Now()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("verify error = %v, want ErrNotConfigured", err)
	}
}

func TestSignURLSignsUploadsAndSkipsExternal(t *testing.T) {
	signer := New("test-secret", time.Minute)
	now := time.Unix(1_700_000_000, 0)

	signed := signer.SignURL("/uploads/2024/01/02/abc.png", "/uploads", now)
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	expires := parsed.Query().Get(QueryExpires)
	signature := parsed.Query().Get(QuerySignature)
	if expires == "" || signature == "" {
		t.Fatalf("signed url %q is missing signature params", signed)
	}
	if err := signer.Verify("2024/01/02/abc.png", expires, signature, now); err != nil {
		t.Fatalf("signature from SignURL does not verify: %v", err)
	}

	// Absolute URL under the prefix is signed too.
	abs := signer.SignURL("https://cdn.example.com/uploads/2024/01/02/abc.png", "/uploads", now)
	if !strings.Contains(abs, QuerySignature+"=") {
		t.Fatalf("absolute url was not signed: %q", abs)
	}

	// URLs outside the prefix stay untouched.
	external := "https://example.com/static/logo.png"
	if got := signer.SignURL(external, "/uploads", now); got != external {
		t.Fatalf("external url changed: %q", got)
	}
	if got := signer.SignURL("", "/uploads", now); got != "" {
		t.Fatalf("empty url changed: %q", got)
	}
}
