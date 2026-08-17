package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/auth/internal/api"
	"github.com/go-admin-kit/services/auth/internal/config"
	authDAO "github.com/go-admin-kit/services/auth/internal/dao/auth"
	systemDAO "github.com/go-admin-kit/services/auth/internal/dao/system"
	"github.com/go-admin-kit/services/auth/internal/events"
	"github.com/go-admin-kit/services/auth/internal/middleware"
	"github.com/go-admin-kit/services/auth/internal/pkg/runtimeconfig"
	authsvc "github.com/go-admin-kit/services/auth/internal/service/auth"
	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	authdao "github.com/go-admin-kit/services/shared/pkg/authdao"
	"github.com/go-admin-kit/services/shared/pkg/cache"
	"github.com/go-admin-kit/services/shared/pkg/captcha"
	"github.com/go-admin-kit/services/shared/pkg/database"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	sharedmetrics "github.com/go-admin-kit/services/shared/pkg/metrics"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/observability"
	"github.com/go-admin-kit/services/shared/pkg/redis"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
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
	fmt.Printf("\n%s v%s\nEnvironment: %s\nServer: http://localhost:%d\nAPI: http://localhost:%d/api/v1\n\n",
		name, version, env, port, port)
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

	sh := graceful.New(graceful.WithTimeout(10 * time.Second))

	sh.Register("logger", func(ctx context.Context) error {
		if logger.Logger != nil {
			return logger.Logger.Sync()
		}
		return nil
	})

	logger.Info("initializing database")
	if err := database.InitDatabase(database.Config{
		DSN:                    config.Cfg.Database.GetDSN(),
		Host:                   config.Cfg.Database.Host,
		Port:                   config.Cfg.Database.Port,
		DBName:                 config.Cfg.Database.DBName,
		MaxIdleConns:           config.Cfg.Database.MaxIdleConns,
		MaxOpenConns:           config.Cfg.Database.MaxOpenConns,
		ConnMaxLifetimeSeconds: config.Cfg.Database.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: config.Cfg.Database.ConnMaxIdleTimeSeconds,
	}); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}
	if err := sharedaudit.Register(database.DB, sharedaudit.Config{
		Targets: []sharedaudit.Target{sharedaudit.UserTarget(&model.User{})},
	}); err != nil {
		return fmt.Errorf("audit trail plugin registration failed: %w", err)
	}

	if err := auditevents.Init(config.Cfg.NATS.URL); err != nil {
		logger.Warn("audit events publisher init failed（回退直写）", logger.Err(err))
	}

	consoleSessionService := authsvc.NewConsoleSessionServiceWithDB(database.DB)
	middleware.SetAuthMiddlewareDependencies(middleware.AuthMiddlewareDependencies{
		Users:           authDAO.NewUserDAO(database.DB),
		Permissions:     authdao.NewPermissionDAO(database.DB),
		ConsoleSessions: &consoleSessionService,
	})
	runtimeconfig.SetSecurityPolicyStore(systemDAO.NewSettingDAO(database.DB))
	runtimeconfig.SetEmailNotificationStore(systemDAO.NewSettingDAO(database.DB))

	sh.Register("database", func(ctx context.Context) error {
		return database.Close()
	})

	logger.Info("initializing redis")
	if err := redis.InitRedis(redis.Config{
		Host:     config.Cfg.Redis.Host,
		Port:     config.Cfg.Redis.Port,
		Password: config.Cfg.Redis.Password,
		DB:       config.Cfg.Redis.DB,
		PoolSize: config.Cfg.Redis.PoolSize,
	}); err != nil {
		return fmt.Errorf("redis initialization failed: %w", err)
	}
	jwt.SetRedis(redis.Client)
	captcha.SetStore(cache.NewCacheService())
	sh.Register("redis", func(ctx context.Context) error {
		return redis.Close()
	})

	lifecycleCtx, cancelLifecycle := context.WithCancel(ctx)
	sh.Register("lifecycle-cancel", func(ctx context.Context) error {
		cancelLifecycle()
		return nil
	})

	if err := runtimeconfig.DefaultSecurityPolicyReader().Refresh(ctx); err != nil {
		logger.Warn("security policy warmup failed", logger.Err(err))
	}

	runtimeConfigListener, err := runtimeconfig.StartInvalidationListener(lifecycleCtx)
	if err != nil {
		logger.Warn("runtime config invalidation listener start failed", logger.Err(err))
	} else {
		sh.Register("runtime-config-listener", func(ctx context.Context) error {
			return runtimeConfigListener.Close()
		})
	}

	publisher, err := events.ConnectRedis(redis.Client)
	if err != nil {
		logger.Warn("redis event publisher connect failed, auth events disabled", logger.Err(err))
	} else if publisher != nil {
		events.SetDefault(publisher)
		logger.Info("redis event publishing enabled")
	}

	tracingCfg := config.Cfg.Observability.Tracing
	shutdownTracing, err := observability.InitTracer(ctx, observability.Config{
		Enabled:      tracingCfg.Enabled,
		ServiceName:  tracingCfg.ServiceName,
		Environment:  tracingCfg.Environment,
		OTLPEndpoint: tracingCfg.OTLPEndpoint,
		SampleRatio:  tracingCfg.SampleRatio,
	})
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
		sh.Register("tracing", func(ctx context.Context) error {
			return shutdownTracing(ctx)
		})
	}

	configureGinWriters(config.Cfg.App.Env)

	router := gin.New()
	if len(config.Cfg.Security.TrustedProxies) > 0 {
		if err := router.SetTrustedProxies(config.Cfg.Security.TrustedProxies); err != nil {
			return fmt.Errorf("trusted proxy config failed: %w", err)
		}
	}

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

	port := config.Cfg.App.Port
	addr := fmt.Sprintf(":%d", port)
	printStartupBanner(config.Cfg.App.Name, config.Cfg.App.Version, config.Cfg.App.Env, port)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", logger.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	sh.Register("http-server", func(ctx context.Context) error {
		return server.Shutdown(ctx)
	})

	if err := sh.WaitAndShutdown(); err != nil {
		logger.Error("graceful shutdown error", logger.Err(err))
	}

	select {
	case err := <-serverErr:
		return err
	default:
		return nil
	}
}
