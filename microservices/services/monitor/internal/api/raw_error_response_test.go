package api

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAPIResponsesDoNotForwardRawErrors(t *testing.T) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`response\.(?:BadRequest|NotFound|Unauthorized|InternalServerError)\(c,\s*err\.Error\(\)\)`),
		regexp.MustCompile(`response\.Error\(c,\s*[^,\n]+,\s*err\.Error\(\)\)`),
		regexp.MustCompile(`return\s+err\.Error\(\)`),
	}

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, pattern := range patterns {
			if pattern.Match(content) {
				t.Errorf("%s forwards err.Error() directly to an API response", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan API sources: %v", err)
	}
}
