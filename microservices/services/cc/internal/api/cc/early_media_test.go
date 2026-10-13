package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/config"
)

func testWAV() []byte {
	b := make([]byte, 80)
	copy(b, "RIFF")
	copy(b[8:], "WAVEfmt ")
	return b
}

func signEarly(raw []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestClassifyEarlyMediaRequiresOperatorPhrase(t *testing.T) {
	cases := []struct{ text, want string }{
		{"您好，您拨打的号码是空号。", "empty"},
		{"对不起，您拨打的用户已关机", "unavailable"},
		{"用户正忙，请稍后再拨", "busy"},
		{"我今天工作很忙，稍后联系", ""},
		{"普通通话内容", ""},
	}
	for _, tc := range cases {
		if got := classifyEarlyMedia(tc.text); got != tc.want {
			t.Errorf("classify(%q)=%q want %q", tc.text, got, tc.want)
		}
	}
}

func TestInternalEarlyMediaClassifySignedAndNoTranscriptLeak(t *testing.T) {
	asr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Fatalf("missing ASR auth")
		}
		_ = r.ParseMultipartForm(1 << 20)
		if r.FormValue("model") != "whisper-1" {
			t.Fatalf("model=%q", r.FormValue("model"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"您拨打的号码是空号"}`))
	}))
	defer asr.Close()

	const secret = "test-secret"
	srv := &Server{
		WebhookSecret: secret,
		Speech: config.NewSpeechReader(nil, config.SpeechSettings{
			ASRURL: asr.URL, ASRKey: "k", ASRModel: "whisper-1",
		}),
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/classify", srv.InternalEarlyMediaClassify)
	raw := testWAV()
	req := httptest.NewRequest(http.MethodPost, "/classify", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "audio/wav")
	req.Header.Set("X-CC-Signature", signEarly(raw, secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"class":"empty"`)) ||
		bytes.Contains(w.Body.Bytes(), []byte("号码是空号")) {
		t.Fatalf("unexpected body=%s", w.Body.String())
	}
}

func TestInternalEarlyMediaClassifyRejectsBadSignature(t *testing.T) {
	srv := &Server{WebhookSecret: "secret"}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/classify", srv.InternalEarlyMediaClassify)
	req := httptest.NewRequest(http.MethodPost, "/classify", bytes.NewReader(testWAV()))
	req.Header.Set("X-CC-Signature", "sha256=bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
