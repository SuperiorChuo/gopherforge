// Package wxarticle prepares article images for WeChat draft publishing.
package wxarticle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	maxHTMLBytes        = 16 << 20
	maxImageNodes       = 20
	maxInlineImageBytes = 1 << 20
	maxTotalBytes       = maxImageNodes * maxInlineImageBytes
)

var (
	// ErrInvalidImage marks invalid HTML image sources, image payloads, or limits.
	ErrInvalidImage = errors.New("invalid article image")
	// ErrFetchImage marks failures while downloading an HTTP image.
	ErrFetchImage = errors.New("fetch article image")
	// ErrUploadImage marks failures or invalid results from WeChat uploadimg.
	ErrUploadImage = errors.New("upload article image")
)

// ImageUploader is implemented by wxclient.Client.
type ImageUploader interface {
	UploadArticleImage(context.Context, *store.MpAccount, string, io.Reader) (string, error)
}

type imageFetcher func(context.Context, string) ([]byte, string, error)

type imageSource struct {
	src   string
	nodes []*html.Node
	data  []byte
	mime  string
}

// RewriteInlineImages uploads non-WeChat inline images and replaces their src
// attributes with the HTTP(S) URLs returned by WeChat.
func RewriteInlineImages(ctx context.Context, uploader ImageUploader, acc *store.MpAccount, rawHTML string) (string, error) {
	return rewriteInlineImages(ctx, uploader, acc, rawHTML, FetchImage)
}

func rewriteInlineImages(ctx context.Context, uploader ImageUploader, acc *store.MpAccount, rawHTML string, fetch imageFetcher) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("%w: %w", ErrFetchImage, err)
	}
	if len(rawHTML) > maxHTMLBytes {
		return "", fmt.Errorf("%w: HTML exceeds %d bytes", ErrInvalidImage, maxHTMLBytes)
	}

	fragmentContext := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragment(strings.NewReader(rawHTML), fragmentContext)
	if err != nil {
		return "", fmt.Errorf("%w: parse HTML fragment", ErrInvalidImage)
	}
	sources, changed, err := collectImageSources(nodes)
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		if !changed {
			return rawHTML, nil
		}
		return renderFragment(nodes, len(rawHTML))
	}
	if uploader == nil || acc == nil {
		return "", fmt.Errorf("%w: uploader or account is not configured", ErrUploadImage)
	}

	// Fetch every unique source before the first upload. This prevents a late
	// fetch or validation failure from leaving a partially uploaded article.
	total := 0
	for i := range sources {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("%w: %w", ErrFetchImage, err)
		}
		data, declared, err := fetch(ctx, sources[i].src)
		if err != nil {
			if errors.Is(err, ErrInvalidImage) || errors.Is(err, ErrFetchImage) {
				return "", fmt.Errorf("image source %d: %w", i+1, err)
			}
			return "", fmt.Errorf("image source %d: %w: fetch failed", i+1, ErrFetchImage)
		}
		mimeType, err := validateRasterImage(data, declared)
		if err != nil {
			return "", fmt.Errorf("image source %d: %w", i+1, err)
		}
		// WeChat uploadImage only accepts JPG/PNG files smaller than 1 MiB.
		if mimeType != "image/jpeg" && mimeType != "image/png" {
			return "", fmt.Errorf("%w: inline image %d must be JPEG or PNG", ErrInvalidImage, i+1)
		}
		if len(data) >= maxInlineImageBytes {
			return "", fmt.Errorf("%w: inline image %d must be smaller than %d bytes", ErrInvalidImage, i+1, maxInlineImageBytes)
		}
		if len(data) > maxTotalBytes-total {
			return "", fmt.Errorf("%w: inline images exceed %d bytes total", ErrInvalidImage, maxTotalBytes)
		}
		total += len(data)
		sources[i].data = data
		sources[i].mime = mimeType
	}

	for i := range sources {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("%w: %w", ErrUploadImage, err)
		}
		filename := fmt.Sprintf("inline-%02d%s", i+1, ExtensionForContentType(sources[i].mime))
		uploadedURL, err := uploader.UploadArticleImage(ctx, acc, filename, bytes.NewReader(sources[i].data))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", fmt.Errorf("%w: image %d: %w", ErrUploadImage, i+1, ctxErr)
			}
			if errors.Is(err, context.Canceled) {
				return "", fmt.Errorf("%w: image %d: %w", ErrUploadImage, i+1, context.Canceled)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return "", fmt.Errorf("%w: image %d: %w", ErrUploadImage, i+1, context.DeadlineExceeded)
			}
			return "", fmt.Errorf("%w: image %d failed", ErrUploadImage, i+1)
		}
		if !validUploadedURL(uploadedURL) {
			return "", fmt.Errorf("%w: image %d returned an invalid WeChat image URL", ErrUploadImage, i+1)
		}
		for _, node := range sources[i].nodes {
			setAttribute(node, "src", uploadedURL)
			removeAttribute(node, "srcset")
		}
	}

	return renderFragment(nodes, len(rawHTML))
}

func renderFragment(nodes []*html.Node, capacity int) (string, error) {
	var rendered bytes.Buffer
	rendered.Grow(capacity)
	for _, node := range nodes {
		if err := html.Render(&rendered, node); err != nil {
			return "", fmt.Errorf("%w: render HTML fragment", ErrInvalidImage)
		}
	}
	return rendered.String(), nil
}

func collectImageSources(roots []*html.Node) ([]imageSource, bool, error) {
	stack := make([]*html.Node, 0, len(roots))
	for i := len(roots) - 1; i >= 0; i-- {
		stack = append(stack, roots[i])
	}

	ordered := make([]imageSource, 0)
	bySource := make(map[string]int)
	imageCount := 0
	changed := false
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for child := node.LastChild; child != nil; child = child.PrevSibling {
			stack = append(stack, child)
		}
		if node.Type == html.ElementNode && node.DataAtom == atom.Source && hasPictureAncestor(node) {
			if removeAttribute(node, "srcset") {
				changed = true
			}
			if removeAttribute(node, "src") {
				changed = true
			}
		}
		if node.Type != html.ElementNode || node.DataAtom != atom.Img {
			continue
		}
		imageCount++
		if imageCount > maxImageNodes {
			return nil, false, fmt.Errorf("%w: HTML contains more than %d image nodes", ErrInvalidImage, maxImageNodes)
		}

		rawSrc, count := attributeValue(node, "src")
		if count > 1 {
			return nil, false, fmt.Errorf("%w: image node has duplicate src attributes", ErrInvalidImage)
		}
		if removeAttribute(node, "srcset") {
			changed = true
		}
		src := strings.TrimSpace(rawSrc)
		if src == "" {
			continue
		}
		if isWeChatImageURL(src) {
			if rawSrc != src {
				setAttribute(node, "src", src)
				changed = true
			}
			continue
		}
		if err := validateImageSource(src); err != nil {
			return nil, false, fmt.Errorf("%w: image source %d", err, imageCount)
		}
		if index, ok := bySource[src]; ok {
			ordered[index].nodes = append(ordered[index].nodes, node)
			continue
		}
		bySource[src] = len(ordered)
		ordered = append(ordered, imageSource{src: src, nodes: []*html.Node{node}})
	}
	return ordered, changed, nil
}

func hasPictureAncestor(node *html.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == html.ElementNode && parent.DataAtom == atom.Picture {
			return true
		}
	}
	return false
}

func attributeValue(node *html.Node, key string) (string, int) {
	value := ""
	count := 0
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			value = attr.Val
			count++
		}
	}
	return value, count
}

func setAttribute(node *html.Node, key, value string) {
	for i := range node.Attr {
		if strings.EqualFold(node.Attr[i].Key, key) {
			node.Attr[i].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, html.Attribute{Key: key, Val: value})
}

func removeAttribute(node *html.Node, key string) bool {
	originalLen := len(node.Attr)
	attrs := node.Attr[:0]
	for _, attr := range node.Attr {
		if !strings.EqualFold(attr.Key, key) {
			attrs = append(attrs, attr)
		}
	}
	node.Attr = attrs
	return len(node.Attr) != originalLen
}

func isWeChatImageURL(src string) bool {
	u, err := url.Parse(src)
	return err == nil && u.IsAbs() && u.Opaque == "" &&
		(u.Scheme == "http" || u.Scheme == "https") && u.User == nil &&
		strings.EqualFold(u.Hostname(), "mmbiz.qpic.cn") && u.Port() == ""
}

func validUploadedURL(raw string) bool {
	return raw != "" && raw == strings.TrimSpace(raw) && isWeChatImageURL(raw)
}
