// Package router composes HTTP route registration across all domains.
package router

import (
	"vexgo/backend/handler"
	"vexgo/backend/internal/message"
	"vexgo/backend/middleware"

	"github.com/gin-gonic/gin"
)

// Deps aggregates the dependencies of every domain package.
type Deps struct {
	Message message.Deps
}

// RegisterAPIRoutes registers all routes under /api.
//
// During the incremental migration the legacy handler package still owns most
// routes; migrated domains register their own. Once the handler package is
// emptied it is removed from this function.
func RegisterAPIRoutes(r *gin.Engine, deps Deps) {
	api := r.Group("/api")
	api.Use(middleware.RequestLogger())
	api.Use(middleware.OptionalJWTAuth())

	handler.RegisterAPIRoutes(api)
	message.NewHandler(deps.Message).RegisterRoutes(api)
}
