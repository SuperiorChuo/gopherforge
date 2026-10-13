package wxarticle

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxImageBytes    = 10 << 20
	maxRedirects     = 3
	imageHTTPTimeout = 30 * time.Second
)

type lookupNetIPFunc func(context.Context, string, string) ([]netip.Addr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

var imageHTTPClient = newImageHTTPClient(
	net.DefaultResolver.LookupNetIP,
	(&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
)

// FetchImage safely reads and verifies an HTTP(S) or base64 data image.
func FetchImage(ctx context.Context, src string) ([]byte, string, error) {
	return fetchImageWithClient(ctx, src, imageHTTPClient)
}

func fetchImageWithClient(ctx context.Context, src string, client *http.Client) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrFetchImage, err)
	}
	src = strings.TrimSpace(src)
	if isDataURL(src) {
		return fetchDataImage(src)
	}
	if err := validateHTTPSource(src); err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid HTTP request", ErrInvalidImage)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", fmt.Errorf("%w: %w", ErrFetchImage, ctxErr)
		}
		return nil, "", fmt.Errorf("%w: request failed", ErrFetchImage)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("%w: unexpected HTTP status %d", ErrFetchImage, resp.StatusCode)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, "", fmt.Errorf("%w: image exceeds %d bytes", ErrInvalidImage, maxImageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", fmt.Errorf("%w: %w", ErrFetchImage, ctxErr)
		}
		return nil, "", fmt.Errorf("%w: read response body failed", ErrFetchImage)
	}
	return validatedImage(data, resp.Header.Get("Content-Type"))
}

func fetchDataImage(src string) ([]byte, string, error) {
	declared, payload, err := parseDataImage(src)
	if err != nil {
		return nil, "", err
	}
	maxEncodedBytes := base64.StdEncoding.EncodedLen(maxImageBytes + 1)
	if len(payload) > maxEncodedBytes {
		return nil, "", fmt.Errorf("%w: encoded data image exceeds limit", ErrInvalidImage)
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	data, err := io.ReadAll(io.LimitReader(decoder, maxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: malformed base64 data image", ErrInvalidImage)
	}
	return validatedImage(data, declared)
}

func validatedImage(data []byte, declared string) ([]byte, string, error) {
	mimeType, err := validateRasterImage(data, declared)
	if err != nil {
		return nil, "", err
	}
	return data, mimeType, nil
}

func validateRasterImage(data []byte, declared string) (string, error) {
	if len(data) > maxImageBytes {
		return "", fmt.Errorf("%w: image exceeds %d bytes", ErrInvalidImage, maxImageBytes)
	}
	actual := canonicalRasterMIME(http.DetectContentType(data))
	if actual == "" {
		return "", fmt.Errorf("%w: payload is not a supported raster image", ErrInvalidImage)
	}
	if strings.TrimSpace(declared) != "" {
		expected := canonicalRasterMIME(declared)
		if expected == "" || expected != actual {
			return "", fmt.Errorf("%w: declared and detected image types differ", ErrInvalidImage)
		}
	}
	return actual, nil
}

// ExtensionForContentType returns the filename extension used for a supported
// raster image MIME type. It returns an empty string for unsupported types.
func ExtensionForContentType(contentType string) string {
	switch canonicalRasterMIME(contentType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func canonicalRasterMIME(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		return ""
	}
	switch strings.ToLower(mediaType) {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/gif":
		return "image/gif"
	case "image/webp":
		return "image/webp"
	default:
		return ""
	}
}

func validateImageSource(src string) error {
	if isDataURL(src) {
		_, _, err := parseDataImage(src)
		return err
	}
	return validateHTTPSource(src)
}

func parseDataImage(src string) (string, string, error) {
	comma := strings.IndexByte(src, ',')
	if comma < len("data:image/x;base64") || comma == len(src)-1 {
		return "", "", fmt.Errorf("%w: malformed data image", ErrInvalidImage)
	}
	meta := src[len("data:"):comma]
	parts := strings.Split(meta, ";")
	if len(parts) != 2 || !strings.EqualFold(parts[1], "base64") {
		return "", "", fmt.Errorf("%w: data image must use base64", ErrInvalidImage)
	}
	declared := canonicalRasterMIME(parts[0])
	if declared == "" {
		return "", "", fmt.Errorf("%w: unsupported data image MIME type", ErrInvalidImage)
	}
	return declared, src[comma+1:], nil
}

func isDataURL(src string) bool {
	return len(src) >= len("data:") && strings.EqualFold(src[:len("data:")], "data:")
}

func validateHTTPSource(src string) error {
	u, err := url.Parse(src)
	if err != nil || !u.IsAbs() || u.Opaque != "" || u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("%w: image source must be an absolute HTTP(S) URL", ErrInvalidImage)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: unsupported image URL scheme", ErrInvalidImage)
	}
	if u.User != nil {
		return fmt.Errorf("%w: image URL must not contain userinfo", ErrInvalidImage)
	}
	if port := u.Port(); port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return fmt.Errorf("%w: invalid image URL port", ErrInvalidImage)
		}
	}
	return nil
}

func newImageHTTPClient(lookup lookupNetIPFunc, dial dialContextFunc) *http.Client {
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            secureDialContext(lookup, dial),
		ForceAttemptHTTP2:      true,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  20 * time.Second,
		IdleConnTimeout:        30 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   imageHTTPTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Del("Referer")
			if len(via) > maxRedirects {
				return fmt.Errorf("%w: too many redirects", ErrFetchImage)
			}
			if err := validateHTTPSource(req.URL.String()); err != nil {
				return fmt.Errorf("%w: invalid redirect target", ErrFetchImage)
			}
			return nil
		},
	}
}

func secureDialContext(lookup lookupNetIPFunc, dial dialContextFunc) dialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid destination address", ErrFetchImage)
		}
		addresses, err := resolveHost(ctx, lookup, host)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			if !allowedRemoteIP(address) {
				return nil, fmt.Errorf("%w: destination address is not public", ErrFetchImage)
			}
		}
		var lastErr error
		for _, address := range addresses {
			conn, err := dial(ctx, network, net.JoinHostPort(address.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, fmt.Errorf("%w: connect failed: %w", ErrFetchImage, lastErr)
		}
		return nil, fmt.Errorf("%w: image host has no addresses", ErrFetchImage)
	}
}

func resolveHost(ctx context.Context, lookup lookupNetIPFunc, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address}, nil
	}
	addresses, err := lookup(ctx, "ip", host)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrFetchImage, ctxErr)
		}
		return nil, fmt.Errorf("%w: resolve image host: %w", ErrFetchImage, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("%w: image host has no addresses", ErrFetchImage)
	}
	return addresses, nil
}

var blockedPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func allowedRemoteIP(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() ||
		address.IsUnspecified() || address.IsLoopback() || address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() || address.IsMulticast() {
		return false
	}
	for _, prefix := range blockedPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
