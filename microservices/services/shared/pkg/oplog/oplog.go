// Package oplog provides the converged, batched async operation-log middleware
// and background processor shared across infrastructure and business services.
package oplog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/mask"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
)

// Default tuning constants.
const (
	DefaultBufferSize       = 1000
	DefaultWriteTimeout     = 2 * time.Second
	DefaultBatchSize        = 200
	DefaultFlushInterval    = 500 * time.Millisecond
	DefaultDropWarnInterval = 10 * time.Second
)

var defaultModuleMap = map[string]string{
	"/api/v1/users":          "User Management",
	"/api/v1/roles":          "Role Management",
	"/api/v1/permissions":    "Permission Management",
	"/api/v1/menus":          "Menu Management",
	"/api/v1/user":           "Profile",
	"/api/v1/oauth":          "OAuth",
	"/api/v1/login":          "Login",
	"/api/v1/register":       "Registration",
	"/api/v1/captcha":        "Captcha",
	"/api/v1/operation-logs": "Operation Logs",
}

var defaultActionMap = map[string]string{
	"GET":    "Query",
	"POST":   "Create",
	"PUT":    "Update",
	"DELETE": "Delete",
}

// Options configures operation logging capture behavior.
type Options struct {
	RecordRequestBody   bool
	RecordResponseBody  bool
	MaxRequestBodySize  int
	MaxResponseBodySize int
	SkipPaths           []string
}

// DefaultOptions returns standard operation-log middleware options.
func DefaultOptions() Options {
	return Options{
		RecordRequestBody:   true,
		RecordResponseBody:  false,
		MaxRequestBodySize:  4096,
		MaxResponseBodySize: 4096,
		SkipPaths: []string{
			"/api/v1/health",
			"/api/v1/captcha",
		},
	}
}

// Entry holds the normalized fields captured from an HTTP request/response pair.
type Entry struct {
	TenantID     uint
	UserID       uint
	Username     string
	ActorType    string
	ActorID      string
	RequestID    string
	Module       string
	Action       string
	Method       string
	Path         string
	Query        string
	RequestBody  string
	ResponseBody string
	Status       int
	IP           string
	UserAgent    string
	Latency      int64
	ErrorMsg     string
}

// Recorder persists individual operation logs queued by the middleware.
type Recorder[M any] interface {
	RecordContext(context.Context, *M) error
}

// BatchRecorder persists several queued operation logs in a single round trip.
type BatchRecorder[M any] interface {
	RecordBatchContext(context.Context, []*M) error
}

// Config configures a per-service Engine instance.
type Config[M any] struct {
	ServiceName   string
	BufferSize    int
	BatchSize     int
	FlushInterval time.Duration
	WriteTimeout  time.Duration
	ExtraModules  map[string]string
	ResolveAction func(method, path string) string
	ResolveActor  func(c *gin.Context) (actorType, actorID string)
	ResolveTenant func(c *gin.Context) uint
	Build         func(Entry) *M
}

// Engine encapsulates the async queue, drop metrics, Gin middleware, and
// batched persistence processor for a single service's operation log model.
type Engine[M any] struct {
	serviceName   string
	queue         chan *M
	bufferSize    int
	batchSize     int
	flushInterval time.Duration
	writeTimeout  time.Duration
	modules       map[string]string
	resolveAction func(method, path string) string
	resolveActor  func(c *gin.Context) (actorType, actorID string)
	resolveTenant func(c *gin.Context) uint
	build         func(Entry) *M

	droppedTotal atomic.Uint64
	dropWarnedAt atomic.Int64
}

// New creates a new operation-log Engine for a service.
func New[M any](cfg Config[M]) *Engine[M] {
	bufSize := cfg.BufferSize
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = DefaultFlushInterval
	}
	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = DefaultWriteTimeout
	}
	mods := make(map[string]string, len(defaultModuleMap)+len(cfg.ExtraModules))
	for k, v := range defaultModuleMap {
		mods[k] = v
	}
	for k, v := range cfg.ExtraModules {
		mods[k] = v
	}
	return &Engine[M]{
		serviceName:   cfg.ServiceName,
		queue:         make(chan *M, bufSize),
		bufferSize:    bufSize,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		writeTimeout:  writeTimeout,
		modules:       mods,
		resolveAction: cfg.ResolveAction,
		resolveActor:  cfg.ResolveActor,
		resolveTenant: cfg.ResolveTenant,
		build:         cfg.Build,
	}
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// DefaultMiddleware returns Gin middleware configured with DefaultOptions.
func (e *Engine[M]) DefaultMiddleware() gin.HandlerFunc {
	return e.Middleware(DefaultOptions())
}

// Middleware returns Gin middleware configured with custom Options.
func (e *Engine[M]) Middleware(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		for _, skipPath := range opts.SkipPaths {
			if strings.HasPrefix(path, skipPath) {
				c.Next()
				return
			}
		}

		start := time.Now()

		var requestBody string
		// GET/HEAD carry no business payload; skip the body-read wrapper.
		if opts.RecordRequestBody && c.Request.Body != nil &&
			c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			bodyPreview, restoredBody, err := ReadRequestBodyForLog(c.Request.Body, opts.MaxRequestBodySize)
			c.Request.Body = restoredBody
			if err == nil {
				requestBody = bodyPreview
			}
		}

		var responseBody string
		if opts.RecordResponseBody {
			blw := &responseBodyWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
			c.Writer = blw
			c.Next()
			if blw.body.Len() > opts.MaxResponseBodySize {
				responseBody = blw.body.String()[:opts.MaxResponseBodySize] + "...[truncated]"
			} else {
				responseBody = blw.body.String()
			}
		} else {
			c.Next()
		}

		fullPath := c.FullPath()
		if fullPath == "" {
			fullPath = c.Request.URL.Path
		}

		var userID uint
		if uid, ok := c.Get("user_id"); ok {
			if v, ok := uid.(uint); ok {
				userID = v
			}
		}

		var username string
		if u, ok := c.Get("username"); ok {
			if v, ok := u.(string); ok {
				username = v
			}
		}

		var actorType, actorID string
		if e.resolveActor != nil {
			actorType, actorID = e.resolveActor(c)
		}

		var tenantID uint
		if e.resolveTenant != nil {
			tenantID = e.resolveTenant(c)
		}

		requestID := sharedmw.GetRequestID(c)

		var errorMsg string
		if len(c.Errors) > 0 {
			errorMsg = c.Errors.String()
		}

		// Masking runs after the response is written so it never adds to request latency.
		requestBody = FilterSensitiveFields(requestBody)

		if e.build == nil {
			return
		}
		record := e.build(Entry{
			TenantID:     tenantID,
			UserID:       userID,
			Username:     username,
			ActorType:    actorType,
			ActorID:      actorID,
			RequestID:    requestID,
			Module:       e.ResolveModule(fullPath),
			Action:       e.ResolveAction(c.Request.Method, fullPath),
			Method:       c.Request.Method,
			Path:         fullPath,
			Query:        c.Request.URL.RawQuery,
			RequestBody:  requestBody,
			ResponseBody: responseBody,
			Status:       c.Writer.Status(),
			IP:           c.ClientIP(),
			UserAgent:    TruncateString(c.Request.UserAgent(), 500),
			Latency:      time.Since(start).Milliseconds(),
			ErrorMsg:     TruncateString(errorMsg, 1024),
		})
		if record == nil {
			return
		}

		select {
		case e.queue <- record:
		default:
			e.NoteDropped()
		}
	}
}

// NoteDropped counts a dropped operation log and emits a throttled warning.
func (e *Engine[M]) NoteDropped() {
	dropped := e.droppedTotal.Add(1)

	now := time.Now().UnixNano()
	last := e.dropWarnedAt.Load()
	if now-last < int64(DefaultDropWarnInterval) && last != 0 {
		return
	}
	if !e.dropWarnedAt.CompareAndSwap(last, now) {
		return
	}
	if logger.Logger == nil {
		return
	}
	logger.Warn(
		"operation log queue full, dropping entries",
		logger.String("service", e.serviceName),
		logger.Int64("dropped_total", int64(dropped)),
		logger.Int("queue_capacity", e.bufferSize),
	)
}

// DroppedTotal returns total dropped entries since process startup.
func (e *Engine[M]) DroppedTotal() uint64 { return e.droppedTotal.Load() }

// QueueLength returns current queued depth.
func (e *Engine[M]) QueueLength() int { return len(e.queue) }

// QueueCapacity returns configured queue capacity.
func (e *Engine[M]) QueueCapacity() int { return e.bufferSize }

// WritePrometheusMetrics appends operation-log queue metrics in Prometheus format.
func (e *Engine[M]) WritePrometheusMetrics(b *strings.Builder) {
	b.WriteString("# HELP go_admin_kit_operation_log_dropped_total Operation logs discarded because the async queue was full.\n")
	b.WriteString("# TYPE go_admin_kit_operation_log_dropped_total counter\n")
	fmt.Fprintf(b, "go_admin_kit_operation_log_dropped_total %d\n", e.DroppedTotal())
	b.WriteString("# HELP go_admin_kit_operation_log_queue_length Operation logs currently waiting to be persisted.\n")
	b.WriteString("# TYPE go_admin_kit_operation_log_queue_length gauge\n")
	fmt.Fprintf(b, "go_admin_kit_operation_log_queue_length %d\n", e.QueueLength())
	b.WriteString("# HELP go_admin_kit_operation_log_queue_capacity Capacity of the async operation log queue.\n")
	b.WriteString("# TYPE go_admin_kit_operation_log_queue_capacity gauge\n")
	fmt.Fprintf(b, "go_admin_kit_operation_log_queue_capacity %d\n", e.QueueCapacity())
}

// Start starts the background batched processor reading from the Engine's queue.
func (e *Engine[M]) Start(ctx context.Context, recorder Recorder[M]) <-chan struct{} {
	return e.ProcessBatched(ctx, e.queue, recorder, e.writeTimeout, e.batchSize, e.flushInterval)
}

// Process persists queued logs from an injected channel using default batch settings.
func (e *Engine[M]) Process(ctx context.Context, queue <-chan *M, recorder Recorder[M], writeTimeout time.Duration) <-chan struct{} {
	return e.ProcessBatched(ctx, queue, recorder, writeTimeout, e.batchSize, e.flushInterval)
}

// ProcessBatched accumulates queued logs and writes them in batches, flushing on
// batchSize, flushInterval, or shutdown drain (with a live context).
func (e *Engine[M]) ProcessBatched(
	ctx context.Context,
	queue <-chan *M,
	recorder Recorder[M],
	writeTimeout time.Duration,
	batchSize int,
	flushInterval time.Duration,
) <-chan struct{} {
	if batchSize <= 0 {
		batchSize = e.batchSize
	}
	if flushInterval <= 0 {
		flushInterval = e.flushInterval
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		batch := make([]*M, 0, batchSize)
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		flush := func(parent context.Context) {
			if len(batch) == 0 {
				return
			}
			e.recordBatch(parent, recorder, batch, writeTimeout)
			batch = batch[:0]
		}

		for {
			select {
			case <-ctx.Done():
				batch = e.drain(queue, recorder, writeTimeout, batch, batchSize)
				flush(context.Background())
				return
			case item, ok := <-queue:
				if !ok {
					flush(context.Background())
					return
				}
				if item == nil {
					continue
				}
				batch = append(batch, item)
				if len(batch) >= batchSize {
					flush(ctx)
					ticker.Reset(flushInterval)
				}
			case <-ticker.C:
				flush(ctx)
			}
		}
	}()
	return done
}

func (e *Engine[M]) drain(
	queue <-chan *M,
	recorder Recorder[M],
	writeTimeout time.Duration,
	batch []*M,
	batchSize int,
) []*M {
	if batchSize <= 0 {
		batchSize = e.batchSize
	}
	for {
		select {
		case item, ok := <-queue:
			if !ok {
				return batch
			}
			if item == nil {
				continue
			}
			batch = append(batch, item)
			if len(batch) >= batchSize {
				e.recordBatch(context.Background(), recorder, batch, writeTimeout)
				batch = batch[:0]
			}
		default:
			return batch
		}
	}
}

func (e *Engine[M]) recordBatch(parent context.Context, recorder Recorder[M], logs []*M, writeTimeout time.Duration) {
	if len(logs) == 0 {
		return
	}
	if auditevents.Enabled() {
		for _, item := range logs {
			if item != nil {
				auditevents.PublishOperationLog(e.serviceName, item)
			}
		}
		return
	}
	if recorder == nil {
		return
	}
	if writeTimeout <= 0 {
		writeTimeout = e.writeTimeout
	}
	if batcher, ok := recorder.(BatchRecorder[M]); ok {
		ctx, cancel := context.WithTimeout(parent, writeTimeout)
		defer cancel()
		_ = batcher.RecordBatchContext(ctx, logs)
		return
	}
	for _, item := range logs {
		if item == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(parent, writeTimeout)
		_ = recorder.RecordContext(ctx, item)
		cancel()
	}
}

// ResolveModule resolves a human-readable module name from a route path.
func (e *Engine[M]) ResolveModule(path string) string {
	for prefix, module := range e.modules {
		if strings.HasPrefix(path, prefix) {
			return module
		}
	}
	return "Other"
}

// ResolveAction resolves an operation action from HTTP method and path.
func (e *Engine[M]) ResolveAction(method, path string) string {
	if e.resolveAction != nil {
		if act := e.resolveAction(method, path); act != "" {
			return act
		}
	}
	return DefaultAction(method, path)
}

// DefaultAction resolves standard action labels from HTTP method and path.
func DefaultAction(method, path string) string {
	if strings.HasSuffix(path, "/login") {
		return "Login"
	}
	if strings.HasSuffix(path, "/register") {
		return "Register"
	}
	if strings.HasSuffix(path, "/password") {
		return "Change Password"
	}
	if strings.Contains(path, "/status") {
		return "Update Status"
	}
	if strings.Contains(path, "/roles") && method == http.MethodPost {
		return "Assign Roles"
	}
	if strings.Contains(path, "/permissions") && method == http.MethodPost {
		return "Assign Permissions"
	}
	if action, ok := defaultActionMap[method]; ok {
		return action
	}
	return method
}

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r replayReadCloser) Close() error { return r.closer.Close() }

// ReadRequestBodyForLog reads up to maxSize+1 bytes for preview while returning
// a ReadCloser that replays the complete original body to downstream handlers.
func ReadRequestBodyForLog(body io.ReadCloser, maxSize int) (string, io.ReadCloser, error) {
	if maxSize < 0 {
		maxSize = 0
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(body, int64(maxSize)+1))
	restoredBody := replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(bodyBytes), body),
		closer: body,
	}
	if err != nil {
		return "", restoredBody, err
	}
	if len(bodyBytes) > maxSize {
		return string(bodyBytes[:maxSize]) + "...[truncated]", restoredBody, nil
	}
	return string(bodyBytes), restoredBody, nil
}

// FilterSensitiveFields masks sensitive JSON fields.
func FilterSensitiveFields(body string) string { return mask.RedactJSON(body) }

// TruncateString truncates s to at most maxLen bytes.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
