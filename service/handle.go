package service

import (
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"claude2api/model"
	"claude2api/utils"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthCheckHandler handles the health check endpoint
func HealthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

func MoudlesHandler(c *gin.Context) {
	resolved := GetResolvedModels()
	models := make([]map[string]interface{}, 0, len(resolved))
	for _, item := range resolved {
		if !item.Enabled || !item.Visible {
			continue
		}
		models = append(models, map[string]interface{}{
			"id": item.PublicID,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": models,
	})
}

// ChatCompletionsHandler handles the chat completions endpoint
func ChatCompletionsHandler(c *gin.Context) {
	startTime := time.Now()

	useMirror, exist := c.Get("UseMirrorApi")
	if exist && useMirror.(bool) {
		MirrorChatHandler(c)
		return
	}

	// Parse and validate request
	req, err := parseAndValidateRequest(c)
	if err != nil {
		logRequest(c, "", -1, 0, 0, false, startTime, "Invalid request: "+err.Error())
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}
	c.Set("request_message_count", len(req.Messages))

	// Process messages into prompt and extract images
	processor := utils.NewChatRequestProcessor()

	// Get model or use default
	selectedModel := ResolveModel(getModelOrDefault(req.Model))
	applyRequestThinkingOptions(&selectedModel, req)
	promptOverride, promptMode := resolvePromptOverride(selectedModel)
	processor.SetPromptOverride(promptOverride, promptMode)
	processor.ProcessMessages(req.Messages)
	model := selectedModel.UpstreamID
	if selectedModel.Thinking {
		model += "-think"
	}
	sessionCount := len(config.ConfigInstance.Sessions)
	if sessionCount == 0 {
		lastError := "no Claude sessions configured"
		logger.Error(lastError)
		logRequest(c, model, -1, 0, 0, false, startTime, lastError)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: lastError,
		})
		return
	}
	startIndex := config.Sr.NextIndex()

	lastError := "request failed"
	lastSessionIdx := -1
	attemptedSessions := 0
	maxAttempts := config.ConfigInstance.RetryCount
	if maxAttempts <= 0 {
		maxAttempts = sessionCount
	}
	if maxAttempts > sessionCount {
		maxAttempts = sessionCount
	}

	// Attempt with retry mechanism
	for scannedSessions := 0; scannedSessions < sessionCount && attemptedSessions < maxAttempts; scannedSessions++ {
		index := (startIndex + scannedSessions) % sessionCount
		if cooldownUntil, coolingDown := config.ConfigInstance.GetSessionCooldownByIndex(index, time.Now()); coolingDown {
			lastError = fmt.Sprintf("session S%d is cooling down until %s after rate limit", index+1, formatChinaTime(cooldownUntil))
			logger.Info(fmt.Sprintf("Skipping session %d due to rate limit cooldown until %s", index+1, formatChinaTime(cooldownUntil)))
			continue
		}

		lastSessionIdx = index
		attemptedSessions++
		session, err := config.ConfigInstance.GetSessionForModel(index)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get session for model %s: %v", model, err))
			lastError = err.Error()
			if attemptedSessions < maxAttempts {
				logger.Info("Retrying another session")
			}
			continue
		}

		logger.Info(fmt.Sprintf("Using session for model %s (requested: %s): %s", model, selectedModel.RequestedModel, session.SessionKey))
		if attemptedSessions > 1 {
			processor.Prompt.Reset()
			processor.Prompt.WriteString(processor.RootPrompt.String())
		}
		// Initialize client and process request
		inputTokens, outputTokens, err := handleChatRequestWithTokens(c, session, model, processor, req.Stream, selectedModel.ThinkingMode, selectedModel.EffortLevel)
		if err == nil {
			logRequest(c, model, index, inputTokens, outputTokens, true, startTime, "")
			return // Success, exit the retry loop
		}

		lastError = core.GetErrorMessage(err)
		rateLimited := core.IsRateLimitError(err)
		if rateLimited {
			cooldownUntil := time.Time{}
			now := time.Now()
			if resetAt, ok := core.GetRateLimitResetAt(err); ok {
				cooldownUntil = config.ConfigInstance.CooldownSessionAfterRateLimit(session.SessionKey, resetAt, now)
			} else {
				cooldownUntil = config.ConfigInstance.CooldownSessionAfterRateLimit(session.SessionKey, time.Time{}, now)
			}
			lastError = fmt.Sprintf("rate limit exceeded - reset at: %s 中国时间", formatChinaTime(cooldownUntil))
			logger.Error(fmt.Sprintf(
				"Session %d (%s) hit rate limit; cooling down until %s",
				index+1,
				maskSessionKey(session.SessionKey),
				formatChinaTime(cooldownUntil),
			))
		}
		if !core.IsRetryableError(err) {
			logger.Error(fmt.Sprintf("Request failed with non-retryable error on session %d: %s", index+1, lastError))
			break
		}
		if rateLimited && attemptedSessions >= maxAttempts && scannedSessions+1 < sessionCount {
			maxAttempts++
		}
		if attemptedSessions < maxAttempts {
			logger.Info(fmt.Sprintf("Retrying another session after retryable error: %s", lastError))
		}
	}

	if attemptedSessions == 0 {
		lastError = "all Claude sessions are cooling down after rate limits"
		logger.Error(lastError)
	} else if attemptedSessions > 1 {
		logger.Error(fmt.Sprintf("Failed after %d attempts", attemptedSessions))
	} else {
		logger.Error("Request failed")
	}
	logRequest(c, model, lastSessionIdx, 0, 0, false, startTime, lastError)
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: lastError,
	})
}

func MirrorChatHandler(c *gin.Context) {
	if !config.ConfigInstance.EnableMirrorApi {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "Mirror API is not enabled",
		})
		return
	}

	// Parse and validate request
	req, err := parseAndValidateRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}
	c.Set("request_message_count", len(req.Messages))

	// Process messages into prompt and extract images
	processor := utils.NewChatRequestProcessor()

	// Get model or use default
	selectedModel := ResolveModel(getModelOrDefault(req.Model))
	applyRequestThinkingOptions(&selectedModel, req)
	promptOverride, promptMode := resolvePromptOverride(selectedModel)
	processor.SetPromptOverride(promptOverride, promptMode)
	processor.ProcessMessages(req.Messages)
	model := selectedModel.UpstreamID
	if selectedModel.Thinking {
		model += "-think"
	}

	// Extract session info from auth header
	session, err := extractSessionFromAuthHeader(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid authorization: %v", err),
		})
		return
	}

	// Process the request with the provided session
	if err := handleChatRequest(c, session, model, processor, req.Stream, selectedModel.ThinkingMode, selectedModel.EffortLevel); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: core.GetErrorMessage(err),
		})
		return
	}
}

// Helper functions

func parseAndValidateRequest(c *gin.Context) (*model.ChatCompletionRequest, error) {
	var req model.ChatCompletionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid request: %v", err),
		})
		return nil, err
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "No messages provided",
		})
		return nil, fmt.Errorf("no messages provided")
	}

	return &req, nil
}

func getModelOrDefault(model string) string {
	if model == "" {
		return "claude-3-7-sonnet-20250219"
	}
	return model
}

func applyRequestThinkingOptions(selected *ResolvedModelSelection, req *model.ChatCompletionRequest) {
	if selected == nil || req == nil {
		return
	}

	if effort := normalizeEffortLevel(req.ReasoningEffort); effort != "" {
		selected.Thinking = true
		selected.EffortLevel = effort
	}
	if effort := normalizeOutputConfigEffort(req.OutputConfig); effort != "" {
		selected.Thinking = true
		selected.EffortLevel = effort
	}

	if mode, ok := normalizeThinkingMode(req.Thinking); ok {
		if mode == "" {
			selected.Thinking = false
			selected.ThinkingMode = ""
			selected.EffortLevel = ""
			return
		}
		selected.Thinking = true
		selected.ThinkingMode = mode
	}

	if selected.Thinking && selected.ThinkingMode == "" {
		selected.ThinkingMode = "extended"
	}
}

func normalizeOutputConfigEffort(outputConfig map[string]interface{}) string {
	if outputConfig == nil {
		return ""
	}
	if effort, ok := outputConfig["effort"].(string); ok {
		return normalizeEffortLevel(effort)
	}
	return ""
}

func normalizeEffortLevel(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "max":
		return strings.ToLower(strings.TrimSpace(effort))
	case "maximum":
		return "max"
	default:
		return ""
	}
}

func normalizeThinkingMode(thinking map[string]interface{}) (string, bool) {
	if thinking == nil {
		return "", false
	}
	if enabled, ok := thinking["enabled"].(bool); ok {
		if enabled {
			return "extended", true
		}
		return "", true
	}
	for _, key := range []string{"type", "mode", "thinking_mode"} {
		value, ok := thinking[key].(string)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "enabled", "on", "extended", "thinking":
			return "extended", true
		case "auto", "adaptive":
			return strings.ToLower(strings.TrimSpace(value)), true
		case "disabled", "off", "none":
			return "", true
		}
	}
	return "", false
}

func extractSessionFromAuthHeader(c *gin.Context) (config.SessionInfo, error) {
	authInfo := c.Request.Header.Get("Authorization")
	authInfo = strings.TrimPrefix(authInfo, "Bearer ")

	if authInfo == "" {
		return config.SessionInfo{SessionKey: "", OrgID: ""}, fmt.Errorf("missing authorization header")
	}

	if strings.Contains(authInfo, ":") {
		parts := strings.Split(authInfo, ":")
		return config.SessionInfo{SessionKey: parts[0], OrgID: parts[1]}, nil
	}

	return config.SessionInfo{SessionKey: authInfo, OrgID: ""}, nil
}

func resolvePromptOverride(selectedModel ResolvedModelSelection) (string, string) {
	if strings.TrimSpace(selectedModel.SystemPromptOverride) != "" {
		return selectedModel.SystemPromptOverride, selectedModel.PromptOverrideMode
	}
	if strings.TrimSpace(config.ConfigInstance.GlobalSystemPromptOverride) != "" {
		return config.ConfigInstance.GlobalSystemPromptOverride, normalizePromptMode(config.ConfigInstance.GlobalPromptOverrideMode)
	}
	return "", "append"
}

func handleChatRequest(c *gin.Context, session config.SessionInfo, model string, processor *utils.ChatRequestProcessor, stream bool, thinkingMode string, effortLevel string) error {
	_, _, err := handleChatRequestWithTokens(c, session, model, processor, stream, thinkingMode, effortLevel)
	return err
}

// handleChatRequestWithTokens handles the chat request and returns token counts
func handleChatRequestWithTokens(c *gin.Context, session config.SessionInfo, model string, processor *utils.ChatRequestProcessor, stream bool, thinkingMode string, effortLevel string) (int, int, error) {
	// Initialize the Claude client
	claudeClient := core.NewClientFromSession(session, config.ConfigInstance.Proxy, model, core.WithThinkingOptions(thinkingMode, effortLevel))

	// Get org ID if not already set
	if session.OrgID == "" {
		orgId, err := claudeClient.GetOrgID()
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get org ID: %v", err))
			return 0, 0, fmt.Errorf("failed to get org ID: %w", err)
		}
		session.OrgID = orgId
		config.ConfigInstance.SetSessionOrgID(session.SessionKey, session.OrgID)
	}

	claudeClient.SetOrgID(session.OrgID)

	// Upload images if any
	if len(processor.ImgDataList) > 0 {
		err := claudeClient.UploadFile(processor.ImgDataList)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to upload file: %v", err))
			return 0, 0, fmt.Errorf("failed to upload file: %w", err)
		}
	}

	// Handle large context if needed
	if processor.Prompt.Len() > config.ConfigInstance.MaxChatHistoryLength {
		claudeClient.SetBigContext(processor.Prompt.String())
		processor.ResetForBigContext()
		logger.Info(fmt.Sprintf("Prompt length exceeds max limit (%d), using file context", config.ConfigInstance.MaxChatHistoryLength))
	}

	// Create conversation
	conversationID, err := claudeClient.CreateConversation()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to create conversation: %v", err))
		return 0, 0, err
	}

	// Send message
	tokenInfo, err := claudeClient.SendMessage(conversationID, processor.Prompt.String(), stream, c)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to send message: %v", err))
		go cleanupConversation(claudeClient, conversationID, 3)
		return 0, 0, err
	}

	// Clean up conversation if enabled
	if config.ConfigInstance.ChatDelete {
		go cleanupConversation(claudeClient, conversationID, 3)
	}

	inputTokens := 0
	outputTokens := 0
	if tokenInfo != nil {
		inputTokens = tokenInfo.InputTokens
		outputTokens = tokenInfo.OutputTokens
	}

	return inputTokens, outputTokens, nil
}

func cleanupConversation(client *core.Client, conversationID string, retry int) {
	for i := 0; i < retry; i++ {
		if err := client.DeleteConversation(conversationID); err != nil {
			logger.Error(fmt.Sprintf("Failed to delete conversation: %v", err))
			time.Sleep(2 * time.Second)
			continue
		}
		logger.Info(fmt.Sprintf("Successfully deleted conversation: %s", conversationID))
		return // 成功后直接返回，不执行后面的错误日志
	}
	// 只有当所有重试都失败后，才会执行到这里
	logger.Error(fmt.Sprintf("Cleanup %s conversation %s failed after %d retries", client.SessionKey, conversationID, retry))
}

// logRequest logs the request to the global request logger
// logRequest logs the request to the global request logger
func logRequest(c *gin.Context, model string, sessionIdx int, inputTokens int, outputTokens int, success bool, startTime time.Time, errMsg string) {
	duration := time.Since(startTime).Milliseconds()

	statusCode := http.StatusOK
	if !success {
		statusCode = http.StatusInternalServerError
	}

	contextCount := 0
	if rawMessages, exists := c.Get("request_message_count"); exists {
		if count, ok := rawMessages.(int); ok {
			contextCount = count
		}
	}

	sessionLabel := "-"
	if sessionIdx >= 0 {
		sessionLabel = fmt.Sprintf("S%d", sessionIdx+1)
	}

	if sessionIdx >= 0 && sessionIdx < len(config.ConfigInstance.Sessions) {
		sessionLabel = fmt.Sprintf("S%d / %s", sessionIdx+1, maskSessionKey(config.ConfigInstance.Sessions[sessionIdx].SessionKey))
	}

	log := logger.RequestLog{
		Timestamp:    startTime,
		Method:       c.Request.Method,
		Path:         c.Request.URL.Path,
		Model:        model,
		StatusCode:   statusCode,
		Duration:     duration,
		Success:      success,
		Error:        errMsg,
		ErrorType:    classifyErrorType(errMsg),
		SessionIdx:   sessionIdx,
		SessionLabel: sessionLabel,
		IsStreaming:  c.Query("stream") == "true" || c.GetBool("stream"),
		ContextCount: contextCount,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}

	logger.GlobalRequestLogger.LogRequest(log)
}

func classifyErrorType(errMsg string) string {
	if errMsg == "" {
		return ""
	}

	lowerErr := strings.ToLower(errMsg)

	switch {
	case strings.Contains(lowerErr, "invalid request") || strings.Contains(lowerErr, "bind") || strings.Contains(lowerErr, "parse"):
		return "请求解析失败"
	case strings.Contains(lowerErr, "rate limit") || strings.Contains(lowerErr, "429") || strings.Contains(lowerErr, "retry-after"):
		return "限流"
	case strings.Contains(lowerErr, "auth") || strings.Contains(lowerErr, "unauthorized") || strings.Contains(lowerErr, "forbidden") || strings.Contains(lowerErr, "api key"):
		return "认证失败"
	case strings.Contains(lowerErr, "claude") || strings.Contains(lowerErr, "conversation") || strings.Contains(lowerErr, "send message"):
		return "Claude 接口错误"
	default:
		return "未知错误"
	}
}
