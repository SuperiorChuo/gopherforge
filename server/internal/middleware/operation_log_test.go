package middleware

import (
	"io"
	"strings"
	"testing"
)

func TestReadRequestBodyForLogLimitsPreviewAndRestoresFullBody(t *testing.T) {
	body := strings.Repeat("a", 16)
	tracked := &trackingReadCloser{reader: strings.NewReader(body)}

	logBody, restored, err := readRequestBodyForLog(tracked, 4)
	if err != nil {
		t.Fatalf("read request body for log: %v", err)
	}
	if tracked.bytesRead != 5 {
		t.Fatalf("bytes read before handler = %d, want 5", tracked.bytesRead)
	}
	if logBody != "aaaa...[truncated]" {
		t.Fatalf("logged body = %q, want truncated preview", logBody)
	}

	restoredBytes, err := io.ReadAll(restored)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(restoredBytes) != body {
		t.Fatalf("restored body = %q, want original body", string(restoredBytes))
	}
}

type trackingReadCloser struct {
	reader    *strings.Reader
	bytesRead int
	closed    bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += n
	return n, err
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}
