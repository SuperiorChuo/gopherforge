package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.Logger

// InitLogger initializes the application logger.
func InitLogger(filePath string, level string, maxSize, maxBackups, maxAge int) {
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

	lumberJackLogger := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   true,
	}

	consoleWriter := zapcore.AddSync(os.Stdout)
	fileWriter := zapcore.AddSync(lumberJackLogger)

	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), fileWriter, logLevel),
		zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleWriter, logLevel),
	)

	Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

// Debug writes a debug log.
func Debug(msg string, fields ...zap.Field) {
	Logger.Debug(msg, fields...)
}

// Info writes an info log.
func Info(msg string, fields ...zap.Field) {
	Logger.Info(msg, fields...)
}

// Warn writes a warning log.
func Warn(msg string, fields ...zap.Field) {
	Logger.Warn(msg, fields...)
}

// Error writes an error log.
func Error(msg string, fields ...zap.Field) {
	Logger.Error(msg, fields...)
}

// Fatal writes a fatal log and exits.
func Fatal(msg string, fields ...zap.Field) {
	Logger.Fatal(msg, fields...)
}

// Field helpers.
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

func Any(key string, value any) zap.Field {
	return zap.Any(key, value)
}

func Int64(key string, value int64) zap.Field {
	return zap.Int64(key, value)
}

// GinWriter writes selected Gin framework logs.
type GinWriter struct{}

func (w *GinWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	if contains(msg, []string{"[GIN-debug]"}) {
		return len(p), nil
	}
	if contains(msg, []string{"[WARNING]", "[ERROR]"}) {
		cleanMsg := msg
		if len(cleanMsg) > 0 && cleanMsg[len(cleanMsg)-1] == '\n' {
			cleanMsg = cleanMsg[:len(cleanMsg)-1]
		}
		Logger.Warn("gin warning", zap.String("message", cleanMsg))
	}
	return len(p), nil
}

// GinErrorWriter writes Gin error logs.
type GinErrorWriter struct{}

func (w *GinErrorWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	Logger.Error("gin error", zap.String("message", msg[:len(msg)-1]))
	return len(p), nil
}

// NewGinWriter creates a Gin log writer.
func NewGinWriter() *GinWriter {
	return &GinWriter{}
}

// NewGinErrorWriter creates a Gin error log writer.
func NewGinErrorWriter() *GinErrorWriter {
	return &GinErrorWriter{}
}

// contains reports whether s contains any of the provided substrings.
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
