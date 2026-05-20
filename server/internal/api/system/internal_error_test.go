package system

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSystemAPIInternalErrorsDoNotExposeDetails(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob system api files: %v", err)
	}

	pattern := regexp.MustCompile(`InternalServerError\(c,\s*.*err\.Error\(\)`)
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}

		if pattern.Find(content) != nil {
			t.Fatalf("%s exposes internal error details in 500 responses", filename)
		}
	}
}
