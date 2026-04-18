package middleware

import (
	"claude2api/config"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware initializes the Claude client from the request header
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip auth for health check and admin static page
		if path == "/health" || strings.HasPrefix(path, "/admin/") || path == "/admin" {
			c.Next()
			return
		}

		if strings.HasPrefix(path, "/admin-api") {
			if path == "/admin-api/login" || path == "/admin-api/auth/status" || path == "/admin-api/logout" {
				c.Next()
				return
			}

			adminToken, err := c.Cookie("admin_auth")
			if err != nil || adminToken != config.ConfigInstance.AdminPassword {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Admin authentication required",
				})
				c.Abort()
				return
			}

			c.Next()
			return
		}

		if config.ConfigInstance.EnableMirrorApi && strings.HasPrefix(path, config.ConfigInstance.MirrorApiPrefix) {
			c.Set("UseMirrorApi", true)
			c.Next()
			return
		}
		Key := c.GetHeader("Authorization")
		if Key != "" {
			Key = strings.TrimPrefix(Key, "Bearer ")
			if Key != config.ConfigInstance.APIKey {
				c.JSON(401, gin.H{
					"error": "Invalid API key",
				})
				c.Abort()
				return
			}
			c.Next()
			return
		}
		c.JSON(401, gin.H{
			"error": "Missing or invalid Authorization header",
		})
		c.Abort()
	}
}
