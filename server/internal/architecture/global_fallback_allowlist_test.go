package architecture

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var allowedFallbackGlobalReferences = map[string]int{
	"dao/auth/console_route.go|NewConsoleRouteDAO|database.DB":           1,
	"dao/auth/console_session.go|Ready|database.DB":                      1,
	"dao/auth/console_session.go|dbWithContext|database.DB":              1,
	"dao/auth/oauth.go|dbWithContext|database.DB":                        1,
	"dao/auth/permission.go|dbWithContext|database.DB":                   1,
	"dao/auth/user.go|dbWithContext|database.DB":                         1,
	"dao/monitor/job.go|NewJobDAO|database.DB":                           1,
	"dao/monitor/job.go|Ready|database.DB":                               1,
	"dao/monitor/job.go|dbWithContext|database.DB":                       1,
	"dao/monitor/mysql.go|ConnectionStatsContext|database.DB":            1,
	"dao/monitor/mysql.go|NewMySQLDAO|database.DB":                       1,
	"dao/monitor/mysql.go|dbWithContext|database.DB":                     1,
	"dao/system/audit_log.go|dbWithContext|database.DB":                  1,
	"dao/system/department.go|dbWithContext|database.DB":                 1,
	"dao/system/dict.go|dbWithContext|database.DB":                       1,
	"dao/system/file.go|dbWithContext|database.DB":                       1,
	"dao/system/login_log.go|dbWithContext|database.DB":                  1,
	"dao/system/menu.go|dbWithContext|database.DB":                       1,
	"dao/system/menu_seed.go|baseDB|database.DB":                         1,
	"dao/system/notice.go|dbWithContext|database.DB":                     1,
	"dao/system/operation_log.go|dbWithContext|database.DB":              1,
	"dao/system/permission.go|dbWithContext|database.DB":                 1,
	"dao/system/permission_cache.go|dbWithContext|database.DB":           1,
	"dao/system/role.go|dbWithContext|database.DB":                       1,
	"dao/system/user.go|dbWithContext|database.DB":                       1,
	"dao/user.go|dbWithContext|database.DB":                              1,
	"middleware/login_limit.go|redisClient|redis.Client":                 1,
	"middleware/metrics.go|DatabaseStats|database.DB":                    2,
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
