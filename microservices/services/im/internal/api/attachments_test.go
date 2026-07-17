package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func uploadFile(t *testing.T, r *gin.Engine, token, filename string, content []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(content)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/api/v1/im/attachments", &buf)
	req.Host = testHost
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	var data map[string]any
	_ = json.Unmarshal(env.Data, &data)
	return w, data
}

func TestAttachmentUploadServeRoundtrip(t *testing.T) {
	r := newTestRouter(t)
	token := guestSession(t, r, "att-guest")

	content := []byte("hello attachment")
	w, data := uploadFile(t, r, token, "note.txt", content)
	if w.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	url, _ := data["url"].(string)
	if !strings.HasPrefix(url, "/im/uploads/") {
		t.Fatalf("unexpected url %q", url)
	}
	if data["msg_type"] != "file" {
		t.Fatalf("msg_type %v", data["msg_type"])
	}

	req := httptest.NewRequest(http.MethodGet, "http://"+testHost+url, nil)
	req.Host = testHost
	dw := httptest.NewRecorder()
	r.ServeHTTP(dw, req)
	if dw.Code != http.StatusOK {
		t.Fatalf("download: %d %s", dw.Code, dw.Body.String())
	}
	got, _ := io.ReadAll(dw.Body)
	if !bytes.Equal(got, content) {
		t.Fatalf("roundtrip mismatch: %q", got)
	}
}

func TestAttachmentRejections(t *testing.T) {
	r := newTestRouter(t)
	token := guestSession(t, r, "att-guest-2")

	t.Run("unsupported extension", func(t *testing.T) {
		w, _ := uploadFile(t, r, token, "evil.svg", []byte("<svg/>"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
	})

	t.Run("no token", func(t *testing.T) {
		w, _ := uploadFile(t, r, "not-a-token", "a.txt", []byte("x"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", w.Code)
		}
	})

	t.Run("path traversal denied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://"+testHost+"/im/uploads/..%2f..%2fetc%2fpasswd", nil)
		req.Host = testHost
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Fatalf("traversal: want 400/404, got %d", w.Code)
		}
	})

	t.Run("missing object 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://"+testHost+"/im/uploads/209901/nope.txt", nil)
		req.Host = testHost
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", w.Code)
		}
	})
}
