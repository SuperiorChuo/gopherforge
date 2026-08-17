package database

import (
	"os"
	"regexp"
	"testing"
)

func TestDatabasePackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("database.go")
	if err != nil {
		t.Fatalf("read database.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("database.go contains non-English source text")
	}
}
