package cache

import (
	"os"
	"regexp"
	"testing"
)

func TestCachePackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("cache.go")
	if err != nil {
		t.Fatalf("read cache source: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("cache.go contains non-English source text")
	}
}
