package system

import (
	"os"
	"regexp"
	"testing"
)

func TestSystemServiceEngineeringFilesUseEnglishSourceText(t *testing.T) {
	for _, file := range []string{
		"../../pkg/cache/cache.go", // moved from this package; keep it covered
		"menu_user.go",
	} {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}

			if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
				t.Fatalf("%s contains non-English source text", file)
			}
		})
	}
}
