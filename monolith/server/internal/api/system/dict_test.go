package system

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDictAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("dict.go")
	if err != nil {
		t.Fatalf("read dict.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("dict.go contains non-English source text")
	}
}

func TestSystemAuxBindingErrorsUseStableMessages(t *testing.T) {
	files := []string{
		"dict.go",
		"notice.go",
		"file.go",
		"operation_log.go",
		"login_log.go",
		"audit_log.go",
	}
	directBindError := regexp.MustCompile(`response\.BadRequest\(c,\s*err\.Error\(\)\)`)

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			source := string(content)

			if directBindError.Find(content) != nil {
				t.Fatalf("%s exposes bind/decoder error details directly", file)
			}
			if strings.Contains(source, "ShouldBindJSON") && !strings.Contains(source, `"invalid request body"`) {
				t.Fatalf("%s does not use a stable JSON bind error message", file)
			}
			if strings.Contains(source, "ShouldBindQuery") && !strings.Contains(source, `"invalid query parameters"`) {
				t.Fatalf("%s does not use a stable query bind error message", file)
			}
		})
	}
}
