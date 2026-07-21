package system

import (
	"os"
	"regexp"
	"testing"
)

func TestPostAPIMessagesUseEnglish(t *testing.T) {
	content, err := os.ReadFile("post.go")
	if err != nil {
		t.Fatalf("read post.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("post.go contains non-English source text")
	}
}
