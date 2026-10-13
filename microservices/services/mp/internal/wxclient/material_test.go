package wxclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestClient 造一对指向同一个 httptest.Server 的 TokenManager + Client。
func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	mgr := NewTokenManager(newMemCache(), staticResolver{appID: "wxtestappid", secret: "s"},
		WithTokenBaseURL(baseURL), WithLockWait(5))
	return NewClient(mgr, WithBaseURL(baseURL))
}

// TestDoJSONTokenExpiredRetry 验证：业务接口返回 40001（token 失效）时，Client
// 会 Invalidate + 重刷 token 并重试一次，第二次带上新 token 成功。
func TestDoJSONTokenExpiredRetry(t *testing.T) {
	var tokenHits int64
	var bizHits int64
	var gotTokens []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			n := atomic.AddInt64(&tokenHits, 1)
			fmt.Fprintf(w, `{"access_token":"TOK-%d","expires_in":7200}`, n)
		case "/cgi-bin/biz":
			n := atomic.AddInt64(&bizHits, 1)
			gotTokens = append(gotTokens, r.URL.Query().Get("access_token"))
			if n == 1 {
				// 第一次：模拟 token 失效。
				fmt.Fprint(w, `{"errcode":40001,"errmsg":"invalid credential"}`)
				return
			}
			fmt.Fprint(w, `{"errcode":0,"result":"ok"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	var out struct {
		Result string `json:"result"`
	}
	err := c.doJSON(context.Background(), testAccount(), http.MethodGet, "/cgi-bin/biz", nil, &out)
	if err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if out.Result != "ok" {
		t.Fatalf("result = %q, want ok", out.Result)
	}
	if bizHits != 2 {
		t.Fatalf("biz endpoint hit %d times, want 2 (fail + retry)", bizHits)
	}
	if tokenHits != 2 {
		t.Fatalf("token endpoint hit %d times, want 2 (initial + refresh)", tokenHits)
	}
	if len(gotTokens) != 2 || gotTokens[0] == gotTokens[1] {
		t.Fatalf("retry should use a fresh token, got %v", gotTokens)
	}
}

// TestDoJSONRetryOnlyOnce 验证：即便重试后仍 40001，也只重试一次（不死循环），
// 最终返回 WxError。
func TestDoJSONRetryOnlyOnce(t *testing.T) {
	var bizHits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			fmt.Fprint(w, `{"access_token":"TOK","expires_in":7200}`)
		case "/cgi-bin/biz":
			atomic.AddInt64(&bizHits, 1)
			fmt.Fprint(w, `{"errcode":40001,"errmsg":"invalid credential"}`)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.doJSON(context.Background(), testAccount(), http.MethodGet, "/cgi-bin/biz", nil, nil)
	we, ok := err.(*WxError)
	if !ok || we.Code != 40001 {
		t.Fatalf("expected WxError 40001, got %T %v", err, err)
	}
	if bizHits != 2 {
		t.Fatalf("biz endpoint hit %d times, want exactly 2 (initial + single retry)", bizHits)
	}
}

// TestUploadTempMedia 验证临时素材上传：multipart 请求正确发出、type query 传对、
// media_id 正确解析。
func TestUploadTempMedia(t *testing.T) {
	var gotType, gotField, gotFilename string
	var gotFileBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			fmt.Fprint(w, `{"access_token":"TOK","expires_in":7200}`)
		case "/cgi-bin/media/upload":
			gotType = r.URL.Query().Get("type")
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm: %v", err)
			}
			for field, fhs := range r.MultipartForm.File {
				gotField = field
				gotFilename = fhs[0].Filename
				f, _ := fhs[0].Open()
				b, _ := io.ReadAll(f)
				gotFileBody = string(b)
				f.Close()
			}
			fmt.Fprint(w, `{"type":"image","media_id":"MEDIA-abc","created_at":123}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	mediaID, err := c.UploadTempMedia(context.Background(), testAccount(), MediaTypeImage, "cover.jpg", strings.NewReader("PNGDATA"))
	if err != nil {
		t.Fatalf("UploadTempMedia: %v", err)
	}
	if mediaID != "MEDIA-abc" {
		t.Fatalf("media_id = %q, want MEDIA-abc", mediaID)
	}
	if gotType != "image" {
		t.Fatalf("type query = %q, want image", gotType)
	}
	if gotField != wxMediaField {
		t.Fatalf("file field = %q, want %q", gotField, wxMediaField)
	}
	if gotFilename != "cover.jpg" {
		t.Fatalf("filename = %q, want cover.jpg", gotFilename)
	}
	if gotFileBody != "PNGDATA" {
		t.Fatalf("uploaded body = %q, want PNGDATA", gotFileBody)
	}
}

// TestAddPermanentMaterial 验证永久素材上传返回 media_id + url。
func TestAddPermanentMaterial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			fmt.Fprint(w, `{"access_token":"TOK","expires_in":7200}`)
		case "/cgi-bin/material/add_material":
			if got := r.URL.Query().Get("type"); got != "thumb" {
				t.Errorf("type = %q, want thumb", got)
			}
			fmt.Fprint(w, `{"media_id":"PERM-1","url":"https://mmbiz.qpic.cn/x"}`)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	mediaID, url, err := c.AddPermanentMaterial(context.Background(), testAccount(), MediaTypeThumb, "thumb.jpg", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("AddPermanentMaterial: %v", err)
	}
	if mediaID != "PERM-1" || url != "https://mmbiz.qpic.cn/x" {
		t.Fatalf("got media_id=%q url=%q", mediaID, url)
	}
}

// TestUploadArticleImage 验证图文正文图片上传只返回 url。
func TestUploadArticleImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			fmt.Fprint(w, `{"access_token":"TOK","expires_in":7200}`)
		case "/cgi-bin/media/uploadimg":
			fmt.Fprint(w, `{"url":"https://mmbiz.qpic.cn/img"}`)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	url, err := c.UploadArticleImage(context.Background(), testAccount(), "inline.png", strings.NewReader("y"))
	if err != nil {
		t.Fatalf("UploadArticleImage: %v", err)
	}
	if url != "https://mmbiz.qpic.cn/img" {
		t.Fatalf("url = %q", url)
	}
}

func TestUploadArticleImageAcceptsOfficialHTTPURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			fmt.Fprint(w, `{"access_token":"TOK","expires_in":7200}`)
		case "/cgi-bin/media/uploadimg":
			fmt.Fprint(w, `{"url":"http://mmbiz.qpic.cn/official"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	imageURL, err := c.UploadArticleImage(context.Background(), testAccount(), "inline.png", strings.NewReader("y"))
	if err != nil {
		t.Fatalf("UploadArticleImage: %v", err)
	}
	if imageURL != "http://mmbiz.qpic.cn/official" {
		t.Fatalf("url = %q", imageURL)
	}
}

func TestUploadArticleImageRejectsInvalidURL(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: `{}`},
		{name: "relative", body: `{"url":"/not-absolute"}`},
		{name: "off-domain", body: `{"url":"https://images.example/img"}`},
		{name: "fake-suffix", body: `{"url":"https://mmbiz.qpic.cn.evil.example/img"}`},
		{name: "userinfo", body: `{"url":"https://user@mmbiz.qpic.cn/img"}`},
		{name: "non-standard-port", body: `{"url":"https://mmbiz.qpic.cn:8443/img"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/cgi-bin/token":
					fmt.Fprint(w, `{"access_token":"TOK","expires_in":7200}`)
				case "/cgi-bin/media/uploadimg":
					fmt.Fprint(w, tt.body)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			if _, err := c.UploadArticleImage(context.Background(), testAccount(), "inline.png", strings.NewReader("y")); err == nil {
				t.Fatal("UploadArticleImage should reject invalid response URL")
			}
		})
	}
}

// TestUploadTokenExpiredRetry 验证上传路径同样支持 token 失效重试（body 缓冲重放）。
func TestUploadTokenExpiredRetry(t *testing.T) {
	var uploadHits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			fmt.Fprint(w, `{"access_token":"TOK","expires_in":7200}`)
		case "/cgi-bin/media/upload":
			n := atomic.AddInt64(&uploadHits, 1)
			// 消费掉 body，确保重试时 body 能重放。
			_ = r.ParseMultipartForm(1 << 20)
			if n == 1 {
				fmt.Fprint(w, `{"errcode":42001,"errmsg":"access_token expired"}`)
				return
			}
			fmt.Fprint(w, `{"media_id":"M-retry"}`)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	mediaID, err := c.UploadTempMedia(context.Background(), testAccount(), MediaTypeImage, "f.jpg", strings.NewReader("DATA"))
	if err != nil {
		t.Fatalf("UploadTempMedia retry: %v", err)
	}
	if mediaID != "M-retry" {
		t.Fatalf("media_id = %q, want M-retry", mediaID)
	}
	if uploadHits != 2 {
		t.Fatalf("upload hit %d times, want 2 (fail + retry)", uploadHits)
	}
}
