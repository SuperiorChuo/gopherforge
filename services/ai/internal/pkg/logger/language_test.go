package logger

import (
	"os"
	"regexp"
	"testing"
)

func TestLoggerUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("logger.go")
	if err != nil {
		t.Fatalf("read logger.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("logger.go contains non-English source text")
	}
}
