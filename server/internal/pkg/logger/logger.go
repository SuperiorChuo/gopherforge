package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.Logger

// InitLogger 初始化日志
func InitLogger(filePath string, level string, maxSize, maxBackups, maxAge int) {
	// 日志级别
	var logLevel zapcore.Level
	switch level {
	case "debug":
		logLevel = zapcore.DebugLevel
	case "info":
		logLevel = zapcore.InfoLevel
	case "warn":
		logLevel = zapcore.WarnLevel
	case "error":
		logLevel = zapcore.ErrorLevel
	default:
		logLevel = zapcore.InfoLevel
	}

	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout(time.RFC3339),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 文件输出配置
	lumberJackLogger := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   true,
	}

	// 控制台输出
	consoleWriter := zapcore.AddSync(os.Stdout)
	// 文件输出
	fileWriter := zapcore.AddSync(lumberJackLogger)

	// 创建Core
	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), fileWriter, logLevel),
		zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleWriter, logLevel),
	)

	// 创建Logger
	Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

// Debug 调试日志
func Debug(msg string, fields ...zap.Field) {
	Logger.Debug(msg, fields...)
}

// Info 信息日志
func Info(msg string, fields ...zap.Field) {
	Logger.Info(msg, fields...)
}

// Warn 警告日志
func Warn(msg string, fields ...zap.Field) {
	Logger.Warn(msg, fields...)
}

// Error 错误日志
func Error(msg string, fields ...zap.Field) {
	Logger.Error(msg, fields...)
}

// Fatal 致命日志
func Fatal(msg string, fields ...zap.Field) {
	Logger.Fatal(msg, fields...)
}

// 辅助函数
func String(key, value string) zap.Field {
	return zap.String(key, value)
}

func Int(key string, value int) zap.Field {
	return zap.Int(key, value)
}

func Err(err error) zap.Field {
	return zap.Error(err)
}

func Uint(key string, value uint) zap.Field {
	return zap.Uint(key, value)
}

func Bool(key string, value bool) zap.Field {
	return zap.Bool(key, value)
}

func Duration(key string, value time.Duration) zap.Field {
	return zap.Duration(key, value)
}

func Any(key string, value interface{}) zap.Field {
	return zap.Any(key, value)
}

func Int64(key string, value int64) zap.Field {
	return zap.Int64(key, value)
}

// GinWriter Gin 日志写入器（用于路由注册日志）
// 完全禁用路由注册日志，只保留警告和错误
type GinWriter struct{}

func (w *GinWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// 过滤掉所有路由注册日志 [GIN-debug]
	if contains(msg, []string{"[GIN-debug]"}) {
		// 完全忽略路由注册日志
		return len(p), nil
	}
	// 只记录警告和错误
	if contains(msg, []string{"[WARNING]", "[ERROR]"}) {
		// 清理消息格式
		cleanMsg := msg
		if len(cleanMsg) > 0 && cleanMsg[len(cleanMsg)-1] == '\n' {
			cleanMsg = cleanMsg[:len(cleanMsg)-1]
		}
		Logger.Warn("Gin 警告", zap.String("消息", cleanMsg))
	}
	return len(p), nil
}

// GinErrorWriter Gin 错误日志写入器
type GinErrorWriter struct{}

func (w *GinErrorWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	Logger.Error("Gin 错误", zap.String("消息", msg[:len(msg)-1]))
	return len(p), nil
}

// NewGinWriter 创建 Gin 日志写入器
func NewGinWriter() *GinWriter {
	return &GinWriter{}
}

// NewGinErrorWriter 创建 Gin 错误日志写入器
func NewGinErrorWriter() *GinErrorWriter {
	return &GinErrorWriter{}
}

// contains 检查字符串是否包含任一子串
func contains(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
