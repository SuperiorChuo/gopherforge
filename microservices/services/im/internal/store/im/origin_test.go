package store

import "testing"

func TestOriginAllowed(t *testing.T) {
	allowed := `["http://ok.test", "https://shop.example.com "]`
	cases := []struct {
		name        string
		allowedJSON string
		origin      string
		want        bool
	}{
		{"exact match", allowed, "http://ok.test", true},
		{"case insensitive", allowed, "HTTP://OK.TEST", true},
		{"config entry with stray space", allowed, "https://shop.example.com", true},
		{"origin with stray space", allowed, " http://ok.test ", true},
		{"not in list", allowed, "http://evil.test", false},
		{"scheme mismatch", allowed, "https://ok.test", false},
		{"empty origin is non-browser client", allowed, "", true},
		{"wildcard", `["*"]`, "http://anything.test", true},
		{"null entry for file pages", `["null"]`, "null", true},
		{"empty list allows all", `[]`, "http://anything.test", true},
		{"malformed json denies", `{oops`, "http://ok.test", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OriginAllowed(tc.allowedJSON, tc.origin); got != tc.want {
				t.Fatalf("OriginAllowed(%q, %q) = %v, want %v", tc.allowedJSON, tc.origin, got, tc.want)
			}
		})
	}
}
