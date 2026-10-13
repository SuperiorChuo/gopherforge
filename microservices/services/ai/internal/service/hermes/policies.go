package service

import (
	"context"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"net/url"
	"sort"
	"strings"
)

type CreateHermesPolicyRequest struct {
	// Profile 为空或 'company' → P1 可下发到设备的形态；'company_cluster' →
	// 开放 dispatcher 的控制面概念（本期 manager 未放行，不可应用到设备）。
	// 详见 localmodel.HermesProfileCompanyCluster 与威胁建模文档。
	Profile                   string                    `json:"profile"`
	Name                      string                    `json:"name"`
	DesiredState              string                    `json:"desired_state"`
	AssistantTemplateID       string                    `json:"assistant_template_id"`
	AssistantTemplateRevision uint64                    `json:"assistant_template_revision"`
	ArtifactID                uint64                    `json:"artifact_id"`
	ModelID                   uint64                    `json:"model_id"`
	ModelName                 string                    `json:"model_name"`
	ModelBaseURL              string                    `json:"model_base_url"`
	Toolsets                  localmodel.HermesToolsets `json:"toolsets"`
	Privacy                   localmodel.HermesPrivacy  `json:"privacy"`
	LeaseSeconds              int                       `json:"lease_seconds"`
	// EnableKnowledgeRAG 是 M4 服务端开关：为真时 Hermes 模型代理在转发前检索
	// 企业知识库并注入 system message。不进签名载荷。
	EnableKnowledgeRAG bool `json:"enable_knowledge_rag"`
}

type hermesPolicySignaturePayload struct {
	Name                      string                    `json:"name"`
	DesiredState              string                    `json:"desired_state"`
	Profile                   string                    `json:"profile"`
	AssistantTemplateID       string                    `json:"assistant_template_id"`
	AssistantTemplateRevision uint64                    `json:"assistant_template_revision"`
	ArtifactID                uint64                    `json:"artifact_id"`
	ModelID                   uint64                    `json:"model_id"`
	ModelName                 string                    `json:"model_name"`
	ModelBaseURL              string                    `json:"model_base_url"`
	Toolsets                  localmodel.HermesToolsets `json:"toolsets"`
	Privacy                   localmodel.HermesPrivacy  `json:"privacy"`
	LeaseSeconds              int                       `json:"lease_seconds"`
}

func (s *HermesService) CreatePolicy(ctx context.Context, actorID uint, request CreateHermesPolicyRequest) (*localmodel.HermesPolicy, error) {
	if s.signer == nil {
		return nil, ErrHermesControlPlaneUnavailable
	}
	request.Name = strings.TrimSpace(request.Name)
	request.DesiredState = strings.ToLower(strings.TrimSpace(request.DesiredState))
	request.ModelName = strings.TrimSpace(request.ModelName)
	request.ModelBaseURL = strings.TrimSpace(request.ModelBaseURL)
	request.Profile = strings.ToLower(strings.TrimSpace(request.Profile))
	if request.Profile == "" {
		request.Profile = localmodel.HermesProfileCompany
	}
	if request.Profile != localmodel.HermesProfileCompany && request.Profile != localmodel.HermesProfileCompanyCluster {
		return nil, fmt.Errorf("%w: invalid profile %q", ErrHermesInvalidInput, request.Profile)
	}
	if len(request.Name) == 0 || len(request.Name) > 80 {
		return nil, fmt.Errorf("%w: invalid policy name", ErrHermesInvalidInput)
	}
	if !containsHermesValue([]string{localmodel.HermesDesiredStateActive, localmodel.HermesDesiredStateStopped, localmodel.HermesDesiredStateRevoked}, request.DesiredState) {
		return nil, fmt.Errorf("%w: invalid desired state", ErrHermesInvalidInput)
	}
	if request.LeaseSeconds < 300 || request.LeaseSeconds > 86400 {
		return nil, fmt.Errorf("%w: lease_seconds must be between 300 and 86400", ErrHermesInvalidInput)
	}
	toolsets, err := normalizeHermesToolsets(request.Profile, request.Toolsets)
	if err != nil {
		return nil, err
	}
	if request.Privacy.UploadConversations || request.Privacy.UploadMemory || request.Privacy.UploadLocalFiles {
		return nil, fmt.Errorf("%w: P1 privacy uploads must remain disabled", ErrHermesInvalidInput)
	}
	if request.ModelBaseURL != "" {
		parsed, err := url.Parse(request.ModelBaseURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return nil, fmt.Errorf("%w: invalid model_base_url", ErrHermesInvalidInput)
		}
	}
	if request.ArtifactID > 0 {
		artifact, err := s.dao.GetArtifact(ctx, request.ArtifactID)
		if err != nil {
			return nil, err
		}
		if artifact.Status != localmodel.HermesArtifactApproved || artifact.Kind != "hermes" {
			return nil, fmt.Errorf("%w: Hermes artifact is not approved", ErrHermesInvalidInput)
		}
	}
	// 模型引用要在发布时就校验，否则错误要等设备实际调用时才暴露
	if request.ModelID > 0 && s.modelLookup != nil {
		aiModel, err := s.modelLookup.GetByID(ctx, request.ModelID)
		if err != nil {
			return nil, fmt.Errorf("%w: referenced model not found", ErrHermesInvalidInput)
		}
		if aiModel.Status != localmodel.AIModelStatusEnabled {
			return nil, fmt.Errorf("%w: referenced model is disabled", ErrHermesInvalidInput)
		}
		// 策略里的 model_name 是下发给 Hermes 的模型标识，留空时用模型配置名兜底
		if request.ModelName == "" {
			request.ModelName = aiModel.Name
		}
	}

	payload := hermesPolicySignaturePayload{
		Name:                      request.Name,
		DesiredState:              request.DesiredState,
		Profile:                   request.Profile,
		AssistantTemplateID:       strings.TrimSpace(request.AssistantTemplateID),
		AssistantTemplateRevision: request.AssistantTemplateRevision,
		ArtifactID:                request.ArtifactID,
		ModelID:                   request.ModelID,
		ModelName:                 request.ModelName,
		ModelBaseURL:              request.ModelBaseURL,
		Toolsets:                  toolsets,
		Privacy:                   request.Privacy,
		LeaseSeconds:              request.LeaseSeconds,
	}
	signature, err := s.signer.SignJSON(payload)
	if err != nil {
		return nil, err
	}
	policy := &localmodel.HermesPolicy{
		TenantID:                  tenant.FromContextOrDefault(ctx),
		Name:                      payload.Name,
		DesiredState:              payload.DesiredState,
		Profile:                   payload.Profile, // request.Profile（company / company_cluster）
		AssistantTemplateID:       payload.AssistantTemplateID,
		AssistantTemplateRevision: payload.AssistantTemplateRevision,
		ArtifactID:                payload.ArtifactID,
		ModelID:                   payload.ModelID,
		ModelName:                 payload.ModelName,
		ModelBaseURL:              payload.ModelBaseURL,
		Toolsets:                  payload.Toolsets,
		Privacy:                   payload.Privacy,
		LeaseSeconds:              payload.LeaseSeconds,
		// M4 服务端开关，不进签名载荷（只影响 proxy 行为）
		EnableKnowledgeRAG: request.EnableKnowledgeRAG,
		Signature:          signature,
		CreatedBy:          actorID,
		CreatedAt:          s.now().UTC(),
	}
	if err := s.dao.CreatePolicy(ctx, policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// hermesP1Toolsets 是 P1 默认策略工具集。manager isExactP1Toolset 只接受这 4 个，
// 因此 company 是唯一可下发到设备的 profile。
var hermesP1Toolsets = []string{"clarify", "memory", "no_mcp", "todo"}

// hermesClusterToolsets 是 company_cluster 策略允许的工具集白名单。dispatcher
// 开放要求 delegation；skills/mcp 可选。注意：本期 manager 未放行这些工具，
// company_cluster 策略只能用于控制面创建/指派/追踪，不可应用到真实设备。
var hermesClusterToolsets = []string{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"}

func normalizeHermesToolsets(profile string, toolsets []string) (localmodel.HermesToolsets, error) {
	want := hermesP1Toolsets
	if profile == localmodel.HermesProfileCompanyCluster {
		want = hermesClusterToolsets
	}
	normalized := make([]string, 0, len(toolsets))
	seen := make(map[string]struct{}, len(toolsets))
	for _, value := range toolsets {
		value = strings.ToLower(strings.TrimSpace(value))
		if !containsHermesValue(want, value) {
			return nil, fmt.Errorf("%w: unsupported toolset %q for profile %s", ErrHermesInvalidInput, value, profile)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	if profile == localmodel.HermesProfileCompany {
		if len(normalized) != len(want) {
			return nil, fmt.Errorf("%w: P1 policy requires clarify, memory, no_mcp and todo", ErrHermesInvalidInput)
		}
	} else if !containsHermesValue(normalized, "delegation") {
		return nil, fmt.Errorf("%w: company_cluster policy requires delegation", ErrHermesInvalidInput)
	}
	return localmodel.HermesToolsets(normalized), nil
}

// UpdatePolicy replaces name, desired_state, model fields, toolsets, privacy,
// enable_knowledge_rag and lease_seconds of an existing policy, increments the
// revision, re-signs and persists.
func (s *HermesService) UpdatePolicy(ctx context.Context, actorID uint, policyID uint64, request CreateHermesPolicyRequest) (*localmodel.HermesPolicy, error) {
	if s.signer == nil {
		return nil, ErrHermesControlPlaneUnavailable
	}
	existing, err := s.dao.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	request.Name = strings.TrimSpace(request.Name)
	request.DesiredState = strings.ToLower(strings.TrimSpace(request.DesiredState))
	request.ModelName = strings.TrimSpace(request.ModelName)
	request.ModelBaseURL = strings.TrimSpace(request.ModelBaseURL)
	request.Profile = strings.ToLower(strings.TrimSpace(request.Profile))
	if request.Profile == "" {
		request.Profile = existing.Profile
	}
	if len(request.Name) == 0 || len(request.Name) > 80 {
		return nil, fmt.Errorf("%w: invalid policy name", ErrHermesInvalidInput)
	}
	if !containsHermesValue([]string{localmodel.HermesDesiredStateActive, localmodel.HermesDesiredStateStopped, localmodel.HermesDesiredStateRevoked}, request.DesiredState) {
		return nil, fmt.Errorf("%w: invalid desired state", ErrHermesInvalidInput)
	}
	if request.LeaseSeconds < 300 || request.LeaseSeconds > 86400 {
		return nil, fmt.Errorf("%w: lease_seconds must be between 300 and 86400", ErrHermesInvalidInput)
	}
	toolsets, err := normalizeHermesToolsets(request.Profile, request.Toolsets)
	if err != nil {
		return nil, err
	}
	payload := hermesPolicySignaturePayload{
		Name: request.Name, DesiredState: request.DesiredState, Profile: request.Profile,
		AssistantTemplateID:       existing.AssistantTemplateID,
		AssistantTemplateRevision: existing.AssistantTemplateRevision,
		ArtifactID:                existing.ArtifactID, ModelID: request.ModelID,
		ModelName: request.ModelName, ModelBaseURL: request.ModelBaseURL,
		Toolsets: toolsets, Privacy: request.Privacy, LeaseSeconds: request.LeaseSeconds,
	}
	signature, err := s.signer.SignJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("hermes policy sign: %w", err)
	}
	existing.Name = request.Name
	existing.DesiredState = request.DesiredState
	existing.ModelName = request.ModelName
	existing.ModelBaseURL = request.ModelBaseURL
	existing.ModelID = request.ModelID
	existing.Toolsets = localmodel.HermesToolsets(toolsets)
	existing.Privacy = request.Privacy
	existing.LeaseSeconds = request.LeaseSeconds
	existing.EnableKnowledgeRAG = request.EnableKnowledgeRAG
	existing.Revision++
	existing.Signature = signature
	if err := s.dao.UpdatePolicy(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeletePolicy 软删除一条策略（吊销期望状态 + 置签名失效）。不允许硬删除，需
// 保留修订历史用于设备收敛。
func (s *HermesService) DeletePolicy(ctx context.Context, actorID uint, policyID uint64) error {
	existing, err := s.dao.GetPolicy(ctx, policyID)
	if err != nil {
		return err
	}
	existing.DesiredState = localmodel.HermesDesiredStateRevoked
	existing.Signature = "revoked"
	if err := s.dao.UpdatePolicy(ctx, existing); err != nil {
		return err
	}
	return nil
}

func (s *HermesService) ListPolicies(ctx context.Context, page, pageSize int) ([]localmodel.HermesPolicy, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	return s.dao.ListPolicies(ctx, page, pageSize)
}
