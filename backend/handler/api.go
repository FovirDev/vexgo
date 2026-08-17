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
		api.GET("/verify-email", VerifyEmail)

		api.GET("/captcha", GenerateCaptcha)
		api.POST("/captcha/verify", VerifyCaptcha)

		api.GET("/stats", GetStats)

		api.GET("/themes", GetThemes)
		api.GET("/theme/:id/preview", GetThemePreview)

		// -------------------- Authentication API --------------------
		logrus.Debug("Registering authentication API routes")
		auth := api.Group("/auth")
		{
			auth.POST("/register", Register)
			auth.POST("/login", Login)
			auth.GET("/me", middleware.JWTAuth(), GetCurrentUser)
			auth.GET("/user", middleware.JWTAuth(), GetCurrentUser)
			auth.PUT("/profile", middleware.JWTAuth(), UpdateProfile)
			auth.PUT("/password", middleware.JWTAuth(), ChangePassword)
			auth.PUT("/email", middleware.JWTAuth(), UpdateEmail)
			auth.PUT("/settings", middleware.JWTAuth(), UpdateSettings)
			auth.POST("/request-password-reset", RequestPasswordReset)
			auth.POST("/reset-password", ResetPassword)
			auth.GET("/verification-status", middleware.JWTAuth(), GetVerificationStatus)
		}

		// -------------------- SSO --------------------
		sso := api.Group("/sso")
		{
			// Public: returns enabled providers, used by frontend to show/hide buttons
			// GET /api/sso/providers
			sso.GET("/providers", SSOProviders)

			// Step 1: open in popup → redirects to provider
			// GET /api/sso/:provider/login?method=sso_get_token|get_sso_id
			sso.GET("/:provider/login", SSOLoginRedirect)

			// Step 2: provider redirects back, popup sends postMessage → closes
			// GET /api/sso/:provider/callback?method=...&code=...&state=...
			sso.GET("/:provider/callback", func(c *gin.Context) {
				SSOCallback(c, DB())
			})
		}

		// -------------------- Business API requiring JWT authentication --------------------
		api.GET("/users", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), GetUserList)
		api.PUT("/users/:id/role", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), UpdateUserRole)
		api.DELETE("/users/:id", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), DeleteUser)

		// Creator application routes
		api.POST("/users/apply-creator", middleware.JWTAuth(), ApplyForCreator)
		api.GET("/users/creator-applications", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), GetCreatorApplications)
		api.PUT("/users/creator-applications/:id/review", middleware.JWTAuth(), middleware.PermissionMiddleware("admin", "super_admin"), ReviewCreatorApplication)

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
