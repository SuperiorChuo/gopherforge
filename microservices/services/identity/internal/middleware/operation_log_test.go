package middleware

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/services/identity/internal/model"
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

func TestFilterSensitiveFieldsMasksCurrentPassword(t *testing.T) {
	body := `{"current_password":"Secret123","totp":{"current_password":"NestedSecret1"},"code":"123456"}`

	got := filterSensitiveFields(body)
	if strings.Contains(got, "Secret123") || strings.Contains(got, "NestedSecret1") {
		t.Fatalf("filterSensitiveFields() leaked current_password: %s", got)
	}
	if !strings.Contains(got, `"current_password":"***"`) {
		t.Fatalf("filterSensitiveFields() did not mask current_password: %s", got)
	}
}

func TestOperationLogProcessorExitsAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	queue := make(chan *model.OperationLog)
	recorder := &operationLogRecorderSpy{}

	done := processLogs(ctx, queue, recorder, 50*time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("processor did not exit after context cancellation")
	}
}

func TestOperationLogProcessorDrainsQueuedLogsAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	queue := make(chan *model.OperationLog, 2)
	queue <- &model.OperationLog{Path: "/queued/1"}
	queue <- &model.OperationLog{Path: "/queued/2"}
	cancel()

	recorder := &operationLogRecorderSpy{}
	done := processLogs(ctx, queue, recorder, 50*time.Millisecond)

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("processor did not exit while draining queued logs")
	}

	if got := recorder.count(); got != 2 {
		t.Fatalf("processed logs = %d, want 2", got)
	}
}

func TestOperationLogProcessorUsesTimeoutForRecordContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := make(chan *model.OperationLog, 1)
	queue <- &model.OperationLog{Path: "/slow-write"}

	recorder := newBlockingOperationLogRecorder()
	done := processLogs(ctx, queue, recorder, 20*time.Millisecond)

	select {
	case <-recorder.started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recorder was not called")
	}

	select {
	case <-recorder.finished:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("RecordContext did not finish via write timeout")
	}

	if !recorder.hadDeadline {
		t.Fatal("RecordContext context did not include a deadline")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("processor did not exit after cancel")
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

type operationLogRecorderSpy struct {
	mu   sync.Mutex
	logs []*model.OperationLog
}

func (r *operationLogRecorderSpy) RecordContext(ctx context.Context, log *model.OperationLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, log)
	return ctx.Err()
}

func (r *operationLogRecorderSpy) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.logs)
}

type blockingOperationLogRecorder struct {
	started     chan struct{}
	finished    chan struct{}
	hadDeadline bool
}

func newBlockingOperationLogRecorder() *blockingOperationLogRecorder {
	return &blockingOperationLogRecorder{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (r *blockingOperationLogRecorder) RecordContext(ctx context.Context, log *model.OperationLog) error {
	if _, ok := ctx.Deadline(); ok {
		r.hadDeadline = true
	}
	close(r.started)
	<-ctx.Done()
	close(r.finished)
	return ctx.Err()
}
