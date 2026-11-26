package auth

import (
	"github.com/go-admin-kit/services/shared/pkg/consoleauth"
	model "github.com/go-admin-kit/services/shared/pkg/model"
)

// ConsoleRoleCodes extracts the sorted, de-duplicated role codes carried on a
// console session.
func ConsoleRoleCodes(roles []model.Role) []string {
	return consoleauth.ConsoleRoleCodes(roles)
}
