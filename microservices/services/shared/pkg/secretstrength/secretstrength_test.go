package secretstrength

import "testing"

func TestIsWeakCredential(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want bool
	}{
		{"empty", "", true},
		{"placeholder-changeme", "changeme", true},
		{"placeholder-your-key", "your-secret-key", true},
		{"weak-123456", "123456", true},
		{"weak-admin", "admin", true},
		{"weak-aws-key", "aws-access-key-id", true},
		{"dev-prefix", "dev-something", true},
		{"dev-im-token", "dev-im-ai-internal-token", true},
		{"strong-random", "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6", false},
		{"strong-complex", "xK9#mP2$vL7@nQ4", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsWeakCredential(c.val); got != c.want {
				t.Fatalf("IsWeakCredential(%q) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

func TestIsPlaceholderValue(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want bool
	}{
		{"empty", "", true},
		{"change-me", "change-me", true},
		{"contains-placeholder", "my-placeholder-value", true},
		{"contains-replace-with", "replace-with-something", true},
		{"your-prefix", "your-api-key", true},
		{"normal-value", "prod-api-key-2024", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsPlaceholderValue(c.val); got != c.want {
				t.Fatalf("IsPlaceholderValue(%q) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

func TestIsStrongSecret(t *testing.T) {
	if !IsStrongSecret("a-strong-secret-key-that-is-32-chars-ok", 32) {
		t.Fatal("strong secret should pass")
	}
	if IsStrongSecret("123456", 32) {
		t.Fatal("weak secret should fail even if long enough")
	}
	if IsStrongSecret("short", 32) {
		t.Fatal("short secret should fail")
	}
}

func TestOAuthConfigValueReady(t *testing.T) {
	if OAuthConfigValueReady("") {
		t.Fatal("empty should not be ready")
	}
	if OAuthConfigValueReady("changeme") {
		t.Fatal("placeholder should not be ready")
	}
	if !OAuthConfigValueReady("real-client-id-12345") {
		t.Fatal("real value should be ready")
	}
}

func TestNormalizeSecretValue(t *testing.T) {
	if got := NormalizeSecretValue("  Hello World  "); got != "hello world" {
		t.Fatalf("NormalizeSecretValue = %q, want %q", got, "hello world")
	}
}
