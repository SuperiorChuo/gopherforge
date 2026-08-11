package middleware

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
	localmodel "github.com/go-admin-kit/services/monitor/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/mask"

	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
)

// Operation module mapping.
var moduleMap = map[string]string{
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

// Operation action mapping.
var actionMap = map[string]string{
	"GET":    "Query",
	"POST":   "Create",
	"PUT":    "Update",
	"DELETE": "Delete",
}

// responseBodyWriter captures response bodies when enabled.
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// OperationLogger records operation logs with default options.
func OperationLogger() gin.HandlerFunc {
	return OperationLoggerWithOptions(OperationLogOptions{
		RecordRequestBody:   true,
		RecordResponseBody:  false,
		MaxRequestBodySize:  4096,
		MaxResponseBodySize: 4096,
		SkipPaths: []string{
			"/api/v1/health",
			"/api/v1/captcha",
		},
	})
}

// OperationLogOptions configures operation logging.
type OperationLogOptions struct {
	RecordRequestBody   bool
	RecordResponseBody  bool
	MaxRequestBodySize  int
	MaxResponseBodySize int
	SkipPaths           []string
}

// OperationLoggerWithOptions records operation logs with custom options.
func OperationLoggerWithOptions(opts OperationLogOptions) gin.HandlerFunc {
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
		// GET/HEAD carry no business payload; skip the body-read wrapper
		// (the old behavior only ever recorded an empty string for them).
		if opts.RecordRequestBody && c.Request.Body != nil &&
			c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			bodyPreview, restoredBody, err := readRequestBodyForLog(c.Request.Body, opts.MaxRequestBodySize)
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
		actor := GetAuditActor(c)
		requestID := sharedmw.GetRequestID(c)

		var errorMsg string
		if len(c.Errors) > 0 {
			errorMsg = c.Errors.String()
		}

		module := getModule(fullPath)
		action := getAction(c.Request.Method, fullPath)

		// Masking runs after the response is written: the full JSON
		// unmarshal/marshal no longer counts against request latency.
		requestBody = filterSensitiveFields(requestBody)

		log := &localmodel.OperationLog{
			UserID:       userID,
			Username:     username,
			ActorType:    actor.ActorType,
			ActorID:      actor.ActorID,
			RequestID:    requestID,
			Module:       module,
			Action:       action,
			Method:       c.Request.Method,
			Path:         fullPath,
			Query:        c.Request.URL.RawQuery,
			RequestBody:  requestBody,
			ResponseBody: responseBody,
			Status:       c.Writer.Status(),
			IP:           c.ClientIP(),
			UserAgent:    truncateString(c.Request.UserAgent(), 500),
			Latency:      time.Since(start).Milliseconds(),
			ErrorMsg:     truncateString(errorMsg, 1024),
		}

		select {
		case logChan <- log:
		default:
			// The queue is full: never drop silently, or a saturated write
			// path looks exactly like "no traffic" from the outside.
			noteOperationLogDropped()
		}
	}
}

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r replayReadCloser) Close() error {
	return r.closer.Close()
}

func readRequestBodyForLog(body io.ReadCloser, maxSize int) (string, io.ReadCloser, error) {
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

// logChanBufferSize limits queued operation log writes.
const logChanBufferSize = 1000

const operationLogWriteTimeout = 2 * time.Second

const (
	// operationLogBatchSize is the number of queued logs that triggers an
	// immediate flush.
	operationLogBatchSize = 200
	// operationLogFlushInterval bounds how long a partial batch waits before
	// being written, so low-traffic deployments still persist promptly.
	operationLogFlushInterval = 500 * time.Millisecond
	// operationLogDropWarnInterval throttles the "queue full" warning so a
	// sustained overload cannot flood the log itself.
	operationLogDropWarnInterval = 10 * time.Second
)

var logChan = make(chan *localmodel.OperationLog, logChanBufferSize)

var (
	operationLogDroppedTotal atomic.Uint64
	operationLogDropWarnedAt atomic.Int64
)

// OperationLogRecorder persists operation logs queued by the middleware.
type OperationLogRecorder interface {
	RecordContext(context.Context, *localmodel.OperationLog) error
}

// OperationLogBatchRecorder persists several queued operation logs in a single
// round trip. Recorders that implement it get batched writes; the rest fall
// back to one RecordContext call per entry.
type OperationLogBatchRecorder interface {
	RecordBatchContext(context.Context, []*localmodel.OperationLog) error
}

type operationLogRecorder = OperationLogRecorder

// noteOperationLogDropped counts a dropped operation log and emits a throttled
// warning so buffer saturation is observable instead of silent.
func noteOperationLogDropped() {
	dropped := operationLogDroppedTotal.Add(1)

	now := time.Now().UnixNano()
	last := operationLogDropWarnedAt.Load()
	if now-last < int64(operationLogDropWarnInterval) && last != 0 {
		return
	}
	if !operationLogDropWarnedAt.CompareAndSwap(last, now) {
		return
	}
	// The counter above is the durable signal; the log line is best effort and
	// must not panic before InitLogger has run (tests, early startup).
	if logger.Logger == nil {
		return
	}
	logger.Warn(
		"operation log queue full, dropping entries",
		logger.Int64("dropped_total", int64(dropped)),
		logger.Int("queue_capacity", logChanBufferSize),
	)
}

// OperationLogDroppedTotal reports how many operation logs were discarded
// because the async queue was full.
func OperationLogDroppedTotal() uint64 {
	return operationLogDroppedTotal.Load()
}

// OperationLogQueueLength reports the current depth of the async queue.
func OperationLogQueueLength() int {
	return len(logChan)
}

// writeOperationLogPrometheusMetrics appends operation-log queue metrics to the
// Prometheus exposition body.
func writeOperationLogPrometheusMetrics(b *strings.Builder) {
	b.WriteString("# HELP go_admin_kit_operation_log_dropped_total Operation logs discarded because the async queue was full.\n")
	b.WriteString("# TYPE go_admin_kit_operation_log_dropped_total counter\n")
	fmt.Fprintf(b, "go_admin_kit_operation_log_dropped_total %d\n", OperationLogDroppedTotal())
	b.WriteString("# HELP go_admin_kit_operation_log_queue_length Operation logs currently waiting to be persisted.\n")
	b.WriteString("# TYPE go_admin_kit_operation_log_queue_length gauge\n")
	fmt.Fprintf(b, "go_admin_kit_operation_log_queue_length %d\n", OperationLogQueueLength())
	b.WriteString("# HELP go_admin_kit_operation_log_queue_capacity Capacity of the async operation log queue.\n")
	b.WriteString("# TYPE go_admin_kit_operation_log_queue_capacity gauge\n")
	fmt.Fprintf(b, "go_admin_kit_operation_log_queue_capacity %d\n", logChanBufferSize)
}

// StartOperationLogProcessor starts the background operation log processor
// backed by the injected recorder.
func StartOperationLogProcessor(ctx context.Context, recorder OperationLogRecorder) <-chan struct{} {
	return processLogs(ctx, logChan, recorder, operationLogWriteTimeout)
}

// processLogs persists queued operation logs until ctx is canceled, using the
// default batch size and flush interval.
func processLogs(ctx context.Context, queue <-chan *localmodel.OperationLog, recorder operationLogRecorder, writeTimeout time.Duration) <-chan struct{} {
	return processLogsBatched(ctx, queue, recorder, writeTimeout, operationLogBatchSize, operationLogFlushInterval)
}

// processLogsBatched accumulates queued logs and writes them in batches, either
// when batchSize entries are pending or when flushInterval elapses. Whatever is
// still buffered at shutdown is drained and flushed before returning.
func processLogsBatched(
	ctx context.Context,
	queue <-chan *localmodel.OperationLog,
	recorder operationLogRecorder,
	writeTimeout time.Duration,
	batchSize int,
	flushInterval time.Duration,
) <-chan struct{} {
	if batchSize <= 0 {
		batchSize = operationLogBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = operationLogFlushInterval
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		batch := make([]*localmodel.OperationLog, 0, batchSize)
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		flush := func(parent context.Context) {
			if len(batch) == 0 {
				return
			}
			recordOperationLogBatch(parent, recorder, batch, writeTimeout)
			batch = batch[:0]
		}

		for {
			select {
			case <-ctx.Done():
				// Shutdown: pull everything still queued and land it with a
				// context that is not already canceled.
				batch = drainOperationLogs(queue, recorder, writeTimeout, batch, batchSize)
				flush(context.Background())
				return
			case log, ok := <-queue:
				if !ok {
					flush(context.Background())
					return
				}
				if log == nil {
					continue
				}
				batch = append(batch, log)
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

// drainOperationLogs pulls every immediately available log out of the queue,
// flushing whenever the pending batch reaches batchSize. Returns the leftover
// partial batch for the caller to flush.
func drainOperationLogs(
	queue <-chan *localmodel.OperationLog,
	recorder operationLogRecorder,
	writeTimeout time.Duration,
	batch []*localmodel.OperationLog,
	batchSize int,
) []*localmodel.OperationLog {
	if batchSize <= 0 {
		batchSize = operationLogBatchSize
	}
	for {
		select {
		case log, ok := <-queue:
			if !ok {
				return batch
			}
			if log == nil {
				continue
			}
			batch = append(batch, log)
			if len(batch) >= batchSize {
				recordOperationLogBatch(context.Background(), recorder, batch, writeTimeout)
				batch = batch[:0]
			}
		default:
			return batch
		}
	}
}

// recordOperationLogBatch writes a batch through the recorder's batch API when
// available, falling back to per-entry writes otherwise.
func recordOperationLogBatch(parent context.Context, recorder operationLogRecorder, logs []*localmodel.OperationLog, writeTimeout time.Duration) {
	if recorder == nil || len(logs) == 0 {
		return
	}
	if writeTimeout <= 0 {
		writeTimeout = operationLogWriteTimeout
	}

	if batcher, ok := recorder.(OperationLogBatchRecorder); ok {
		ctx, cancel := context.WithTimeout(parent, writeTimeout)
		defer cancel()
		_ = batcher.RecordBatchContext(ctx, logs)
		return
	}

	for _, log := range logs {
		recordOperationLog(parent, recorder, log, writeTimeout)
	}
}

func recordOperationLog(parent context.Context, recorder operationLogRecorder, log *localmodel.OperationLog, writeTimeout time.Duration) {
	if recorder == nil || log == nil {
		return
	}
	if writeTimeout <= 0 {
		writeTimeout = operationLogWriteTimeout
	}
	ctx, cancel := context.WithTimeout(parent, writeTimeout)
	defer cancel()
	_ = recorder.RecordContext(ctx, log)
}

// getModule resolves a module name from a route path.
func getModule(path string) string {
	for prefix, module := range moduleMap {
		if strings.HasPrefix(path, prefix) {
			return module
		}
	}
	return "Other"
}

// getAction resolves an operation action from method and path.
func getAction(method, path string) string {
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
	if strings.Contains(path, "/roles") && method == "POST" {
		return "Assign Roles"
	}
	if strings.Contains(path, "/permissions") && method == "POST" {
		return "Assign Permissions"
	}

	if action, ok := actionMap[method]; ok {
		return action
	}
	return method
}

// filterSensitiveFields masks sensitive JSON fields.
func filterSensitiveFields(body string) string {
	return mask.RedactJSON(body)
}

// truncateString truncates a string to maxLen bytes.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
