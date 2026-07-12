package architecture

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var allowedFallbackGlobalReferences = map[string]int{
	"middleware/login_limit.go|redisClient|redis.Client":                 1,
	"middleware/rate_limit.go|redisClient|redis.Client":                  1,
	"pkg/authz/data_scope.go|GetDepartmentTree|redis.Client":             2,
	"pkg/authz/data_scope.go|InvalidateDepartmentTree|redis.Client":      1,
	"pkg/authz/data_scope.go|deleteRemote|redis.Client":                  2,
	"pkg/authz/data_scope.go|ListDepartments|database.DB":                1,
	"pkg/authz/data_scope.go|ListRoleDataScopeDepartmentIDs|database.DB": 1,
	"pkg/authz/data_scope.go|SetDepartmentTree|redis.Client":             2,
	"pkg/cache/cache.go|redisClient|redis.Client":                        1,
	"pkg/jwt/jwt.go|HasTokenID|redis.Client":                             2,
	"pkg/jwt/jwt.go|SetTokenID|redis.Client":                             2,
	"pkg/jwt/jwt.go|ConsumeTokenID|redis.Client":                         2,
	"pkg/runtimeconfig/security_policy.go|GetByKeyContext|database.DB":   1,
	"service/monitor/redis.go|redisClient|redis.Client":                  1,
	"service/system/online_user.go|redisClient|redis.Client":             1,
}

func TestDAOServicePkgMiddlewareGlobalFallbacksStayAllowlisted(t *testing.T) {
	scanDirs := []string{"dao", "service", "pkg", "middleware"}
	allowances := cloneAllowances(allowedFallbackGlobalReferences)
	var violations []string

	for _, dir := range scanDirs {
		root := filepath.Join(internalDir, dir)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fileViolations, err := globalClientReferences(path, allowances)
			if err != nil {
				return err
			}
			violations = append(violations, fileViolations...)
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s files: %v", dir, err)
		}
	}

	violations = append(violations, unusedAllowances(allowances)...)
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("DAO/service/pkg/middleware global DB/Redis fallbacks must stay on the explicit compatibility allowlist:\n%s", strings.Join(violations, "\n"))
	}
}

func unusedAllowances(allowances map[string]int) []string {
	var violations []string
	for key, remaining := range allowances {
		if remaining > 0 {
			violations = append(violations, "unused allowlist entry: "+key)
		}
	}
	return violations
}
