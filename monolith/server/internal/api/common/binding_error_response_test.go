package common

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestBindErrorsDoNotExposeDecoderMessages(t *testing.T) {
	files := []string{
		"ipinfo.go",
	}
	assertNoBindErrorPassthrough(t, files)
}

func assertNoBindErrorPassthrough(t *testing.T, files []string) {
	t.Helper()

	pattern := regexp.MustCompile(`(?s)ShouldBind(?:JSON|Query)\([^)]*\); err != nil[^{]*\{\s*response\.BadRequest\(c,\s*err\.Error\(\)\)`)
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if pattern.Match(content) {
				t.Fatalf("%s exposes raw bind error through response.BadRequest", file)
			}
		})
	}
}

func TestIPInfoErrorsDoNotExposeClientDetails(t *testing.T) {
	content, err := os.ReadFile("ipinfo.go")
	if err != nil {
		t.Fatalf("read ipinfo.go: %v", err)
	}

	source := string(content)
	if strings.Contains(source, "response.BadRequest(c, err.Error())") {
		t.Fatal("ipinfo.go exposes client error details directly")
	}
	if !strings.Contains(source, `"failed to lookup IP information"`) {
		t.Fatal("ipinfo.go does not use a stable IP lookup error message")
	}
}
