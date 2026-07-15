package common

import (
	"os"
	"regexp"
	"testing"
)

func TestCommonAPIUsesEnglishSourceText(t *testing.T) {
	for _, filename := range []string{
		"health.go",
		"ipinfo.go",
	} {
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}

		if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
			t.Fatalf("%s contains non-English source text", filename)
		}
	}
}
