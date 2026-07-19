package authz

import (
	"os"
	"regexp"
	"testing"
)

func TestAuthzPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("data_scope.go")
	if err != nil {
		t.Fatalf("read data scope source: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("data_scope.go contains non-English source text")
	}
}
