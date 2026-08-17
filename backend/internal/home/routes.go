package home

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the home domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/stats", h.GetStats)
}
