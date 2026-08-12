package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"go.uber.org/zap"
)

// ServerConfig HTTP 服务监听与超时配置。
type ServerConfig struct {
	Addr            string
	Handler         http.Handler
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// DefaultServerConfig 默认 15s 读写超时、30s 关闭超时。
func DefaultServerConfig(addr string, handler http.Handler) ServerConfig {
	return ServerConfig{
		Addr:            addr,
		Handler:         handler,
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
}

// ShutdownHook 有序关闭钩子（先注册的后关闭，LIFO）。
type ShutdownHook struct {
	Name string
	Fn   func(ctx context.Context) error
}

// ServeWithGraceful 启动 HTTP 并在 SIGINT/SIGTERM 时：
//  1. 先停 HTTP（拒绝新连接、排空在途请求）
//  2. 再按 LIFO 执行 hooks（workers → redis → db …）
//
// hooks 应按「底层依赖 → 上层资源」顺序传入；HTTP 由本函数最后注册，确保最先关闭。
func ServeWithGraceful(cfg ServerConfig, hooks ...ShutdownHook) error {
	if cfg.Handler == nil {
		return errors.New("middleware.ServeWithGraceful: Handler is required")
	}
	if cfg.Addr == "" {
		return errors.New("middleware.ServeWithGraceful: Addr is required")
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 15 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 15 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      cfg.Handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	sh := graceful.New(
		graceful.WithTimeout(cfg.ShutdownTimeout),
		graceful.WithHardTimeout(cfg.ShutdownTimeout+10*time.Second),
	)
	for _, h := range hooks {
		if h.Name == "" || h.Fn == nil {
			continue
		}
		sh.Register(h.Name, h.Fn)
	}
	// HTTP 最后注册 → LIFO 下最先关闭
	sh.Register("http-server", func(ctx context.Context) error {
		return srv.Shutdown(ctx)
	})

	serverErr := make(chan error, 1)
	go func() {
		if logger.Logger != nil {
			logger.Info("server listening", zap.String("addr", cfg.Addr))
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	if err := sh.WaitAndShutdown(); err != nil {
		if logger.Logger != nil {
			logger.Error("graceful shutdown error", logger.Err(err))
		}
	}

	select {
	case err := <-serverErr:
		return err
	default:
		return nil
	}
}
