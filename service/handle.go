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
	models := []map[string]interface{}{
		{"id": "claude-3-7-sonnet-20250219"},
		{"id": "claude-sonnet-4-20250514"},
		{"id": "claude-sonnet-4-6-20260217"},
		{"id": "claude-opus-4-20250514"},
	}

	extendedModels := make([]map[string]interface{}, 0, len(models)*2)
	for _, m := range models {
		// 保留原有 id
		extendedModels = append(extendedModels, m)
		// 追加 -think 版本
		if id, ok := m["id"].(string); ok {
			extendedModels = append(extendedModels, map[string]interface{}{
				"id": id + "-think",
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": extendedModels,
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
	processor.ProcessMessages(req.Messages)

	// Get model or use default
	model := getModelOrDefault(req.Model)
	index := config.Sr.NextIndex()
	
	lastError := "request failed"
	lastSessionIdx := -1
	attemptedSessions := 0
	maxAttempts := config.ConfigInstance.RetryCount
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if sessionCount := len(config.ConfigInstance.Sessions); sessionCount > 0 && maxAttempts > sessionCount {
		maxAttempts = sessionCount
	}
	
	// Attempt with retry mechanism
	for i := 0; i < maxAttempts; i++ {
		index = (index + 1) % len(config.ConfigInstance.Sessions)
		lastSessionIdx = index
		attemptedSessions++
		session, err := config.ConfigInstance.GetSessionForModel(index)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get session for model %s: %v", model, err))
			lastError = err.Error()
			if i < maxAttempts-1 {
				logger.Info("Retrying another session")
			}
			continue
		}

		logger.Info(fmt.Sprintf("Using session for model %s: %s", model, session.SessionKey))
		if i > 0 {
			processor.Prompt.Reset()
			processor.Prompt.WriteString(processor.RootPrompt.String())
		}
		// Initialize client and process request
		inputTokens, outputTokens, err := handleChatRequestWithTokens(c, session, model, processor, req.Stream)
		if err == nil {
			logRequest(c, model, index, inputTokens, outputTokens, true, startTime, "")
			return // Success, exit the retry loop
		}

		lastError = core.GetErrorMessage(err)
		if !core.IsRetryableError(err) {
			logger.Error(fmt.Sprintf("Request failed with non-retryable error on session %d: %s", index+1, lastError))
			break
		}
		if i < maxAttempts-1 {
			logger.Info(fmt.Sprintf("Retrying another session after retryable error: %s", lastError))
		}
	}

	if attemptedSessions > 1 {
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
	processor.ProcessMessages(req.Messages)

	// Get model or use default
	model := getModelOrDefault(req.Model)

	// Extract session info from auth header
	session, err := extractSessionFromAuthHeader(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid authorization: %v", err),
		})
		return
	}

	// Process the request with the provided session
	if err := handleChatRequest(c, session, model, processor, req.Stream); err != nil {
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

func handleChatRequest(c *gin.Context, session config.SessionInfo, model string, processor *utils.ChatRequestProcessor, stream bool) error {
	_, _, err := handleChatRequestWithTokens(c, session, model, processor, stream)
	return err
}

// handleChatRequestWithTokens handles the chat request and returns token counts
func handleChatRequestWithTokens(c *gin.Context, session config.SessionInfo, model string, processor *utils.ChatRequestProcessor, stream bool) (int, int, error) {
	// Initialize the Claude client
	claudeClient := core.NewClientFromSession(session, config.ConfigInstance.Proxy, model)

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
