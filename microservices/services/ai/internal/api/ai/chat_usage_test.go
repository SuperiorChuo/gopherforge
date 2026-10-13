package ai

import (
	"context"
	"fmt"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

func TestRecordUsageAttributesModelCalculatesCostAndOutlivesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:chat-usage-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&localmodel.AIModel{}, &localmodel.AIUsageLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	configuredModel := &localmodel.AIModel{
		Name: "priced-chat", DisplayName: "Priced Chat", Provider: "openai",
		Status: localmodel.AIModelStatusEnabled, CostPer1KInput: 0.01, CostPer1KOutput: 0.03,
	}
	if err := db.Create(configuredModel).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	handler := NewHandler(func(context.Context) StatusInfo {
		return StatusInfo{Configured: true, ChatModel: configuredModel.Name}
	}, aisvc.ChatService{}, aisvc.KnowledgeService{}, aisvc.InsightService{})
	handler.SetModelService(service.NewAIModelService(dao.NewAIModelDAO(db), nil))
	handler.SetUsageService(service.NewAIUsageService(dao.NewAIUsageLogDAO(db)))

	requestCtx, cancelRequest := context.WithCancel(tenant.WithContext(context.Background(), 7))
	cancelRequest()
	req := httptest.NewRequest("POST", "/api/v1/ai/chat", nil).WithContext(requestCtx)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = req
	ginCtx.Set("username", "alice")

	handler.recordUsage(ginCtx, 42, &aiclient.Usage{
		PromptTokens: 1500, CompletionTokens: 500, TotalTokens: 2000,
	}, nil)

	var usageLog localmodel.AIUsageLog
	deadline := time.Now().Add(2 * time.Second)
	for {
		queryErr := db.First(&usageLog).Error
		if queryErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("usage log was not persisted after request cancellation: %v", queryErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if usageLog.ModelID != configuredModel.ID || usageLog.ModelName != configuredModel.Name {
		t.Fatalf("model attribution = (%d, %q), want (%d, %q)", usageLog.ModelID, usageLog.ModelName, configuredModel.ID, configuredModel.Name)
	}
	if usageLog.TenantID == nil || *usageLog.TenantID != 7 {
		t.Fatalf("tenant attribution = %v, want 7", usageLog.TenantID)
	}
	if math.Abs(usageLog.Cost-0.03) > 0.000001 {
		t.Fatalf("usage cost = %.6f, want 0.030000", usageLog.Cost)
	}
}
