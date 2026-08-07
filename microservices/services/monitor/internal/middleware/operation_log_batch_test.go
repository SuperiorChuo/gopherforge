package middleware

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/model"
)

// batchRecorderSpy implements both the single and batch recorder interfaces so
// tests can assert which path the processor took.
type batchRecorderSpy struct {
	mu           sync.Mutex
	batches      [][]*model.OperationLog
	singleWrites int
	flushed      chan struct{}
}

func newBatchRecorderSpy() *batchRecorderSpy {
	return &batchRecorderSpy{flushed: make(chan struct{}, 64)}
}

func (r *batchRecorderSpy) RecordContext(_ context.Context, log *model.OperationLog) error {
	r.mu.Lock()
	r.singleWrites++
	r.mu.Unlock()
	return nil
}

func (r *batchRecorderSpy) RecordBatchContext(_ context.Context, logs []*model.OperationLog) error {
	// The processor reuses its slice after flushing, so copy before storing.
	batch := make([]*model.OperationLog, len(logs))
	copy(batch, logs)

	r.mu.Lock()
	r.batches = append(r.batches, batch)
	r.mu.Unlock()

	select {
	case r.flushed <- struct{}{}:
	default:
	}
	return nil
}

func (r *batchRecorderSpy) snapshot() ([][]*model.OperationLog, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	batches := make([][]*model.OperationLog, len(r.batches))
	copy(batches, r.batches)
	return batches, r.singleWrites
}

func (r *batchRecorderSpy) totalLogs() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, batch := range r.batches {
		total += len(batch)
	}
	return total
}

func (r *batchRecorderSpy) waitForLogs(t *testing.T, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if r.totalLogs() >= want {
			return
		}
		select {
		case <-r.flushed:
		case <-deadline:
			t.Fatalf("persisted logs = %d, want %d within %s", r.totalLogs(), want, timeout)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestOperationLogProcessorFlushesWhenBatchSizeReached(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const batchSize = 5
	queue := make(chan *model.OperationLog, 32)
	recorder := newBatchRecorderSpy()

	// A long flush interval guarantees any write we observe came from the
	// size trigger, not the timer.
	done := processLogsBatched(ctx, queue, recorder, 500*time.Millisecond, batchSize, time.Hour)

	for i := 0; i < batchSize; i++ {
		queue <- &model.OperationLog{Path: "/batched"}
	}

	recorder.waitForLogs(t, batchSize, time.Second)

	batches, singleWrites := recorder.snapshot()
	if len(batches) != 1 {
		t.Fatalf("batch count = %d, want 1", len(batches))
	}
	if len(batches[0]) != batchSize {
		t.Fatalf("batch size = %d, want %d", len(batches[0]), batchSize)
	}
	if singleWrites != 0 {
		t.Fatalf("per-entry writes = %d, want 0 for a batch-capable recorder", singleWrites)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processor did not exit after cancel")
	}
}

func TestOperationLogProcessorFlushesPartialBatchOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := make(chan *model.OperationLog, 8)
	recorder := newBatchRecorderSpy()

	// Batch size far above what we enqueue: only the timer can flush.
	done := processLogsBatched(ctx, queue, recorder, 500*time.Millisecond, 1000, 20*time.Millisecond)

	queue <- &model.OperationLog{Path: "/timer/1"}
	queue <- &model.OperationLog{Path: "/timer/2"}

	recorder.waitForLogs(t, 2, time.Second)

	batches, _ := recorder.snapshot()
	if len(batches) == 0 {
		t.Fatal("timer did not flush the partial batch")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processor did not exit after cancel")
	}
}

func TestOperationLogProcessorFlushesBufferedLogsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	queue := make(chan *model.OperationLog, 8)
	queue <- &model.OperationLog{Path: "/shutdown/1"}
	queue <- &model.OperationLog{Path: "/shutdown/2"}
	queue <- &model.OperationLog{Path: "/shutdown/3"}
	cancel()

	recorder := newBatchRecorderSpy()
	// Batch size and interval both large: without shutdown draining nothing
	// would ever be written.
	done := processLogsBatched(ctx, queue, recorder, 500*time.Millisecond, 1000, time.Hour)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processor did not exit while draining queued logs")
	}

	if got := recorder.totalLogs(); got != 3 {
		t.Fatalf("persisted logs on shutdown = %d, want 3", got)
	}
}

func TestOperationLogProcessorShutdownFlushUsesLiveContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	queue := make(chan *model.OperationLog, 4)
	queue <- &model.OperationLog{Path: "/shutdown/ctx"}
	cancel()

	recorder := &contextCapturingBatchRecorder{}
	done := processLogsBatched(ctx, queue, recorder, 500*time.Millisecond, 1000, time.Hour)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processor did not exit while draining queued logs")
	}

	if recorder.calls == 0 {
		t.Fatal("shutdown flush did not reach the recorder")
	}
	if recorder.ctxErr != nil {
		t.Fatalf("shutdown flush context was already canceled: %v", recorder.ctxErr)
	}
	if !recorder.hadDeadline {
		t.Fatal("shutdown flush context did not carry the write timeout")
	}
}

func TestOperationLogProcessorFallsBackToPerEntryWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := make(chan *model.OperationLog, 8)
	// operationLogRecorderSpy implements only RecordContext.
	recorder := &operationLogRecorderSpy{}
	done := processLogsBatched(ctx, queue, recorder, 500*time.Millisecond, 2, 20*time.Millisecond)

	queue <- &model.OperationLog{Path: "/fallback/1"}
	queue <- &model.OperationLog{Path: "/fallback/2"}

	deadline := time.After(time.Second)
	for recorder.count() < 2 {
		select {
		case <-deadline:
			t.Fatalf("per-entry writes = %d, want 2", recorder.count())
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processor did not exit after cancel")
	}
}

func TestOperationLogDropIsObservable(t *testing.T) {
	before := OperationLogDroppedTotal()

	noteOperationLogDropped()

	if got := OperationLogDroppedTotal(); got != before+1 {
		t.Fatalf("dropped total = %d, want %d", got, before+1)
	}

	metrics := PrometheusMetrics()
	if !strings.Contains(metrics, "go_admin_kit_operation_log_dropped_total") {
		t.Fatal("Prometheus output is missing the operation log drop counter")
	}
	if !strings.Contains(metrics, "go_admin_kit_operation_log_queue_length") {
		t.Fatal("Prometheus output is missing the operation log queue gauge")
	}
}

func TestOperationLoggerDropsAreCountedWhenQueueIsFull(t *testing.T) {
	before := OperationLogDroppedTotal()

	full := make(chan *model.OperationLog, 1)
	full <- &model.OperationLog{Path: "/occupied"}

	// Mirrors the middleware's non-blocking enqueue.
	select {
	case full <- &model.OperationLog{Path: "/dropped"}:
		t.Fatal("enqueue unexpectedly succeeded on a full queue")
	default:
		noteOperationLogDropped()
	}

	if got := OperationLogDroppedTotal(); got != before+1 {
		t.Fatalf("dropped total = %d, want %d", got, before+1)
	}
}

type contextCapturingBatchRecorder struct {
	mu          sync.Mutex
	calls       int
	ctxErr      error
	hadDeadline bool
}

func (r *contextCapturingBatchRecorder) RecordContext(context.Context, *model.OperationLog) error {
	return nil
}

func (r *contextCapturingBatchRecorder) RecordBatchContext(ctx context.Context, logs []*model.OperationLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.ctxErr = ctx.Err()
	_, r.hadDeadline = ctx.Deadline()
	return nil
}
