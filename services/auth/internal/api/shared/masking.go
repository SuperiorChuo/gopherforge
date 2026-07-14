package shared

import "strings"

func ShouldMask(actorUserID uint, targetUserID *uint, roleCodes []string) bool {
	if targetUserID != nil && actorUserID != 0 && *targetUserID == actorUserID {
		return false
	}
	if HasRole(roleCodes, "super_admin") {
		return false
	}
	return true
}

func HasRole(roleCodes []string, target string) bool {
	for _, roleCode := range roleCodes {
		if strings.TrimSpace(roleCode) == target {
			return true
		}
	}
	return false
}
