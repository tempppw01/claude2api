package router

import (
	"claude2api/config"
	"claude2api/middleware"
	"claude2api/service"
	"claude2api/web"
	"net/http"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	// Apply middleware
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.AuthMiddleware())

	// Health check endpoint
	r.GET("/health", service.HealthCheckHandler)

	// Admin API endpoint (no auth required)
	r.GET("/admin-api/status", service.AdminStatusHandler)
	r.POST("/admin-api/config", service.AdminUpdateConfigHandler)

	// Admin static files (no auth required)
	r.GET("/admin", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/admin/")
	})

	// Serve embedded static files
	staticContent, _ := web.GetStaticFS()
	r.StaticFS("/admin/", http.FS(staticContent))

	// Chat completions endpoint (OpenAI-compatible)
	r.POST("/v1/chat/completions", service.ChatCompletionsHandler)
	r.GET("/v1/models", service.MoudlesHandler)

	if config.ConfigInstance.EnableMirrorApi {
		r.POST(config.ConfigInstance.MirrorApiPrefix+"/v1/chat/completions", service.MirrorChatHandler)
		r.GET(config.ConfigInstance.MirrorApiPrefix+"/v1/models", service.MoudlesHandler)
	}

	// HuggingFace compatible routes
	hfRouter := r.Group("/hf")
	{
		v1Router := hfRouter.Group("/v1")
		{
			v1Router.POST("/chat/completions", service.ChatCompletionsHandler)
			v1Router.GET("/models", service.MoudlesHandler)
		}
	}
}
