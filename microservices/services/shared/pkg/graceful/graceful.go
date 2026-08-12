// Package graceful 有序优雅关闭：确保关闭顺序正确，避免数据丢失。
//
// 关闭顺序 = 注册顺序的逆序（LIFO，类似 defer）：
//
//	sh.Register("database", closeDB)   // 最后关
//	sh.Register("redis", closeRedis)
//	sh.Register("http", srv.Shutdown) // 最先关
//
// 推荐注册顺序（先注册底层依赖，后注册入口）：
//  1. tracing / logger
//  2. database / redis / nats
//  3. workers（cancel lifecycle ctx）
//  4. outbox / 异步队列
//  5. http / gRPC server
//
// 用法：
//
//	g := graceful.New(graceful.WithTimeout(10 * time.Second))
//	g.Register("database", func(ctx context.Context) error {
//	    sqlDB, _ := db.DB()
//	    return sqlDB.Close()
//	})
//	g.Register("http", func(ctx context.Context) error {
//	    return server.Shutdown(ctx)
//	})
//	if err := g.WaitAndShutdown(); err != nil {
//	    log.Printf("graceful shutdown: %v", err)
//	}
package graceful

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/logger"
	"go.uber.org/zap"
)

type shutdownFunc func(ctx context.Context) error

// Shutdowner 按 LIFO 顺序执行已注册的关闭钩子。
type Shutdowner struct {
	mu          sync.Mutex
	order       []string
	handlers    map[string]shutdownFunc
	timeout     time.Duration
	hardTimeout time.Duration
}

// Option 配置 Shutdowner。
type Option func(*Shutdowner)

// WithTimeout 设置单个钩子的超时（默认 10s）。
func WithTimeout(d time.Duration) Option {
	return func(s *Shutdowner) { s.timeout = d }
}

// WithHardTimeout 设置整轮关闭的硬超时上限（默认 30s）。
func WithHardTimeout(d time.Duration) Option {
	return func(s *Shutdowner) { s.hardTimeout = d }
}

// New 创建 Shutdowner。
func New(opts ...Option) *Shutdowner {
	s := &Shutdowner{
		handlers:    make(map[string]shutdownFunc),
		timeout:     10 * time.Second,
		hardTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Register 注册关闭钩子。同名会覆盖函数但保持首次注册位置。
// 关闭时按注册逆序执行（后注册的先关）。
func (s *Shutdowner) Register(name string, fn shutdownFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.handlers[name]; !exists {
		s.order = append(s.order, name)
	}
	s.handlers[name] = fn
}

// ListenSignals 在后台监听信号并触发 Shutdown（非阻塞主流程时用）。
func (s *Shutdowner) ListenSignals(ctx context.Context, sigs ...os.Signal) {
	if len(sigs) == 0 {
		sigs = []os.Signal{os.Interrupt, syscall.SIGTERM}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)
	go func() {
		select {
		case sig := <-ch:
			info("shutdown signal received", zap.String("signal", sig.String()))
			if err := s.Shutdown(); err != nil {
				errorf("graceful shutdown failed", err)
			}
		case <-ctx.Done():
			return
		}
	}()
}

// Shutdown 按 LIFO（注册逆序）执行全部钩子。
func (s *Shutdowner) Shutdown() error {
	s.mu.Lock()
	names := make([]string, len(s.order))
	copy(names, s.order)
	handlers := make(map[string]shutdownFunc, len(s.handlers))
	for k, v := range s.handlers {
		handlers[k] = v
	}
	timeout := s.timeout
	hardTimeout := s.hardTimeout
	s.mu.Unlock()

	// LIFO：后注册的先执行
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}

	hardCtx, hardCancel := context.WithTimeout(context.Background(), hardTimeout)
	defer hardCancel()

	var errs []error
	for _, name := range names {
		fn := handlers[name]
		if fn == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(hardCtx, timeout)
		err := fn(ctx)
		cancel()
		if err != nil {
			errorf("shutdown handler failed: "+name, err)
			errs = append(errs, errors.New(name+": "+err.Error()))
		} else {
			info("shutdown handler completed", zap.String("handler", name))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// HandlerNames 返回注册顺序（非关闭顺序）的钩子名副本。
func (s *Shutdowner) HandlerNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.order))
	copy(result, s.order)
	return result
}

// WaitAndShutdown 阻塞直到 SIGINT/SIGTERM，然后执行 Shutdown。
func (s *Shutdowner) WaitAndShutdown() error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	received := <-sig
	signal.Stop(sig)
	info("shutdown signal received", zap.String("signal", received.String()))
	return s.Shutdown()
}

// ListenSignalsWithCallback 收到信号后先执行 callback，再 Shutdown。
func (s *Shutdowner) ListenSignalsWithCallback(ctx context.Context, callback func(), sigs ...os.Signal) {
	if len(sigs) == 0 {
		sigs = []os.Signal{os.Interrupt, syscall.SIGTERM}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)
	go func() {
		select {
		case sig := <-ch:
			info("shutdown signal received", zap.String("signal", sig.String()))
			if callback != nil {
				callback()
			}
			if err := s.Shutdown(); err != nil {
				errorf("graceful shutdown failed", err)
			}
		case <-ctx.Done():
			return
		}
	}()
}

// ShutdownOnSignal 等待信号，先关 server 再跑钩子（兼容旧调用方）。
func (s *Shutdowner) ShutdownOnSignal(ctx context.Context, server interface{ Shutdown(context.Context) error }) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case received := <-sig:
		info("shutdown signal received", zap.String("signal", received.String()))
	case <-ctx.Done():
		return ctx.Err()
	}
	if server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			errorf("server shutdown error", err)
		}
	}
	return s.Shutdown()
}

func info(msg string, fields ...zap.Field) {
	if logger.Logger != nil {
		logger.Info(msg, fields...)
		return
	}
	log.Print(msg)
}

func errorf(msg string, err error) {
	if logger.Logger != nil {
		logger.Error(msg, logger.Err(err))
		return
	}
	log.Printf("%s: %v", msg, err)
}
