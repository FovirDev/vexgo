package sso

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the sso domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	sso := api.Group("/sso")
	{
		// Public: returns enabled providers, used by frontend to show/hide buttons
		// GET /api/sso/providers
		sso.GET("/providers", h.SSOProviders)

		// Step 1: open in popup → redirects to provider
		// GET /api/sso/:provider/login?method=sso_get_token|get_sso_id
		sso.GET("/:provider/login", h.SSOLoginRedirect)

		// Step 2: provider redirects back, popup sends postMessage → closes
		// GET /api/sso/:provider/callback?method=...&code=...&state=...
		sso.GET("/:provider/callback", h.SSOCallback)
	}
}
