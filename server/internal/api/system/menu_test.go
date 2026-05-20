package system

import (
	"os"
	"strings"
	"testing"
)

func TestMenuAPICommentsUseEnglish(t *testing.T) {
	content, err := os.ReadFile("menu.go")
	if err != nil {
		t.Fatalf("read menu.go: %v", err)
	}
	source := string(content)
	for _, phrase := range []string{
		"菜单",
		"创建",
		"获取",
		"更新",
		"删除",
		"解析",
		"默认分页",
	} {
		if strings.Contains(source, phrase) {
			t.Fatalf("menu.go contains non-English phrase %q", phrase)
		}
	}
}
