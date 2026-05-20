package system

import (
	"os"
	"regexp"
	"testing"
)

func TestFileAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("file.go")
	if err != nil {
		t.Fatalf("read file.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("file.go contains non-English source text")
	}
}
