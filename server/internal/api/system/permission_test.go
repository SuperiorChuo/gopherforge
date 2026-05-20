package system

import (
	"os"
	"strings"
	"testing"
)

func TestPermissionAPICommentsUseEnglish(t *testing.T) {
	content, err := os.ReadFile("permission.go")
	if err != nil {
		t.Fatalf("read permission.go: %v", err)
	}

	source := string(content)
	for _, phrase := range []string{
		"权限",
		"创建",
		"获取",
		"更新",
		"删除",
		"默认分页",
		"解析类型",
	} {
		if strings.Contains(source, phrase) {
			t.Fatalf("permission.go contains non-English phrase %q", phrase)
		}
	}
}
