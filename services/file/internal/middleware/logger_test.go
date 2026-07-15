package middleware

import (
	"net/url"
	"strings"
	"testing"
)

func TestRequestLogPathMasksSensitiveQueryTokens(t *testing.T) {
	u, err := url.Parse("/api/v1/ws/notifications?ticket=secret-ticket&token=secret-token&page=1&refresh_token=refresh-value")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	got := requestLogPath(u)
	if got != "/api/v1/ws/notifications?page=1&refresh_token=%2A%2A%2A&ticket=%2A%2A%2A&token=%2A%2A%2A" {
		t.Fatalf("requestLogPath() = %q", got)
	}
	if containsAny(got, "secret-ticket", "secret-token", "refresh-value") {
		t.Fatalf("requestLogPath() leaked sensitive query value: %q", got)
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
