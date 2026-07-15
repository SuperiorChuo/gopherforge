package captcha

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
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

func TestGetTextCaptchaDoesNotExposeCodeHint(t *testing.T) {
	setupCaptchaTestRedis(t)

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
	setupCaptchaTestRedis(t)

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

func setupCaptchaTestRedis(t *testing.T) {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	oldClient := redisstore.Client
	client := goredis.NewClient(&goredis.Options{Addr: store.Addr()})
	redisstore.Client = client

	t.Cleanup(func() {
		_ = client.Close()
		redisstore.Client = oldClient
		store.Close()
	})
}
