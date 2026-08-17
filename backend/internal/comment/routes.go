package comment

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the comment domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/comments/post/:id", h.GetComments)
	api.POST("/comments", middleware.JWTAuth(), h.CreateComment)
	api.DELETE("/comments/:id", middleware.JWTAuth(), h.DeleteComment)

	admin := middleware.PermissionMiddleware("admin", "super_admin")
	api.GET("/moderation/comments/pending", middleware.JWTAuth(), admin, h.GetPendingComments)
	api.GET("/moderation/comments/approved", middleware.JWTAuth(), admin, h.GetApprovedComments)
	api.GET("/moderation/comments/rejected", middleware.JWTAuth(), admin, h.GetRejectedComments)
	api.PUT("/moderation/comments/approve/:id", middleware.JWTAuth(), admin, h.ApproveComment)
	api.PUT("/moderation/comments/reject/:id", middleware.JWTAuth(), admin, h.RejectComment)
	api.GET("/moderation/comments/config", middleware.JWTAuth(), admin, h.GetCommentModerationConfig)
	api.PUT("/moderation/comments/config", middleware.JWTAuth(), admin, h.UpdateCommentModerationConfig)
}
