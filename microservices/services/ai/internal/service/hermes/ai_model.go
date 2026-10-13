package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg"
	"gorm.io/gorm"
)

// AIModelService 模型配置管理服务
type AIModelService struct {
	dao       *dao.AIModelDAO
	encryptor *pkg.Encryptor
}

// AIModelUsagePricing is the non-secret model metadata needed for usage
// attribution and cost calculation.
type AIModelUsagePricing struct {
	ID              uint64
	Name            string
	CostPer1KInput  float64
	CostPer1KOutput float64
}

// CostForUsage 按 token 消耗折算费用：prompt/1000*输入单价 + completion/1000*输出单价。
// 与 console 用量（api/ai/chat.go calculateUsageCost）同一公式，这里下沉到 service
// 供 console 与 Hermes proxy 共用。
func (p *AIModelUsagePricing) CostForUsage(usage *aiclient.Usage) float64 {
	if p == nil || usage == nil {
		return 0
	}
	return float64(usage.PromptTokens)/1000*p.CostPer1KInput +
		float64(usage.CompletionTokens)/1000*p.CostPer1KOutput
}

func NewAIModelService(dao *dao.AIModelDAO, encryptor *pkg.Encryptor) *AIModelService {
	return &AIModelService{
		dao:       dao,
		encryptor: encryptor,
	}
}

// ListModels 分页查询模型列表
func (s *AIModelService) ListModels(ctx context.Context, page, pageSize int, provider string, status *int) ([]*localmodel.AIModel, int64, error) {
	models, total, err := s.dao.List(ctx, page, pageSize, provider, status)
	if err != nil {
		return nil, 0, err
	}

	// 隐藏 API Key（前端只显示是否已配置）
	for _, m := range models {
		if m.APIKey != "" {
			m.APIKey = "********" // 脱敏
		}
	}
	return models, total, nil
}

// GetModel 根据ID查询模型（包含解密的API Key，仅内部使用）
func (s *AIModelService) GetModel(ctx context.Context, id uint64) (*localmodel.AIModel, error) {
	m, err := s.dao.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 解密 API Key
	if m.APIKey != "" && m.APIKeyIV != "" {
		decrypted, err := s.encryptor.Decrypt(m.APIKey, m.APIKeyIV)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key: %w", err)
		}
		m.APIKey = decrypted
	}

	return m, nil
}

// GetModelForDisplay 根据ID查询模型（脱敏，前端显示用）
func (s *AIModelService) GetModelForDisplay(ctx context.Context, id uint64) (*localmodel.AIModel, error) {
	m, err := s.dao.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 脱敏
	if m.APIKey != "" {
		m.APIKey = "********"
	}
	m.APIKeyIV = ""

	return m, nil
}

// GetModelByName 根据名称查询模型（内部使用，包含解密）
func (s *AIModelService) GetModelByName(ctx context.Context, name string) (*localmodel.AIModel, error) {
	m, err := s.dao.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	// 解密 API Key
	if m.APIKey != "" && m.APIKeyIV != "" {
		decrypted, err := s.encryptor.Decrypt(m.APIKey, m.APIKeyIV)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key: %w", err)
		}
		m.APIKey = decrypted
	}

	return m, nil
}

// GetUsagePricingByName resolves usage metadata without decrypting or exposing
// the model API key.
func (s *AIModelService) GetUsagePricingByName(ctx context.Context, name string) (*AIModelUsagePricing, error) {
	m, err := s.dao.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return &AIModelUsagePricing{
		ID:              m.ID,
		Name:            m.Name,
		CostPer1KInput:  m.CostPer1KInput,
		CostPer1KOutput: m.CostPer1KOutput,
	}, nil
}

// GetUsagePricingByID 按模型 ID 取价，供 Hermes proxy 按策略引用的模型归账——
// 规避策略 ModelName 与 ai_models.name 不一致导致按名查不到的问题。
func (s *AIModelService) GetUsagePricingByID(ctx context.Context, id uint64) (*AIModelUsagePricing, error) {
	m, err := s.dao.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &AIModelUsagePricing{
		ID:              m.ID,
		Name:            m.Name,
		CostPer1KInput:  m.CostPer1KInput,
		CostPer1KOutput: m.CostPer1KOutput,
	}, nil
}

// CreateModel 创建模型
func (s *AIModelService) CreateModel(ctx context.Context, m *localmodel.AIModel) error {
	// 检查名称唯一性
	existing, err := s.dao.GetByName(ctx, m.Name)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && existing != nil {
		return fmt.Errorf("模型名称 %s 已存在", m.Name)
	}

	// 加密 API Key
	if m.APIKey != "" {
		encrypted, iv, err := s.encryptor.Encrypt(m.APIKey)
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
		m.APIKey = encrypted
		m.APIKeyIV = iv
	}

	return s.dao.Create(ctx, m)
}

// UpdateModel 更新模型
func (s *AIModelService) UpdateModel(ctx context.Context, id uint64, updates *localmodel.AIModel) error {
	existing, err := s.dao.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 检查名称唯一性（如果修改了名称）
	if updates.Name != "" && updates.Name != existing.Name {
		dup, err := s.dao.GetByName(ctx, updates.Name)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && dup != nil {
			return fmt.Errorf("模型名称 %s 已存在", updates.Name)
		}
		existing.Name = updates.Name
	}

	// 更新字段
	if updates.DisplayName != "" {
		existing.DisplayName = updates.DisplayName
	}
	if updates.Provider != "" {
		existing.Provider = updates.Provider
	}
	if updates.BaseURL != "" {
		existing.BaseURL = updates.BaseURL
	}
	if updates.ModelConfig != nil {
		existing.ModelConfig = updates.ModelConfig
	}
	if updates.Capabilities != nil {
		existing.Capabilities = updates.Capabilities
	}
	if updates.Status != 0 {
		existing.Status = updates.Status
	}
	if updates.CostPer1KInput != 0 {
		existing.CostPer1KInput = updates.CostPer1KInput
	}
	if updates.CostPer1KOutput != 0 {
		existing.CostPer1KOutput = updates.CostPer1KOutput
	}
	if updates.CostPerImage != 0 {
		existing.CostPerImage = updates.CostPerImage
	}
	if updates.DailyLimit != nil {
		existing.DailyLimit = updates.DailyLimit
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if updates.Sort != 0 {
		existing.Sort = updates.Sort
	}

	// 处理 API Key 更新（如果传了新的明文密钥）
	if updates.APIKey != "" && updates.APIKey != "********" {
		encrypted, iv, err := s.encryptor.Encrypt(updates.APIKey)
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
		existing.APIKey = encrypted
		existing.APIKeyIV = iv
	}

	return s.dao.Update(ctx, existing)
}

// DeleteModel 删除模型
func (s *AIModelService) DeleteModel(ctx context.Context, id uint64) error {
	return s.dao.Delete(ctx, id)
}

// ListEnabledModels 查询所有启用的模型（用于下拉选择）
func (s *AIModelService) ListEnabledModels(ctx context.Context) ([]*localmodel.AIModel, error) {
	models, err := s.dao.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}

	// 脱敏
	for _, m := range models {
		if m.APIKey != "" {
			m.APIKey = "********"
		}
		m.APIKeyIV = ""
	}
	return models, nil
}

// AIUsageService 调用记录与统计服务
type AIUsageService struct {
	dao *dao.AIUsageLogDAO
}

func NewAIUsageService(dao *dao.AIUsageLogDAO) *AIUsageService {
	return &AIUsageService{dao: dao}
}

// RecordUsage 记录一次AI调用
func (s *AIUsageService) RecordUsage(ctx context.Context, log *localmodel.AIUsageLog) error {
	return s.dao.Create(ctx, log)
}

// ListUsageLogs 分页查询调用记录
func (s *AIUsageService) ListUsageLogs(ctx context.Context, page, pageSize int, modelID, userID *uint64, status string, startTime, endTime *time.Time) ([]*localmodel.AIUsageLog, int64, error) {
	return s.dao.List(ctx, page, pageSize, modelID, userID, status, startTime, endTime)
}

// GetDailyStats 获取每日统计
func (s *AIUsageService) GetDailyStats(ctx context.Context, days int, userID, tenantID *uint64) ([]dao.DailyStats, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)
	return s.dao.GetDailyStats(ctx, startDate, endDate, userID, tenantID)
}

// GetTopUsers 获取Top用户排行
func (s *AIUsageService) GetTopUsers(ctx context.Context, days int, limit int) ([]dao.UserStats, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -days)
	return s.dao.GetTopUsers(ctx, startTime, endTime, limit)
}

// GetUsageSummary 获取用量汇总
func (s *AIUsageService) GetUsageSummary(ctx context.Context, startTime, endTime time.Time, userID, tenantID *uint64) (*dao.UsageSummary, error) {
	return s.dao.GetUsageSummary(ctx, startTime, endTime, userID, tenantID)
}

// AIQuotaService 配额管理服务
type AIQuotaService struct {
	dao *dao.AIQuotaDAO
}

func NewAIQuotaService(dao *dao.AIQuotaDAO) *AIQuotaService {
	return &AIQuotaService{dao: dao}
}

// GetQuota 获取配额（按优先级：user > tenant > global）
func (s *AIQuotaService) GetQuota(ctx context.Context, userID, tenantID *uint64) (*localmodel.AIQuota, error) {
	// 1. 尝试获取用户配额
	if userID != nil {
		quota, err := s.dao.GetByScope(ctx, localmodel.AIQuotaScopeUser, userID)
		if err == nil && quota.Enabled == 1 {
			return quota, nil
		}
	}

	// 2. 尝试获取租户配额
	if tenantID != nil {
		quota, err := s.dao.GetByScope(ctx, localmodel.AIQuotaScopeTenant, tenantID)
		if err == nil && quota.Enabled == 1 {
			return quota, nil
		}
	}

	// 3. 返回全局配额
	return s.dao.GetByScope(ctx, localmodel.AIQuotaScopeGlobal, nil)
}

// UpsertQuota 创建或更新配额
func (s *AIQuotaService) UpsertQuota(ctx context.Context, quota *localmodel.AIQuota) error {
	return s.dao.Upsert(ctx, quota)
}

// ListQuotas 查询配额列表
func (s *AIQuotaService) ListQuotas(ctx context.Context, scope string) ([]*localmodel.AIQuota, error) {
	return s.dao.List(ctx, scope)
}

// DeleteQuota 删除配额
func (s *AIQuotaService) DeleteQuota(ctx context.Context, id uint64) error {
	return s.dao.Delete(ctx, id)
}
