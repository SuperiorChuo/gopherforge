// Package api wires the AI service HTTP surface. All AI endpoints live under
// /api/v1/ai and require an authenticated console user; no permission codes
// are enforced beyond authentication.
package api

import (
	"github.com/gin-gonic/gin"

	aihandler "github.com/go-admin-kit/services/ai/internal/api/ai"
	"github.com/go-admin-kit/services/ai/internal/api/common"
	sharedapi "github.com/go-admin-kit/services/ai/internal/api/shared"
	"github.com/go-admin-kit/services/ai/internal/middleware"
)

// SetupRoutesWithDeps mounts the API with injected infrastructure handles
// and the AI endpoint handler assembled at the composition root.
func SetupRoutesWithDeps(router *gin.Engine, deps sharedapi.Dependencies, handler *aihandler.Handler) {
	api := router.Group("/api/v1")

	common.RegisterPublicRoutesWithDeps(api, deps)

	protected := api.Group("/ai")
	protected.Use(middleware.AuthMiddleware(), middleware.OperationLogger())
	{
		protected.GET("/status", handler.GetStatus)

		protected.POST("/chat", handler.Chat)
		protected.GET("/conversations", handler.ListConversations)
		protected.GET("/conversations/:id/messages", handler.ListConversationMessages)
		protected.DELETE("/conversations/:id", handler.DeleteConversation)

		protected.POST("/kb/documents", handler.CreateDocument)
		protected.GET("/kb/documents", handler.ListDocuments)
		protected.DELETE("/kb/documents/:id", handler.DeleteDocument)
		protected.POST("/kb/search", handler.SearchDocuments)

		protected.POST("/logs/insight", handler.GenerateLogInsight)
		protected.POST("/compose", handler.Compose)
	}
}
