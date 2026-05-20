package captcha

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateTextCaptchaCodeUsesReadableAlphabet(t *testing.T) {
	code, err := generateTextCaptchaCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != textCaptchaLength {
		t.Fatalf("code length = %d, want %d", len(code), textCaptchaLength)
	}
	for _, ch := range code {
		if !strings.ContainsRune(textCaptchaChars, ch) {
			t.Fatalf("code contains unsupported character %q", ch)
		}
	}
}

func TestTextCaptchaMatchesIgnoresCaseAndSpace(t *testing.T) {
	if !textCaptchaMatches("A7K9", " a7k9 ") {
		t.Fatal("expected code comparison to ignore case and surrounding spaces")
	}
	if textCaptchaMatches("A7K9", "A7K8") {
		t.Fatal("expected different codes to fail")
	}
}

func TestRenderTextCaptchaPNGReturnsBase64Image(t *testing.T) {
	imageBase64, err := renderTextCaptchaPNG("A7K9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base64.StdEncoding.DecodeString(imageBase64); err != nil {
		t.Fatalf("captcha image is not valid base64: %v", err)
	}
}
