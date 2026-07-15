package system

import (
	"os"
	"regexp"
	"testing"
)

func TestLoginLogAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("login_log.go")
	if err != nil {
		t.Fatalf("read login_log.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("login_log.go contains non-English source text")
	}
}

func TestLoginLogAPIInternalErrorsDoNotExposeDetails(t *testing.T) {
	content, err := os.ReadFile("login_log.go")
	if err != nil {
		t.Fatalf("read login_log.go: %v", err)
	}

	if regexp.MustCompile(`InternalServerError\(c,\s*.*err\.Error\(\)`).Find(content) != nil {
		t.Fatal("login_log.go exposes internal error details in 500 responses")
	}
}
