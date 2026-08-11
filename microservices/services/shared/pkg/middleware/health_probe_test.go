package middleware

import (
	"testing"
)

func TestIsHealthProbePath(t *testing.T) {
	cases := map[string]bool{
		"/api/v1/health/live":     true,
		"/api/v1/health/ready":    true,
		"/api/v1/im/health/ready": true,
		"/api/v1/users":           false,
		"/api/v1/health/readyz":   false,
	}
	for path, want := range cases {
		if got := IsHealthProbePath(path); got != want {
			t.Errorf("IsHealthProbePath(%q) = %v, want %v", path, got, want)
		}
	}
}
