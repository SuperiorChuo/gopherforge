package auth

import (
	"sort"
	"strings"

	"github.com/go-admin-kit/services/identity/internal/model"
)

// ConsoleRoleCodes extracts the sorted, de-duplicated role codes carried on a
// console session. Mirrors services/auth; only the session validation path
// needs it here.
func ConsoleRoleCodes(roles []model.Role) []string {
	set := map[string]bool{}
	for _, role := range roles {
		if code := strings.TrimSpace(role.Code); code != "" {
			set[code] = true
		}
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
