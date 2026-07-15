package auth

import (
	"os"
	"regexp"
	"testing"
)

func TestUserServiceUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("user.go")
	if err != nil {
		t.Fatalf("read user.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("user.go contains non-English source text")
	}
}
