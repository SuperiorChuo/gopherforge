package system

import (
	"os"
	"regexp"
	"testing"
)

func TestDepartmentAPIMessagesUseEnglish(t *testing.T) {
	content, err := os.ReadFile("department.go")
	if err != nil {
		t.Fatalf("read department.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("department.go contains non-English source text")
	}
}
