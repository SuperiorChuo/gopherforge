package system

import (
	"os"
	"regexp"
	"testing"
)

func TestRoleAPICommentsUseEnglish(t *testing.T) {
	content, err := os.ReadFile("role.go")
	if err != nil {
		t.Fatalf("read role.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("role.go contains non-English source text")
	}
}
