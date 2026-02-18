package service

import (
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// AdminStatusHandler handles the admin status endpoint
func AdminStatusHandler(c *gin.Context) {
	// Get model list
	models := getModelList()

	// Build session list (mask sensitive data)
	sessions := make([]map[string]interface{}, 0)
	for i, session := range config.ConfigInstance.Sessions {
		maskedKey := maskSessionKey(session.SessionKey)
		sessions = append(sessions, map[string]interface{}{
			"index":       i,
			"session_key": maskedKey,
			"org_id":      session.OrgID,
		})
	}

	// Build response
	response := gin.H{
		"status":        "ok",
		"session_count": len(config.ConfigInstance.Sessions),
		"sessions":      sessions,
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

// maskSessionKey masks the session key for display
func maskSessionKey(key string) string {
	if len(key) <= 20 {
		return key[:5] + "..." + key[len(key)-3:]
	}
	return key[:10] + "..." + key[len(key)-5:]
}

// AddSessionRequest represents the request body for adding a session
type AddSessionRequest struct {
	SessionKey string `json:"session_key" binding:"required"`
	OrgID      string `json:"org_id"`
}

// AdminAddSessionHandler handles adding a new session
func AdminAddSessionHandler(c *gin.Context) {
	var req AddSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate session key format
	if !strings.HasPrefix(req.SessionKey, "sk-ant-sid") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session key format. Must start with 'sk-ant-sid'"})
		return
	}

	// Check for duplicate
	for _, s := range config.ConfigInstance.Sessions {
		if s.SessionKey == req.SessionKey {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Session key already exists"})
			return
		}
	}

	// Add to config
	newSession := config.SessionInfo{
		SessionKey: req.SessionKey,
		OrgID:      req.OrgID,
	}
	config.ConfigInstance.Sessions = append(config.ConfigInstance.Sessions, newSession)
	config.ConfigInstance.RetryCount = len(config.ConfigInstance.Sessions)
	if config.ConfigInstance.RetryCount > 5 {
		config.ConfigInstance.RetryCount = 5
	}

	// Save to YAML
	if err := saveConfigToYAML(); err != nil {
		logger.Error(fmt.Sprintf("Failed to save config: %v", err))
	}

	logger.Info(fmt.Sprintf("Added new session: %s", maskSessionKey(req.SessionKey)))

	c.JSON(http.StatusOK, gin.H{
		"status":        "added",
		"session_count": len(config.ConfigInstance.Sessions),
	})
}

// AdminRemoveSessionHandler handles removing a session
func AdminRemoveSessionHandler(c *gin.Context) {
	indexStr := c.Param("index")
	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index"})
		return
	}

	if index < 0 || index >= len(config.ConfigInstance.Sessions) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Index out of range"})
		return
	}

	// Remove session
	removed := config.ConfigInstance.Sessions[index]
	config.ConfigInstance.Sessions = append(
		config.ConfigInstance.Sessions[:index],
		config.ConfigInstance.Sessions[index+1:]...,
	)
	config.ConfigInstance.RetryCount = len(config.ConfigInstance.Sessions)
	if config.ConfigInstance.RetryCount > 5 {
		config.ConfigInstance.RetryCount = 5
	}

	// Save to YAML
	if err := saveConfigToYAML(); err != nil {
		logger.Error(fmt.Sprintf("Failed to save config: %v", err))
	}

	logger.Info(fmt.Sprintf("Removed session: %s", maskSessionKey(removed.SessionKey)))

	c.JSON(http.StatusOK, gin.H{
		"status":        "removed",
		"session_count": len(config.ConfigInstance.Sessions),
	})
}

// TestSessionRequest represents the request body for testing a session
type TestSessionRequest struct {
	SessionKey string `json:"session_key"`
	Index      int    `json:"index"`
}

// AdminTestSessionHandler handles testing a session
func AdminTestSessionHandler(c *gin.Context) {
	var req TestSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var sessionKey string
	if req.SessionKey != "" {
		sessionKey = req.SessionKey
	} else if req.Index >= 0 && req.Index < len(config.ConfigInstance.Sessions) {
		sessionKey = config.ConfigInstance.Sessions[req.Index].SessionKey
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session key or index"})
		return
	}

	// Create client and test
	client := core.NewClient(sessionKey, config.ConfigInstance.Proxy, "claude-sonnet-4-6-20260217")

	// Try to get org ID as a test
	orgID, err := client.GetOrgID()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Failed to connect: %v", err),
		})
		return
	}

	// Update org ID if not set
	if req.Index >= 0 && req.Index < len(config.ConfigInstance.Sessions) {
		if config.ConfigInstance.Sessions[req.Index].OrgID == "" {
			config.ConfigInstance.SetSessionOrgID(sessionKey, orgID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Session is valid",
		"org_id":  orgID,
	})
}

// UpdateConfigRequest represents the request body for updating config
type UpdateConfigRequest struct {
	MaxChatHistoryLength    *int    `json:"max_chat_history_length"`
	ChatDelete              *bool   `json:"chat_delete"`
	NoRolePrefix            *bool   `json:"no_role_prefix"`
	PromptDisableArtifacts  *bool   `json:"prompt_disable_artifacts"`
	EnableMirrorApi         *bool   `json:"enable_mirror_api"`
	MirrorApiPrefix         *string `json:"mirror_api_prefix"`
}

// AdminUpdateConfigHandler handles updating configuration
func AdminUpdateConfigHandler(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Update fields if provided
	if req.MaxChatHistoryLength != nil {
		if *req.MaxChatHistoryLength < 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Max chat history length must be at least 1000"})
			return
		}
		config.ConfigInstance.MaxChatHistoryLength = *req.MaxChatHistoryLength
	}

	if req.ChatDelete != nil {
		config.ConfigInstance.ChatDelete = *req.ChatDelete
	}

	if req.NoRolePrefix != nil {
		config.ConfigInstance.NoRolePrefix = *req.NoRolePrefix
	}

	if req.PromptDisableArtifacts != nil {
		config.ConfigInstance.PromptDisableArtifacts = *req.PromptDisableArtifacts
	}

	if req.EnableMirrorApi != nil {
		config.ConfigInstance.EnableMirrorApi = *req.EnableMirrorApi
	}

	if req.MirrorApiPrefix != nil {
		config.ConfigInstance.MirrorApiPrefix = *req.MirrorApiPrefix
	}

	// Try to save to config.yaml
	if err := saveConfigToYAML(); err != nil {
		logger.Error(fmt.Sprintf("Failed to save config to YAML: %v", err))
		// Still return success as in-memory config is updated
		c.JSON(http.StatusOK, gin.H{
			"status":  "updated",
			"message": "Config updated in memory (could not save to file: " + err.Error() + ")",
			"config": gin.H{
				"max_chat_history_length":    config.ConfigInstance.MaxChatHistoryLength,
				"chat_delete":                config.ConfigInstance.ChatDelete,
				"no_role_prefix":             config.ConfigInstance.NoRolePrefix,
				"prompt_disable_artifacts":   config.ConfigInstance.PromptDisableArtifacts,
				"enable_mirror_api":          config.ConfigInstance.EnableMirrorApi,
				"mirror_api_prefix":          config.ConfigInstance.MirrorApiPrefix,
			},
		})
		return
	}

	logger.Info("Config updated successfully")

	c.JSON(http.StatusOK, gin.H{
		"status":  "updated",
		"message": "Config saved successfully",
		"config": gin.H{
			"max_chat_history_length":    config.ConfigInstance.MaxChatHistoryLength,
			"chat_delete":                config.ConfigInstance.ChatDelete,
			"no_role_prefix":             config.ConfigInstance.NoRolePrefix,
			"prompt_disable_artifacts":   config.ConfigInstance.PromptDisableArtifacts,
			"enable_mirror_api":          config.ConfigInstance.EnableMirrorApi,
			"mirror_api_prefix":          config.ConfigInstance.MirrorApiPrefix,
		},
	})
}

// AdminStatsHandler handles the stats endpoint
func AdminStatsHandler(c *gin.Context) {
	stats := logger.GlobalRequestLogger.GetStats()
	c.JSON(http.StatusOK, stats)
}

// AdminLogsHandler handles the logs endpoint
func AdminLogsHandler(c *gin.Context) {
	limit := 100 // default limit
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
		if limit > 1000 {
			limit = 1000
		}
	}

	logs := logger.GlobalRequestLogger.GetRecentLogs(limit)
	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"count": len(logs),
	})
}

// AdminClearLogsHandler handles clearing logs
func AdminClearLogsHandler(c *gin.Context) {
	logger.GlobalRequestLogger.Clear()
	c.JSON(http.StatusOK, gin.H{
		"status":  "cleared",
		"message": "All logs have been cleared",
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
