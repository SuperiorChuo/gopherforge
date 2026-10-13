package wxarticle

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

type uploadCall struct {
	filename string
	data     []byte
}

type recordingUploader struct {
	calls   []uploadCall
	failAt  int
	failErr error
	urls    []string
}

func (u *recordingUploader) UploadArticleImage(_ context.Context, _ *store.MpAccount, filename string, reader io.Reader) (string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	u.calls = append(u.calls, uploadCall{filename: filename, data: data})
	if u.failAt == len(u.calls) {
		if u.failErr != nil {
			return "", u.failErr
		}
		return "", errors.New("upstream upload failed")
	}
	if len(u.urls) >= len(u.calls) {
		return u.urls[len(u.calls)-1], nil
	}
	return fmt.Sprintf("https://mmbiz.qpic.cn/upload/%d", len(u.calls)), nil
}

func TestRewriteInlineImagesDataURL(t *testing.T) {
	image := testPNG(64)
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	raw := `<section><img alt="inline" src="` + src + `" srcset="https://old.example/2x.png 2x"></section>`
	uploader := &recordingUploader{urls: []string{"https://mmbiz.qpic.cn/upload/new.png"}}

	got, err := RewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, raw)
	if err != nil {
		t.Fatalf("RewriteInlineImages() error = %v", err)
	}
	if len(uploader.calls) != 1 {
		t.Fatalf("upload calls = %d, want 1", len(uploader.calls))
	}
	if uploader.calls[0].filename != "inline-01.png" {
		t.Fatalf("filename = %q, want inline-01.png", uploader.calls[0].filename)
	}
	if !bytes.Equal(uploader.calls[0].data, image) {
		t.Fatal("uploaded bytes differ from decoded data URL")
	}
	if !strings.Contains(got, `src="https://mmbiz.qpic.cn/upload/new.png"`) {
		t.Fatalf("rewritten HTML does not contain uploaded URL: %s", got)
	}
	if strings.Contains(got, "srcset") {
		t.Fatalf("rewritten HTML retained srcset: %s", got)
	}
}

func TestRewriteInlineImagesDeduplicatesTrimmedSources(t *testing.T) {
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNG(32))
	raw := `<img src="` + src + `"><img src="  ` + src + `  " srcset="old.png 2x">`
	uploader := &recordingUploader{urls: []string{"https://mmbiz.qpic.cn/shared.png"}}

	got, err := RewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, raw)
	if err != nil {
		t.Fatalf("RewriteInlineImages() error = %v", err)
	}
	if len(uploader.calls) != 1 {
		t.Fatalf("upload calls = %d, want 1", len(uploader.calls))
	}
	if count := strings.Count(got, `src="https://mmbiz.qpic.cn/shared.png"`); count != 2 {
		t.Fatalf("rewritten URL count = %d, want 2; HTML: %s", count, got)
	}
	if strings.Contains(got, "srcset") {
		t.Fatalf("rewritten HTML retained srcset: %s", got)
	}
}

func TestRewriteInlineImagesReturnsOriginalWhenNoRewriteNeeded(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "no image", raw: `<p class='original'>hello & raw</p>`},
		{name: "existing WeChat HTTPS source", raw: `<img src="https://mmbiz.qpic.cn/a.png?x=1">`},
		{name: "official WeChat HTTP source", raw: `<img src="http://mmbiz.qpic.cn/a.png?x=1">`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RewriteInlineImages(context.Background(), nil, nil, tt.raw)
			if err != nil {
				t.Fatalf("RewriteInlineImages() error = %v", err)
			}
			if got != tt.raw {
				t.Fatalf("HTML changed:\n got: %q\nwant: %q", got, tt.raw)
			}
		})
	}
}

func TestRewriteInlineImagesStripsResponsiveSourceCandidates(t *testing.T) {
	raw := `<picture><source src="https://tracker.example/source.png" srcset="https://tracker.example/source-2x.png 2x"><img src="https://mmbiz.qpic.cn/a.png" srcset="https://tracker.example/img-2x.png 2x"></picture>` +
		`<img src="" srcset="https://external.example/empty.png 1x">`

	got, err := RewriteInlineImages(context.Background(), nil, nil, raw)
	if err != nil {
		t.Fatalf("RewriteInlineImages() error = %v", err)
	}
	if strings.Contains(got, "srcset") || strings.Contains(got, "tracker.example") || strings.Contains(got, "external.example") {
		t.Fatalf("responsive source bypass remained in HTML: %s", got)
	}
	if !strings.Contains(got, `src="https://mmbiz.qpic.cn/a.png"`) {
		t.Fatalf("trusted image source was lost: %s", got)
	}
}

func TestRewriteInlineImagesRejectsUnsafeSources(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "relative", src: "/image.png"},
		{name: "javascript", src: "javascript:alert(1)"},
		{name: "blob", src: "blob:https://example.com/id"},
		{name: "file", src: "file:///tmp/image.png"},
		{name: "userinfo", src: "https://user@example.com/image.png"},
		{name: "non-image data", src: "data:text/html;base64,PGgxPmJhZDwvaDE+"},
		{name: "non-base64 data", src: "data:image/png,raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := &recordingUploader{}
			_, err := RewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, `<img src="`+tt.src+`">`)
			if !errors.Is(err, ErrInvalidImage) {
				t.Fatalf("error = %v, want ErrInvalidImage", err)
			}
			if len(uploader.calls) != 0 {
				t.Fatalf("upload calls = %d, want 0", len(uploader.calls))
			}
		})
	}
}

func TestRewriteInlineImagesOnlySkipsExactWeChatHost(t *testing.T) {
	fetches := 0
	fetch := func(_ context.Context, src string) ([]byte, string, error) {
		fetches++
		if src != "https://mmbiz.qpic.cn.evil.example/image.png" {
			t.Fatalf("fetch source = %q", src)
		}
		return testPNG(32), "image/png", nil
	}
	uploader := &recordingUploader{}
	_, err := rewriteInlineImages(
		context.Background(),
		uploader,
		&store.MpAccount{ID: 1},
		`<img src="https://mmbiz.qpic.cn.evil.example/image.png">`,
		fetch,
	)
	if err != nil {
		t.Fatalf("rewriteInlineImages() error = %v", err)
	}
	if fetches != 1 || len(uploader.calls) != 1 {
		t.Fatalf("fetches = %d, uploads = %d; want 1 and 1", fetches, len(uploader.calls))
	}
}

func TestRewriteInlineImagesRejectsImageNodeLimit(t *testing.T) {
	raw := strings.Repeat(`<img src="">`, maxImageNodes+1)
	_, err := RewriteInlineImages(context.Background(), nil, nil, raw)
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("error = %v, want ErrInvalidImage", err)
	}
}

func TestRewriteInlineImagesRejectsHTMLLimit(t *testing.T) {
	raw := strings.Repeat("x", maxHTMLBytes+1)
	_, err := RewriteInlineImages(context.Background(), nil, nil, raw)
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("error = %v, want ErrInvalidImage", err)
	}
}

func TestFetchImageRejectsSingleImageLimit(t *testing.T) {
	oversized := testPNG(maxImageBytes + 1)
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(oversized)
	_, _, err := FetchImage(context.Background(), src)
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("error = %v, want ErrInvalidImage", err)
	}
}

func TestRewriteInlineImagesRejectsUploadImageContractBeforeUpload(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		mime string
	}{
		{name: "one MiB is not below limit", data: testPNG(maxInlineImageBytes), mime: "image/png"},
		{name: "GIF is unsupported", data: testGIF(32), mime: "image/gif"},
		{name: "WebP is unsupported", data: testWebP(32), mime: "image/webp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := &recordingUploader{}
			fetch := func(_ context.Context, _ string) ([]byte, string, error) {
				return tt.data, tt.mime, nil
			}
			_, err := rewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, `<img src="https://example.com/a">`, fetch)
			if !errors.Is(err, ErrInvalidImage) {
				t.Fatalf("error = %v, want ErrInvalidImage", err)
			}
			if len(uploader.calls) != 0 {
				t.Fatalf("upload calls = %d, want 0", len(uploader.calls))
			}
		})
	}
}

func TestRewriteInlineImagesPreflightsAllUploadConstraints(t *testing.T) {
	fetches := 0
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		fetches++
		if fetches == 2 {
			return testGIF(32), "image/gif", nil
		}
		return testPNG(32), "image/png", nil
	}
	uploader := &recordingUploader{}
	raw := `<img src="https://one.example/a.png"><img src="https://two.example/b.gif">`

	_, err := rewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, raw, fetch)
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("error = %v, want ErrInvalidImage", err)
	}
	if fetches != 2 || len(uploader.calls) != 0 {
		t.Fatalf("fetches = %d uploads = %d, want 2 and 0", fetches, len(uploader.calls))
	}
}

func TestRewriteInlineImagesFetchesAllBeforeUpload(t *testing.T) {
	fetches := 0
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		fetches++
		if fetches == 2 {
			return nil, "", errors.New("second fetch failed")
		}
		return testPNG(32), "image/png", nil
	}
	uploader := &recordingUploader{}
	raw := `<img src="https://one.example/a.png"><img src="https://two.example/b.png">`

	_, err := rewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, raw, fetch)
	if !errors.Is(err, ErrFetchImage) {
		t.Fatalf("error = %v, want ErrFetchImage", err)
	}
	if len(uploader.calls) != 0 {
		t.Fatalf("upload calls = %d, want 0", len(uploader.calls))
	}
}

func TestRewriteInlineImagesSecondUploadFails(t *testing.T) {
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		return testPNG(32), "image/png", nil
	}
	uploader := &recordingUploader{failAt: 2}
	raw := `<img src="https://one.example/a.png"><img src="https://two.example/b.png">`

	got, err := rewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, raw, fetch)
	if !errors.Is(err, ErrUploadImage) {
		t.Fatalf("error = %v, want ErrUploadImage", err)
	}
	if got != "" {
		t.Fatalf("HTML = %q, want empty on upload failure", got)
	}
	if len(uploader.calls) != 2 {
		t.Fatalf("upload calls = %d, want 2", len(uploader.calls))
	}
}

func TestRewriteInlineImagesSanitizesUploadError(t *testing.T) {
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		return testPNG(32), "image/png", nil
	}
	uploader := &recordingUploader{
		failAt:  1,
		failErr: errors.New("POST https://api.weixin.example/upload?access_token=upload-secret failed"),
	}

	_, err := rewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, `<img src="https://example.com/a.png">`, fetch)
	if !errors.Is(err, ErrUploadImage) {
		t.Fatalf("error = %v, want ErrUploadImage", err)
	}
	if strings.Contains(err.Error(), "upload-secret") || strings.Contains(err.Error(), "access_token") {
		t.Fatalf("error leaked upload URL: %v", err)
	}
}

func TestRewriteInlineImagesRejectsInvalidUploadURL(t *testing.T) {
	tests := []string{
		"",
		"/relative",
		"https://images.example/image.png",
		"https://mmbiz.qpic.cn.evil.example/image.png",
		"https://user@mmbiz.qpic.cn/image.png",
		"https://mmbiz.qpic.cn:8443/image.png",
	}
	for _, uploadedURL := range tests {
		t.Run(fmt.Sprintf("url=%q", uploadedURL), func(t *testing.T) {
			uploader := &recordingUploader{urls: []string{uploadedURL}}
			fetch := func(_ context.Context, _ string) ([]byte, string, error) {
				return testPNG(32), "image/png", nil
			}
			_, err := rewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, `<img src="https://example.com/a.png">`, fetch)
			if !errors.Is(err, ErrUploadImage) {
				t.Fatalf("error = %v, want ErrUploadImage", err)
			}
		})
	}
}

func TestRewriteInlineImagesAcceptsOfficialHTTPUploadURL(t *testing.T) {
	uploader := &recordingUploader{urls: []string{"http://mmbiz.qpic.cn/official.png"}}
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		return testPNG(32), "image/png", nil
	}

	got, err := rewriteInlineImages(context.Background(), uploader, &store.MpAccount{ID: 1}, `<img src="https://example.com/a.png">`, fetch)
	if err != nil {
		t.Fatalf("rewriteInlineImages() error = %v", err)
	}
	if !strings.Contains(got, `src="http://mmbiz.qpic.cn/official.png"`) {
		t.Fatalf("official upload URL missing from HTML: %s", got)
	}
}

func TestRewriteInlineImagesContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RewriteInlineImages(ctx, &recordingUploader{}, &store.MpAccount{ID: 1}, `<img src="https://example.com/a.png">`)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrFetchImage) {
		t.Fatalf("error = %v, want context.Canceled and ErrFetchImage", err)
	}
}

func testPNG(size int) []byte {
	if size < 8 {
		size = 8
	}
	data := make([]byte, size)
	copy(data, []byte("\x89PNG\r\n\x1a\n"))
	return data
}

func testGIF(size int) []byte {
	if size < 6 {
		size = 6
	}
	data := make([]byte, size)
	copy(data, []byte("GIF89a"))
	return data
}

func testWebP(size int) []byte {
	if size < 12 {
		size = 12
	}
	data := make([]byte, size)
	copy(data, []byte("RIFF\x00\x00\x00\x00WEBP"))
	return data
}
