package monitor

import (
	"os"
	"regexp"
	"testing"
)

func TestMonitorDAOUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("job.go")
	if err != nil {
		t.Fatalf("read job DAO source: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("job.go contains non-English source text")
	}
}
