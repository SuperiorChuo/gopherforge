package system

import (
	"os"
	"strings"
	"testing"
)

func TestDepartmentAPIMessagesUseEnglish(t *testing.T) {
	content, err := os.ReadFile("department.go")
	if err != nil {
		t.Fatalf("read department.go: %v", err)
	}
	source := string(content)
	for _, phrase := range []string{
		"部门",
		"创建成功",
		"更新成功",
		"删除成功",
		"无效",
		"不存在",
		"创建",
		"获取",
		"更新",
		"删除",
		"默认分页",
	} {
		if strings.Contains(source, phrase) {
			t.Fatalf("department.go contains non-English phrase %q", phrase)
		}
	}
}
