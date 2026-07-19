package auth

import "testing"

func TestTenantCodeFromHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"acme.localhost:3000", "acme"},
		{"acme.example.com", "acme"},
		{"www.example.com", ""},
		{"localhost", ""},
		{"127.0.0.1:8000", ""},
		{"api.example.com", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := tenantCodeFromHost(tc.in); got != tc.want {
			t.Fatalf("tenantCodeFromHost(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
