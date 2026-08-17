package authz

import "testing"

func TestMatchesPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		required    string
		want        bool
	}{
		{
			name:        "exact match",
			permissions: []string{"system:user:list", "system:user:update"},
			required:    "system:user:update",
			want:        true,
		},
		{
			name:        "global wildcard",
			permissions: []string{"*:*:*"},
			required:    "system:user:update",
			want:        true,
		},
		{
			name:        "legacy wildcard",
			permissions: []string{"*"},
			required:    "system:user:update",
			want:        true,
		},
		{
			name:        "list permission is not update permission",
			permissions: []string{"system:user:list"},
			required:    "system:user:update",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesPermission(tt.permissions, tt.required); got != tt.want {
				t.Fatalf("MatchesPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}
