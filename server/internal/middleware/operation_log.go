package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/service/system"
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
		if opts.RecordRequestBody && c.Request.Body != nil {
			bodyPreview, restoredBody, err := readRequestBodyForLog(c.Request.Body, opts.MaxRequestBodySize)
			c.Request.Body = restoredBody
			if err == nil {
				requestBody = bodyPreview
			}
		}

		requestBody = filterSensitiveFields(requestBody)

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
		requestID := GetRequestID(c)

		var errorMsg string
		if len(c.Errors) > 0 {
			errorMsg = c.Errors.String()
		}

		module := getModule(fullPath)
		action := getAction(c.Request.Method, fullPath)

		log := &model.OperationLog{
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

// logChanBufferSize limits queued operation log writes.
const logChanBufferSize = 1000

var logChan = make(chan *model.OperationLog, logChanBufferSize)

func init() {
	go processLogs()
}

// processLogs persists queued operation logs.
func processLogs() {
	service := &system.OperationLogService{}
	for log := range logChan {
		_ = service.RecordContext(context.Background(), log)
	}
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
	if body == "" {
		return body
	}

	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return body
	}

	maskSensitivePayload(payload)

	masked, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return string(masked)
}

func maskSensitivePayload(payload any) {
	switch value := payload.(type) {
	case map[string]any:
		for key, item := range value {
			if isSensitiveField(key) {
				value[key] = "***"
				continue
			}
			maskSensitivePayload(item)
		}
	case []any:
		for _, item := range value {
			maskSensitivePayload(item)
		}
	}
}

func isSensitiveField(field string) bool {
	switch strings.ToLower(field) {
	case "password", "old_password", "new_password", "token", "access_token", "refresh_token", "secret":
		return true
	default:
		return false
	}
}

// truncateString truncates a string to maxLen bytes.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
