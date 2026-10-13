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
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/agentadapter"
	"github.com/go-admin-kit/services/ai/internal/agentcore"
	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/api"
	aihandler "github.com/go-admin-kit/services/ai/internal/api/ai"
	"github.com/go-admin-kit/services/ai/internal/config"
	"github.com/go-admin-kit/services/ai/internal/dao"
	aidao "github.com/go-admin-kit/services/ai/internal/dao/ai"
	authDAO "github.com/go-admin-kit/services/ai/internal/dao/auth"
	systemDAO "github.com/go-admin-kit/services/ai/internal/dao/system"
	"github.com/go-admin-kit/services/ai/internal/middleware"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg"
	"github.com/go-admin-kit/services/ai/internal/pkg/runtimeconfig"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	authsvc "github.com/go-admin-kit/services/ai/internal/service/auth"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	systemsvc "github.com/go-admin-kit/services/ai/internal/service/system"
	aiv1 "github.com/go-admin-kit/services/api/gen/ai/v1"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	"github.com/go-admin-kit/services/shared/pkg/database"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/identityclient"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	sharedmetrics "github.com/go-admin-kit/services/shared/pkg/metrics"
	sharedmw "github.com/go-admin-kit/services/shared/pkg/middleware"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/observability"
	"github.com/go-admin-kit/services/shared/pkg/redis"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
	tenantscope "github.com/go-admin-kit/services/shared/pkg/tenant"
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
	jwt.SetConfig(jwt.JWTConfig{Secret: config.Cfg.JWT.Secret, Issuer: config.Cfg.JWT.Issuer, AccessTokenExpire: config.Cfg.JWT.AccessTokenExpire, RefreshTokenExpire: config.Cfg.JWT.RefreshTokenExpire, RefreshTokenRotation: config.Cfg.JWT.RefreshTokenRotation})

	logCfg := config.Cfg.Logger
	logger.InitLogger(logCfg.FilePath, logCfg.Level, logCfg.MaxSize, logCfg.MaxBackups, logCfg.MaxAge)
	defer func() {
		if logger.Logger != nil {
			_ = logger.Logger.Sync()
		}
	}()

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
	// B3：Hermes 权益/策略管理写审计（设备心跳与 token 不注册）
	if err := sharedaudit.Register(database.DB, sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &localmodel.HermesEntitlement{}, Table: "hermes_entitlements", TargetType: "hermes_entitlement", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "UserID", "Enabled", "PolicyID", "DeviceLimit", "CreatedBy", "UpdatedBy", "CreatedAt", "UpdatedAt"}},
		{Model: &localmodel.HermesPolicy{}, Table: "hermes_policies", TargetType: "hermes_policy", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Revision", "Name", "DesiredState", "Profile", "AssistantTemplateID", "ArtifactID", "ModelID", "ModelName", "EnableKnowledgeRAG", "CreatedBy", "CreatedAt"}},
	}}); err != nil {
		return fmt.Errorf("audit trail registration failed: %w", err)
	}
	if err := authz.RegisterDataScopePlugin(database.DB); err != nil {
		return fmt.Errorf("data scope plugin registration failed: %w", err)
	}
	authz.SetDefaultDB(database.DB)
	registerAuthzScopedModels()
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
	runtimeconfig.SetAIProviderStore(systemDAO.NewSettingDAO(database.DB))
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("database close failed", logger.Err(err))
		}
	}()

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
	authz.SetRemoteCache(authz.NewGoRedisRemoteCache(redis.Client))
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
	router.Use(sharedaudit.AuditHeaderMiddleware())
	setupCORS(router)

	// 初始化加密器（用于 API Key 加密存储）
	encryptor, err := pkg.NewEncryptor(config.Cfg.Encryption.Key)
	if err != nil {
		return fmt.Errorf("encryptor initialization failed: %w", err)
	}

	// 初始化 AI 模型配置管理
	modelDAO := dao.NewAIModelDAO(database.DB)
	usageLogDAO := dao.NewAIUsageLogDAO(database.DB)
	quotaDAO := dao.NewAIQuotaDAO(database.DB)

	modelService := service.NewAIModelService(modelDAO, encryptor)
	usageService := service.NewAIUsageService(usageLogDAO)
	quotaService := service.NewAIQuotaService(quotaDAO)

	// 配额检查中间件（在所有业务中间件之后注册）
	quotaChecker := middleware.NewAIQuotaChecker(quotaService, usageLogDAO, database.DB)
	router.Use(quotaChecker.CheckQuota())

	// Assemble the AI provider stack. Settings resolve per request from the
	// ai.provider system setting layered over AI_* env vars, so console edits
	// apply without a restart. An unconfigured provider keeps the service
	// running (health checks included) while AI endpoints return 503.
	aiSettingsReader := runtimeconfig.DefaultAIProviderReader()
	if err := aiSettingsReader.Refresh(ctx); err != nil {
		logger.Warn("AI provider settings warmup failed", logger.Err(err))
	}
	resolveAIOptions := func(ctx context.Context) aiclient.Options {
		s := aiSettingsReader.AIProviderSettings(ctx)
		return aiclient.Options{
			Provider:      s.Provider,
			BaseURL:       s.BaseURL,
			APIKey:        s.APIKey,
			ChatModel:     s.ChatModel,
			EmbedModel:    s.EmbedModel,
			ImageModel:    s.ImageModel,
			EmbedProvider: s.EmbedProvider,
			EmbedBaseURL:  s.EmbedBaseURL,
			EmbedAPIKey:   s.EmbedAPIKey,
		}
	}
	providers := aiclient.NewDynamicProviders(resolveAIOptions)
	configuredFunc := func(ctx context.Context) bool {
		return aiSettingsReader.AIProviderSettings(ctx).Configured()
	}
	knowledgeSvc := aisvc.NewKnowledgeServiceWithDB(database.DB, providers)
	handler := aihandler.NewHandler(
		func(ctx context.Context) aihandler.StatusInfo {
			s := aiSettingsReader.AIProviderSettings(ctx)
			providerName := s.Provider
			if providerName != "anthropic" {
				providerName = "openai" // factory 对未知/空名回落 OpenAI 兼容客户端
			}
			return aihandler.StatusInfo{
				Configured: s.Configured(),
				Provider:   providerName,
				ChatModel:  s.ChatModel,
				EmbedModel: s.EmbedModel,
			}
		},
		aisvc.NewChatServiceWithDB(database.DB, providers),
		knowledgeSvc,
		aisvc.NewInsightServiceWithDB(database.DB, providers),
	)
	// Phase 3：ai 知识库检索 gRPC server（Consul 发现，供 im 机器人 RAG 调）。
	go startAiGRPC(knowledgeSvc, configuredFunc)
	if !aiSettingsReader.AIProviderSettings(ctx).Configured() {
		logger.Warn("AI provider is not configured; AI endpoints return 503 until an API key is set in system settings or AI_API_KEY")
	}

	// 注入 usage service 到 handler（用于自动记录 AI 调用）
	handler.SetUsageService(usageService)
	handler.SetModelService(modelService)

	modelHandler := aihandler.NewAIModelHandler(modelService)
	usageHandler := aihandler.NewAIUsageHandler(usageService)
	quotaHandler := aihandler.NewAIQuotaHandler(quotaService)

	// Hermes is an optional control-plane capability. A missing or malformed
	// dedicated Ed25519 key disables signed policy/enrollment completion but
	// must not prevent unrelated AI endpoints from starting.
	hermesSigner, hermesSignerErr := service.NewHermesPolicySigner(config.Cfg.Hermes.PolicySigningKey)
	if hermesSignerErr != nil {
		logger.Warn("Hermes policy signing is unavailable", logger.Err(hermesSignerErr))
		hermesSigner = nil
	} else if hermesSigner == nil {
		logger.Warn("Hermes policy signing is not configured; signed Hermes endpoints return 503")
	}
	// 技能供应链验签信任根：配置了才验签（未配置 = 降级不验，同 policy signer）。
	skillVerifier, skillVerifierErr := service.NewHermesPublicKeyVerifier(config.Cfg.Hermes.SkillSigningPublicKey)
	if skillVerifierErr != nil {
		logger.Warn("Hermes skill signing verifier is unavailable", logger.Err(skillVerifierErr))
		skillVerifier = nil
	} else if skillVerifier == nil {
		logger.Info("Hermes skill signing public key not configured; skill signatures are not verified")
	}
	hermesDAO := dao.NewHermesDAO(database.DB)
	hermesOfficeDAO := dao.NewHermesOfficeDAO(database.DB)
	// B2：Hermes 用户显示名/存在性走 identity owner API，禁止 JOIN public.users。
	if idBase := strings.TrimSpace(os.Getenv("IDENTITY_API_BASE")); idBase != "" {
		if idc, err := identityclient.New(idBase, os.Getenv("INTERNAL_TOKEN")); err != nil {
			logger.Warn("identity client init failed（Hermes 用户显示名降级为空）", logger.Err(err))
		} else {
			hermesDAO.SetIdentity(idc)
			hermesOfficeDAO.SetIdentity(idc)
			logger.Info("hermes identity client enabled", logger.String("base", idBase))
		}
	} else {
		logger.Warn("IDENTITY_API_BASE unset; Hermes 用户显示名/存在性校验不可用")
	}
	hermesService := service.NewHermesService(hermesDAO, hermesSigner, service.HermesServiceOptions{
		VerificationURI:       config.Cfg.Hermes.VerificationURI,
		SkillSigningPublicKey: skillVerifier,
	})
	hermesHandler := aihandler.NewHermesHandler(hermesService)
	hermesDeviceAuth := middleware.HermesDeviceAuth(
		hermesDAO,
		middleware.NewRedisHermesNonceStore(redis.Client),
		time.Now,
	)

	hermesOfficeHandler := aihandler.NewHermesOfficeHandler(service.NewHermesOfficeService(hermesOfficeDAO))

	// 企业模型代理：供应商 key 只在服务端，设备用短期 token 调用。
	hermesTokenDAO := dao.NewHermesModelTokenDAO(database.DB)
	// 周期清理过期 token 行。TTL 只有 30 分钟，鉴权路径按 expires_at 判定不受
	// 残留行影响，这里纯粹防表无界增长；留 24h 缓冲便于按 last_used_at 排障。
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-lifecycleCtx.Done():
				return
			case <-ticker.C:
				if err := hermesTokenDAO.DeleteExpired(lifecycleCtx, time.Now().Add(-24*time.Hour)); err != nil {
					logger.Warn("hermes model token cleanup failed", logger.Err(err))
				}
			}
		}
	}()
	// 额度按 tenant/user 判定，与控制台自身的 AI 配额同一套配置
	hermesQuotaGuard := service.NewHermesQuotaGuard(quotaService, usageLogDAO, time.Now)
	hermesProxyService := service.NewHermesProxyService(
		hermesService, hermesTokenDAO, usageService,
		providers, modelService, hermesQuotaGuard,
		aidao.NewDocumentDAO(database.DB), time.Now,
	)
	hermesProxyHandler := aihandler.NewHermesProxyHandler(hermesProxyService)
	hermesUsageHandler := aihandler.NewHermesUsageHandler(
		service.NewHermesUsageService(dao.NewHermesUsageDAO(database.DB), time.Now),
	)
	// 设备吊销 / 权益停用后立即失效模型访问，而不是等 token 自然过期
	hermesService.SetModelTokenRevoker(hermesProxyService)
	// 发布策略时就校验模型引用，而不是等设备调用才暴露
	hermesService.SetModelLookup(modelDAO)

	// Agent Kernel 仅依赖通用 Port。模型目录、配额、用量和知识库都在组合根
	// 适配；agentcore 不导入 Hermes、供应商协议或具体 Runtime。
	agentTools := agentcore.NewToolRegistry()
	if err := agentadapter.RegisterKnowledgeSearch(agentTools, func(ctx context.Context, query string, topK int) (any, error) {
		return knowledgeSvc.Search(ctx, query, topK)
	}); err != nil {
		return fmt.Errorf("agent knowledge tool registration failed: %w", err)
	}
	agentModels := agentadapter.NewModelPort(
		func(ctx context.Context, modelID uint64) (*agentadapter.ModelConfig, error) {
			configuredModel, err := modelService.GetModel(ctx, modelID)
			if err != nil {
				return nil, err
			}
			return &agentadapter.ModelConfig{
				ID: configuredModel.ID, Name: configuredModel.Name, Provider: configuredModel.Provider,
				BaseURL: configuredModel.BaseURL, APIKey: configuredModel.APIKey,
				Enabled:        configuredModel.Status == localmodel.AIModelStatusEnabled,
				CostPer1KInput: configuredModel.CostPer1KInput, CostPer1KOutput: configuredModel.CostPer1KOutput,
			}, nil
		},
		func(ctx context.Context, actorID uint64, tenantID uint) error {
			return hermesQuotaGuard.Check(ctx, uint(actorID), tenantID)
		},
		func(ctx context.Context, record agentadapter.UsageRecord) error {
			promptTokens, completionTokens, totalTokens, latencyMS := record.InputTokens, record.OutputTokens, record.TotalTokens, record.LatencyMS
			userID, tenantID := record.ActorID, uint64(record.TenantID)
			return usageService.RecordUsage(ctx, &localmodel.AIUsageLog{
				ModelID: record.ModelID, ModelName: record.ModelName, UserID: &userID, TenantID: &tenantID,
				Operation: localmodel.AIOperationAgent, PromptTokens: &promptTokens, CompletionTokens: &completionTokens,
				TotalTokens: &totalTokens, Cost: record.Cost, LatencyMs: &latencyMS, Status: record.Status,
				ErrorMsg: record.ErrorMessage, RequestID: fmt.Sprintf("agent-run-%d", record.RunID), CreatedAt: time.Now().UTC(),
			})
		},
		func(err error) { logger.Warn("agent usage record failed", logger.Err(err)) },
	)
	agentRepository := agentcore.NewRepository(database.DB)
	agentService := agentcore.NewService(agentRepository, agentModels, agentTools)
	handler.SetAgentService(agentService)
	agentWorker := agentcore.NewWorker(agentRepository, agentModels, agentTools, agentcore.WorkerConfig{
		PollInterval: 2 * time.Second, LeaseDuration: 30 * time.Second,
		OnError: func(err error) { logger.Warn("agent worker iteration failed", logger.Err(err)) },
	})
	go agentWorker.Run(lifecycleCtx)

	api.SetupRoutesWithDeps(router, sharedapi.Dependencies{DB: database.DB, Redis: redis.Client}, handler, modelHandler, usageHandler, quotaHandler, hermesHandler, hermesOfficeHandler, hermesProxyHandler, hermesUsageHandler, hermesDeviceAuth)

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

// startAiGRPC 启动 ai 知识库检索 gRPC 服务并注册 Consul（Phase 3）。
// CONSUL_ADDR=disabled 时跳过注册；注册失败仅告警不阻断 HTTP。
func startAiGRPC(knowledge aisvc.KnowledgeService, configured func(context.Context) bool) {
	port := 9087
	if v := os.Getenv("GRPC_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Warn("ai grpc listen failed", logger.Err(err))
		return
	}
	grpcSrv := grpcx.NewServer()
	aiv1.RegisterAiServiceServer(grpcSrv, api.NewAiGRPC(knowledge, configured))
	go func() { _ = grpcSrv.Serve(lis) }()
	logger.Info("ai grpc server started", logger.Int("port", port))

	if consulAddr := os.Getenv("CONSUL_ADDR"); consulAddr != "disabled" {
		host := grpcx.LocalIP()
		if host == "" {
			host = "127.0.0.1"
		}
		if deregister, regErr := grpcx.Register(consulAddr, grpcx.Instance{
			ServiceName: "ai-service",
			Host:        host,
			Port:        port,
		}); regErr == nil {
			defer deregister()
		}
	}
	select {}
}

func registerAuthzScopedModels() {
	authz.RegisterScopedModel(reflect.TypeOf(model.File{}), authz.ScopeByOwner)
	authz.RegisterScopedModel(reflect.TypeOf(localmodel.LoginLog{}), authz.ScopeByOwner)
	authz.RegisterScopedModel(reflect.TypeOf(localmodel.OperationLog{}), authz.ScopeByOwner)
}
