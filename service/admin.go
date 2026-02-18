package service

import (
	"claude2api/config"
	"claude2api/logger"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
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

// UpdateConfigRequest represents the request body for updating config
type UpdateConfigRequest struct {
	MaxChatHistoryLength int `json:"max_chat_history_length"`
}

// AdminUpdateConfigHandler handles updating configuration
func AdminUpdateConfigHandler(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate the value
	if req.MaxChatHistoryLength < 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Max chat history length must be at least 1000"})
		return
	}

	// Update in-memory config
	config.ConfigInstance.MaxChatHistoryLength = req.MaxChatHistoryLength

	// Try to save to config.yaml
	if err := saveConfigToYAML(); err != nil {
		logger.Error(fmt.Sprintf("Failed to save config to YAML: %v", err))
		// Still return success as in-memory config is updated
		c.JSON(http.StatusOK, gin.H{
			"status":  "updated",
			"message": "Config updated in memory (could not save to file: " + err.Error() + ")",
			"config": gin.H{
				"max_chat_history_length": config.ConfigInstance.MaxChatHistoryLength,
			},
		})
		return
	}

	logger.Info(fmt.Sprintf("Config updated: max_chat_history_length = %d", req.MaxChatHistoryLength))

	c.JSON(http.StatusOK, gin.H{
		"status":  "updated",
		"message": "Config saved successfully",
		"config": gin.H{
			"max_chat_history_length": config.ConfigInstance.MaxChatHistoryLength,
		},
	})
}

// saveConfigToYAML saves the current config to config.yaml
func saveConfigToYAML() error {
	// Find config file path
	configPath := findConfigPath()
	if configPath == "" {
		// Create new config file in working directory
		workDir, _ := os.Getwd()
		configPath = filepath.Join(workDir, "config.yaml")
	}

	// Build config structure for YAML
	configData := map[string]interface{}{
		"sessions":                config.ConfigInstance.Sessions,
		"address":                 config.ConfigInstance.Address,
		"apiKey":                  config.ConfigInstance.APIKey,
		"proxy":                   config.ConfigInstance.Proxy,
		"chatDelete":              config.ConfigInstance.ChatDelete,
		"maxChatHistoryLength":    config.ConfigInstance.MaxChatHistoryLength,
		"noRolePrefix":            config.ConfigInstance.NoRolePrefix,
		"promptDisableArtifacts":  config.ConfigInstance.PromptDisableArtifacts,
		"enableMirrorApi":         config.ConfigInstance.EnableMirrorApi,
		"mirrorApiPrefix":         config.ConfigInstance.MirrorApiPrefix,
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// findConfigPath finds the existing config.yaml path
func findConfigPath() string {
	execDir := filepath.Dir(os.Args[0])
	workDir, _ := os.Getwd()

	exeConfigPath := filepath.Join(execDir, "config.yaml")
	if _, err := os.Stat(exeConfigPath); err == nil {
		return exeConfigPath
	}

	workConfigPath := filepath.Join(workDir, "config.yaml")
	if _, err := os.Stat(workConfigPath); err == nil {
		return workConfigPath
	}

	return ""
}

func getModelList() []string {
	return []string{
		"claude-3-7-sonnet-20250219",
		"claude-sonnet-4-20250514",
		"claude-sonnet-4-6-20260217",
		"claude-opus-4-20250514",
	}
}
