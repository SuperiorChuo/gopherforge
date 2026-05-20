package system

import (
	"os"
	"regexp"
	"testing"
)

func TestDictAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("dict.go")
	if err != nil {
		t.Fatalf("read dict.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("dict.go contains non-English source text")
	}
}
