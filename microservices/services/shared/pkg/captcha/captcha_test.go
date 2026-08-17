package captcha

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
)

type memoryStore struct {
	mu   sync.Mutex
	data map[string]string
	err  error
}

func (s *memoryStore) SetLoginCaptchaContext(ctx context.Context, key string, captcha string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string]string{}
	}
	s.data[key] = captcha
	return nil
}

func (s *memoryStore) GetLoginCaptchaContext(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.err != nil {
		return "", s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[key]
	if !ok {
		return "", errors.New("captcha not found")
	}
	return value, nil
}

func (s *memoryStore) DelLoginCaptchaContext(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func setupCaptchaTestStore(t *testing.T) *memoryStore {
	t.Helper()
	store := &memoryStore{}
	previous := currentStore()
	SetStore(store)
	t.Cleanup(func() {
		SetStore(previous)
	})
	return store
}

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

func TestGetTextCaptchaDoesNotExposeCodeHint(t *testing.T) {
	setupCaptchaTestStore(t)

	data, err := GetTextCaptchaContext(context.Background(), "unit-test-captcha")
	if err != nil {
		t.Fatal(err)
	}

	payload, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("captcha payload type = %T, want map[string]any", data)
	}
	if _, ok := payload["code_hint"]; ok {
		t.Fatal("captcha response must not expose code_hint")
	}
	if payload["image"] == "" {
		t.Fatal("captcha response should include an image")
	}
}

func TestTextCaptchaContextMethodsHonorCanceledContext(t *testing.T) {
	setupCaptchaTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetTextCaptchaContext(ctx, "unit-test-canceled-captcha")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetTextCaptchaContext error = %v, want context.Canceled", err)
	}
	if CheckTextCaptchaContext(ctx, "unit-test-canceled-captcha", "A7K9") {
		t.Fatal("CheckTextCaptchaContext should fail when context is canceled")
	}
	if VerifyTextCaptchaContext(ctx, "unit-test-canceled-captcha", "A7K9") {
		t.Fatal("VerifyTextCaptchaContext should fail when context is canceled")
	}
}

func TestGetTextCaptchaFailsWhenStoreMissing(t *testing.T) {
	previous := currentStore()
	SetStore(nil)
	t.Cleanup(func() {
		SetStore(previous)
	})

	if _, err := GetTextCaptchaContext(context.Background(), "missing-store"); err == nil {
		t.Fatal("expected error when captcha store is missing")
	}
}
