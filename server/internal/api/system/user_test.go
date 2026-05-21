package system

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestUserAPIUsesEnglishSourceText(t *testing.T) {
	content, err := os.ReadFile("user.go")
	if err != nil {
		t.Fatalf("read user.go: %v", err)
	}

	if regexp.MustCompile(`\p{Han}`).Find(content) != nil {
		t.Fatal("user.go contains non-English source text")
	}
}

func TestSystemCoreBindingErrorsAreNotForwarded(t *testing.T) {
	files := []string{
		"user.go",
		"role.go",
		"permission.go",
		"menu.go",
		"department.go",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}

			if strings.Contains(string(content), "response.BadRequest(c, err.Error())") {
				t.Fatalf("%s forwards binding errors directly", file)
			}
		})
	}
}
