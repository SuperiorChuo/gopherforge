package auth

import (
	"sort"
	"strings"

	model "github.com/go-admin-kit/services/shared/pkg/model"
)

// ConsoleRoleCodes 提取控制台会话携带的、去重并排序后的角色码。
// 与 services/auth 保持一致；此处仅会话校验路径需要使用。
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
