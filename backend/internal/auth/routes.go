package auth

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the auth domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	{
		auth.POST("/register", h.Register)
		auth.POST("/login", h.Login)
		auth.GET("/me", middleware.JWTAuth(), h.GetCurrentUser)
		auth.GET("/user", middleware.JWTAuth(), h.GetCurrentUser)
		auth.PUT("/profile", middleware.JWTAuth(), h.UpdateProfile)
		auth.PUT("/password", middleware.JWTAuth(), h.ChangePassword)
		auth.PUT("/email", middleware.JWTAuth(), h.UpdateEmail)
		auth.PUT("/settings", middleware.JWTAuth(), h.UpdateSettings)
		auth.POST("/request-password-reset", h.RequestPasswordReset)
		auth.POST("/reset-password", h.ResetPassword)
	}
}
