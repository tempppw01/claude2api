package service

import (
	"claude2api/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminStatusHandler handles the admin status endpoint
func AdminStatusHandler(c *gin.Context) {
	// Get model list
	models := getModelList()

	// Build response
	response := gin.H{
		"status":        "ok",
		"session_count": len(config.ConfigInstance.Sessions),
		"models":        models,
		"config": gin.H{
			"address":                  config.ConfigInstance.Address,
			"proxy":                    config.ConfigInstance.Proxy,
			"chat_delete":              config.ConfigInstance.ChatDelete,
			"max_chat_history_length":  config.ConfigInstance.MaxChatHistoryLength,
			"no_role_prefix":           config.ConfigInstance.NoRolePrefix,
			"prompt_disable_artifacts": config.ConfigInstance.PromptDisableArtifacts,
			"enable_mirror_api":        config.ConfigInstance.EnableMirrorApi,
			"mirror_api_prefix":        config.ConfigInstance.MirrorApiPrefix,
		},
	}

	c.JSON(http.StatusOK, response)
}

func getModelList() []string {
	return []string{
		"claude-3-7-sonnet-20250219",
		"claude-sonnet-4-20250514",
		"claude-sonnet-4-6-20260217",
		"claude-opus-4-20250514",
	}
}
