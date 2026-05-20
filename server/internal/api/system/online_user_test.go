package system

import (
	"os"
	"regexp"
	"testing"
)

func TestOnlineUserAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("online_user.go")
	if err != nil {
		t.Fatalf("read online_user.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("online_user.go contains non-English source text")
	}
}

func TestOnlineUserAPIInternalErrorsDoNotExposeDetails(t *testing.T) {
	content, err := os.ReadFile("online_user.go")
	if err != nil {
		t.Fatalf("read online_user.go: %v", err)
	}

	if regexp.MustCompile(`InternalServerError\(c,\s*.*err\.Error\(\)`).Find(content) != nil {
		t.Fatal("online_user.go exposes internal error details in 500 responses")
	}
}
