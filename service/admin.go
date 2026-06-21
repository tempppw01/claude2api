package service

import (
	"claude2api/adminauth"
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"claude2api/utils"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

const AppVersion = "v1.0.0"

// AdminStatusHandler handles the admin status endpoint
func AdminStatusHandler(c *gin.Context) {
	// Get model list
	models := GetAdminModelSummaries()
	lastSuccessBySession := logger.GlobalRequestLogger.GetLastSuccessBySession()
	lastErrorBySession := logger.GlobalRequestLogger.GetLastErrorBySession()
	statsBySession := logger.GlobalRequestLogger.GetStatsBySession()

	// Build session list (mask sensitive data)
	sessions := make([]map[string]interface{}, 0)
	now := time.Now()
	for i, session := range config.ConfigInstance.Sessions {
		maskedKey := maskSessionKey(session.SessionKey)
		cooldownUntil := ""
		cooldownSource := ""
		coolingDown := false
		if until, source, ok := config.ConfigInstance.GetSessionCooldownInfoByIndex(i, now); ok {
			coolingDown = true
			cooldownUntil = formatChinaTime(until)
			cooldownSource = source
		}
		lastSuccessAt := ""
		if lastSuccess, ok := lastSuccessBySession[i]; ok {
			lastSuccessAt = formatChinaTime(lastSuccess)
		}
		lastErrorAt := ""
		lastErrorType := ""
		lastErrorMessage := ""
		lastErrorModel := ""
		if lastError, ok := lastErrorBySession[i]; ok {
			lastErrorAt = formatChinaTime(lastError.Timestamp)
			lastErrorType = lastError.ErrorType
			lastErrorMessage = lastError.Error
			lastErrorModel = lastError.Model
		}
		sessionStats := statsBySession[i]
		sessions = append(sessions, map[string]interface{}{
			"index":                           i,
			"session_key":                     maskedKey,
			"org_id":                          session.OrgID,
			"cf_clearance":                    session.CFClearance != "",
			"cookie_string":                   session.CookieString != "",
			"cookie_preview":                  maskCookiePreview(session),
			"cooling_down":                    coolingDown,
			"cooldown_until":                  cooldownUntil,
			"cooldown_source":                 cooldownSource,
			"cooldown_official":               cooldownSource == config.CooldownSourceOfficial,
			"last_success_at":                 lastSuccessAt,
			"last_error_at":                   lastErrorAt,
			"last_error_type":                 lastErrorType,
			"last_error":                      lastErrorMessage,
			"last_error_model":                lastErrorModel,
			"total_requests":                  sessionStats.TotalRequests,
			"success_requests":                sessionStats.SuccessRequests,
			"failed_requests":                 sessionStats.FailedRequests,
			"success_rate":                    sessionStats.SuccessRate,
			"rate_limit_requests":             sessionStats.RateLimitRequests,
			"input_tokens":                    sessionStats.InputTokens,
			"output_tokens":                   sessionStats.OutputTokens,
			"total_tokens":                    sessionStats.TotalTokens,
			"avg_tokens_per_success":          sessionStats.AvgTokensPerSuccess,
			"avg_successes_before_rate_limit": sessionStats.AvgSuccessesBeforeRateLimit,
		})
	}

	// Build response
	response := gin.H{
		"status":        "ok",
		"version":       AppVersion,
		"session_count": len(config.ConfigInstance.Sessions),
		"sessions":      sessions,
		"models":        models,
		"config":        buildAdminConfigResponse(),
	}

	c.JSON(http.StatusOK, response)
}

type AdminLoginRequest struct {
	Password string `json:"password" binding:"required"`
}

// AdminLoginHandler handles admin login and sets auth cookie
func AdminLoginHandler(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if strings.TrimSpace(req.Password) != config.ConfigInstance.AdminPassword {
		clearAdminAuthCookie(c)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}

	c.SetCookie(adminauth.CookieName, adminauth.NewToken(config.ConfigInstance.AdminPassword), int(adminauth.SessionDuration.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// AdminAuthStatusHandler returns current admin auth status
func AdminAuthStatusHandler(c *gin.Context) {
	adminToken, err := c.Cookie(adminauth.CookieName)
	authenticated := err == nil && adminauth.ValidateToken(adminToken, config.ConfigInstance.AdminPassword)
	if !authenticated {
		clearAdminAuthCookie(c)
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": authenticated,
	})
}

// AdminLogoutHandler clears admin auth cookie
func AdminLogoutHandler(c *gin.Context) {
	clearAdminAuthCookie(c)
	c.JSON(http.StatusOK, gin.H{
		"status": "logged_out",
	})
}

// maskSessionKey masks the session key for display
func maskSessionKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	if len(key) <= 20 {
		return key[:5] + "..." + key[len(key)-3:]
	}
	return key[:10] + "..." + key[len(key)-5:]
}

// AddSessionRequest represents the request body for adding a session
type AddSessionRequest struct {
	SessionKey   string `json:"session_key" binding:"required"`
	OrgID        string `json:"org_id"`
	CFClearance  string `json:"cf_clearance"`
	CookieString string `json:"cookie_string"`
}

type ImportSessionsRequest struct {
	Sessions     string   `json:"sessions"`
	SessionKeys  []string `json:"session_keys"`
	CFClearance  string   `json:"cf_clearance"`
	CookieString string   `json:"cookie_string"`
}

type BatchTestSessionsRequest struct {
	Indices []int  `json:"indices"`
	Model   string `json:"model"`
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
		SessionKey:   req.SessionKey,
		OrgID:        req.OrgID,
		CFClearance:  strings.TrimSpace(req.CFClearance),
		CookieString: strings.TrimSpace(req.CookieString),
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

// AdminImportSessionsHandler handles bulk importing session keys.
func AdminImportSessionsHandler(c *gin.Context) {
	var req ImportSessionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	candidates := parseImportSessionCandidates(req)
	if len(candidates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid session keys found"})
		return
	}

	existing := make(map[string]bool, len(config.ConfigInstance.Sessions))
	for _, session := range config.ConfigInstance.Sessions {
		existing[session.SessionKey] = true
	}

	added := 0
	duplicates := 0
	invalid := 0
	imported := make([]gin.H, 0, len(candidates))
	for _, candidate := range candidates {
		if !strings.HasPrefix(candidate.SessionKey, "sk-ant-sid") {
			invalid++
			continue
		}
		if existing[candidate.SessionKey] {
			duplicates++
			continue
		}
		existing[candidate.SessionKey] = true
		config.ConfigInstance.Sessions = append(config.ConfigInstance.Sessions, candidate)
		imported = append(imported, gin.H{
			"session_key": maskSessionKey(candidate.SessionKey),
			"org_id":      candidate.OrgID,
		})
		added++
	}
	config.ConfigInstance.RetryCount = len(config.ConfigInstance.Sessions)
	if config.ConfigInstance.RetryCount > 5 {
		config.ConfigInstance.RetryCount = 5
	}

	if added > 0 {
		if err := saveConfigToYAML(); err != nil {
			logger.Error(fmt.Sprintf("Failed to save config: %v", err))
		}
	}

	logger.Info(fmt.Sprintf("Imported %d sessions, skipped %d duplicates and %d invalid entries", added, duplicates, invalid))
	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"added":         added,
		"duplicates":    duplicates,
		"invalid":       invalid,
		"session_count": len(config.ConfigInstance.Sessions),
		"imported":      imported,
	})
}

func parseImportSessionCandidates(req ImportSessionsRequest) []config.SessionInfo {
	lines := make([]string, 0)
	lines = append(lines, req.SessionKeys...)
	for _, line := range strings.FieldsFunc(req.Sessions, func(r rune) bool {
		return r == '\n' || r == '\r'
	}) {
		lines = append(lines, line)
	}

	candidates := make([]config.SessionInfo, 0, len(lines))
	defaultCFClearance := strings.TrimSpace(req.CFClearance)
	defaultCookieString := strings.TrimSpace(req.CookieString)
	for _, line := range lines {
		session := parseImportSessionLine(line)
		if session.SessionKey == "" {
			continue
		}
		if session.CFClearance == "" {
			session.CFClearance = defaultCFClearance
		}
		if session.CookieString == "" {
			session.CookieString = defaultCookieString
		}
		candidates = append(candidates, session)
	}
	return candidates
}

func parseImportSessionLine(line string) config.SessionInfo {
	line = strings.TrimSpace(line)
	if line == "" {
		return config.SessionInfo{}
	}
	line = strings.Trim(line, `"'`)
	var parts []string
	if strings.Contains(line, "|") {
		parts = strings.SplitN(line, "|", 4)
	} else if strings.Contains(line, "\t") {
		parts = strings.SplitN(line, "\t", 4)
	} else {
		parts = strings.SplitN(line, ":", 2)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(strings.Trim(parts[i], `"'`))
	}
	session := config.SessionInfo{SessionKey: parts[0]}
	if len(parts) > 1 {
		session.OrgID = parts[1]
	}
	if len(parts) > 2 {
		session.CFClearance = parts[2]
	}
	if len(parts) > 3 {
		session.CookieString = parts[3]
	}
	return session
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
	config.ConfigInstance.ClearSessionCooldown(removed.SessionKey)
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

// AdminClearSessionCooldownHandler clears a runtime cooldown for a session.
func AdminClearSessionCooldownHandler(c *gin.Context) {
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

	session := config.ConfigInstance.Sessions[index]
	config.ConfigInstance.ClearSessionCooldown(session.SessionKey)
	logger.Info(fmt.Sprintf("Cleared cooldown for session %d (%s)", index+1, maskSessionKey(session.SessionKey)))

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "cooldown cleared",
	})
}

// TestSessionRequest represents the request body for testing a session
type TestSessionRequest struct {
	SessionKey   string `json:"session_key"`
	Index        int    `json:"index"`
	CFClearance  string `json:"cf_clearance"`
	CookieString string `json:"cookie_string"`
	Model        string `json:"model"`
}

// AdminTestSessionHandler handles testing a session
func AdminTestSessionHandler(c *gin.Context) {
	var req TestSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	testSession := config.SessionInfo{
		SessionKey:   strings.TrimSpace(req.SessionKey),
		CFClearance:  strings.TrimSpace(req.CFClearance),
		CookieString: strings.TrimSpace(req.CookieString),
	}
	if testSession.SessionKey != "" {
		testSession.OrgID = ""
	} else if req.Index >= 0 && req.Index < len(config.ConfigInstance.Sessions) {
		testSession = config.ConfigInstance.Sessions[req.Index]
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session key or index"})
		return
	}

	result, err := runAdminOpenAITestRequest(c, testSession, req.Index, strings.TrimSpace(req.Model))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	// Update org ID if not set
	if req.Index >= 0 && req.Index < len(config.ConfigInstance.Sessions) {
		if config.ConfigInstance.Sessions[req.Index].OrgID == "" && result.OrgID != "" {
			config.ConfigInstance.SetSessionOrgID(testSession.SessionKey, result.OrgID)
		}
		config.ConfigInstance.ClearSessionCooldown(testSession.SessionKey)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"message":       "OpenAI request test passed",
		"model":         result.Model,
		"org_id":        result.OrgID,
		"input_tokens":  result.InputTokens,
		"output_tokens": result.OutputTokens,
		"total_tokens":  result.InputTokens + result.OutputTokens,
	})
}

// AdminBatchTestSessionsHandler tests multiple configured sessions.
func AdminBatchTestSessionsHandler(c *gin.Context) {
	var req BatchTestSessionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	indices := normalizeBatchTestIndices(req.Indices)
	results := make([]gin.H, 0, len(indices))
	successCount := 0
	for position, index := range indices {
		if index < 0 || index >= len(config.ConfigInstance.Sessions) {
			results = append(results, gin.H{
				"index":   index,
				"status":  "error",
				"message": "index out of range",
			})
			continue
		}
		session := config.ConfigInstance.Sessions[index]
		result, err := runAdminOpenAITestRequest(c, session, index, strings.TrimSpace(req.Model))
		if err != nil {
			results = append(results, gin.H{
				"index":       index,
				"session_key": maskSessionKey(session.SessionKey),
				"status":      "error",
				"message":     err.Error(),
			})
			sleepBetweenBatchSessionTests(position, len(indices))
			continue
		}
		if config.ConfigInstance.Sessions[index].OrgID == "" && result.OrgID != "" {
			config.ConfigInstance.SetSessionOrgID(session.SessionKey, result.OrgID)
		}
		config.ConfigInstance.ClearSessionCooldown(session.SessionKey)
		successCount++
		results = append(results, gin.H{
			"index":        index,
			"session_key":  maskSessionKey(session.SessionKey),
			"status":       "ok",
			"message":      "OpenAI request test passed",
			"model":        result.Model,
			"org_id":       result.OrgID,
			"total_tokens": result.InputTokens + result.OutputTokens,
		})
		sleepBetweenBatchSessionTests(position, len(indices))
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"tested":        len(indices),
		"success":       successCount,
		"failed":        len(indices) - successCount,
		"auto_unfrozen": successCount,
		"results":       results,
	})
}

func sleepBetweenBatchSessionTests(position int, total int) {
	if position >= total-1 {
		return
	}
	time.Sleep(800 * time.Millisecond)
}

func normalizeBatchTestIndices(indices []int) []int {
	if len(indices) == 0 {
		all := make([]int, len(config.ConfigInstance.Sessions))
		for i := range config.ConfigInstance.Sessions {
			all[i] = i
		}
		return all
	}
	seen := make(map[int]bool, len(indices))
	normalized := make([]int, 0, len(indices))
	for _, index := range indices {
		if seen[index] {
			continue
		}
		seen[index] = true
		normalized = append(normalized, index)
	}
	return normalized
}

type adminSessionTestResult struct {
	Model        string
	OrgID        string
	InputTokens  int
	OutputTokens int
}

func runAdminOpenAITestRequest(c *gin.Context, session config.SessionInfo, sessionIdx int, requestedModel string) (adminSessionTestResult, error) {
	startTime := time.Now()
	c.Set("request_message_count", 1)
	if requestedModel == "" {
		requestedModel = "claude-sonnet-4-6"
	}

	selectedModel := ResolveModel(requestedModel)
	promptOverride, promptMode := resolvePromptOverride(selectedModel)
	processor := utils.NewChatRequestProcessor()
	processor.SetPromptOverride(promptOverride, promptMode)
	processor.ProcessMessages([]map[string]interface{}{
		{
			"role":    "user",
			"content": "Reply with OK only.",
		},
	})

	recorder := httptest.NewRecorder()
	testContext, _ := gin.CreateTestContext(recorder)
	testContext.Request = c.Request.Clone(c.Request.Context())

	modelName := selectedModel.UpstreamID
	if selectedModel.Thinking {
		modelName += "-think"
	}

	if session.OrgID == "" {
		client := core.NewClientFromSession(session, config.ConfigInstance.Proxy, modelName)
		orgID, err := client.GetOrgID()
		if err != nil {
			errorMessage := fmt.Sprintf("failed to get org ID: %s", core.GetErrorMessage(err))
			logRequest(c, modelName, sessionIdx, 0, 0, false, startTime, errorMessage)
			return adminSessionTestResult{Model: selectedModel.PublicID}, fmt.Errorf("OpenAI request test failed: %s", errorMessage)
		}
		session.OrgID = orgID
		if sessionIdx >= 0 && sessionIdx < len(config.ConfigInstance.Sessions) {
			config.ConfigInstance.SetSessionOrgID(session.SessionKey, orgID)
		}
	}

	inputTokens, outputTokens, err := handleChatRequestWithTokens(
		testContext,
		session,
		modelName,
		processor,
		false,
		selectedModel.ThinkingMode,
		selectedModel.EffortLevel,
	)
	if err != nil {
		errorMessage := core.GetErrorMessage(err)
		if core.IsRateLimitError(err) {
			cooldownUntil := time.Time{}
			cooldownSource := ""
			now := time.Now()
			if resetAt, ok := core.GetRateLimitResetAt(err); ok {
				cooldownUntil, cooldownSource = config.ConfigInstance.CooldownSessionAfterRateLimit(session.SessionKey, resetAt, now)
			} else {
				cooldownUntil, cooldownSource = config.ConfigInstance.CooldownSessionAfterRateLimit(session.SessionKey, time.Time{}, now)
			}
			if cooldownSource == config.CooldownSourceOfficial {
				errorMessage = fmt.Sprintf("rate limit exceeded - Claude official reset at: %s 中国时间", formatChinaTime(cooldownUntil))
			} else {
				errorMessage = fmt.Sprintf("rate limit exceeded - estimated cooldown until: %s 中国时间 (Claude did not return a usable future reset time)", formatChinaTime(cooldownUntil))
			}
			logger.Error(fmt.Sprintf(
				"Admin session test hit rate limit for S%d (%s); cooling down until %s (source: %s)",
				sessionIdx+1,
				maskSessionKey(session.SessionKey),
				formatChinaTime(cooldownUntil),
				cooldownSource,
			))
		}
		logRequest(c, modelName, sessionIdx, 0, 0, false, startTime, errorMessage)
		return adminSessionTestResult{Model: selectedModel.PublicID}, fmt.Errorf("OpenAI request test failed: %s", errorMessage)
	}
	if inputTokens == 0 && outputTokens == 0 {
		outputTokens = 1
	}

	logRequest(c, modelName, sessionIdx, inputTokens, outputTokens, true, startTime, "")
	return adminSessionTestResult{
		Model:        selectedModel.PublicID,
		OrgID:        getSessionOrgID(session, sessionIdx),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

func getSessionOrgID(session config.SessionInfo, sessionIdx int) string {
	if session.OrgID != "" {
		return session.OrgID
	}
	if sessionIdx >= 0 && sessionIdx < len(config.ConfigInstance.Sessions) {
		return config.ConfigInstance.Sessions[sessionIdx].OrgID
	}
	return ""
}

// UpdateConfigRequest represents the request body for updating config
type UpdateConfigRequest struct {
	MaxChatHistoryLength   *int                      `json:"max_chat_history_length"`
	InternalRetryCount     *int                      `json:"internal_retry_count"`
	ChatDelete             *bool                     `json:"chat_delete"`
	NoRolePrefix           *bool                     `json:"no_role_prefix"`
	PromptDisableArtifacts *bool                     `json:"prompt_disable_artifacts"`
	EnableMirrorApi        *bool                     `json:"enable_mirror_api"`
	MirrorApiPrefix        *string                   `json:"mirror_api_prefix"`
	APIKey                 *string                   `json:"api_key"`
	Proxy                  *string                   `json:"proxy"`
	AdminPassword          *string                   `json:"admin_password"`
	GlobalSystemPrompt     *string                   `json:"global_system_prompt_override"`
	GlobalPromptMode       *string                   `json:"global_prompt_override_mode"`
	ModelDefinitions       *[]config.ModelDefinition `json:"model_definitions"`
	RequestLogRetention    *int                      `json:"request_log_retention"`
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

	if req.InternalRetryCount != nil {
		if *req.InternalRetryCount < 1 || *req.InternalRetryCount > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Internal retry count must be between 1 and 10"})
			return
		}
		config.ConfigInstance.InternalRetryCount = config.NormalizeInternalRetryCount(*req.InternalRetryCount)
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

	if req.APIKey != nil && *req.APIKey != "" {
		config.ConfigInstance.APIKey = *req.APIKey
	}

	if req.Proxy != nil {
		config.ConfigInstance.Proxy = *req.Proxy
	}

	if req.AdminPassword != nil {
		trimmedPassword := strings.TrimSpace(*req.AdminPassword)
		if trimmedPassword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Admin password cannot be empty"})
			return
		}
		config.ConfigInstance.AdminPassword = trimmedPassword
	}

	if req.GlobalSystemPrompt != nil {
		config.ConfigInstance.GlobalSystemPromptOverride = strings.TrimSpace(*req.GlobalSystemPrompt)
	}

	if req.GlobalPromptMode != nil {
		config.ConfigInstance.GlobalPromptOverrideMode = normalizePromptMode(*req.GlobalPromptMode)
	}

	if req.ModelDefinitions != nil {
		definitions := make([]config.ModelDefinition, 0, len(*req.ModelDefinitions))
		for _, item := range *req.ModelDefinitions {
			normalized := normalizeModelDefinition(item)
			if normalized.PublicID == "" {
				continue
			}
			definitions = append(definitions, normalized)
		}
		config.ConfigInstance.ModelDefinitions = definitions
	}

	if req.RequestLogRetention != nil {
		normalizedRetention := config.NormalizeRequestLogRetention(*req.RequestLogRetention)
		if normalizedRetention != *req.RequestLogRetention {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Request log retention must be one of 100, 500, 1000, 3000"})
			return
		}
		config.ConfigInstance.RequestLogRetention = normalizedRetention
		logger.GlobalRequestLogger.SetMaxLogs(normalizedRetention)
	}

	// Try to save to config.yaml
	if err := saveConfigToYAML(); err != nil {
		logger.Error(fmt.Sprintf("Failed to save config to YAML: %v", err))
		// Still return success as in-memory config is updated
		c.JSON(http.StatusOK, gin.H{
			"status":  "updated",
			"message": "Config updated in memory (could not save to file: " + err.Error() + ")",
			"config":  buildAdminConfigResponse(),
		})
		return
	}

	logger.Info("Config updated successfully")

	c.JSON(http.StatusOK, gin.H{
		"status":  "updated",
		"message": "Config saved successfully",
		"config":  buildAdminConfigResponse(),
	})
}

// maskAPIKey masks the API key for display
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func maskCookiePreview(session config.SessionInfo) string {
	parts := make([]string, 0, 2)
	if session.CFClearance != "" {
		parts = append(parts, "cf_clearance")
	}
	if session.CookieString != "" {
		parts = append(parts, "custom cookies")
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func clearAdminAuthCookie(c *gin.Context) {
	c.SetCookie(adminauth.CookieName, "", -1, "/", "", false, true)
}

func formatChinaTime(t time.Time) string {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	return t.In(location).Format("2006-01-02 15:04:05")
}

// AdminStatsHandler handles the stats endpoint
func AdminStatsHandler(c *gin.Context) {
	stats := logger.GlobalRequestLogger.GetStats()
	c.JSON(http.StatusOK, stats)
}

// AdminLogsHandler handles the logs endpoint
// Supports both legacy limit parameter and new pagination parameters
func AdminLogsHandler(c *gin.Context) {
	// Check for pagination parameters
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")

	if pageStr != "" || pageSizeStr != "" {
		// Use pagination mode
		page := 1
		pageSize := 10

		if pageStr != "" {
			fmt.Sscanf(pageStr, "%d", &page)
		}
		if pageSizeStr != "" {
			fmt.Sscanf(pageSizeStr, "%d", &pageSize)
		}

		logs, total, hasMore := logger.GlobalRequestLogger.GetLogsWithPagination(page, pageSize)
		c.JSON(http.StatusOK, gin.H{
			"logs":      logs,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
			"has_more":  hasMore,
		})
		return
	}

	// Legacy mode: use limit parameter
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

func buildAdminConfigResponse() gin.H {
	return gin.H{
		"address":                       config.ConfigInstance.Address,
		"proxy":                         config.ConfigInstance.Proxy,
		"chat_delete":                   config.ConfigInstance.ChatDelete,
		"max_chat_history_length":       config.ConfigInstance.MaxChatHistoryLength,
		"internal_retry_count":          config.NormalizeInternalRetryCount(config.ConfigInstance.InternalRetryCount),
		"no_role_prefix":                config.ConfigInstance.NoRolePrefix,
		"prompt_disable_artifacts":      config.ConfigInstance.PromptDisableArtifacts,
		"enable_mirror_api":             config.ConfigInstance.EnableMirrorApi,
		"mirror_api_prefix":             config.ConfigInstance.MirrorApiPrefix,
		"api_key":                       maskAPIKey(config.ConfigInstance.APIKey),
		"admin_password_set":            strings.TrimSpace(config.ConfigInstance.AdminPassword) != "",
		"global_system_prompt_override": config.ConfigInstance.GlobalSystemPromptOverride,
		"global_prompt_override_mode":   normalizePromptMode(config.ConfigInstance.GlobalPromptOverrideMode),
		"model_definition_count":        len(config.ConfigInstance.ModelDefinitions),
		"model_definitions":             config.ConfigInstance.ModelDefinitions,
		"request_log_retention":         config.ConfigInstance.RequestLogRetention,
	}
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
		"sessions":                   config.ConfigInstance.Sessions,
		"address":                    config.ConfigInstance.Address,
		"apiKey":                     config.ConfigInstance.APIKey,
		"proxy":                      config.ConfigInstance.Proxy,
		"chatDelete":                 config.ConfigInstance.ChatDelete,
		"maxChatHistoryLength":       config.ConfigInstance.MaxChatHistoryLength,
		"internalRetryCount":         config.NormalizeInternalRetryCount(config.ConfigInstance.InternalRetryCount),
		"noRolePrefix":               config.ConfigInstance.NoRolePrefix,
		"promptDisableArtifacts":     config.ConfigInstance.PromptDisableArtifacts,
		"enableMirrorApi":            config.ConfigInstance.EnableMirrorApi,
		"mirrorApiPrefix":            config.ConfigInstance.MirrorApiPrefix,
		"adminPassword":              config.ConfigInstance.AdminPassword,
		"globalSystemPromptOverride": config.ConfigInstance.GlobalSystemPromptOverride,
		"globalPromptOverrideMode":   normalizePromptMode(config.ConfigInstance.GlobalPromptOverrideMode),
		"modelDefinitions":           config.ConfigInstance.ModelDefinitions,
		"requestLogRetention":        config.ConfigInstance.RequestLogRetention,
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
	resolved := GetResolvedModels()
	models := make([]string, 0, len(resolved))
	for _, item := range resolved {
		if !item.Enabled || !item.Visible {
			continue
		}
		models = append(models, item.PublicID)
	}
	return models
}

// AdminExportSessionsHandler handles exporting sessions to CSV or TXT
func AdminExportSessionsHandler(c *gin.Context) {
	format := c.Query("format")
	if format == "" {
		format = "csv"
	}

	if format != "csv" && format != "txt" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid format. Use 'csv' or 'txt'"})
		return
	}

	sessions := config.ConfigInstance.Sessions
	if len(sessions) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "No sessions to export"})
		return
	}

	var content string
	filename := "sessions." + format

	if format == "csv" {
		content = "index,session_key,org_id,cf_clearance,cookie_string\n"
		for i, s := range sessions {
			content += fmt.Sprintf("%d,%s,%s,%t,%q\n", i, s.SessionKey, s.OrgID, s.CFClearance != "", s.CookieString)
		}
	} else {
		// TXT format
		for i, s := range sessions {
			content += fmt.Sprintf("Session %d:\n", i+1)
			content += fmt.Sprintf("  Key: %s\n", s.SessionKey)
			content += fmt.Sprintf("  Org ID: %s\n", s.OrgID)
			content += fmt.Sprintf("  CF Clearance: %t\n", s.CFClearance != "")
			content += fmt.Sprintf("  Cookie String: %s\n\n", s.CookieString)
		}
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(content))
}
