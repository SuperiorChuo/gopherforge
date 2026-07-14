package redis

import (
	"os"
	"regexp"
	"testing"
)

func TestRedisPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("redis.go")
	if err != nil {
		t.Fatalf("read redis.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("redis.go contains non-English source text")
	}
}
