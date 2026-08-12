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
	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	authdao "github.com/go-admin-kit/services/shared/pkg/authdao"
	"github.com/go-admin-kit/services/shared/pkg/exportproof"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	sharedmetrics "github.com/go-admin-kit/services/shared/pkg/metrics"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/secretbox"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
	tenantscope "github.com/go-admin-kit/services/shared/pkg/tenant"
	"github.com/go-admin-kit/services/system/internal/api"
	systemAPI "github.com/go-admin-kit/services/system/internal/api/system"
	"github.com/go-admin-kit/services/system/internal/config"
	authDAO "github.com/go-admin-kit/services/system/internal/dao/auth"
	systemDAO "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/edgecert"
	"github.com/go-admin-kit/services/system/internal/middleware"
	"github.com/go-admin-kit/services/system/internal/pkg/authz"
	"github.com/go-admin-kit/services/system/internal/pkg/database"
	"github.com/go-admin-kit/services/system/internal/pkg/observability"
	"github.com/go-admin-kit/services/system/internal/pkg/redis"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	authsvc "github.com/go-admin-kit/services/system/internal/service/auth"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
	"gorm.io/gorm"
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

func buildEdgeCertificateRuntime(cfg config.EdgeCertConfig, db *gorm.DB) (*edgecert.Service, error) {
	currentID, currentMaterial, previousID, previousMaterial, err := cfg.KeyMaterials()
	if err != nil {
		return nil, err
	}
	defer clear(currentMaterial)
	defer clear(previousMaterial)

	var keyring *secretbox.Keyring
	if currentID != "" {
		previous := make([]secretbox.Key, 0, 1)
		if previousID != "" {
			previous = append(previous, secretbox.Key{ID: previousID, Material: previousMaterial})
		}
		keyring, err = secretbox.NewKeyring(
			secretbox.Key{ID: currentID, Material: currentMaterial},
			previous...,
		)
		if err != nil {
			return nil, fmt.Errorf("initialize edge certificate keyring: %w", err)
		}
	}

	var deployer *edgecert.FileDeployer
	if cfg.StorageRoot != "" && cfg.TraefikDynamicDir != "" {
		deployer = &edgecert.FileDeployer{
			CertDir:           cfg.StorageRoot,
			DynamicConfigDir:  cfg.TraefikDynamicDir,
			ContainerCertDir:  cfg.StorageRoot,
			GatewayTLSAddress: cfg.GatewayTLSAddress,
			ProbeTimeout:      5 * time.Second,
		}
	}
	issuer := edgecert.ACMEIssuer{
		ChallengeTTL: time.Duration(cfg.ChallengeTTLMinutes) * time.Minute,
	}
	service := edgecert.NewService(db, keyring, issuer, deployer)
	service.RenewBefore = time.Duration(cfg.RenewBeforeDays) * 24 * time.Hour
	service.WorkerEnabled = cfg.WorkerEnabled
	service.ClearLegacySecrets = cfg.ClearLegacySecrets
	return service, nil
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
	if err := sharedaudit.Register(database.DB, sharedaudit.Config{
		Targets: []sharedaudit.Target{
			sharedaudit.MenuTarget(&model.Menu{}),
			sharedaudit.EdgeTLSCertificateTarget(&edgecert.Certificate{}),
		},
	}); err != nil {
		return fmt.Errorf("audit trail plugin registration failed: %w", err)
	}
	// Phase 2D：审计事件化——init 发布器（NATS 配置后走事件，否则回退直写）。
	if err := auditevents.Init(config.Cfg.NATS.URL); err != nil {
		logger.Warn("audit events publisher init failed（回退直写）", logger.Err(err))
	} else if config.Cfg.NATS.URL != "" {
		logger.Info("audit events publisher enabled")
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
	runtimeconfig.SetErrorCodeStore(systemDAO.NewErrCodeDAO(database.DB))
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

	edgeCertificateService, err := buildEdgeCertificateRuntime(config.Cfg.EdgeCert, database.DB)
	if err != nil {
		return fmt.Errorf("edge certificate runtime configuration failed: %w", err)
	}
	// Backfill and key rotation must finish before HTTP opens. A wrong/missing
	// key therefore fails closed without exposing a partially migrated service.
	if err := edgeCertificateService.MigrateLegacySecrets(ctx); err != nil {
		return fmt.Errorf("edge certificate secret migration failed: %w", err)
	}
	if err := edgeCertificateService.ValidateSecurityState(ctx); err != nil {
		return fmt.Errorf("edge certificate security state invalid: %w", err)
	}
	systemAPI.ConfigureEdgeCertAPI(edgeCertificateService, exportproof.NewStore(redis.Client))

	lifecycleCtx, cancelLifecycle := context.WithCancel(ctx)
	defer cancelLifecycle()
	var edgeCertificateWorkerDone chan struct{}
	var edgeCertificateWorkerErr error
	if config.Cfg.EdgeCert.WorkerEnabled {
		worker := edgecert.NewWorker(edgeCertificateService, "")
		worker.PollInterval = time.Duration(config.Cfg.EdgeCert.TaskPollSeconds) * time.Second
		worker.OnError = func(err error) {
			logger.Error("edge certificate worker task failed", logger.Err(err))
		}
		edgeCertificateWorkerDone = make(chan struct{})
		go func() {
			edgeCertificateWorkerErr = worker.Run(lifecycleCtx)
			close(edgeCertificateWorkerDone)
		}()
	}

	// system 服务是菜单数据的 owner，独占默认菜单播种（按 ID 补插缺失行）。
	menuBootstrapCtx := sharedaudit.WithTenantID(
		sharedaudit.WithActor(ctx, "system", "system-service/bootstrap"),
		1,
	)
	if menuResult, err := systemsvc.BootstrapDefaultMenusContext(menuBootstrapCtx, database.DB); err != nil {
		return fmt.Errorf("default menu bootstrap failed: %w", err)
	} else if menuResult.Menus > 0 {
		logger.Info("default menus bootstrapped", logger.Int("menus", menuResult.Menus))
	}

	// 持久化本服务现持有的管理端 CRUD 操作日志。
	operationLogService := systemsvc.NewOperationLogServiceWithDB(database.DB)
	operationLogDone := middleware.StartOperationLogProcessor(lifecycleCtx, &operationLogService)
	defer func() {
		if err := stopOperationLogProcessor(cancelLifecycle, operationLogDone, 5*time.Second); err != nil {
			logger.Warn("operation log processor shutdown timeout", logger.Err(err))
		}
	}()

	// 当其他实例（或单体版）变更部门时，刷新缓存的部门树。
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

	// 将（任一实例）发布到 Redis 的通知事件，扇出给连接本实例的
	// WebSocket 客户端。
	if err := systemAPI.StartNotificationRedisBridge(lifecycleCtx, systemsvc.DefaultNotificationBroadcaster()); err != nil {
		logger.Warn("notification redis bridge start failed", logger.Err(err))
	} else {
		defer func() {
			if err := systemAPI.StopNotificationRedisBridge(); err != nil {
				logger.Warn("notification redis bridge close failed", logger.Err(err))
			}
		}()
	}

	// 预热运行时安全策略缓存；失败时首次请求回退到静态配置默认值。
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

	port := config.Cfg.App.Port
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: router}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("server listen failed: %w", err)
	}
	printStartupBanner(config.Cfg.App.Name, config.Cfg.App.Version, config.Cfg.App.Env, port)
	sh := graceful.New(graceful.WithTimeout(15 * time.Second))
	if edgeCertificateWorkerDone != nil {
		sh.Register("edge-certificate-worker", func(ctx context.Context) error {
			cancelLifecycle()
			select {
			case <-edgeCertificateWorkerDone:
				return edgeCertificateWorkerErr
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", logger.String("addr", server.Addr))
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	sh.Register("http-server", func(ctx context.Context) error {
		return server.Shutdown(ctx)
	})
	// Derive signal handling from the caller so tests and embedding runtimes can
	// stop the service without synthesizing an OS signal.
	signalCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	var runtimeErr error
	select {
	case <-signalCtx.Done():
	case err := <-serverErr:
		runtimeErr = err
	case <-edgeCertificateWorkerDone:
		if edgeCertificateWorkerErr != nil {
			runtimeErr = fmt.Errorf("edge certificate worker exited: %w", edgeCertificateWorkerErr)
		} else {
			runtimeErr = errors.New("edge certificate worker exited unexpectedly")
		}
	}
	if err := sh.Shutdown(); err != nil {
		logger.Error("graceful shutdown error", logger.Err(err))
		runtimeErr = errors.Join(runtimeErr, err)
	}
	return runtimeErr
}
