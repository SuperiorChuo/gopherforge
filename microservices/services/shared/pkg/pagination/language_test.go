package pagination

import (
	"os"
	"regexp"
	"testing"
)

func TestPaginationPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("pagination.go")
	if err != nil {
		t.Fatalf("read pagination.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("pagination.go contains non-English source text")
	}
}
