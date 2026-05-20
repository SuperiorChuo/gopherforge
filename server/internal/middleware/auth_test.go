package middleware

import "testing"

func TestHasAnyRequiredPermission(t *testing.T) {
	tests := []struct {
		name        string
		granted     []string
		required    []string
		wantAllowed bool
	}{
		{
			name:        "single required permission",
			granted:     []string{"system:user:list"},
			required:    []string{"system:user:list"},
			wantAllowed: true,
		},
		{
			name:        "any required permission",
			granted:     []string{"system:user:update"},
			required:    []string{"system:user:list", "system:user:update"},
			wantAllowed: true,
		},
		{
			name:        "wildcard permission",
			granted:     []string{"*:*:*"},
			required:    []string{"system:user:delete"},
			wantAllowed: true,
		},
		{
			name:        "missing permission",
			granted:     []string{"system:user:list"},
			required:    []string{"system:user:update", "system:user:delete"},
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAnyRequiredPermission(tt.granted, tt.required)
			if got != tt.wantAllowed {
				t.Fatalf("hasAnyRequiredPermission() = %v, want %v", got, tt.wantAllowed)
			}
		})
	}
}
