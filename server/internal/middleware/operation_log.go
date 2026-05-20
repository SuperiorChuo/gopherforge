package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/service/system"
)

// 操作模块映射
var moduleMap = map[string]string{
	"/api/v1/users":          "用户管理",
	"/api/v1/roles":          "角色管理",
	"/api/v1/permissions":    "权限管理",
	"/api/v1/menus":          "菜单管理",
	"/api/v1/user":           "个人中心",
	"/api/v1/oauth":          "OAuth认证",
	"/api/v1/login":          "系统登录",
	"/api/v1/register":       "用户注册",
	"/api/v1/captcha":        "验证码",
	"/api/v1/operation-logs": "操作日志",
}

// 操作类型映射
var actionMap = map[string]string{
	"GET":    "查询",
	"POST":   "新增",
	"PUT":    "修改",
	"DELETE": "删除",
}

// responseBodyWriter 用于捕获响应体
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// OperationLogger 操作日志中间件
func OperationLogger() gin.HandlerFunc {
	return OperationLoggerWithOptions(OperationLogOptions{
		RecordRequestBody:   true,
		RecordResponseBody:  false, // 默认不记录响应体，可能会很大
		MaxRequestBodySize:  4096,  // 最大请求体大小 4KB
		MaxResponseBodySize: 4096,  // 最大响应体大小 4KB
		SkipPaths: []string{
			"/api/v1/health",
			"/api/v1/captcha",
		},
	})
}

// OperationLogOptions 操作日志配置选项
type OperationLogOptions struct {
	RecordRequestBody   bool     // 是否记录请求体
	RecordResponseBody  bool     // 是否记录响应体
	MaxRequestBodySize  int      // 最大请求体大小
	MaxResponseBodySize int      // 最大响应体大小
	SkipPaths           []string // 跳过的路径
}

// OperationLoggerWithOptions 带配置的操作日志中间件
func OperationLoggerWithOptions(opts OperationLogOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过 OPTIONS 预检请求
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// 检查是否跳过当前路径
		path := c.Request.URL.Path
		for _, skipPath := range opts.SkipPaths {
			if strings.HasPrefix(path, skipPath) {
				c.Next()
				return
			}
		}

		start := time.Now()

		// 读取请求体
		var requestBody string
		if opts.RecordRequestBody && c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				// 截断过长的请求体
				if len(bodyBytes) > opts.MaxRequestBodySize {
					requestBody = string(bodyBytes[:opts.MaxRequestBodySize]) + "...[truncated]"
				} else {
					requestBody = string(bodyBytes)
				}
				// 重置请求体，以便后续处理器可以读取
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}

		// 过滤敏感字段
		requestBody = filterSensitiveFields(requestBody)

		// 包装响应写入器以捕获响应体
		var responseBody string
		if opts.RecordResponseBody {
			blw := &responseBodyWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
			c.Writer = blw

			c.Next()

			// 截断过长的响应体
			if blw.body.Len() > opts.MaxResponseBodySize {
				responseBody = blw.body.String()[:opts.MaxResponseBodySize] + "...[truncated]"
			} else {
				responseBody = blw.body.String()
			}
		} else {
			c.Next()
		}

		// 获取完整路径
		fullPath := c.FullPath()
		if fullPath == "" {
			fullPath = c.Request.URL.Path
		}

		// 获取用户信息
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

		// 获取错误信息
		var errorMsg string
		if len(c.Errors) > 0 {
			errorMsg = c.Errors.String()
		}

		// 解析模块和操作类型
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

		// 使用带缓冲的通道记录日志，避免阻塞和无限 Goroutine
		select {
		case logChan <- log:
			// 写入成功
		default:
			// 通道已满，丢弃日志以保护服务
			// 实际生产中可以记录错误日志或监控指标
		}
	}
}

// 日志通道缓冲大小
const logChanBufferSize = 1000

// 日志通道
var logChan = make(chan *model.OperationLog, logChanBufferSize)

func init() {
	// 启动日志处理协程
	go processLogs()
}

// processLogs 处理日志
func processLogs() {
	service := &system.OperationLogService{}
	for log := range logChan {
		_ = service.Record(log)
	}
}

// getModule 根据路径获取模块名
func getModule(path string) string {
	for prefix, module := range moduleMap {
		if strings.HasPrefix(path, prefix) {
			return module
		}
	}
	return "其他"
}

// getAction 根据方法和路径获取操作类型
func getAction(method, path string) string {
	// 特殊处理登录、注册等操作
	if strings.HasSuffix(path, "/login") {
		return "登录"
	}
	if strings.HasSuffix(path, "/register") {
		return "注册"
	}
	if strings.HasSuffix(path, "/password") {
		return "修改密码"
	}
	if strings.Contains(path, "/status") {
		return "修改状态"
	}
	if strings.Contains(path, "/roles") && method == "POST" {
		return "分配角色"
	}
	if strings.Contains(path, "/permissions") && method == "POST" {
		return "分配权限"
	}

	// 默认使用 HTTP 方法映射
	if action, ok := actionMap[method]; ok {
		return action
	}
	return method
}

// filterSensitiveFields 过滤敏感字段
func filterSensitiveFields(body string) string {
	if body == "" {
		return body
	}

	var payload interface{}
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

func maskSensitivePayload(payload interface{}) {
	switch value := payload.(type) {
	case map[string]interface{}:
		for key, item := range value {
			if isSensitiveField(key) {
				value[key] = "***"
				continue
			}
			maskSensitivePayload(item)
		}
	case []interface{}:
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

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
