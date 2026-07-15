package config

import (
	"os"
	"regexp"
	"testing"
)

func TestConfigPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("read config source: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("config.go contains non-English source text")
	}
}
