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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/audit/internal/api"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
	"github.com/go-admin-kit/services/audit/internal/config"
	authDAO "github.com/go-admin-kit/services/audit/internal/dao/auth"
	authdao "github.com/go-admin-kit/services/shared/pkg/authdao"
	systemDAO "github.com/go-admin-kit/services/audit/internal/dao/system"
	"github.com/go-admin-kit/services/audit/internal/events"
	"github.com/go-admin-kit/services/audit/internal/middleware"
	"github.com/go-admin-kit/services/audit/internal/pkg/authz"
	"github.com/go-admin-kit/services/audit/internal/pkg/database"
	"github.com/go-admin-kit/services/audit/internal/pkg/observability"
	"github.com/go-admin-kit/services/audit/internal/pkg/redis"
	"github.com/go-admin-kit/services/audit/internal/pkg/runtimeconfig"
	authsvc "github.com/go-admin-kit/services/audit/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	sharedmetrics "github.com/go-admin-kit/services/shared/pkg/metrics"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	tenantscope "github.com/go-admin-kit/services/shared/pkg/tenant"

	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"

	auditv1 "github.com/go-admin-kit/services/api/gen/audit/v1"
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

// envInt 读环境变量整数，空/非法用默认值。
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// startGRPCServer 启动 gRPC 服务并注册 Consul（Phase 1 服务发现试点）。
// CONSUL_ADDR=disabled 时跳过注册（本机/单测不依赖 Consul）；注册失败仅告警不阻断 HTTP。
func startGRPCServer(ctx context.Context, grpcPort int) (cleanup func(), err error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return nil, fmt.Errorf("grpc listen: %w", err)
	}
	srv := grpcx.NewServer()
	auditv1.RegisterAuditServiceServer(srv, api.NewAuditGRPC(database.DB))
	go func() {
		_ = srv.Serve(lis) // 退出时 GracefulStop 接管
	}()
	logger.Info("grpc server started", logger.Int("port", grpcPort))

	consulAddr := os.Getenv("CONSUL_ADDR")
	if consulAddr == "disabled" {
		return func() { srv.GracefulStop() }, nil
	}
	host := grpcx.LocalIP()
	if host == "" {
		host = "127.0.0.1"
	}
	deregister, regErr := grpcx.Register(consulAddr, grpcx.Instance{
		ServiceName: "audit-service",
		Host:        host,
		Port:        grpcPort,
	})
	if regErr != nil {
		logger.Warn("consul register failed（跳过，HTTP 不受影响）", logger.Err(regErr))
	}
	return func() {
		if deregister != nil {
			deregister()
		}
		srv.GracefulStop()
	}, nil
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
	jwt.SetConfig(jwt.JWTConfig{Secret: config.Cfg.JWT.Secret, Issuer: config.Cfg.JWT.Issuer, AccessTokenExpire: config.Cfg.JWT.AccessTokenExpire, RefreshTokenExpire: config.Cfg.JWT.RefreshTokenExpire, RefreshTokenRotation: config.Cfg.JWT.RefreshTokenRotation})

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
		Permissions:     authdao.NewPermissionDAO(database.DB),
		ConsoleSessions: &consoleSessionService,
	})
	authz.SetPersistence(authz.Persistence{
		Users:       authDAO.NewUserDAO(database.DB),
		Permissions: authdao.NewPermissionDAO(database.DB),
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
	jwt.SetRedis(redis.Client)
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

	// Consume auth-service login events via Redis pub/sub into login_logs.
	loginNotify := notifyclient.New(config.Cfg.Notify.APIBase, config.Cfg.Notify.Token)
	loginLogService := systemsvc.NewLoginLogServiceWithDB(database.DB).WithNotifier(loginNotify)
	authEventConsumer, err := events.StartRedisLoginConsumer(lifecycleCtx, redis.Client, &loginLogService)
	if err != nil {
		logger.Warn("redis login event consumer start failed, login logs disabled", logger.Err(err))
	} else if authEventConsumer != nil {
		defer authEventConsumer.Close()
		logger.Info("redis login event consumer enabled")
	}

	// 日志保留策略：按 AUDIT_LOG_RETENTION_DAYS 周期清理操作/登录日志
	// （默认关闭；audit_logs 不清理，见 service/system/log_retention.go）。
	if systemsvc.StartLogRetentionCleaner(lifecycleCtx, database.DB, &operationLogService, &loginLogService, systemsvc.LogRetentionOptions{
		RetentionDays: config.Cfg.Retention.LogRetentionDays,
		ScanInterval:  time.Duration(config.Cfg.Retention.LogRetentionScanIntervalSeconds) * time.Second,
	}) {
		logger.Info("log retention cleaner enabled",
			logger.Int("retention_days", config.Cfg.Retention.LogRetentionDays))
	}

	// 安全事件检测器：扫审计日志异常模式（写入激增/权限风暴/失败激增），
	// 命中落 security_events + 站内信通知平台管理员（notify 未配时静默跳过）。
	notifyClient := notifyclient.New(config.Cfg.Notify.APIBase, config.Cfg.Notify.Token)
	systemsvc.StartSecurityEventDetector(lifecycleCtx, database.DB, notifyClient, systemsvc.SecurityDetectorOptions{
		ScanInterval:        60 * time.Second,
		Window:              10 * time.Minute,
		WriteThreshold:      config.Cfg.SecurityDetect.WriteThreshold,
		PermissionThreshold: config.Cfg.SecurityDetect.PermissionThreshold,
		FailureThreshold:    config.Cfg.SecurityDetect.FailureThreshold,
		NotifyUserID:        1,
		NotifyURL:           "/system/security-events",
	})

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
	router.Use(sharedmw.RequestID(config.Cfg.Observability.RequestIDHeader))
	if tracingCfg.Enabled {
		router.Use(observability.GinTracing(tracingCfg.ServiceName, sharedmw.RequestIDKey))
	}
	router.Use(sharedmw.SecurityHeaders(config.Cfg.Security.Headers.Enabled, config.Cfg.Security.Headers.HSTS))
	router.Use(sharedmw.Recovery())
	router.Use(middleware.DynamicRateLimit(runtimeconfig.DefaultSecurityPolicyReader()))
	router.Use(sharedmw.RequestLogger())
	router.Use(sharedmw.ErrorHandler())
	setupCORS(router)
	api.SetupRoutesWithDeps(router, sharedapi.Dependencies{DB: database.DB, Redis: redis.Client})

	grpcPort := envInt("GRPC_PORT", 9082)
	grpcCleanup, err := startGRPCServer(ctx, grpcPort)
	if err != nil {
		return fmt.Errorf("grpc server start failed: %w", err)
	}
	defer grpcCleanup()

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
