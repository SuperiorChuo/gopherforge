package system

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFileAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("file.go")
	if err != nil {
		t.Fatalf("read file.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("file.go contains non-English source text")
	}
}

func TestFileDownloadDispositionSanitizesFilename(t *testing.T) {
	got := fileDownloadDisposition("report\r\nfinal.txt")

	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("content disposition contains a line break: %q", got)
	}
	if !strings.HasPrefix(got, "attachment;") || !strings.Contains(got, "filename=") {
		t.Fatalf("content disposition = %q, want attachment with filename", got)
	}
}
