package upload

import (
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the upload domain routes on the /api group.
// Route paths and middleware chains are identical to the original registration
// in the legacy handler package.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	api.POST("/upload/file", middleware.JWTAuth(), h.UploadFile)
	api.POST("/upload/files", middleware.JWTAuth(), h.UploadFiles)
	api.GET("/upload/my-files", middleware.JWTAuth(), h.GetMyFiles)
	api.DELETE("/upload/:id", middleware.JWTAuth(), h.DeleteFile)
}
