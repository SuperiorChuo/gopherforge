package system

import (
	"os"
	"regexp"
	"testing"
)

func TestOperationLogAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("operation_log.go")
	if err != nil {
		t.Fatalf("read operation_log.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("operation_log.go contains non-English source text")
	}
}

func TestOperationLogAPIInternalErrorsDoNotExposeDetails(t *testing.T) {
	content, err := os.ReadFile("operation_log.go")
	if err != nil {
		t.Fatalf("read operation_log.go: %v", err)
	}

	if regexp.MustCompile(`InternalServerError\(c,\s*.*err\.Error\(\)`).Find(content) != nil {
		t.Fatal("operation_log.go exposes internal error details in 500 responses")
	}
}
