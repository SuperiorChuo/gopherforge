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
	"github.com/go-admin-kit/services/monitor/internal/api"
	sharedapi "github.com/go-admin-kit/services/monitor/internal/api/shared"
	"github.com/go-admin-kit/services/monitor/internal/config"
	authDAO "github.com/go-admin-kit/services/monitor/internal/dao/auth"
	authdao "github.com/go-admin-kit/services/shared/pkg/authdao"
	monitordao "github.com/go-admin-kit/services/monitor/internal/dao/monitor"
	systemDAO "github.com/go-admin-kit/services/monitor/internal/dao/system"
	"github.com/go-admin-kit/services/monitor/internal/middleware"
	"github.com/go-admin-kit/services/monitor/internal/pkg/authz"
	"github.com/go-admin-kit/services/monitor/internal/pkg/database"
	"github.com/go-admin-kit/services/monitor/internal/pkg/observability"
	"github.com/go-admin-kit/services/monitor/internal/pkg/redis"
	"github.com/go-admin-kit/services/monitor/internal/pkg/runtimeconfig"
	authsvc "github.com/go-admin-kit/services/monitor/internal/service/auth"
	monitorSvc "github.com/go-admin-kit/services/monitor/internal/service/monitor"
	systemSvc "github.com/go-admin-kit/services/monitor/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"

	"github.com/go-admin-kit/services/shared/pkg/logger"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"

	monitorv1 "github.com/go-admin-kit/services/api/gen/monitor/v1"
	monitorapi "github.com/go-admin-kit/services/monitor/internal/api/monitor"
)

// @title           Go Admin Kit API
// @version         1.0
// @description     A Gin + GORM + Redis admin backend starter.
// @host            localhost:8081
// @BasePath        /api/v1
// @securityDefinitions.apikey Bearer
// @in              header
// @name            Authorization
// @description     Type "Bearer" followed by a space and JWT token.

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

func startDepartmentTreeInvalidationListener(ctx context.Context) (*redis.StringSubscriber, error) {
	return authz.StartDepartmentTreeInvalidationListener(ctx)
}

func startRuntimeConfigInvalidationListener(ctx context.Context) (*redis.StringSubscriber, error) {
	return runtimeconfig.StartInvalidationListener(ctx)
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

type jobSchedulerShutdowner interface {
	Shutdown() context.Context
}

func shutdownJobScheduler(scheduler jobSchedulerShutdowner, timeout time.Duration) error {
	if scheduler == nil {
		return nil
	}
	ctx := scheduler.Shutdown()
	if ctx == nil {
		return nil
	}
	if timeout <= 0 {
		<-ctx.Done()
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil
	case <-timer.C:
		return fmt.Errorf("job scheduler shutdown timed out after %s", timeout)
	}
}

// startGRPCServer 启动 gRPC 服务并注册 Consul（Phase 1 服务发现试点）。
// CONSUL_ADDR=disabled 时跳过注册（本机/单测不依赖 Consul）；注册失败仅告警不阻断 HTTP。
func startGRPCServer(ctx context.Context, grpcPort int) (cleanup func(), err error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return nil, fmt.Errorf("grpc listen: %w", err)
	}
	srv := grpcx.NewServer()
	monitorv1.RegisterMonitorServiceServer(srv, monitorapi.NewMonitorGRPC())
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
		ServiceName: "monitor-service",
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

// envInt 读环境变量整数，空/非法用默认值。
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
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
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}
	if err := config.LoadConfig(configPath); err != nil {
		return fmt.Errorf("config load failed: %w", err)
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
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
	middleware.SetAuthMiddlewareDependencies(middleware.AuthMiddlewareDependencies{
		Users:           authDAO.NewUserDAO(database.DB),
		Permissions:     authdao.NewPermissionDAO(database.DB),
		ConsoleSessions: authsvc.NewConsoleSessionServiceWithDB(database.DB),
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

	lifecycleCtx, cancelLifecycle := context.WithCancel(ctx)
	defer cancelLifecycle()
	operationLogService := systemSvc.NewOperationLogServiceWithDB(database.DB)
	operationLogDone := middleware.StartOperationLogProcessor(lifecycleCtx, &operationLogService)
	defer func() {
		if err := stopOperationLogProcessor(cancelLifecycle, operationLogDone, 5*time.Second); err != nil {
			logger.Warn("operation log processor shutdown timeout", logger.Err(err))
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

	departmentTreeListener, err := startDepartmentTreeInvalidationListener(lifecycleCtx)
	if err != nil {
		logger.Warn("department tree invalidation listener start failed", logger.Err(err))
	} else {
		defer func() {
			if err := departmentTreeListener.Close(); err != nil {
				logger.Warn("department tree invalidation listener close failed", logger.Err(err))
			}
		}()
	}

	runtimeConfigListener, err := startRuntimeConfigInvalidationListener(lifecycleCtx)
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

	router.Use(sharedmw.RequestID(config.Cfg.Observability.RequestIDHeader))
	if config.Cfg.Observability.Tracing.Enabled {
		router.Use(observability.GinTracing(config.Cfg.Observability.Tracing.ServiceName, sharedmw.RequestIDKey))
	}
	if config.Cfg.Observability.MetricsEnabled {
		router.Use(middleware.Metrics())
	}
	router.Use(sharedmw.SecurityHeaders(config.Cfg.Security.Headers.Enabled, config.Cfg.Security.Headers.HSTS))
	router.Use(sharedmw.Recovery())

	router.Use(middleware.DynamicRateLimit(runtimeconfig.DefaultSecurityPolicyReader()))

	router.Use(sharedmw.RequestLogger())
	router.Use(sharedmw.ErrorHandler())
	setupCORS(router)
	jobScheduler := monitorSvc.InitJobService(database.DB)
	alertService := monitorSvc.NewAlertService(database.DB, redis.Client)
	alertEvaluator := monitorSvc.StartAlertEvaluator(lifecycleCtx, alertService, monitorSvc.DefaultAlertEvaluationInterval)
	metricSampler := monitorSvc.StartMetricSampler(
		lifecycleCtx,
		monitorSvc.NewDefaultAlertMetricCollector(database.DB, redis.Client),
		monitordao.NewMetricSampleDAO(database.DB),
		monitorSvc.DefaultSamplingInterval,
	)
	api.SetupRoutesWithDeps(router, sharedapi.Dependencies{DB: database.DB, Redis: redis.Client})
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := alertEvaluator.Shutdown(shutdownCtx); err != nil {
			logger.Warn("alert evaluator shutdown timeout", logger.Err(err))
		}
		if err := metricSampler.Shutdown(shutdownCtx); err != nil {
			logger.Warn("metric sampler shutdown timeout", logger.Err(err))
		}
	}()
	defer func() {
		if err := shutdownJobScheduler(jobScheduler, 5*time.Second); err != nil {
			logger.Warn("job scheduler shutdown timeout", logger.Err(err))
		}
	}()

	grpcPort := envInt("GRPC_PORT", 9081)
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
