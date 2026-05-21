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
	"github.com/go-admin-kit/server/internal/api"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/middleware"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"github.com/go-admin-kit/server/internal/pkg/observability"
	"github.com/go-admin-kit/server/internal/pkg/redis"
	systemSvc "github.com/go-admin-kit/server/internal/service/system"
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

func main() {
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}
	if err := config.LoadConfig(configPath); err != nil {
		panic(fmt.Sprintf("config load failed: %v", err))
	}
	if err := config.Validate(); err != nil {
		panic(fmt.Sprintf("config validation failed: %v", err))
	}

	logCfg := config.Cfg.Logger
	logger.InitLogger(logCfg.FilePath, logCfg.Level, logCfg.MaxSize, logCfg.MaxBackups, logCfg.MaxAge)
	defer logger.Logger.Sync()

	logger.Info("initializing database")
	if err := database.InitDatabase(); err != nil {
		logger.Fatal("database initialization failed", logger.Err(err))
	}
	if err := authz.RegisterDataScopePlugin(database.DB); err != nil {
		logger.Fatal("data scope plugin registration failed", logger.Err(err))
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("database close failed", logger.Err(err))
		}
	}()
	if menuResult, err := systemSvc.BootstrapDefaultMenusContext(context.Background()); err != nil {
		logger.Fatal("default menu bootstrap failed", logger.Err(err))
	} else if menuResult.Menus > 0 {
		logger.Info("default menus bootstrapped", logger.Int("menus", menuResult.Menus))
	}

	logger.Info("initializing redis")
	if err := redis.InitRedis(); err != nil {
		logger.Fatal("redis initialization failed", logger.Err(err))
	}
	defer func() {
		if err := redis.Close(); err != nil {
			logger.Error("redis close failed", logger.Err(err))
		}
	}()

	listenerCtx, cancelDepartmentTreeListener := context.WithCancel(context.Background())
	defer cancelDepartmentTreeListener()
	departmentTreeListener, err := startDepartmentTreeInvalidationListener(listenerCtx)
	if err != nil {
		logger.Warn("department tree invalidation listener start failed", logger.Err(err))
	} else {
		defer func() {
			if err := departmentTreeListener.Close(); err != nil {
				logger.Warn("department tree invalidation listener close failed", logger.Err(err))
			}
		}()
	}

	tracingCfg := config.Cfg.Observability.Tracing
	shutdownTracing, err := observability.InitTracer(context.Background(), tracingCfg)
	if err != nil {
		logger.Fatal("tracing initialization failed", logger.Err(err))
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
			logger.Fatal("trusted proxy config failed", logger.Err(err))
		}
	}

	router.Use(middleware.RequestID(config.Cfg.Observability.RequestIDHeader))
	if config.Cfg.Observability.Tracing.Enabled {
		router.Use(observability.GinTracing(config.Cfg.Observability.Tracing.ServiceName, middleware.RequestIDKey))
	}
	if config.Cfg.Observability.MetricsEnabled {
		router.Use(middleware.Metrics())
	}
	router.Use(middleware.SecurityHeaders(config.Cfg.Security.Headers.Enabled, config.Cfg.Security.Headers.HSTS))
	router.Use(middleware.Recovery())

	if config.Cfg.Security.RateLimit.Enabled {
		window := time.Duration(config.Cfg.Security.RateLimit.WindowSeconds) * time.Second
		if window <= 0 {
			window = time.Second
		}
		maxRequests := config.Cfg.Security.RateLimit.MaxRequests
		if maxRequests <= 0 {
			maxRequests = 100
		}
		router.Use(middleware.RateLimit(middleware.RateLimitConfig{
			Window:      window,
			MaxRequests: maxRequests,
			KeyPrefix:   "rate_limit",
		}))
	}

	router.Use(middleware.RequestLogger())
	router.Use(middleware.ErrorHandler())
	setupCORS(router)
	api.SetupRoutes(router)

	port := config.Cfg.App.Port
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: router}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		logger.Fatal("server listen failed", logger.Err(err))
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdown)

	printStartupBanner(config.Cfg.App.Name, config.Cfg.App.Version, config.Cfg.App.Env, port)
	if err := serveHTTPServer(server, listener, 15*time.Second, shutdown); err != nil {
		logger.Fatal("server start failed", logger.Err(err))
	}
}
