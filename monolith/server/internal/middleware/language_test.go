package middleware

import (
	"os"
	"regexp"
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

			if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
				t.Fatalf("%s contains non-English internal source text", file)
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
