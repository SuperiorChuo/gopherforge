package errors

import (
	"os"
	"regexp"
	"testing"
)

func TestErrorsPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("errors.go")
	if err != nil {
		t.Fatalf("read errors.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("errors.go contains non-English source text")
	}
}
