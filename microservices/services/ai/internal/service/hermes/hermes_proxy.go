package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	aichat "github.com/go-admin-kit/services/ai/internal/service/ai"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

var (
	ErrHermesProxyUnauthorized = errors.New("hermes model proxy unauthorized")
	ErrHermesProxyForbidden    = errors.New("hermes model proxy forbidden")
)

// hermesModelTokenTTL 是短期 token 的有效期。取值权衡：太短则 manager 频繁
// 换发、Hermes 长会话中途失效；太长则「停用」到实际失效的窗口变大。
// 30 分钟配合每次请求的实时权益校验——即使 token 未过期，权益一撤销下一个
// 请求就会被拒。
const hermesModelTokenTTL = 30 * time.Minute

// HermesProxyService 是企业模型代理。
//
// 数据路径（已确认的产品决定）：员工 prompt 穿过 ai-service 转发给模型后端，
// 只记录模型/token 数/耗时/状态/费用等元数据，**不落 prompt 与 response 正文**。
// 供应商 API key 只存在于服务端，员工电脑永远拿不到。
type HermesProxyService struct {
	hermes *HermesService
	tokens *dao.HermesModelTokenDAO
	usage  *AIUsageService
	// providers 是全局兜底通道：策略未指定 model_id 时使用（等价于控制台自身的
	// AI 配置）。策略指定了模型就走 models 解析出的专属通道。
	providers aiclient.Providers
	models    *AIModelService
	quota     *HermesQuotaGuard
	// documents 是知识库检索器（M4 RAG）：策略 enable_knowledge_rag 为真时，
	// 转发前对最后一条 user 消息检索企业知识库，把相关块折进 system message。
	// *aidao.DocumentDAO 实现该接口。
	documents aichat.ChunkSearcher
	now       func() time.Time
}

func NewHermesProxyService(
	hermes *HermesService,
	tokens *dao.HermesModelTokenDAO,
	usage *AIUsageService,
	providers aiclient.Providers,
	models *AIModelService,
	quota *HermesQuotaGuard,
	documents aichat.ChunkSearcher,
	now func() time.Time,
) *HermesProxyService {
	if now == nil {
		now = time.Now
	}
	return &HermesProxyService{
		hermes:    hermes,
		tokens:    tokens,
		usage:     usage,
		providers: providers,
		models:    models,
		quota:     quota,
		documents: documents,
		now:       now,
	}
}

// ---------------------------------------------------------------------------
// token 签发与吊销
// ---------------------------------------------------------------------------

type HermesModelTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	BaseURL   string    `json:"base_url"`
}

func hashHermesModelToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IssueModelToken 由已通过设备签名鉴权的 manager 调用。
// 签发前校验权益与设备状态，因此被停用的员工换不到新 token。
func (s *HermesProxyService) IssueModelToken(ctx context.Context, device *localmodel.HermesDevice) (*HermesModelTokenResponse, error) {
	if device == nil {
		return nil, ErrHermesProxyUnauthorized
	}
	if device.RevokedAt != nil {
		return nil, fmt.Errorf("%w: device revoked", ErrHermesProxyForbidden)
	}
	if err := s.assertEntitledOnly(ctx, device); err != nil {
		return nil, err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	token := "hmt_" + base64.RawURLEncoding.EncodeToString(raw)
	now := s.now().UTC()
	// 一设备一活跃 token：换发即吊销旧的。否则 manager 每 30 分钟例行换发一次，
	// 旧 token 在各自 TTL 内全部同时有效——token 被窃后连"换发止损"都做不到。
	if err := s.tokens.RevokeByDevice(ctx, device.DeviceID, now); err != nil {
		return nil, err
	}
	record := &localmodel.HermesModelToken{
		TenantID:  device.TenantID,
		TokenHash: hashHermesModelToken(token),
		DeviceID:  device.DeviceID,
		UserID:    device.UserID,
		ExpiresAt: now.Add(hermesModelTokenTTL),
		CreatedAt: now,
	}
	if err := s.tokens.Create(ctx, record); err != nil {
		return nil, err
	}
	return &HermesModelTokenResponse{Token: token, ExpiresAt: record.ExpiresAt}, nil
}

// RevokeDeviceTokens 在设备吊销时调用，使模型访问立即失效。
func (s *HermesProxyService) RevokeDeviceTokens(ctx context.Context, deviceID string) error {
	return s.tokens.RevokeByDevice(ctx, deviceID, s.now().UTC())
}

// RevokeUserTokens 在员工权益停用时调用。
func (s *HermesProxyService) RevokeUserTokens(ctx context.Context, userID uint) error {
	return s.tokens.RevokeByUser(ctx, userID, s.now().UTC())
}

// ---------------------------------------------------------------------------
// 请求鉴权
// ---------------------------------------------------------------------------

// HermesProxyCaller 是一次代理调用的主体身份。
type HermesProxyCaller struct {
	TenantID uint
	UserID   uint
	DeviceID string
	TokenID  uint64
	// 生效策略。模型路由与用量归属都以它为准，而不是信任请求体里的 model 字段。
	Policy *localmodel.HermesPolicy
}

// Authorize 解析 bearer token 并**实时**校验此刻是否仍有权访问。
//
// 关键：token 未过期不代表仍然有效。员工停用、设备吊销、策略切到 stopped/revoked
// 都必须立刻拒绝——这正是文档所说「服务端立即强制」的落点，也是本地策略被篡改
// 时仍能兜底的原因。
func (s *HermesProxyService) Authorize(ctx context.Context, bearer string) (*HermesProxyCaller, context.Context, error) {
	bearer = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(bearer), "Bearer "))
	if bearer == "" {
		return nil, ctx, ErrHermesProxyUnauthorized
	}
	now := s.now().UTC()
	record, err := s.tokens.ResolveActive(ctx, hashHermesModelToken(bearer), now)
	if errors.Is(err, dao.ErrHermesModelTokenInvalid) || errors.Is(err, dao.ErrHermesModelTokenExpired) {
		return nil, ctx, ErrHermesProxyUnauthorized
	}
	if err != nil {
		return nil, ctx, err
	}

	// 后续查询都在该 token 所属租户下进行
	scoped := tenant.WithContext(ctx, record.TenantID)

	device, err := s.hermes.dao.GetDevice(scoped, record.DeviceID)
	if err != nil {
		return nil, ctx, ErrHermesProxyUnauthorized
	}
	if device.RevokedAt != nil {
		return nil, ctx, fmt.Errorf("%w: device revoked", ErrHermesProxyForbidden)
	}
	policy, err := s.assertEntitled(scoped, device)
	if err != nil {
		return nil, ctx, err
	}

	s.tokens.TouchLastUsed(scoped, record.ID, now)
	return &HermesProxyCaller{
		TenantID: record.TenantID,
		UserID:   record.UserID,
		DeviceID: record.DeviceID,
		TokenID:  record.ID,
		Policy:   policy,
	}, scoped, nil
}

// assertEntitled 校验员工权益开启，且生效策略的期望状态不是 stopped/revoked，
// 并把生效策略返回给调用方做模型路由。
func (s *HermesProxyService) assertEntitled(ctx context.Context, device *localmodel.HermesDevice) (*localmodel.HermesPolicy, error) {
	if device == nil {
		return nil, ErrHermesProxyUnauthorized
	}
	policy, enabled, err := s.hermes.resolveDevicePolicy(ctx, device)
	if err != nil {
		if errors.Is(err, ErrHermesPolicyNotConfigured) {
			return nil, fmt.Errorf("%w: no Hermes policy", ErrHermesProxyForbidden)
		}
		return nil, err
	}
	if !enabled {
		return nil, fmt.Errorf("%w: entitlement or instance disabled", ErrHermesProxyForbidden)
	}
	if policy.DesiredState != localmodel.HermesDesiredStateActive {
		return nil, fmt.Errorf("%w: policy desired state is %s", ErrHermesProxyForbidden, policy.DesiredState)
	}
	return policy, nil
}

// ---------------------------------------------------------------------------
// 转发
// ---------------------------------------------------------------------------

type HermesProxyMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type HermesProxyRequest struct {
	Model    string               `json:"model"`
	Messages []HermesProxyMessage `json:"messages"`
	Stream   bool                 `json:"stream"`
}

// assertEntitledOnly 是 assertEntitled 的丢弃返回值版本，用于只关心是否放行的场合。
func (s *HermesProxyService) assertEntitledOnly(ctx context.Context, device *localmodel.HermesDevice) error {
	_, err := s.assertEntitled(ctx, device)
	return err
}

// resolveProvider 按生效策略解析该员工应当使用的模型通道。
//
// 这是「模型授权」的落点：员工用哪个模型由策略决定，不是由请求体里的 model
// 字段决定——否则本机改一行配置就能越权用别的模型。策略未指定 model_id 时
// 回落到全局通道（等价于控制台自身的 AI 配置）。
func (s *HermesProxyService) resolveProvider(ctx context.Context, policy *localmodel.HermesPolicy) (aiclient.Provider, string, error) {
	if policy == nil || policy.ModelID == 0 || s.models == nil {
		if s.providers.Chat == nil {
			return nil, "", fmt.Errorf("%w: no model channel configured", ErrHermesProxyForbidden)
		}
		name := ""
		if policy != nil {
			name = policy.ModelName
		}
		return s.providers.Chat, name, nil
	}

	// GetModel 会解密 API key；密钥只在服务端内存里存在，不落日志也不回给设备
	aiModel, err := s.models.GetModel(ctx, policy.ModelID)
	if err != nil {
		return nil, "", fmt.Errorf("%w: policy model unavailable", ErrHermesProxyForbidden)
	}
	if aiModel.Status != localmodel.AIModelStatusEnabled {
		return nil, "", fmt.Errorf("%w: policy model is disabled", ErrHermesProxyForbidden)
	}
	if strings.TrimSpace(aiModel.APIKey) == "" {
		return nil, "", fmt.Errorf("%w: policy model has no credential", ErrHermesProxyForbidden)
	}

	chatModel := policy.ModelName
	if chatModel == "" {
		chatModel = aiModel.Name
	}
	built := aiclient.NewProviders(aiclient.Options{
		Provider:  aiModel.Provider,
		BaseURL:   aiModel.BaseURL,
		APIKey:    aiModel.APIKey,
		ChatModel: chatModel,
	})
	return built.Chat, chatModel, nil
}

// Precheck 做转发前的配额与模型可用性校验，不产生任何上游调用。
//
// 存在的理由：流式响应一旦写出 SSE 头就只能返回 200，客户端再也拿不到
// 429/403。handler 用它在开流前把这类错误判掉。
func (s *HermesProxyService) Precheck(ctx context.Context, caller *HermesProxyCaller) error {
	if caller == nil {
		return ErrHermesProxyUnauthorized
	}
	if s.quota != nil {
		if err := s.quota.Check(ctx, caller.UserID, caller.TenantID); err != nil {
			return err
		}
	}
	_, _, err := s.resolveProvider(ctx, caller.Policy)
	return err
}

// proxyKnowledgePreamble 是 Hermes 模型代理的 RAG persona（M4）。与 console
// /ai/chat 的 persona 区分：代理是员工侧透明通道，员工感知不到开关存在。
const proxyKnowledgePreamble = "你是企业员工的 AI 助手。请优先根据下面的企业知识库内容回答员工问题；如果知识库内容与问题无关，再依据你自己的知识回答，并说明该回答不来自知识库。"

// lastUserMessage 返回消息序列中最后一条 user 消息的内容，用于 RAG 检索。
func lastUserMessage(messages []HermesProxyMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			return messages[i].Content
		}
	}
	return ""
}

// injectKnowledgeRAG 在策略 enable_knowledge_rag 为真时检索企业知识库并返回
// system message 注入串；关闭或检索无命中时返回空串。embed/检索失败降级为
// 空串并记日志——RAG 是增强而非必需，不能因此打断员工的对话。
func (s *HermesProxyService) injectKnowledgeRAG(ctx context.Context, policy *localmodel.HermesPolicy, messages []HermesProxyMessage) string {
	if policy == nil || !policy.EnableKnowledgeRAG {
		return ""
	}
	query := lastUserMessage(messages)
	if query == "" {
		return ""
	}
	knowledge, err := aichat.BuildKnowledgePrompt(ctx, s.providers.Embed, s.documents, query, proxyKnowledgePreamble)
	if err != nil {
		if logger.Logger != nil {
			logger.Warn("hermes proxy knowledge RAG degrade", logger.Err(err), logger.Uint("tenant_id", policy.TenantID))
		}
		return ""
	}
	return knowledge
}

// StreamChat 把请求转发给模型后端，逐帧回调 onDelta。
//
// 不记录 prompt 与 response 正文：onDelta 里的内容直接流回调用方，本方法只在
// 结束时写一条用量元数据。
func (s *HermesProxyService) StreamChat(
	ctx context.Context,
	caller *HermesProxyCaller,
	req HermesProxyRequest,
	onDelta func(aiclient.ChatDelta) error,
) error {
	if len(req.Messages) == 0 {
		return fmt.Errorf("%w: messages is required", ErrHermesInvalidInput)
	}

	// 配额在转发前校验：超限就不该产生真实调用与费用
	if s.quota != nil {
		if err := s.quota.Check(ctx, caller.UserID, caller.TenantID); err != nil {
			return err
		}
	}

	provider, modelName, err := s.resolveProvider(ctx, caller.Policy)
	if err != nil {
		return err
	}

	msgs := make([]aiclient.ChatMessage, 0, len(req.Messages)+1)
	// M4 企业知识库 RAG：服务端开关，检索结果折成 system message 后置前，
	// 员工端对开关无感知。
	if knowledge := s.injectKnowledgeRAG(ctx, caller.Policy, req.Messages); knowledge != "" {
		msgs = append(msgs, aiclient.ChatMessage{Role: aiclient.RoleSystem, Content: knowledge})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, aiclient.ChatMessage{Role: m.Role, Content: m.Content})
	}

	started := s.now()
	var usage *aiclient.Usage
	callErr := provider.Chat(ctx, msgs, func(delta aiclient.ChatDelta) error {
		if delta.Done && delta.Usage != nil {
			usage = delta.Usage
		}
		return onDelta(delta)
	})

	// 用量按策略解析出的模型名归属，而不是请求体里的 model——后者是设备可控的
	s.recordUsage(ctx, caller, modelName, usage, time.Since(started), callErr)
	return callErr
}

// recordUsage 只写元数据。ErrorMsg 仅在失败时记错误摘要，正文永不入库。
func (s *HermesProxyService) recordUsage(
	ctx context.Context,
	caller *HermesProxyCaller,
	modelName string,
	usage *aiclient.Usage,
	elapsed time.Duration,
	callErr error,
) {
	if s.usage == nil || caller == nil {
		return
	}
	userID := uint64(caller.UserID)
	tenantID := uint64(caller.TenantID)
	latency := int(elapsed.Milliseconds())
	status := "success"
	errMsg := ""
	if callErr != nil {
		status = "error"
		// 只保留错误摘要，且截断——上游错误里可能回显请求片段
		errMsg = truncateProxyError(callErr.Error())
	}
	entry := &localmodel.AIUsageLog{
		ModelName: modelName,
		UserID:    &userID,
		TenantID:  &tenantID,
		DeviceID:  caller.DeviceID,
		Operation: "hermes_proxy_chat",
		LatencyMs: &latency,
		Status:    status,
		ErrorMsg:  errMsg,
		CreatedAt: s.now().UTC(),
	}
	if usage != nil {
		entry.PromptTokens = &usage.PromptTokens
		entry.CompletionTokens = &usage.CompletionTokens
		entry.TotalTokens = &usage.TotalTokens
		// M4 后补费用归账：优先按策略引用的模型 ID 取价（规避 ModelName 与
		// ai_models.name 不一致查不到），否则按解析出的模型名回退。查不到就 0。
		if s.models != nil {
			if pricing := s.resolveUsagePricing(ctx, caller, modelName); pricing != nil {
				entry.ModelID = pricing.ID
				entry.ModelName = pricing.Name
				entry.Cost = pricing.CostForUsage(usage)
			}
		}
	}
	// 用量落库失败不该影响已经成功的模型调用
	_ = s.usage.RecordUsage(ctx, entry)
}

// resolveUsagePricing 解析本次调用应归账的模型单价。优先 caller.Policy.ModelID
// （精确到 ai_models 行），回退到按 modelName 查（覆盖策略未指定模型的全局通道）。
func (s *HermesProxyService) resolveUsagePricing(ctx context.Context, caller *HermesProxyCaller, modelName string) *AIModelUsagePricing {
	if caller != nil && caller.Policy != nil && caller.Policy.ModelID != 0 {
		if pricing, err := s.models.GetUsagePricingByID(ctx, caller.Policy.ModelID); err == nil {
			return pricing
		}
	}
	if modelName != "" {
		if pricing, err := s.models.GetUsagePricingByName(ctx, modelName); err == nil {
			return pricing
		}
	}
	return nil
}

func truncateProxyError(msg string) string {
	const maxLen = 200
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen]
}
