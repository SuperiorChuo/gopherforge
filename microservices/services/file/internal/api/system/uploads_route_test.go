package system

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/file/internal/config"
	"github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/file/internal/pkg/urlsign"
)

// newUploadsTestRouter points config.Cfg at a temp local storage dir with one
// stored object and mounts the /uploads route. It restores config.Cfg on
// cleanup because ServeStaticFiles reads the global config.
func newUploadsTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	objectKey := "2024/01/02/deadbeefcafebabe.txt"
	target := filepath.Join(dir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("static payload"), 0644); err != nil {
		t.Fatal(err)
	}

	previous := config.Cfg
	t.Cleanup(func() { config.Cfg = previous })
	config.Cfg = config.Defaults()
	config.Cfg.Upload.StorageType = "local"
	config.Cfg.Upload.LocalPath = dir
	config.Cfg.Upload.Local.Path = dir
	config.Cfg.Upload.URLSignSecret = "uploads-test-secret"
	config.Cfg.Upload.URLSignTTLSeconds = 60

	router := gin.New()
	ServeStaticFiles(router)
	return router, objectKey
}

func getUploads(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestServeStaticFilesRequiresValidSignature(t *testing.T) {
	router, objectKey := newUploadsTestRouter(t)
	signer := newUploadURLSigner()

	expires, signature, err := signer.Sign(objectKey, time.Now())
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	signedPath := "/uploads/" + objectKey + "?" + url.Values{
		urlsign.QueryExpires:   {expires},
		urlsign.QuerySignature: {signature},
	}.Encode()

	t.Run("valid signature streams the object", func(t *testing.T) {
		w := getUploads(t, router, signedPath)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
		}
		if w.Body.String() != "static payload" {
			t.Fatalf("body = %q", w.Body.String())
		}
		if cc := w.Header().Get("Cache-Control"); cc == "" || cc == "public, max-age=31536000" {
			t.Fatalf("cache-control %q must be private for signed urls", cc)
		}
	})

	t.Run("unsigned request is rejected", func(t *testing.T) {
		w := getUploads(t, router, "/uploads/"+objectKey)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("tampered key is rejected", func(t *testing.T) {
		other := "/uploads/2024/01/02/other.txt?" + url.Values{
			urlsign.QueryExpires:   {expires},
			urlsign.QuerySignature: {signature},
		}.Encode()
		w := getUploads(t, router, other)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("expired signature is rejected", func(t *testing.T) {
		pastExpires, pastSignature, err := signer.Sign(objectKey, time.Now().Add(-2*time.Minute))
		if err != nil {
			t.Fatalf("sign failed: %v", err)
		}
		expired := "/uploads/" + objectKey + "?" + url.Values{
			urlsign.QueryExpires:   {pastExpires},
			urlsign.QuerySignature: {pastSignature},
		}.Encode()
		w := getUploads(t, router, expired)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

func TestSignFileURLsSignsStoredUploadsURLs(t *testing.T) {
	previous := config.Cfg
	t.Cleanup(func() { config.Cfg = previous })
	config.Cfg = config.Defaults()
	config.Cfg.Upload.URLSignSecret = "uploads-test-secret"

	record := &model.File{
		URL:          "/uploads/2024/01/02/a.png",
		ThumbnailURL: "/uploads/2024/01/02/thumbs/a_10x10.png",
	}
	signFileURLs(record)

	for _, raw := range []string{record.URL, record.ThumbnailURL} {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if parsed.Query().Get(urlsign.QuerySignature) == "" || parsed.Query().Get(urlsign.QueryExpires) == "" {
			t.Fatalf("url %q was not signed", raw)
		}
	}
}
