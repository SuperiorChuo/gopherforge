package auth

import (
	"os"
	"regexp"
	"testing"
)

func TestConsoleSessionServiceUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("console_session.go")
	if err != nil {
		t.Fatalf("read console_session.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("console_session.go contains non-English source text")
	}
}
