package router

import (
	"claude2api/config"
	"claude2api/middleware"
	"claude2api/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	// Apply middleware
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.AuthMiddleware())

	// Health check endpoint
	r.GET("/health", service.HealthCheckHandler)

	// Admin routes (no auth required for status page)
	r.GET("/admin/status", service.AdminStatusHandler)
	r.GET("/admin", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/admin/")
	})
	r.Static("/admin/", "./web/static")

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
