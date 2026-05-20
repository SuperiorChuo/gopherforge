package system

import (
	"os"
	"strings"
	"testing"
)

func TestUserAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("user.go")
	if err != nil {
		t.Fatalf("read user.go: %v", err)
	}

	source := string(content)
	for _, phrase := range []string{
		"用户",
		"创建",
		"获取",
		"更新",
		"删除",
		"状态",
		"分配",
		"角色",
		"默认分页",
		"解析状态",
		"前端期望字段",
	} {
		if strings.Contains(source, phrase) {
			t.Fatalf("user.go contains non-English phrase %q", phrase)
		}
	}
}
