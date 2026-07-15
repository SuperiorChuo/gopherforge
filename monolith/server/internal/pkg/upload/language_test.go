package upload

import (
	"os"
	"regexp"
	"testing"
)

func TestUploadPackageUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("upload.go")
	if err != nil {
		t.Fatalf("read upload source: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("upload.go contains non-English source text")
	}
}
