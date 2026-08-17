package post

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the post domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/posts", h.GetPosts)
	api.GET("/posts/:id", h.GetPost)

	api.GET("/categories", h.GetCategories)
	api.GET("/tags", h.GetTags)

	api.GET("/stats/popular-posts", h.GetPopularPosts)
	api.GET("/stats/latest-posts", h.GetLatestPosts)

	api.GET("/likes/:postId", h.GetLikeStatus)
	api.GET("/posts/user/:id", h.GetUserPosts)

	api.POST("/posts", middleware.JWTAuth(), h.CreatePost)
	api.GET("/posts/user/my-posts", middleware.JWTAuth(), h.GetMyPosts)
	api.GET("/posts/drafts", middleware.JWTAuth(), h.GetDraftPosts)
	api.PUT("/posts/:id", middleware.JWTAuth(), h.UpdatePost)
	api.DELETE("/posts/:id", middleware.JWTAuth(), h.DeletePost)

	api.POST("/categories", middleware.JWTAuth(), h.CreateCategory)
	api.POST("/tags", middleware.JWTAuth(), h.CreateTag)

	api.POST("/likes/:postId", middleware.JWTAuth(), h.ToggleLike)

	admin := middleware.PermissionMiddleware("admin", "super_admin")
	api.GET("/moderation/pending", middleware.JWTAuth(), admin, h.GetPendingPosts)
	api.GET("/moderation/approved", middleware.JWTAuth(), admin, h.GetApprovedPosts)
	api.GET("/moderation/rejected", middleware.JWTAuth(), admin, h.GetRejectedPosts)
	api.PUT("/moderation/approve/:id", middleware.JWTAuth(), admin, h.ApprovePost)
	api.PUT("/moderation/reject/:id", middleware.JWTAuth(), admin, h.RejectPost)
	api.PUT("/moderation/resubmit/:id", middleware.JWTAuth(), admin, h.ResubmitPost)
}
