package identityclient_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 业务服务禁止直查 public.users（Phase 2C/B2）。
// 平台层（dao/auth、middleware、pkg/authz）与 identity/system/auth 本身除外。
// 本测试在 shared 模块内扫兄弟服务目录，命中 JOIN/Table("users") 即失败。

var businessServices = []string{
	"ai", "bpm", "cc", "crm", "im", "mp", "notify", "pay", "ticket", "visibility",
}

var forbiddenUsersSQL = regexp.MustCompile(`(?i)(JOIN\s+users\b|Table\(\s*"users"\s*\)|FROM\s+users\b)`)

// 允许出现的假阳性片段（规则名常量、注释里的说明等）
var allowSubstr = []string{
	`RuleUsers`,
	`"users"`, // JSON 字段名 "users": 常见；JOIN 另由正则卡
	`// `,
	`/*`,
}

func TestBusinessServicesNoDirectUsersSQL(t *testing.T) {
	// shared/pkg/identityclient → services/
	servicesRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	var offenders []string
	for _, svc := range businessServices {
		root := filepath.Join(servicesRoot, svc)
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// 平台鉴权拷贝允许直读
			rel, _ := filepath.Rel(root, path)
			if strings.Contains(rel, string(filepath.Separator)+"dao"+string(filepath.Separator)+"auth"+string(filepath.Separator)) ||
				strings.Contains(rel, string(filepath.Separator)+"middleware"+string(filepath.Separator)) ||
				strings.Contains(rel, string(filepath.Separator)+"pkg"+string(filepath.Separator)+"authz"+string(filepath.Separator)) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			text := string(data)
			if !forbiddenUsersSQL.MatchString(text) {
				return nil
			}
			// 逐行确认，排除纯注释与 JSON key
			for i, line := range strings.Split(text, "\n") {
				trim := strings.TrimSpace(line)
				if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "*") {
					continue
				}
				if !forbiddenUsersSQL.MatchString(line) {
					continue
				}
				// RuleUsers = "users" 等常量赋值
				if strings.Contains(line, "RuleUsers") || strings.Contains(line, `= "users"`) {
					continue
				}
				// gin JSON key "users":
				if strings.Contains(line, `"users":`) || strings.Contains(line, `"users" :`) {
					continue
				}
				offenders = append(offenders, relPath(servicesRoot, path)+":"+itoa(i+1)+": "+trim)
			}
			return nil
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("业务服务禁止直查 users 表（应走 identityclient）。命中 %d 处:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

func relPath(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return r
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
