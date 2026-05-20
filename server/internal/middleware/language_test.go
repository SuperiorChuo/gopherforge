package middleware

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestMiddlewareInternalMessagesUseEnglish(t *testing.T) {
	files := []string{
		"error_handler.go",
		"login_limit.go",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			source := string(content)
			for _, phrase := range []string{
				"请求错误",
				"内部服务器错误",
				"Panic 已恢复",
				"错误",
				"账户因登录失败次数过多已被锁定",
				"标识",
				"失败次数",
			} {
				if strings.Contains(source, phrase) {
					t.Fatalf("%s contains non-English internal message %q", file, phrase)
				}
			}
		})
	}
}

func TestMiddlewareRuntimeLogsUseEnglishSourceText(t *testing.T) {
	for _, file := range []string{
		"logger.go",
		"operation_log.go",
	} {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
			t.Fatalf("%s contains non-English source text", file)
		}
	}
}
