package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	"github.com/go-admin-kit/services/shared/pkg/mask"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
	localmodel "github.com/go-admin-kit/services/system/internal/model"
)

// 操作日志模块映射。
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
	"/api/v1/edge-certs":     "Edge Certificates",
}

// 操作日志动作映射。
var actionMap = map[string]string{
	"GET":    "Query",
	"POST":   "Create",
	"PUT":    "Update",
	"DELETE": "Delete",
}

// responseBodyWriter 在开启时捕获响应体。
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// OperationLogger 使用默认选项记录操作日志。
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

// OperationLogOptions 配置操作日志记录。
type OperationLogOptions struct {
	RecordRequestBody   bool
	RecordResponseBody  bool
	MaxRequestBodySize  int
	MaxResponseBodySize int
	SkipPaths           []string
}

// OperationLoggerWithOptions 使用自定义选项记录操作日志。
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
		// GET/HEAD 不携带业务负载，跳过请求体读取包装器
		// （旧行为对它们也只会记录空字符串）。
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

		// 脱敏在响应写出之后执行：完整的 JSON 反序列化/序列化
		// 不再计入请求延迟。
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

// logChanBufferSize 限制排队中的操作日志写入数量。
const logChanBufferSize = 1000

const operationLogWriteTimeout = 2 * time.Second

var logChan = make(chan *localmodel.OperationLog, logChanBufferSize)

// OperationLogRecorder 持久化由中间件排队的操作日志。
type OperationLogRecorder interface {
	RecordContext(context.Context, *localmodel.OperationLog) error
}

type operationLogRecorder = OperationLogRecorder

// StartOperationLogProcessor 启动由注入的 recorder 支撑的后台操作日志处理器。
func StartOperationLogProcessor(ctx context.Context, recorder OperationLogRecorder) <-chan struct{} {
	return processLogs(ctx, logChan, recorder, operationLogWriteTimeout)
}

// processLogs 持久化排队中的操作日志，直到 ctx 被取消。
func processLogs(ctx context.Context, queue <-chan *localmodel.OperationLog, recorder operationLogRecorder, writeTimeout time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				drainOperationLogs(queue, recorder, writeTimeout)
				return
			case log, ok := <-queue:
				if !ok {
					return
				}
				recordOperationLog(ctx, recorder, log, writeTimeout)
			}
		}
	}()
	return done
}

func drainOperationLogs(queue <-chan *localmodel.OperationLog, recorder operationLogRecorder, writeTimeout time.Duration) {
	for {
		select {
		case log, ok := <-queue:
			if !ok {
				return
			}
			recordOperationLog(context.Background(), recorder, log, writeTimeout)
		default:
			return
		}
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
	if auditevents.Enabled() {
		auditevents.PublishOperationLog("system-service", log)
		return
	}
	_ = recorder.RecordContext(ctx, log)
}

// getModule 从路由路径解析模块名称。
func getModule(path string) string {
	for prefix, module := range moduleMap {
		if strings.HasPrefix(path, prefix) {
			return module
		}
	}
	return "Other"
}

// getAction 从方法与路径解析操作动作。
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
	isEdgeCertificateAction := strings.HasPrefix(path, "/api/v1/edge-certs/")
	if isEdgeCertificateAction && strings.HasSuffix(path, "/export") && method == http.MethodPost {
		return "Export Private Key"
	}
	if isEdgeCertificateAction && strings.HasSuffix(path, "/issue") && method == http.MethodPost {
		return "Queue Certificate Issue"
	}
	if isEdgeCertificateAction && strings.HasSuffix(path, "/renew") && method == http.MethodPost {
		return "Queue Certificate Renewal"
	}
	if isEdgeCertificateAction && strings.HasSuffix(path, "/deploy") && method == http.MethodPost {
		return "Queue Certificate Deploy"
	}
	if isEdgeCertificateAction && strings.HasSuffix(path, "/probe") && method == http.MethodPost {
		return "Queue Certificate Probe"
	}

	if action, ok := actionMap[method]; ok {
		return action
	}
	return method
}

// filterSensitiveFields 对敏感 JSON 字段进行脱敏。
func filterSensitiveFields(body string) string {
	return mask.RedactJSON(body)
}

// truncateString 将字符串截断到 maxLen 字节。
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
