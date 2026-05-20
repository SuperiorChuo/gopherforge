package system

import (
	"os"
	"regexp"
	"testing"
)

func TestMenuAPICommentsUseEnglish(t *testing.T) {
	content, err := os.ReadFile("menu.go")
	if err != nil {
		t.Fatalf("read menu.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("menu.go contains non-English source text")
	}
}
