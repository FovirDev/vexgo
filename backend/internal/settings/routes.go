package settings

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the settings domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	admin := middleware.PermissionMiddleware("admin", "super_admin")

	api.GET("/themes", h.GetThemes)
	api.GET("/theme/:id/preview", h.GetThemePreview)

	api.GET("/config/smtp", middleware.JWTAuth(), admin, h.GetSMTPConfig)
	api.PUT("/config/smtp", middleware.JWTAuth(), admin, h.UpdateSMTPConfig)
	api.POST("/config/smtp/test", middleware.JWTAuth(), admin, h.TestSMTP)

	api.GET("/config/ai", middleware.JWTAuth(), admin, h.GetAIConfig)
	api.PUT("/config/ai", middleware.JWTAuth(), admin, h.UpdateAIConfig)
	api.POST("/config/ai/test", middleware.JWTAuth(), admin, h.TestAI)
	api.GET("/config/ai/models", middleware.JWTAuth(), admin, h.GetAIModels)

	api.GET("/config/general", h.GetGeneralSettings)
	api.PUT("/config/general", middleware.JWTAuth(), admin, h.UpdateGeneralSettings)

	api.GET("/config/theme", h.GetThemeConfig)
	api.PUT("/config/theme", middleware.JWTAuth(), admin, h.UpdateThemeConfig)

	// Theme upload endpoint
	api.POST("/themes/upload", middleware.JWTAuth(), admin, h.UploadTheme)
}
