package system

import (
	"os"
	"regexp"
	"testing"
)

func TestPermissionAPICommentsUseEnglish(t *testing.T) {
	content, err := os.ReadFile("permission.go")
	if err != nil {
		t.Fatalf("read permission.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("permission.go contains non-English source text")
	}
}
