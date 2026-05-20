package system

import (
	"os"
	"strings"
	"testing"
)

func TestRoleAPICommentsUseEnglish(t *testing.T) {
	content, err := os.ReadFile("role.go")
	if err != nil {
		t.Fatalf("read role.go: %v", err)
	}

	source := string(content)
	for _, phrase := range []string{
		"角色",
		"创建",
		"获取",
		"更新",
		"删除",
		"权限",
		"默认分页",
		"分配",
	} {
		if strings.Contains(source, phrase) {
			t.Fatalf("role.go contains non-English phrase %q", phrase)
		}
	}
}
