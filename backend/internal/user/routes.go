package user

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the user domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	admin := middleware.PermissionMiddleware("admin", "super_admin")

	api.GET("/users", middleware.JWTAuth(), admin, h.GetUserList)
	api.PUT("/users/:id/role", middleware.JWTAuth(), admin, h.UpdateUserRole)
	api.DELETE("/users/:id", middleware.JWTAuth(), admin, h.DeleteUser)

	// Creator application routes
	api.POST("/users/apply-creator", middleware.JWTAuth(), h.ApplyForCreator)
	api.GET("/users/creator-applications", middleware.JWTAuth(), admin, h.GetCreatorApplications)
	api.PUT("/users/creator-applications/:id/review", middleware.JWTAuth(), admin, h.ReviewCreatorApplication)
}
