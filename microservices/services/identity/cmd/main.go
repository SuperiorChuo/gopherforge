package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/identity/internal/api"
	sharedapi "github.com/go-admin-kit/services/identity/internal/api/shared"
	"github.com/go-admin-kit/services/identity/internal/config"
	authDAO "github.com/go-admin-kit/services/identity/internal/dao/auth"
	systemDAO "github.com/go-admin-kit/services/identity/internal/dao/system"
	"github.com/go-admin-kit/services/identity/internal/middleware"
	"github.com/go-admin-kit/services/identity/internal/pkg/authz"
	"github.com/go-admin-kit/services/identity/internal/pkg/database"
	"github.com/go-admin-kit/services/identity/internal/pkg/observability"
	"github.com/go-admin-kit/services/identity/internal/pkg/redis"
	"github.com/go-admin-kit/services/identity/internal/pkg/runtimeconfig"
	tenantscope "github.com/go-admin-kit/services/identity/internal/pkg/tenant"
	authsvc "github.com/go-admin-kit/services/identity/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	sharedmetrics "github.com/go-admin-kit/services/shared/pkg/metrics"
)

func setupCORS(router *gin.Engine) {
	cfg := config.Cfg.CORS
	corsConfig := cors.Config{
		AllowMethods:     cfg.AllowMethods,
		AllowHeaders:     cfg.AllowHeaders,
		ExposeHeaders:    cfg.ExposeHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           time.Duration(cfg.MaxAge) * time.Hour,
	}

	if config.Cfg.App.Env == "development" {
		allowedOrigins := make(map[string]struct{}, len(cfg.AllowOrigins))
		for _, origin := range cfg.AllowOrigins {
			allowedOrigins[strings.TrimSpace(origin)] = struct{}{}
		}
		corsConfig.AllowOrigins = nil
		corsConfig.AllowOriginFunc = func(origin string) bool {
			if _, ok := allowedOrigins[origin]; ok {
				return true
			}
			return isLocalDevelopmentOrigin(origin)
		}
		router.Use(cors.New(corsConfig))
		return
	}

	if cfg.AllowCredentials {
		if len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*" {
			logger.Warn("production CORS cannot use '*' with credentials enabled")
			corsConfig.AllowOrigins = []string{}
		} else {
			corsConfig.AllowOrigins = cfg.AllowOrigins
		}
	} else if len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*" {
		corsConfig.AllowAllOrigins = true
	} else {
		corsConfig.AllowOrigins = cfg.AllowOrigins
	}

	router.Use(cors.New(corsConfig))
}

func isLocalDevelopmentOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func printStartupBanner(name, version, env string, port int) {
	fmt.Printf("\n%s v%s\nEnvironment: %s\nServer: http://localhost:%d\nAPI: http://localhost:%d/api/v1\n\n", name, version, env, port, port)
	logger.Info("server started",
		logger.String("app", name),
		logger.String("version", version),
		logger.String("env", env),
		logger.Int("port", port),
	)
}

func configureGinWriters(env string) {
	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
		gin.DefaultWriter = io.Discard
		gin.DefaultErrorWriter = io.Discard
		return
	}

	gin.DefaultWriter = logger.NewGinWriter()
	gin.DefaultErrorWriter = logger.NewGinErrorWriter()
}

func serveHTTPServer(server *http.Server, listener net.Listener, shutdownTimeout time.Duration, shutdown <-chan os.Signal) error {
	serverErr := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErr <- err
	}()

	select {
	case err := <-serverErr:
		return err
	case sig := <-shutdown:
		if logger.Logger != nil && sig != nil {
			logger.Info("shutdown signal received", logger.String("signal", sig.String()))
		}
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}
		return <-serverErr
	}
}

func stopOperationLogProcessor(cancel context.CancelFunc, done <-chan struct{}, timeout time.Duration) error {
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	if timeout <= 0 {
		<-done
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("operation log processor shutdown timed out after %s", timeout)
	}
}

func main() {
	if err := run(context.Background()); err != nil {
		if logger.Logger != nil {
			logger.Error("server exited with error", logger.Err(err))
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "server exited with error: %v\n", err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	if err := config.Load(); err != nil {
		return fmt.Errorf("config load failed: %w", err)
	}

	logCfg := config.Cfg.Logger
	logger.InitLogger(logCfg.FilePath, logCfg.Level, logCfg.MaxSize, logCfg.MaxBackups, logCfg.MaxAge)
	defer func() {
		if logger.Logger != nil {
			_ = logger.Logger.Sync()
		}
	}()

	logger.Info("initializing database")
	if err := database.InitDatabase(); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}
	if err := authz.RegisterDataScopePlugin(database.DB); err != nil {
		return fmt.Errorf("data scope plugin registration failed: %w", err)
	}
	if err := tenantscope.Register(database.DB); err != nil {
		return fmt.Errorf("tenant scope plugin registration failed: %w", err)
	}
	consoleSessionService := authsvc.NewConsoleSessionServiceWithDB(database.DB)
	middleware.SetAuthMiddlewareDependencies(middleware.AuthMiddlewareDependencies{
		Users:           authDAO.NewUserDAO(database.DB),
		Permissions:     authDAO.NewPermissionDAO(database.DB),
		ConsoleSessions: &consoleSessionService,
	})
	authz.SetPersistence(authz.Persistence{
		Users:       authDAO.NewUserDAO(database.DB),
		Permissions: authDAO.NewPermissionDAO(database.DB),
		DataScope:   authz.NewDatabaseDataScopeStore(database.DB),
	})
	runtimeconfig.SetSecurityPolicyStore(systemDAO.NewSettingDAO(database.DB))
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("database close failed", logger.Err(err))
		}
	}()

	logger.Info("initializing redis")
	if err := redis.InitRedis(); err != nil {
		return fmt.Errorf("redis initialization failed: %w", err)
	}
	defer func() {
		if err := redis.Close(); err != nil {
			logger.Error("redis close failed", logger.Err(err))
		}
	}()

	lifecycleCtx, cancelLifecycle := context.WithCancel(ctx)
	defer cancelLifecycle()

	// Persist operation logs for the admin CRUD this service now owns.
	operationLogService := systemsvc.NewOperationLogServiceWithDB(database.DB)
	operationLogDone := middleware.StartOperationLogProcessor(lifecycleCtx, &operationLogService)
	defer func() {
		if err := stopOperationLogProcessor(cancelLifecycle, operationLogDone, 5*time.Second); err != nil {
			logger.Warn("operation log processor shutdown timeout", logger.Err(err))
		}
	}()

	// Refresh cached department trees when another instance (or the monolith)
	// changes departments.
	departmentTreeListener, err := authz.StartDepartmentTreeInvalidationListener(lifecycleCtx)
	if err != nil {
		logger.Warn("department tree invalidation listener start failed", logger.Err(err))
	} else {
		defer func() {
			if err := departmentTreeListener.Close(); err != nil {
				logger.Warn("department tree invalidation listener close failed", logger.Err(err))
			}
		}()
	}

	// Warm up the runtime security policy cache; failures fall back to the
	// static config defaults on first request.
	if err := runtimeconfig.DefaultSecurityPolicyReader().Refresh(ctx); err != nil {
		logger.Warn("security policy warmup failed", logger.Err(err))
	}

	runtimeConfigListener, err := runtimeconfig.StartInvalidationListener(lifecycleCtx)
	if err != nil {
		logger.Warn("runtime config invalidation listener start failed", logger.Err(err))
	} else {
		defer func() {
			if err := runtimeConfigListener.Close(); err != nil {
				logger.Warn("runtime config invalidation listener close failed", logger.Err(err))
			}
		}()
	}

	tracingCfg := config.Cfg.Observability.Tracing
	shutdownTracing, err := observability.InitTracer(ctx, tracingCfg)
	if err != nil {
		return fmt.Errorf("tracing initialization failed: %w", err)
	}
	if tracingCfg.Enabled {
		logger.Info("tracing enabled",
			logger.String("service", tracingCfg.ServiceName),
			logger.String("env", tracingCfg.Environment),
			logger.String("otlp", tracingCfg.OTLPEndpoint),
			logger.Any("sample_ratio", tracingCfg.SampleRatio),
		)
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownTracing(ctx); err != nil {
				logger.Error("tracing shutdown failed", logger.Err(err))
			}
		}()
	}

	configureGinWriters(config.Cfg.App.Env)

	router := gin.New()
	if len(config.Cfg.Security.TrustedProxies) > 0 {
		if err := router.SetTrustedProxies(config.Cfg.Security.TrustedProxies); err != nil {
			return fmt.Errorf("trusted proxy config failed: %w", err)
		}
	}

	// HTTP 指标（GET /metrics，Prometheus 抓取）：先于其余中间件注册，
	// 端点不进日志/限流链；METRICS_ENABLED=false 关闭
	sharedmetrics.Install(router)
	if sqlDB, err := database.DB.DB(); err == nil {
		sharedmetrics.SetDBStats(sqlDB.Stats)
	}
	router.Use(middleware.RequestID(config.Cfg.Observability.RequestIDHeader))
	if tracingCfg.Enabled {
		router.Use(observability.GinTracing(tracingCfg.ServiceName, middleware.RequestIDKey))
	}
	router.Use(middleware.SecurityHeaders(config.Cfg.Security.Headers.Enabled, config.Cfg.Security.Headers.HSTS))
	router.Use(middleware.Recovery())
	router.Use(middleware.DynamicRateLimit(runtimeconfig.DefaultSecurityPolicyReader()))
	router.Use(middleware.RequestLogger())
	router.Use(middleware.ErrorHandler())
	setupCORS(router)
	api.SetupRoutesWithDeps(router, sharedapi.Dependencies{DB: database.DB, Redis: redis.Client})

	port := config.Cfg.App.Port
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: router}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("server listen failed: %w", err)
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdown)

	printStartupBanner(config.Cfg.App.Name, config.Cfg.App.Version, config.Cfg.App.Env, port)
	if err := serveHTTPServer(server, listener, 15*time.Second, shutdown); err != nil {
		return fmt.Errorf("server start failed: %w", err)
	}
	return nil
}
