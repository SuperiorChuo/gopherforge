package auth

import (
	"os"
	"regexp"
	"testing"
)

func TestAuthAPIUsesEnglishSourceText(t *testing.T) {
	for _, filename := range []string{
		"user.go",
		"user_dto.go",
		"menu.go",
		"oauth.go",
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

func TestAuthAPIInternalErrorsDoNotExposeDetails(t *testing.T) {
	for _, filename := range []string{
		"user.go",
		"menu.go",
		"oauth.go",
		"console_routes.go",
		"console_compat.go",
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
