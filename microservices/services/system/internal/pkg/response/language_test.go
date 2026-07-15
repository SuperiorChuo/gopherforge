package response

import (
	"os"
	"regexp"
	"testing"
)

func TestResponsePackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("response.go")
	if err != nil {
		t.Fatalf("read response.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("response.go contains non-English source text")
	}
}
