package message

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the message domain routes on the /api group.
// The route paths and middleware chains are identical to the original
// registration in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/messages", h.mw.JWTAuth(), h.GetMessages)
	api.GET("/messages/unread-count", h.mw.JWTAuth(), h.GetUnreadCount)
	api.PUT("/messages/:id/read", h.mw.JWTAuth(), h.MarkAsRead)
	api.PUT("/messages/read-all", h.mw.JWTAuth(), h.MarkAllAsRead)
	api.DELETE("/messages/:id", h.mw.JWTAuth(), h.DeleteMessage)
}
