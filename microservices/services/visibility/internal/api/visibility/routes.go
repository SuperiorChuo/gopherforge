package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
)

func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/v1/health/live", s.HealthLive)
	r.GET("/api/v1/health/ready", s.HealthReady)
	r.GET("/api/v1/visibility/health/live", s.HealthLive)
	r.GET("/api/v1/visibility/health/ready", s.HealthReady)

	r.Use(gatewayauth.RequireReadWritePermissions("visibility:read", "visibility:write"))

	r.GET("/api/v1/visibility/overview", s.Overview)
	// 演示数据播种：会往租户库写演示品牌/关键词/检测问题。生产环境连路由都不挂
	// （404），不靠前端不点——这是个开放的 POST 端点。
	if s.DemoMode {
		r.POST("/api/v1/visibility/seed-demo", s.SeedDemo)
	}

	r.GET("/api/v1/visibility/brands", s.ListBrands)
	r.POST("/api/v1/visibility/brands", s.CreateBrand)
	r.POST("/api/v1/visibility/brands/generate", s.GenerateBrands)
	r.POST("/api/v1/visibility/brands/batch", s.BatchCreateBrands)
	r.POST("/api/v1/visibility/brands/name-analysis", s.BrandNameAnalysis)
	r.PUT("/api/v1/visibility/brands/:id", s.UpdateBrand)
	r.DELETE("/api/v1/visibility/brands/:id", s.DeleteBrand)

	r.GET("/api/v1/visibility/keywords", s.ListKeywords)
	r.POST("/api/v1/visibility/keywords", s.CreateKeyword)
	r.POST("/api/v1/visibility/keywords/generate", s.GenerateKeywords)
	r.POST("/api/v1/visibility/keywords/batch", s.BatchCreateKeywords)
	r.PUT("/api/v1/visibility/keywords/:id", s.UpdateKeyword)
	r.DELETE("/api/v1/visibility/keywords/:id", s.DeleteKeyword)
	r.GET("/api/v1/visibility/keywords/:id/history", s.KeywordHistory)

	r.POST("/api/v1/visibility/serp/checks", s.CreateSerpCheck)
	r.GET("/api/v1/visibility/serp/checks", s.ListSerpTasks)
	r.GET("/api/v1/visibility/serp/checks/:id", s.GetSerpTask)

	r.GET("/api/v1/visibility/geo/prompts", s.ListGeoPrompts)
	r.POST("/api/v1/visibility/geo/prompts", s.CreateGeoPrompt)
	r.POST("/api/v1/visibility/geo/prompts/generate", s.GenerateGeoPrompts)
	r.POST("/api/v1/visibility/geo/prompts/batch", s.BatchCreateGeoPrompts)
	r.PUT("/api/v1/visibility/geo/prompts/:id", s.UpdateGeoPrompt)
	r.DELETE("/api/v1/visibility/geo/prompts/:id", s.DeleteGeoPrompt)

	r.GET("/api/v1/visibility/geo/leaderboard", s.GeoLeaderboard)
	r.GET("/api/v1/visibility/geo/runs", s.ListGeoRuns)
	r.POST("/api/v1/visibility/geo/runs", s.CreateGeoRun)
	r.GET("/api/v1/visibility/geo/runs/:id", s.GetGeoRun)
	r.POST("/api/v1/visibility/geo/runs/:id/advice", s.GenerateGeoAdvice)
	r.PUT("/api/v1/visibility/geo/runs/:id/advice-items", s.PatchGeoAdviceItems)
	r.GET("/api/v1/visibility/geo/runs/:id/compare", s.CompareGeoRun)
}
