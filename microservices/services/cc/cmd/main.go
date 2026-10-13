package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/api/cc"
	"github.com/go-admin-kit/services/cc/internal/config"
	"github.com/go-admin-kit/services/cc/internal/crmclient"
	"github.com/go-admin-kit/services/cc/internal/fsclient"
	"github.com/go-admin-kit/services/cc/internal/hub"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
)

func main() {
	cfg := config.Load()
	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	st, err := store.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// 数据变更审计：坐席/联系人/费率/账户/质检词/风控策略与名单为单行写审计目标。管理面写
	// 方法均已收 ctx（见 internal/store），插件随事务同生共死；话单 ApplyEvent
	// 是 webhook 驱动无 actor，插件静默跳过（刻意不审计 cc_calls）。
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &store.Agent{}, Table: "cc_agents", TargetType: "cc_agent", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "UserID", "Extension", "DisplayName"}},
		{Model: &store.Contact{}, Table: "cc_contacts", TargetType: "cc_contact", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Number", "Name", "Remark"}},
		{Model: &store.CCRate{}, Table: "cc_rates", TargetType: "cc_rate", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Prefix", "Name", "RatePerMin", "BillingStep", "ConnectFee"}},
		{Model: &store.CCBillingAccount{}, Table: "cc_billing_accounts", TargetType: "cc_billing_account", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Balance", "CreditLimit", "Status"}},
		{Model: &store.CCQcKeyword{}, Table: "cc_qc_keywords", TargetType: "cc_qc_keyword", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Word", "Type", "Remark"}},
		// cc_bills 由 GenerateBills 批量 upsert（多租户多行），撞 MaxRows=1 保护模型，不注册。
		{Model: &store.CCAccountTxn{}, Table: "cc_account_txns", TargetType: "cc_account_txn", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Kind", "Amount", "BalanceAfter", "OperatorID", "Remark"}},
		{Model: &store.CCRiskPolicy{}, Table: "cc_risk_policies", TargetType: "cc_risk_policy", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "OutboundStart", "OutboundEnd", "OutboundWeekdays", "MaxCallsPerNumberPerHour", "MaxCallsPerAgentPerHour", "MaxCallsPerTenantPerMin", "BlockedPrefixes", "MaxRatePerMin"}},
		{Model: &store.CCRiskListEntry{}, Table: "cc_risk_list_entries", TargetType: "cc_risk_list", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Number", "Match", "ListType", "Remark"}},
	}}); err != nil {
		log.Fatalf("audit trail plugin registration failed: %v", err)
	}

	srv := &api.Server{
		Store:          st,
		FS:             fsclient.New(cfg.FSAPIBase, cfg.FSAPIToken),
		CRM:            crmclient.New(cfg.CRMAPIBase, cfg.CRMInternalToken),
		Secret:         cfg.JWTSecret,
		WebhookSecret:  cfg.WebhookSecret,
		CDRAuth:        cfg.CDRAuth,
		ProductionMode: cfg.ProductionMode(),
		// 实时事件通道：webhook/cdr 入库后经 hub 广播给坐席浏览器（来电弹屏底座）。
		Hub: hub.New(),
		// TTS/ASR：DB（system_settings cc.tts/cc.asr）热配置优先，env 兜底
		Speech: config.NewSpeechReader(st.DB(), config.SpeechSettings{
			TTSURL: cfg.TTSURL, TTSKey: cfg.TTSKey, TTSModel: cfg.TTSModel, TTSVoice: cfg.TTSVoice,
			ASRURL: cfg.ASRURL, ASRKey: cfg.ASRKey, ASRModel: cfg.ASRModel,
		}),
		InternalToken: cfg.InternalToken,
	}
	log.Printf("cc-service fs_api=%q webhook_signed=%v crm_linked=%v", cfg.FSAPIBase, cfg.WebhookSecret != "", srv.CRM.Enabled())

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "cc")
	if err != nil {
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	r := gin.New()
	r.Use(observability.GinTracing("cc", "request_id"))
	// HTTP 指标（GET /metrics，Prometheus 抓取）；先于 Logger 注册，抓取不刷访问日志
	metrics.Install(r)
	// 健康探针每 10s 一次；成功探测不进访问日志，失败（>=400）仍记录
	r.Use(gin.Recovery(), gin.LoggerWithConfig(gin.LoggerConfig{
		Skip: func(c *gin.Context) bool {
			p := c.Request.URL.Path
			return c.Writer.Status() < 400 &&
				(strings.HasSuffix(p, "/health/live") || strings.HasSuffix(p, "/health/ready"))
		},
	}))

	// health (no auth)
	r.GET("/api/v1/health/live", srv.HealthLive)
	r.GET("/api/v1/health/ready", srv.HealthReady)
	r.GET("/api/v1/cc/health/live", srv.HealthLive)
	r.GET("/api/v1/cc/health/ready", srv.HealthReady)

	// webhook：freeswitch-cc control-api 推送，HMAC 验签（不走网关 ForwardAuth）
	r.POST("/api/v1/cc/webhook", srv.Webhook)
	// mod_json_cdr 话单直投（Basic 认证，不走网关 ForwardAuth）
	r.POST("/api/v1/cc/cdr", srv.ReceiveCDR)
	// 来显归属：mod_cidlookup 直连反查（无鉴权，裸文本客户名/404）
	r.GET("/api/v1/cc/cid", srv.CidLookup)
	// 实时事件 WS（坐席浏览器弹屏底座）：握手带不了 Authorization 头，走 ?token=
	// 自校验，故排在网关 cc-open 白名单（不挂 ForwardAuth）。
	r.GET("/api/v1/cc/ws", srv.WebSocket)
	// 内部：notify → 语音通知入队（X-Internal-Token，内网直连，不走网关）
	r.POST("/api/v1/cc/internal/voice-notify", srv.InternalVoiceNotify)
	r.POST("/api/v1/cc/internal/risk-check", srv.InternalRiskCheck)
	r.POST("/api/v1/cc/internal/early-media/classify", srv.InternalEarlyMediaClassify)

	// admin（网关 X-Auth-* 头优先，Bearer JWT 兜底）
	r.Use(gatewayauth.RequireReadWritePermissions("cc:read", "cc:write"), sharedaudit.AuditHeaderMiddleware())
	r.GET("/api/v1/cc/calls", srv.ListCalls)
	// 号码识客聚合（坐席工作台来电弹屏）
	r.GET("/api/v1/cc/lookup", srv.Lookup)
	r.GET("/api/v1/cc/agents", srv.ListAgents)
	r.PUT("/api/v1/cc/agents/:user_id", srv.BindAgent)
	r.DELETE("/api/v1/cc/agents/:user_id", srv.UnbindAgent)
	// 来显归属：联系人库维护（话单页"备注姓名"）
	r.GET("/api/v1/cc/contacts", srv.ListContacts)
	r.POST("/api/v1/cc/contacts", srv.UpsertContact)
	r.DELETE("/api/v1/cc/contacts/:number", srv.DeleteContact)
	// 呼叫报表（纯 cc_calls 聚合）
	r.GET("/api/v1/cc/reports/overview", srv.ReportsOverview)
	r.GET("/api/v1/cc/fs/status", srv.FSStatus)
	r.POST("/api/v1/cc/fs/webhook-test", srv.WebhookTest)
	// 呼叫控制（fscontrol.go）
	r.GET("/api/v1/cc/fs/registrations", srv.FSRegistrations)
	r.GET("/api/v1/cc/fs/channels", srv.FSChannels)
	r.POST("/api/v1/cc/fs/originate", srv.FSOriginate)
	r.POST("/api/v1/cc/fs/hangup", srv.FSHangup)
	// 坐席通话操控（fscontrol.go）：转移/保持/停泊/DTMF
	r.POST("/api/v1/cc/fs/transfer", srv.FSTransfer)
	r.POST("/api/v1/cc/fs/hold", srv.FSHold)
	r.POST("/api/v1/cc/fs/park", srv.FSPark)
	r.POST("/api/v1/cc/fs/dtmf", srv.FSDTMF)
	r.GET("/api/v1/cc/fs/parked", srv.FSParked)
	// 分机管理 + 录音（fscontrol.go）
	r.GET("/api/v1/cc/fs/extensions", srv.FSExtensions)
	r.POST("/api/v1/cc/fs/extensions", srv.FSCreateExtension)
	r.PUT("/api/v1/cc/fs/extensions/:ext", srv.FSEditExtension)
	r.DELETE("/api/v1/cc/fs/extensions/:ext", srv.FSDeleteExtension)
	r.GET("/api/v1/cc/fs/recordings", srv.FSRecordings)
	r.GET("/api/v1/cc/fs/recordings/:file", srv.FSRecordingFile)
	// 语音信箱留言（fscontrol.go）
	r.GET("/api/v1/cc/fs/voicemails", srv.FSVoicemails)
	r.GET("/api/v1/cc/fs/voicemails/:ext/:file", srv.FSVoicemailFile)
	r.DELETE("/api/v1/cc/fs/voicemails/:ext/:file", srv.FSVoicemailDelete)
	// SIP 线路 + IVR（fscontrol.go）
	r.GET("/api/v1/cc/fs/gateways", srv.FSGateways)
	r.POST("/api/v1/cc/fs/gateways", srv.FSCreateGateway)
	r.PUT("/api/v1/cc/fs/gateways/:name", srv.FSUpdateGateway)
	r.DELETE("/api/v1/cc/fs/gateways/:name", srv.FSDeleteGateway)
	// LCR 选路预览（纯内存计算，代理 control-api）
	r.GET("/api/v1/cc/fs/lcr-preview", srv.FSLCRPreview)
	r.GET("/api/v1/cc/fs/ivrs", srv.FSIVRs)
	r.POST("/api/v1/cc/fs/ivrs", srv.FSCreateIVR)
	r.PUT("/api/v1/cc/fs/ivrs/:name", srv.FSUpdateIVR)
	r.DELETE("/api/v1/cc/fs/ivrs/:name", srv.FSDeleteIVR)
	// 队列 + 坐席（fsqueues.go）
	r.GET("/api/v1/cc/fs/queues", srv.FSCCQueues)
	r.POST("/api/v1/cc/fs/queues", srv.FSCreateCCQueue)
	r.PUT("/api/v1/cc/fs/queues/:name", srv.FSUpdateCCQueue)
	r.DELETE("/api/v1/cc/fs/queues/:name", srv.FSDeleteCCQueue)
	r.PUT("/api/v1/cc/fs/queues/:name/agents", srv.FSUpsertQueueAgent)
	r.DELETE("/api/v1/cc/fs/queues/:name/agents/:ext", srv.FSRemoveQueueAgent)
	r.POST("/api/v1/cc/fs/agent-status", srv.FSSetAgentStatus)
	// 营业时间路由（fstimeroutes.go）
	r.GET("/api/v1/cc/fs/time-routes", srv.FSTimeRoutes)
	r.POST("/api/v1/cc/fs/time-routes", srv.FSCreateTimeRoute)
	r.PUT("/api/v1/cc/fs/time-routes/:name", srv.FSUpdateTimeRoute)
	r.DELETE("/api/v1/cc/fs/time-routes/:name", srv.FSDeleteTimeRoute)
	// 响铃组（fstimeroutes.go）
	r.GET("/api/v1/cc/fs/ring-groups", srv.FSRingGroups)
	r.POST("/api/v1/cc/fs/ring-groups", srv.FSCreateRingGroup)
	r.PUT("/api/v1/cc/fs/ring-groups/:name", srv.FSUpdateRingGroup)
	r.DELETE("/api/v1/cc/fs/ring-groups/:name", srv.FSDeleteRingGroup)
	// 班长话务监控台聚合
	r.GET("/api/v1/cc/fs/monitor", srv.FSMonitor)
	// 会议室（fsconferences.go）
	r.GET("/api/v1/cc/fs/conferences", srv.FSConferences)
	r.POST("/api/v1/cc/fs/conferences", srv.FSCreateConference)
	r.PUT("/api/v1/cc/fs/conferences/:name", srv.FSUpdateConference)
	r.DELETE("/api/v1/cc/fs/conferences/:name", srv.FSDeleteConference)
	r.POST("/api/v1/cc/fs/conferences/:name/action", srv.FSConferenceAction)
	// 质检（监听/耳语/强插 + 通话中录音）
	r.POST("/api/v1/cc/fs/supervise", srv.FSSupervise)
	r.POST("/api/v1/cc/fs/uuid-record", srv.FSUUIDRecord)
	// 排队音乐
	r.GET("/api/v1/cc/fs/moh", srv.FSMOHList)
	r.POST("/api/v1/cc/fs/moh", srv.FSMOHUpload)
	r.GET("/api/v1/cc/fs/moh/:file", srv.FSMOHFile)
	r.DELETE("/api/v1/cc/fs/moh/:file", srv.FSMOHDelete)
	// 提示音库（上传 + TTS 预生成）
	r.GET("/api/v1/cc/fs/prompts", srv.FSPromptsList)
	r.POST("/api/v1/cc/fs/prompts", srv.FSPromptUpload)
	r.POST("/api/v1/cc/fs/prompts/tts", srv.FSPromptTTS)
	r.GET("/api/v1/cc/fs/prompts/:file", srv.FSPromptFile)
	r.DELETE("/api/v1/cc/fs/prompts/:file", srv.FSPromptDelete)
	// 黑名单
	r.GET("/api/v1/cc/fs/blacklist", srv.FSBlacklist)
	r.POST("/api/v1/cc/fs/blacklist", srv.FSBlacklistAdd)
	r.DELETE("/api/v1/cc/fs/blacklist/:number", srv.FSBlacklistRemove)
	// 计费管理（billing.go）：费率 / 账户余额 / 话费流水 / 资金流水
	r.GET("/api/v1/cc/billing/rates", srv.ListRates)
	r.POST("/api/v1/cc/billing/rates", srv.UpsertRate)
	r.DELETE("/api/v1/cc/billing/rates/:id", srv.DeleteRate)
	r.GET("/api/v1/cc/billing/account", srv.GetAccount)
	r.POST("/api/v1/cc/billing/recharge", srv.Recharge)
	r.GET("/api/v1/cc/billing/charges", srv.ListCharges)
	r.GET("/api/v1/cc/billing/txns", srv.ListAccountTxns)
	// 月账单（billing_admin.go）：平台看全部、租户看自己；导出 CSV 明细
	r.GET("/api/v1/cc/billing/bills", srv.ListBills)
	r.GET("/api/v1/cc/billing/bills/:id/export", srv.ExportBillCSV)
	// 平台侧账务（billing_admin.go，仅 tenant=1）：总览 / 代充值 / 冻结 / 出账
	r.GET("/api/v1/cc/billing/admin/overview", srv.AdminAccountsOverview)
	r.POST("/api/v1/cc/billing/admin/recharge", srv.AdminRecharge)
	r.POST("/api/v1/cc/billing/admin/status", srv.AdminSetStatus)
	r.GET("/api/v1/cc/billing/admin/txns", srv.AdminListTxns)
	r.POST("/api/v1/cc/billing/admin/bills/generate", srv.AdminGenerateBills)
	// 录音转写质检（qc.go）：转写入队（202 + job id）、任务查询、单查、结果列表、
	// 关键词库 CRUD。转写实际执行在后台 worker（qc_worker.go）。
	r.POST("/api/v1/cc/qc/transcribe", srv.QcTranscribe)
	r.GET("/api/v1/cc/qc/transcribe/jobs", srv.QcTranscribeJobByCall)
	r.GET("/api/v1/cc/qc/transcribe/jobs/:id", srv.QcTranscribeJob)
	r.GET("/api/v1/cc/qc/transcripts", srv.QcTranscript)
	r.GET("/api/v1/cc/qc/results", srv.QcResults)
	r.GET("/api/v1/cc/qc/keywords", srv.QcKeywords)
	r.POST("/api/v1/cc/qc/keywords", srv.QcCreateKeyword)
	r.DELETE("/api/v1/cc/qc/keywords/:id", srv.QcDeleteKeyword)
	// 外呼风控（risk.go）：租户策略 + 黑白名单；originate 路径强制执行
	r.GET("/api/v1/cc/risk/policy", srv.GetRiskPolicy)
	r.PUT("/api/v1/cc/risk/policy", srv.SaveRiskPolicy)
	r.GET("/api/v1/cc/risk/lists", srv.ListRiskLists)
	r.POST("/api/v1/cc/risk/lists", srv.CreateRiskList)
	r.DELETE("/api/v1/cc/risk/lists/:id", srv.DeleteRiskList)
	r.GET("/api/v1/cc/voice", srv.ListVoiceNotifies)
	r.POST("/api/v1/cc/voice", srv.CreateVoiceNotify)

	// 后台循环：余额预警/欠费自动冻结（每小时）+ 日常维护（webhook 日志清理、
	// 每月 1 号自动出账，每日）。notify 未配置时通知静默跳过，扫描与清理照常。
	loopCtx, loopCancel := context.WithCancel(context.Background())
	notify := notifyclient.New(cfg.NotifyAPIBase, cfg.NotifyInternalToken)
	go srv.RunBalanceWarnLoop(loopCtx, notify, api.BalanceWarnOptions{
		ThresholdFen: cfg.BalanceWarnFen,
		WarnUserID:   cfg.BalanceWarnUserID,
		AutoFreeze:   cfg.AutoFreeze,
	})
	go srv.RunHousekeepingLoop(loopCtx, notify, cfg.BalanceWarnUserID, cfg.WebhookLogKeepDays)
	// 录音转写异步消费（同步转写会把整段 wav 读进内存，两个并发即打爆 256M 限额）。
	// 并发上限默认 1：转写是 IO 等待型但内存与 ASR 配额都要收着走。
	go srv.RunTranscribeLoop(loopCtx, api.TranscribeWorkerOptions{
		Concurrency: cfg.TranscribeConcurrency,
	})
	go srv.RunVoiceNotifyLoop(loopCtx)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("cc-service listening on :%s", cfg.AppPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sh := graceful.New(graceful.WithTimeout(10 * time.Second))
	sh.Register("workers", func(ctx context.Context) error {
		loopCancel()
		return nil
	})
	sh.Register("http", func(ctx context.Context) error {
		return httpSrv.Shutdown(ctx)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("cc graceful shutdown: %v", err)
	}
}
