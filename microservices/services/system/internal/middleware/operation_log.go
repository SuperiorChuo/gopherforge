package middleware

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/oplog"
	localmodel "github.com/go-admin-kit/services/system/internal/model"
)

// OperationLogOptions 配置操作日志记录。
type OperationLogOptions = oplog.Options

// OperationLogRecorder 持久化中间件入队的操作日志。
type OperationLogRecorder = oplog.Recorder[localmodel.OperationLog]

// OperationLogBatchRecorder 支持一次批量持久化多条操作日志。
type OperationLogBatchRecorder = oplog.BatchRecorder[localmodel.OperationLog]

type operationLogRecorder = OperationLogRecorder

const operationLogBatchSize = oplog.DefaultBatchSize

var opLogEngine = oplog.New(oplog.Config[localmodel.OperationLog]{
	ServiceName: "system-service",
	ExtraModules: map[string]string{
		"/api/v1/edge-certs": "Edge Certificates",
	},
	ResolveAction: func(method, path string) string {
		if !strings.HasPrefix(path, "/api/v1/edge-certs/") || method != http.MethodPost {
			return ""
		}
		switch {
		case strings.HasSuffix(path, "/export"):
			return "Export Private Key"
		case strings.HasSuffix(path, "/issue"):
			return "Queue Certificate Issue"
		case strings.HasSuffix(path, "/renew"):
			return "Queue Certificate Renewal"
		case strings.HasSuffix(path, "/deploy"):
			return "Queue Certificate Deploy"
		case strings.HasSuffix(path, "/probe"):
			return "Queue Certificate Probe"
		default:
			return ""
		}
	},
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

// OperationLogger 使用默认选项记录操作日志。
func OperationLogger() gin.HandlerFunc { return opLogEngine.DefaultMiddleware() }

// OperationLoggerWithOptions 使用自定义选项记录操作日志。
func OperationLoggerWithOptions(opts OperationLogOptions) gin.HandlerFunc {
	return opLogEngine.Middleware(opts)
}

// StartOperationLogProcessor 启动后台批量操作日志处理器。
func StartOperationLogProcessor(ctx context.Context, recorder OperationLogRecorder) <-chan struct{} {
	return opLogEngine.Start(ctx, recorder)
}

func getAction(method, path string) string { return opLogEngine.ResolveAction(method, path) }

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
