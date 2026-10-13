package wxarticle

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestFetchImageDataURLAndMIMEValidation(t *testing.T) {
	png := testPNG(64)
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	got, mimeType, err := FetchImage(context.Background(), src)
	if err != nil {
		t.Fatalf("FetchImage() error = %v", err)
	}
	if string(got) != string(png) || mimeType != "image/png" {
		t.Fatalf("FetchImage() = (%d bytes, %q), want (%d bytes, image/png)", len(got), mimeType, len(png))
	}

	mismatch := "data:image/gif;base64," + base64.StdEncoding.EncodeToString(png)
	_, _, err = FetchImage(context.Background(), mismatch)
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("MIME mismatch error = %v, want ErrInvalidImage", err)
	}

	svg := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg></svg>`))
	_, _, err = FetchImage(context.Background(), svg)
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("SVG error = %v, want ErrInvalidImage", err)
	}
}

func TestExtensionForContentType(t *testing.T) {
	tests := map[string]string{
		"image/jpeg":                ".jpg",
		"image/jpg":                 ".jpg",
		"image/png; charset=binary": ".png",
		"image/gif":                 ".gif",
		"image/webp":                ".webp",
		"image/svg+xml":             "",
		"application/octet-stream":  "",
	}
	for contentType, want := range tests {
		if got := ExtensionForContentType(contentType); got != want {
			t.Errorf("ExtensionForContentType(%q) = %q, want %q", contentType, got, want)
		}
	}
}

func TestFetchHTTPImageSuccessUsesResolvedPublicIP(t *testing.T) {
	var dialed string
	client, baseURL := routedImageClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testPNG(64))
	}, func(address string) {
		dialed = address
	})

	data, mimeType, err := fetchImageWithClient(context.Background(), baseURL+"/image.png?token=secret", client)
	if err != nil {
		t.Fatalf("fetchImageWithClient() error = %v", err)
	}
	if len(data) != 64 || mimeType != "image/png" {
		t.Fatalf("result = (%d bytes, %q), want (64 bytes, image/png)", len(data), mimeType)
	}
	host, _, err := net.SplitHostPort(dialed)
	if err != nil {
		t.Fatalf("dialed address %q: %v", dialed, err)
	}
	if host != "8.8.8.8" {
		t.Fatalf("dialed host = %q, want resolved IP 8.8.8.8", host)
	}
	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil {
		t.Fatal("production image transport must not use a proxy")
	}
}

func TestFetchHTTPImageRejectsNon2xxAndFakeMIME(t *testing.T) {
	t.Run("non-2xx", func(t *testing.T) {
		client, baseURL := routedImageClient(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}, nil)
		_, _, err := fetchImageWithClient(context.Background(), baseURL+"/missing.png", client)
		if !errors.Is(err, ErrFetchImage) {
			t.Fatalf("error = %v, want ErrFetchImage", err)
		}
	})

	t.Run("HTML declared as PNG", func(t *testing.T) {
		client, baseURL := routedImageClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("<!doctype html><title>not an image</title>"))
		}, nil)
		_, _, err := fetchImageWithClient(context.Background(), baseURL+"/fake.png", client)
		if !errors.Is(err, ErrInvalidImage) {
			t.Fatalf("error = %v, want ErrInvalidImage", err)
		}
	})
}

func TestFetchHTTPImageRejectsUnknownLengthBodyOverLimit(t *testing.T) {
	client, baseURL := routedImageClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write(testPNG(maxImageBytes + 1))
	}, nil)

	_, _, err := fetchImageWithClient(context.Background(), baseURL+"/chunked.png", client)
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("error = %v, want ErrInvalidImage", err)
	}
}

func TestSecureDialRejectsNonPublicAddresses(t *testing.T) {
	tests := []string{
		"127.0.0.1",
		"10.0.0.1",
		"0.0.0.0",
		"169.254.169.254",
		"224.0.0.1",
		"::1",
		"fe80::1",
		"ff02::1",
		"::ffff:10.0.0.1",
		"::10.0.0.1",
		"64:ff9b::a00:1",
		"64:ff9b:1::a00:1",
		"2002:a00:1::1",
	}
	for _, address := range tests {
		t.Run(address, func(t *testing.T) {
			dialCalls := 0
			dial := secureDialContext(nil, func(context.Context, string, string) (net.Conn, error) {
				dialCalls++
				return nil, errors.New("must not dial")
			})
			_, err := dial(context.Background(), "tcp", net.JoinHostPort(address, "80"))
			if !errors.Is(err, ErrFetchImage) {
				t.Fatalf("error = %v, want ErrFetchImage", err)
			}
			if dialCalls != 0 {
				t.Fatalf("raw dial calls = %d, want 0", dialCalls)
			}
		})
	}
}

func TestSecureDialRejectsMixedDNSAnswers(t *testing.T) {
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")}, nil
	}
	dialCalls := 0
	dial := secureDialContext(lookup, func(context.Context, string, string) (net.Conn, error) {
		dialCalls++
		return nil, errors.New("must not dial")
	})
	_, err := dial(context.Background(), "tcp", "mixed.example:80")
	if !errors.Is(err, ErrFetchImage) {
		t.Fatalf("error = %v, want ErrFetchImage", err)
	}
	if dialCalls != 0 {
		t.Fatalf("raw dial calls = %d, want 0", dialCalls)
	}
}

func TestFetchHTTPImageRejectsPrivateRedirect(t *testing.T) {
	var baseURL string
	client, baseURL := routedImageClientWithDNS(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Host, "public.test") {
			u, _ := url.Parse(baseURL)
			http.Redirect(w, r, "http://private.test:"+u.Port()+"/secret", http.StatusFound)
			return
		}
		_, _ = w.Write(testPNG(64))
	}, map[string][]netip.Addr{
		"public.test":  {netip.MustParseAddr("8.8.8.8")},
		"private.test": {netip.MustParseAddr("10.0.0.1")},
	}, nil)

	_, _, err := fetchImageWithClient(context.Background(), baseURL+"/redirect", client)
	if !errors.Is(err, ErrFetchImage) {
		t.Fatalf("error = %v, want ErrFetchImage", err)
	}
}

func TestFetchHTTPImageRejectsTooManyRedirects(t *testing.T) {
	var baseURL string
	client, baseURL := routedImageClient(t, func(w http.ResponseWriter, r *http.Request) {
		step, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		http.Redirect(w, r, baseURL+"/"+strconv.Itoa(step+1), http.StatusFound)
	}, nil)

	_, _, err := fetchImageWithClient(context.Background(), baseURL+"/0", client)
	if !errors.Is(err, ErrFetchImage) {
		t.Fatalf("error = %v, want ErrFetchImage", err)
	}
}

func TestFetchHTTPImageDoesNotForwardRefererOnRedirect(t *testing.T) {
	var baseURL string
	var gotReferer string
	client, baseURL := routedImageClientWithDNS(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Host, "public.test") {
			u, _ := url.Parse(baseURL)
			http.Redirect(w, r, "http://redirect.test:"+u.Port()+"/image.png", http.StatusFound)
			return
		}
		gotReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testPNG(64))
	}, map[string][]netip.Addr{
		"public.test":   {netip.MustParseAddr("8.8.8.8")},
		"redirect.test": {netip.MustParseAddr("8.8.8.8")},
	}, nil)

	_, _, err := fetchImageWithClient(context.Background(), baseURL+"/start.png?signature=source-secret", client)
	if err != nil {
		t.Fatalf("fetchImageWithClient() error = %v", err)
	}
	if gotReferer != "" {
		t.Fatalf("redirect Referer = %q, want empty", gotReferer)
	}
}

func TestFetchImageErrorsDoNotEchoSensitiveSource(t *testing.T) {
	client := newImageHTTPClient(
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("network unavailable")
		},
	)
	raw := "https://images.example/path/image.png?token=secret-value"
	_, _, err := fetchImageWithClient(context.Background(), raw, client)
	if err == nil {
		t.Fatal("expected request failure")
	}
	if strings.Contains(err.Error(), raw) || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("error leaked source URL: %v", err)
	}

	dataURL := "data:image/png;base64,definitely-secret-not-base64"
	_, _, err = FetchImage(context.Background(), dataURL)
	if err == nil {
		t.Fatal("expected data URL failure")
	}
	if strings.Contains(err.Error(), dataURL) || strings.Contains(err.Error(), "definitely-secret") {
		t.Fatalf("error leaked data URL: %v", err)
	}

	redirectClient, redirectBaseURL := routedImageClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://[::1/image.png?token=redirect-secret")
		w.WriteHeader(http.StatusFound)
	}, nil)
	_, _, err = fetchImageWithClient(context.Background(), redirectBaseURL+"/redirect", redirectClient)
	if err == nil {
		t.Fatal("expected malformed redirect failure")
	}
	if strings.Contains(err.Error(), "redirect-secret") || strings.Contains(err.Error(), "Location") {
		t.Fatalf("error leaked redirect target: %v", err)
	}

	readErrorClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body: io.NopCloser(errorReader{
				err: errors.New("read https://images.example/a.png?token=body-secret failed"),
			}),
		}, nil
	})}
	_, _, err = fetchImageWithClient(context.Background(), "https://images.example/a.png", readErrorClient)
	if err == nil {
		t.Fatal("expected response body failure")
	}
	if strings.Contains(err.Error(), "body-secret") || strings.Contains(err.Error(), "images.example") {
		t.Fatalf("error leaked response body error: %v", err)
	}
}

func TestFetchImageContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := FetchImage(ctx, "https://example.com/image.png")
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrFetchImage) {
		t.Fatalf("error = %v, want context.Canceled and ErrFetchImage", err)
	}
}

func routedImageClient(t *testing.T, handler http.HandlerFunc, onDial func(string)) (*http.Client, string) {
	return routedImageClientWithDNS(t, handler, map[string][]netip.Addr{
		"public.test": {netip.MustParseAddr("8.8.8.8")},
	}, onDial)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func routedImageClientWithDNS(t *testing.T, handler http.HandlerFunc, dns map[string][]netip.Addr, onDial func(string)) (*http.Client, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	_, port, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatalf("split test server address: %v", err)
	}
	lookup := func(_ context.Context, _ string, host string) ([]netip.Addr, error) {
		addresses, ok := dns[host]
		if !ok {
			return nil, fmt.Errorf("unknown test host %s", host)
		}
		return addresses, nil
	}
	rawDial := func(ctx context.Context, network, address string) (net.Conn, error) {
		if onDial != nil {
			onDial(address)
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if host != "8.8.8.8" {
			return nil, fmt.Errorf("unexpected raw dial host %s", host)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	client := newImageHTTPClient(lookup, rawDial)
	t.Cleanup(client.CloseIdleConnections)
	return client, "http://public.test:" + port
}
