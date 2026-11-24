package middleware

import (
	"context"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	localmodel "github.com/go-admin-kit/services/identity/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/oplog"
)

// OperationLogOptions configures operation logging.
type OperationLogOptions = oplog.Options

// OperationLogRecorder persists operation logs queued by the middleware.
type OperationLogRecorder = oplog.Recorder[localmodel.OperationLog]

// OperationLogBatchRecorder persists several queued operation logs in one round trip.
type OperationLogBatchRecorder = oplog.BatchRecorder[localmodel.OperationLog]

type operationLogRecorder = OperationLogRecorder

const operationLogBatchSize = oplog.DefaultBatchSize

var opLogEngine = oplog.New(oplog.Config[localmodel.OperationLog]{
	ServiceName: "identity-service",
	ResolveActor: func(c *gin.Context) (string, string) {
		actor := GetAuditActor(c)
		return actor.ActorType, actor.ActorID
	},
	Build: func(e oplog.Entry) *localmodel.OperationLog {
		return &localmodel.OperationLog{
			UserID:       e.UserID,
			Username:     e.Username,
			ActorType:    e.ActorType,
			ActorID:      e.ActorID,
			RequestID:    e.RequestID,
			Module:       e.Module,
			Action:       e.Action,
			Method:       e.Method,
			Path:         e.Path,
			Query:        e.Query,
			RequestBody:  e.RequestBody,
			ResponseBody: e.ResponseBody,
			Status:       e.Status,
			IP:           e.IP,
			UserAgent:    e.UserAgent,
			Latency:      e.Latency,
			ErrorMsg:     e.ErrorMsg,
		}
	},
})

// OperationLogger records operation logs with default options.
func OperationLogger() gin.HandlerFunc { return opLogEngine.DefaultMiddleware() }

// OperationLoggerWithOptions records operation logs with custom options.
func OperationLoggerWithOptions(opts OperationLogOptions) gin.HandlerFunc {
	return opLogEngine.Middleware(opts)
}

// StartOperationLogProcessor starts the background batched operation log processor.
func StartOperationLogProcessor(ctx context.Context, recorder OperationLogRecorder) <-chan struct{} {
	return opLogEngine.Start(ctx, recorder)
}

func readRequestBodyForLog(body io.ReadCloser, maxSize int) (string, io.ReadCloser, error) {
	return oplog.ReadRequestBodyForLog(body, maxSize)
}

func filterSensitiveFields(body string) string { return oplog.FilterSensitiveFields(body) }

func processLogs(ctx context.Context, queue <-chan *localmodel.OperationLog, recorder operationLogRecorder, writeTimeout time.Duration) <-chan struct{} {
	return opLogEngine.Process(ctx, queue, recorder, writeTimeout)
}

func processLogsBatched(ctx context.Context, queue <-chan *localmodel.OperationLog, recorder operationLogRecorder, writeTimeout time.Duration, batchSize int, flushInterval time.Duration) <-chan struct{} {
	return opLogEngine.ProcessBatched(ctx, queue, recorder, writeTimeout, batchSize, flushInterval)
}
