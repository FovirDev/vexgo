package handler

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// RegisterAPIRoutes registers the legacy handler routes on the /api group.
// During the incremental migration to internal/ domain packages, this function
// shrinks as each domain moves to its own RegisterRoutes.
func RegisterAPIRoutes(api *gin.RouterGroup) {
	logrus.Debug("Registering legacy API routes")
	{
		// -------------------- Public API (no JWT authentication required) --------------------
		logrus.Debug("Registering public API routes")
		api.GET("/stats", GetStats)

		api.GET("/themes", GetThemes)
		api.GET("/theme/:id/preview", GetThemePreview)

		// -------------------- Business API requiring JWT authentication --------------------
		api.GET("/config/smtp", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), GetSMTPConfig)
		api.PUT("/config/smtp", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), UpdateSMTPConfig)
		api.POST("/config/smtp/test", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), TestSMTP)

		api.GET("/config/ai", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), GetAIConfig)
		api.PUT("/config/ai", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), UpdateAIConfig)
		api.POST("/config/ai/test", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), TestAI)
		api.GET("/config/ai/models", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), GetAIModels)

		api.GET("/config/general", GetGeneralSettings)
		api.PUT("/config/general", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), UpdateGeneralSettings)

		api.GET("/config/theme", GetThemeConfig)
		api.PUT("/config/theme", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), UpdateThemeConfig)

		// Theme upload endpoint
		api.POST("/themes/upload", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), UploadTheme)
	}
}
