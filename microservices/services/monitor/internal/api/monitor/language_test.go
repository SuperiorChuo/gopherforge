package monitor

import (
	"os"
	"regexp"
	"testing"
)

func TestMonitorAPIUsesEnglishSourceText(t *testing.T) {
	for _, filename := range []string{
		"job.go",
		"server.go",
		"mysql.go",
		"redis.go",
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

func TestMonitorAPIInternalErrorsDoNotExposeDetails(t *testing.T) {
	for _, filename := range []string{
		"job.go",
		"server.go",
		"mysql.go",
		"redis.go",
	} {
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}

		if regexp.MustCompile(`InternalServerError\(c,\s*.*err\.Error\(\)`).Find(content) != nil {
			t.Fatalf("%s exposes internal error details in 500 responses", filename)
		}
	}
}
