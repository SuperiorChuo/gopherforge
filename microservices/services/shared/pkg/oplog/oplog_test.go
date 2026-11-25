package oplog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type sampleLog struct {
	TenantID uint
	Path     string
	Module   string
	Action   string
	Body     string
	ActorID  string
}

type spyBatchRecorder struct {
	mu      sync.Mutex
	batches [][]*sampleLog
	singles int
	flushed chan struct{}
}

func (s *spyBatchRecorder) RecordContext(_ context.Context, _ *sampleLog) error {
	s.mu.Lock()
	s.singles++
	s.mu.Unlock()
	return nil
}

func (s *spyBatchRecorder) RecordBatchContext(_ context.Context, logs []*sampleLog) error {
	cp := make([]*sampleLog, len(logs))
	copy(cp, logs)
	s.mu.Lock()
	s.batches = append(s.batches, cp)
	s.mu.Unlock()
	select {
	case s.flushed <- struct{}{}:
	default:
	}
	return nil
}

func (s *spyBatchRecorder) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, b := range s.batches {
		n += len(b)
	}
	return n
}

func TestEngineMiddlewareCapturesAndBatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := New(Config[sampleLog]{
		ServiceName:   "test-service",
		BatchSize:     2,
		FlushInterval: 20 * time.Millisecond,
		ExtraModules:  map[string]string{"/api/v1/custom": "Custom Module"},
		ResolveActor:  func(*gin.Context) (string, string) { return "operator", "u-1" },
		ResolveTenant: func(*gin.Context) uint { return 42 },
		Build: func(e Entry) *sampleLog {
			return &sampleLog{
				TenantID: e.TenantID,
				Path:     e.Path,
				Module:   e.Module,
				Action:   e.Action,
				Body:     e.RequestBody,
				ActorID:  e.ActorID,
			}
		},
	})

	rec := &spyBatchRecorder{flushed: make(chan struct{}, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	done := eng.Start(ctx, rec)

	r := gin.New()
	r.Use(eng.DefaultMiddleware())
	r.POST("/api/v1/custom/items", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/api/v1/health/live", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// Skipped path must not enqueue
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))

	// Two POSTs hit BatchSize=2 and mask password
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/custom/items", strings.NewReader(`{"password":"secret","name":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	select {
	case <-rec.flushed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for batch flush")
	}

	cancel()
	<-done

	if rec.total() != 2 || rec.singles != 0 {
		t.Fatalf("total=%d singles=%d, want 2 batched writes", rec.total(), rec.singles)
	}
	first := rec.batches[0][0]
	if first.TenantID != 42 || first.Module != "Custom Module" || first.Action != "Create" || first.ActorID != "u-1" {
		t.Fatalf("unexpected log entry: %+v", first)
	}
	if strings.Contains(first.Body, "secret") {
		t.Fatalf("password leaked into request body: %s", first.Body)
	}

	// Drop metrics exposition
	eng.NoteDropped()
	var b strings.Builder
	eng.WritePrometheusMetrics(&b)
	if !strings.Contains(b.String(), "go_admin_kit_operation_log_dropped_total 1") {
		t.Fatalf("unexpected metrics: %s", b.String())
	}
}
