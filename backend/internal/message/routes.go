package message

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the message domain routes on the /api group.
// The route paths and middleware chains are identical to the original
// registration in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/messages", middleware.JWTAuth(), h.GetMessages)
	api.GET("/messages/unread-count", middleware.JWTAuth(), h.GetUnreadCount)
	api.PUT("/messages/:id/read", middleware.JWTAuth(), h.MarkAsRead)
	api.PUT("/messages/read-all", middleware.JWTAuth(), h.MarkAllAsRead)
	api.DELETE("/messages/:id", middleware.JWTAuth(), h.DeleteMessage)
}
