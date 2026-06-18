package core

import (
	"bufio"
	"claude2api/config"
	"claude2api/logger"
	"claude2api/model"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
)

type Client struct {
	SessionKey   string
	orgID        string
	client       *req.Client
	model        string
	thinkingMode string
	effortLevel  string
	defaultAttrs map[string]interface{}
}

type ResponseEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
	} `json:"content_block"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		THINKING string `json:"thinking"`
		// partial_json
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

const maxCitationSources = 10

type citationSource struct {
	Title string
	URL   string
}

type citationCollector struct {
	seen    map[string]struct{}
	sources []citationSource
}

func newCitationCollector() *citationCollector {
	return &citationCollector{
		seen: make(map[string]struct{}),
	}
}

func (c *citationCollector) AddFrom(value interface{}) {
	if c == nil || len(c.sources) >= maxCitationSources {
		return
	}

	switch typed := value.(type) {
	case map[string]interface{}:
		if rawURL := findCitationURL(typed); rawURL != "" {
			c.add(findCitationTitle(typed), rawURL)
		}
		for _, child := range typed {
			c.AddFrom(child)
			if len(c.sources) >= maxCitationSources {
				return
			}
		}
	case []interface{}:
		for _, child := range typed {
			c.AddFrom(child)
			if len(c.sources) >= maxCitationSources {
				return
			}
		}
	}
}

func (c *citationCollector) add(title string, rawURL string) {
	if c == nil || len(c.sources) >= maxCitationSources {
		return
	}
	rawURL = strings.TrimSpace(rawURL)
	if !isHTTPURL(rawURL) {
		return
	}
	if _, ok := c.seen[rawURL]; ok {
		return
	}
	c.seen[rawURL] = struct{}{}
	title = strings.TrimSpace(title)
	if title == "" {
		title = citationTitleFromURL(rawURL)
	}
	c.sources = append(c.sources, citationSource{Title: title, URL: rawURL})
}

func (c *citationCollector) Markdown() string {
	if c == nil || len(c.sources) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n\n来源：")
	for index, source := range c.sources {
		title := source.Title
		if title == "" {
			title = source.URL
		}
		builder.WriteString(fmt.Sprintf("\n%d. [%s](%s)", index+1, escapeMarkdownLinkText(title), source.URL))
	}
	return builder.String()
}

func (c *citationCollector) Annotations() []interface{} {
	if c == nil || len(c.sources) == 0 {
		return nil
	}
	annotations := make([]interface{}, 0, len(c.sources))
	for _, source := range c.sources {
		annotations = append(annotations, map[string]interface{}{
			"type": "url_citation",
			"url_citation": map[string]interface{}{
				"url":   source.URL,
				"title": source.Title,
			},
		})
	}
	return annotations
}

func findCitationURL(value map[string]interface{}) string {
	for _, key := range []string{"url", "uri", "source_url", "web_url", "href"} {
		if raw, ok := value[key].(string); ok && isHTTPURL(raw) {
			return raw
		}
	}
	for key, raw := range value {
		key = strings.ToLower(key)
		if !strings.Contains(key, "url") && !strings.Contains(key, "uri") && !strings.Contains(key, "href") {
			continue
		}
		if rawString, ok := raw.(string); ok && isHTTPURL(rawString) {
			return rawString
		}
	}
	return ""
}

func findCitationTitle(value map[string]interface{}) string {
	for _, key := range []string{"title", "name", "source_title", "site_name", "domain"} {
		if raw, ok := value[key].(string); ok && strings.TrimSpace(raw) != "" {
			return raw
		}
	}
	return ""
}

func isHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func citationTitleFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	return parsed.Host
}

func escapeMarkdownLinkText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "[", "\\[")
	text = strings.ReplaceAll(text, "]", "\\]")
	return text
}

// TokenInfo represents token usage information
type TokenInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type APIError struct {
	Message   string
	Retryable bool
}

func (e *APIError) Error() string {
	return e.Message
}

func NewAPIError(message string, retryable bool) error {
	return &APIError{
		Message:   message,
		Retryable: retryable,
	}
}

func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if IsRateLimitError(err) {
		return true
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable
	}

	lowerErr := strings.ToLower(err.Error())
	return strings.Contains(lowerErr, "rate limit") ||
		strings.Contains(lowerErr, "timeout") ||
		strings.Contains(lowerErr, "tempor") ||
		strings.Contains(lowerErr, "connection reset") ||
		strings.Contains(lowerErr, "unexpected eof")
}

func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	lowerErr := strings.ToLower(err.Error())
	return strings.Contains(lowerErr, "rate limit") ||
		strings.Contains(lowerErr, "too many requests") ||
		strings.Contains(lowerErr, "retry-after") ||
		strings.Contains(lowerErr, "429")
}

func GetErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}

	return err.Error()
}

func NewClient(sessionKey string, proxy string, model string) *Client {
	return NewClientFromSession(config.SessionInfo{SessionKey: sessionKey}, proxy, model)
}

type ClientOption func(*Client)

func WithThinkingOptions(thinkingMode string, effortLevel string) ClientOption {
	return func(c *Client) {
		c.thinkingMode = normalizeWebThinkingMode(thinkingMode)
		c.effortLevel = normalizeWebEffortLevel(effortLevel)
	}
}

func NewClientFromSession(session config.SessionInfo, proxy string, model string, opts ...ClientOption) *Client {
	client := req.C().ImpersonateChrome().SetTimeout(time.Minute * 5)
	client.Transport.SetResponseHeaderTimeout(time.Second * 10)
	if proxy != "" {
		client.SetProxyURL(proxy)
	}
	// Set common headers
	headers := map[string]string{
		"accept":                    "text/event-stream, text/event-stream",
		"accept-language":           "zh-CN,zh;q=0.9",
		"anthropic-client-platform": "web_claude_ai",
		"content-type":              "application/json",
		"origin":                    "https://claude.ai",
		"priority":                  "u=1, i",
	}
	for key, value := range headers {
		client.SetCommonHeader(key, value)
	}
	applySessionCookies(client, session)
	// Create default client with session key
	c := &Client{
		SessionKey: session.SessionKey,
		client:     client,
		model:      model,
		defaultAttrs: map[string]interface{}{
			"personalized_styles": []map[string]interface{}{
				{
					"type":       "default",
					"key":        "Default",
					"name":       "Normal",
					"nameKey":    "normal_style_name",
					"prompt":     "Normal",
					"summary":    "Default responses from Claude",
					"summaryKey": "normal_style_summary",
					"isDefault":  true,
				},
			},
			"tools": []map[string]interface{}{
				{
					"type": "web_search_v0",
					"name": "web_search",
				},
				{"type": "artifacts_v0", "name": "artifacts"},
				{"type": "repl_v0", "name": "repl"},
			},
			"parent_message_uuid": "00000000-0000-4000-8000-000000000000",
			"attachments":         []interface{}{},
			"files":               []interface{}{},
			"sync_sources":        []interface{}{},
			"rendering_mode":      "messages",
			"timezone":            "America/Los_Angeles",
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func normalizeWebThinkingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "enabled", "on", "extended", "thinking":
		return "extended"
	case "auto", "adaptive":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return ""
	}
}

func normalizeWebEffortLevel(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "max":
		return strings.ToLower(strings.TrimSpace(effort))
	case "maximum":
		return "max"
	default:
		return ""
	}
}

func isUnsupportedRequestShapeStatus(statusCode int) bool {
	return statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity
}

func applySessionCookies(client *req.Client, session config.SessionInfo) {
	cookies := []*http.Cookie{
		{
			Name:  "sessionKey",
			Value: session.SessionKey,
		},
	}

	if session.CFClearance != "" {
		cookies = append(cookies, &http.Cookie{
			Name:  "cf_clearance",
			Value: session.CFClearance,
		})
		logger.Info("Applied cf_clearance cookie for Claude session")
	}

	for _, cookie := range parseCookieString(session.CookieString) {
		cookies = append(cookies, cookie)
	}

	client.SetCommonCookies(cookies...)
}

func parseCookieString(raw string) []*http.Cookie {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ";")
	cookies := make([]*http.Cookie, 0, len(parts))
	seen := map[string]struct{}{
		"sessionKey": {},
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		segments := strings.SplitN(part, "=", 2)
		if len(segments) != 2 {
			continue
		}

		name := strings.TrimSpace(segments[0])
		value := strings.TrimSpace(segments[1])
		if name == "" || value == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		cookies = append(cookies, &http.Cookie{Name: name, Value: value})
	}

	return cookies
}

// SetOrgID sets the organization ID for the client
func (c *Client) SetOrgID(orgID string) {
	c.orgID = orgID
}
func (c *Client) GetOrgID() (string, error) {
	url := "https://claude.ai/api/organizations"
	resp, err := c.client.R().
		SetHeader("referer", "https://claude.ai/new").
		Get(url)
	if err != nil {
		return "", NewAPIError(fmt.Sprintf("request failed: %v", err), true)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", NewAPIError(fmt.Sprintf("failed to get organizations: unauthorized status code %d", resp.StatusCode), false)
	}
	if resp.StatusCode != http.StatusOK {
		return "", NewAPIError(fmt.Sprintf("failed to get organizations: unexpected status code %d", resp.StatusCode), resp.StatusCode >= http.StatusInternalServerError)
	}
	type OrgResponse []struct {
		ID            int    `json:"id"`
		UUID          string `json:"uuid"`
		Name          string `json:"name"`
		RateLimitTier string `json:"rate_limit_tier"`
	}

	var orgs OrgResponse
	if err := json.Unmarshal(resp.Bytes(), &orgs); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(orgs) == 0 {
		return "", errors.New("no organizations found")
	}
	if len(orgs) == 1 {
		return orgs[0].UUID, nil
	}
	for _, org := range orgs {
		if org.RateLimitTier == "default_claude_ai" || org.RateLimitTier == "default_claude_max_20x" || org.RateLimitTier == "default_raven_enterprise" {
			return org.UUID, nil
		}
	}
	return "", errors.New("no default organization found")

}

// CreateConversation creates a new conversation and returns its UUID
func (c *Client) CreateConversation() (string, error) {
	if c.orgID == "" {
		return "", errors.New("organization ID not set")
	}
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations", c.orgID)
	thinkingMode := c.thinkingMode
	hasThinkingSuffix := strings.HasSuffix(c.model, "-think")
	// 如果以-think结尾
	if strings.HasSuffix(c.model, "-think") {
		c.model = strings.TrimSuffix(c.model, "-think")
	}
	if hasThinkingSuffix || thinkingMode != "" {
		if thinkingMode == "" {
			thinkingMode = "extended"
		}
		c.thinkingMode = thinkingMode
		if err := c.UpdateUserSetting("paprika_mode", thinkingMode); err != nil {
			logger.Error(fmt.Sprintf("Failed to update paprika_mode: %v", err))
		}
	} else {
		if err := c.UpdateUserSetting("paprika_mode", nil); err != nil {
			logger.Error(fmt.Sprintf("Failed to update paprika_mode: %v", err))
		}
	}
	if c.effortLevel != "" {
		if err := c.UpdateUserSetting("effort_level", c.effortLevel); err != nil {
			logger.Error(fmt.Sprintf("Failed to update effort_level: %v", err))
		}
	} else {
		if err := c.UpdateUserSetting("effort_level", nil); err != nil {
			logger.Error(fmt.Sprintf("Failed to clear effort_level: %v", err))
		}
	}
	requestBody := map[string]interface{}{
		"model":                            c.model,
		"uuid":                             uuid.New().String(),
		"name":                             "",
		"include_conversation_preferences": true,
	}
	if thinkingMode != "" {
		requestBody["thinking_mode"] = thinkingMode
	}
	if c.effortLevel != "" {
		requestBody["effort_level"] = c.effortLevel
	}
	if c.model == "claude-sonnet-4-20250514" || c.model == "claude-sonnet-4-6-20260217" {
		// 删除model - 免费模型不需要发送model字段
		delete(requestBody, "model")
	}

	resp, err := c.client.R().
		SetHeader("referer", "https://claude.ai/new").
		SetBody(requestBody).
		Post(url)
	if err != nil {
		return "", NewAPIError(fmt.Sprintf("request failed: %v", err), true)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", NewAPIError(fmt.Sprintf("failed to create conversation: unauthorized status code %d", resp.StatusCode), false)
	}
	if isUnsupportedRequestShapeStatus(resp.StatusCode) && (thinkingMode != "" || c.effortLevel != "") {
		delete(requestBody, "thinking_mode")
		delete(requestBody, "effort_level")
		resp, err = c.client.R().
			SetHeader("referer", "https://claude.ai/new").
			SetBody(requestBody).
			Post(url)
		if err != nil {
			return "", NewAPIError(fmt.Sprintf("request failed: %v", err), true)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return "", NewAPIError(fmt.Sprintf("failed to create conversation: unauthorized status code %d", resp.StatusCode), false)
		}
	}
	if resp.StatusCode != http.StatusCreated {
		return "", NewAPIError(fmt.Sprintf("failed to create conversation: unexpected status code %d", resp.StatusCode), resp.StatusCode >= http.StatusInternalServerError)
	}
	var result map[string]interface{}
	// logger.Info(fmt.Sprintf("create conversation response: %s", resp.String()))
	if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	logger.Info(fmt.Sprintf("create conversation response: %s", resp.String()))
	uuid, ok := result["uuid"].(string)
	if !ok {
		return "", errors.New("conversation UUID not found in response")
	}
	return uuid, nil
}

// SendMessage sends a message to a conversation and returns token info and error
func (c *Client) SendMessage(conversationID string, message string, stream bool, gc *gin.Context) (*TokenInfo, error) {
	if c.orgID == "" {
		return nil, errors.New("organization ID not set")
	}
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s/completion",
		c.orgID, conversationID)
	// Create request body with default attributes
	requestBody := c.defaultAttrs
	requestBody["prompt"] = message
	if c.thinkingMode != "" {
		requestBody["thinking_mode"] = c.thinkingMode
	}
	if c.effortLevel != "" {
		requestBody["effort_level"] = c.effortLevel
	}
	if c.model != "claude-sonnet-4-20250514" && c.model != "claude-sonnet-4-6-20260217" {
		requestBody["model"] = c.model
	}
	// Set up streaming response
	resp, err := c.client.R().DisableAutoReadResponse().
		SetHeader("referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID)).
		SetHeader("accept", "text/event-stream, text/event-stream").
		SetHeader("anthropic-client-platform", "web_claude_ai").
		SetHeader("cache-control", "no-cache").
		SetBody(requestBody).
		Post(url)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("request failed: %v", err), true)
	}
	if isUnsupportedRequestShapeStatus(resp.StatusCode) && (c.thinkingMode != "" || c.effortLevel != "") {
		if resp.Body != nil {
			resp.Body.Close()
		}
		delete(requestBody, "thinking_mode")
		delete(requestBody, "effort_level")
		resp, err = c.client.R().DisableAutoReadResponse().
			SetHeader("referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID)).
			SetHeader("accept", "text/event-stream, text/event-stream").
			SetHeader("anthropic-client-platform", "web_claude_ai").
			SetHeader("cache-control", "no-cache").
			SetBody(requestBody).
			Post(url)
		if err != nil {
			return nil, NewAPIError(fmt.Sprintf("request failed: %v", err), true)
		}
	}
	logger.Info(fmt.Sprintf("Claude response status code: %d", resp.StatusCode))
	if resp.StatusCode == http.StatusTooManyRequests {
		// Try to parse rate limit reset time from headers or response body
		resetInfo := parseRateLimitReset(resp)
		if resetInfo != "" {
			return nil, NewAPIError(fmt.Sprintf("rate limit exceeded - reset at: %s", resetInfo), true)
		}
		return nil, NewAPIError("rate limit exceeded", true)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if len(bodyBytes) > 0 {
			return nil, NewAPIError(fmt.Sprintf("authentication failed: %s", string(bodyBytes)), false)
		}
		return nil, NewAPIError(fmt.Sprintf("authentication failed with status code %d", resp.StatusCode), false)
	}
	if resp.StatusCode != http.StatusOK {
		// Try to read error body for more details
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if len(bodyBytes) > 0 {
			return nil, NewAPIError(fmt.Sprintf("unexpected status code: %d, response: %s", resp.StatusCode, string(bodyBytes)), resp.StatusCode >= http.StatusInternalServerError)
		}
		return nil, NewAPIError(fmt.Sprintf("unexpected status code: %d", resp.StatusCode), resp.StatusCode >= http.StatusInternalServerError)
	}
	return c.HandleResponse(resp.Body, stream, gc)
}

// parseRateLimitReset attempts to extract rate limit reset time from response
func parseRateLimitReset(resp *req.Response) string {
	// Try standard Retry-After header (seconds or date)
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		// Check if it's a number (seconds) or a date
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			resetTime := time.Now().Add(time.Duration(seconds) * time.Second)
			return resetTime.Format("15:04:05 MST")
		}
		// It might be a date string
		return retryAfter
	}

	// Try x-ratelimit-reset header
	if reset := resp.Header.Get("x-ratelimit-reset"); reset != "" {
		return reset
	}

	// Try other common rate limit headers
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		return reset
	}

	// Try to parse from response body
	if resp.Body != nil {
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && len(bodyBytes) > 0 {
			var body map[string]interface{}
			if json.Unmarshal(bodyBytes, &body) == nil {
				// Check common fields
				if reset, ok := body["reset_at"].(string); ok {
					return reset
				}
				if reset, ok := body["retry_after"].(string); ok {
					return reset
				}
				if reset, ok := body["error"].(map[string]interface{}); ok {
					if msg, ok := reset["message"].(string); ok {
						return msg
					}
				}
			}
		}
	}

	return ""
}

// HandleResponse converts Claude's SSE format to OpenAI format and writes to the response writer
func (c *Client) HandleResponse(body io.ReadCloser, stream bool, gc *gin.Context) (*TokenInfo, error) {
	defer body.Close()
	// Set headers for streaming
	if stream {
		gc.Writer.Header().Set("Content-Type", "text/event-stream")
		gc.Writer.Header().Set("Cache-Control", "no-cache")
		gc.Writer.Header().Set("Connection", "keep-alive")
		// 发送200状态码
		gc.Writer.WriteHeader(http.StatusOK)
		gc.Writer.Flush()
	}
	scanner := bufio.NewScanner(body)
	clientDone := gc.Request.Context().Done()
	// Keep track of the full response for the final message
	thinkingShown := false
	res_all_text := ""
	partial_json_shown := false
	useTool := false
	useToolEnd := false
	nextLanguage := false
	languageStr := "md"
	citations := newCitationCollector()
	// Token tracking
	inputTokens := 0
	outputTokens := 0
	for scanner.Scan() {
		select {
		case <-clientDone:
			// 客户端已断开连接，清理资源并退出
			logger.Info("Client closed connection")
			return &TokenInfo{InputTokens: inputTokens, OutputTokens: outputTokens}, nil
		default:
			// 继续处理响应
		}
		line := scanner.Text()
		// logger.Info(fmt.Sprintf("Claude SSE line: %s", line))
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}
		var rawEvent map[string]interface{}
		if err := json.Unmarshal([]byte(data), &rawEvent); err == nil {
			citations.AddFrom(rawEvent)
		}
		var event ResponseEvent
		if err := json.Unmarshal([]byte(data), &event); err == nil {
			if event.Type == "error" && event.Error.Message != "" {
				model.ReturnOpenAIResponse(event.Error.Message, stream, gc)
				return &TokenInfo{InputTokens: inputTokens, OutputTokens: outputTokens}, nil
			}
			if event.ContentBlock.Type == "tool_use" {
				useTool = true
			}
			if event.ContentBlock.Type == "tool_result" {
				useToolEnd = true
			}
			if event.Type == "content_block_stop" {
				res_text := ""
				if thinkingShown {
					res_text = "</think>\n"
					thinkingShown = false
				}
				if partial_json_shown {
					res_text = "\n```\n"
					partial_json_shown = false
				}
				res_all_text += res_text
				if !stream {
					continue
				}
				model.ReturnOpenAIResponse(res_text, stream, gc)
				continue
			}
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				res_text := event.Delta.Text
				res_all_text += res_text
				if !stream {
					continue
				}
				model.ReturnOpenAIResponse(res_text, stream, gc)
				continue
			}
			if event.Delta.Type == "thinking_delta" {
				res_text := event.Delta.THINKING
				if !thinkingShown {
					res_text = "<think> " + res_text
					thinkingShown = true
				}
				res_all_text += res_text
				if !stream {
					continue
				}
				model.ReturnOpenAIResponse(res_text, stream, gc)
				continue
			}
			if event.Delta.Type == "input_json_delta" {
				res_text := event.Delta.PartialJSON
				//结束使用工具了
				if useTool && res_text == ",\"content\":" {
					useTool = false
					partial_json_shown = false
					continue
				}
				//获取语言,下一次就是了
				if res_text == ",\"language\":" || res_text == ",\"type\":" {
					nextLanguage = true
					continue
				}
				//获取语言注入
				if nextLanguage {
					languageStr = res_text[1:]
					logger.Info(fmt.Sprintf("获取的语言为:%s", languageStr))
					if languageStr == "text/html" {
						languageStr = "html"
					}
					nextLanguage = false
				}
				//使用工具
				if useTool {
					logger.Info(fmt.Sprintf("useTool res_text:%s", res_text))
					continue
				}
				//使用了工具结束拉
				if useToolEnd {
					useToolEnd = false
					continue
				}
				//存在代码首字母为"的情况,特殊处理
				if strings.HasPrefix(res_text, "\"") {
					res_text = res_text[1:]
				}
				//可能会存在多出一个}的情况
				if res_text == "\"}" || res_text == "}" {
					res_text = ""
				}
				//转义
				unquote, err := strconv.Unquote(fmt.Sprintf("\"%s\"", res_text))
				if err == nil {
					res_text = unquote
				} else {
					logger.Error(fmt.Sprintf("转化出错:%s", err.Error()))
					res_text = strings.ReplaceAll(res_text, "\\\\n", "")
					res_text = strings.ReplaceAll(res_text, "\\\\u", "\\u")
					res_text = strings.ReplaceAll(res_text, "\\\"", "\"")
					res_text = strings.ReplaceAll(res_text, "\\\\'", "'")
					res_text = strings.ReplaceAll(res_text, "\\n", "\n")
					res_text = strings.ReplaceAll(res_text, "\\t", "\t")
					res_text = decodeUnicodeEscape(res_text)
				}

				if !partial_json_shown {
					res_text = "\n```" + languageStr + "\n" + res_text
					partial_json_shown = true
				}
				res_all_text += res_text
				if !stream {
					continue
				}
				model.ReturnOpenAIResponse(res_text, stream, gc)
				continue
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}
	sourceMarkdown := citations.Markdown()
	if sourceMarkdown != "" {
		res_all_text += sourceMarkdown
		if stream {
			model.ReturnOpenAIResponse(sourceMarkdown, stream, gc)
		}
	}
	if !stream {
		model.ReturnOpenAIResponseWithAnnotations(res_all_text, stream, citations.Annotations(), gc)
	} else {
		// 发送结束标志
		gc.Writer.Write([]byte("data: [DONE]\n\n"))
		gc.Writer.Flush()
	}

	// Estimate tokens: roughly 4 characters per token for most languages
	inputTokens = len(res_all_text) / 4
	outputTokens = len(res_all_text) / 4

	return &TokenInfo{InputTokens: inputTokens, OutputTokens: outputTokens}, nil
}
func decodeUnicodeEscape(s string) string {
	var result []rune
	for i := 0; i < len(s); i++ {
		// 检查是否是 Unicode 转义序列
		if len(s)-i >= 6 && s[i:i+2] == "\\u" {
			// 尝试解析 Unicode 码点
			code, err := strconv.ParseInt(s[i+2:i+6], 16, 32)
			if err == nil {
				// 将码点转换为字符
				result = append(result, rune(code))
				// 跳过已处理的 Unicode 转义序列
				i += 5
			} else {
				// 如果解析失败，保留原始字符
				result = append(result, rune(s[i]))
			}
		} else {
			result = append(result, rune(s[i]))
		}
	}
	return string(result)
}

// DeleteConversation deletes a conversation by ID
func (c *Client) DeleteConversation(conversationID string) error {
	if c.orgID == "" {
		return errors.New("organization ID not set")
	}
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s",
		c.orgID, conversationID)
	requestBody := map[string]string{
		"uuid": conversationID,
	}
	resp, err := c.client.R().
		SetHeader("referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID)).
		SetBody(requestBody).
		Delete(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

// UploadFile uploads files to Claude and adds them to the client's default attributes
// fileData should be in the format: data:image/jpeg;base64,/9j/4AA...
func (c *Client) UploadFile(fileData []string) error {
	if c.orgID == "" {
		return errors.New("organization ID not set")
	}
	if len(fileData) == 0 {
		return errors.New("empty file data")
	}

	// Initialize files array in default attributes if it doesn't exist
	if _, ok := c.defaultAttrs["files"]; !ok {
		c.defaultAttrs["files"] = []interface{}{}
	}

	// Process each file
	for _, fd := range fileData {
		if fd == "" {
			continue // Skip empty entries
		}

		// Parse the base64 data
		parts := strings.SplitN(fd, ",", 2)
		if len(parts) != 2 {
			return errors.New("invalid file data format")
		}

		// Get the content type from the data URI
		metaParts := strings.SplitN(parts[0], ":", 2)
		if len(metaParts) != 2 {
			return errors.New("invalid content type in file data")
		}

		metaInfo := strings.SplitN(metaParts[1], ";", 2)
		if len(metaInfo) != 2 || metaInfo[1] != "base64" {
			return errors.New("invalid encoding in file data")
		}

		contentType := metaInfo[0]

		// Decode the base64 data
		fileBytes, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return fmt.Errorf("failed to decode base64 data: %w", err)
		}

		// Determine filename based on content type
		var filename string
		switch contentType {
		case "image/jpeg":
			filename = "image.jpg"
		case "image/png":
			filename = "image.png"
		case "application/pdf":
			filename = "document.pdf"
		default:
			filename = "file"
		}

		// Create the upload URL
		url := fmt.Sprintf("https://claude.ai/api/%s/upload", c.orgID)

		// Create a multipart form request
		resp, err := c.client.R().
			SetHeader("referer", "https://claude.ai/new").
			SetHeader("anthropic-client-platform", "web_claude_ai").
			SetFileBytes("file", filename, fileBytes).
			SetContentType("multipart/form-data").
			Post(url)

		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, resp.String())
		}

		// Parse the response
		var result struct {
			FileUUID string `json:"file_uuid"`
		}

		if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if result.FileUUID == "" {
			return errors.New("file UUID not found in response")
		}

		// Add file to default attributes
		c.defaultAttrs["files"] = append(c.defaultAttrs["files"].([]interface{}), result.FileUUID)
	}

	return nil
}

func (c *Client) SetBigContext(context string) {
	c.defaultAttrs["attachments"] = []map[string]interface{}{
		{
			"file_name":         "context.txt",
			"file_type":         "text/plain",
			"file_size":         len(context),
			"extracted_content": context,
		},
	}

}

// / UpdateUserSetting updates a single user setting on Claude.ai while preserving all other settings
func (c *Client) UpdateUserSetting(key string, value interface{}) error {
	url := "https://claude.ai/api/account?statsig_hashing_algorithm=djb2"

	// Default settings structure with all possible fields
	settings := map[string]interface{}{
		"input_menu_pinned_items":          nil,
		"has_seen_mm_examples":             nil,
		"has_seen_starter_prompts":         nil,
		"has_started_claudeai_onboarding":  true,
		"has_finished_claudeai_onboarding": true,
		"dismissed_claudeai_banners":       []interface{}{},
		"dismissed_artifacts_announcement": nil,
		"preview_feature_uses_artifacts":   nil,
		"preview_feature_uses_latex":       nil,
		"preview_feature_uses_citations":   nil,
		"preview_feature_uses_harmony":     nil,
		"enabled_artifacts_attachments":    true,
		"enabled_turmeric":                 nil,
		"enable_chat_suggestions":          nil,
		"dismissed_artifact_feedback_form": nil,
		"enabled_mm_pdfs":                  nil,
		"enabled_gdrive":                   nil,
		"enabled_bananagrams":              nil,
		"enabled_gdrive_indexing":          nil,
		"enabled_web_search":               true,
		"enabled_compass":                  nil,
		"enabled_sourdough":                nil,
		"enabled_foccacia":                 nil,
		"dismissed_claude_code_spotlight":  nil,
		"enabled_geolocation":              nil,
		"enabled_mcp_tools":                nil,
		"paprika_mode":                     nil,
		"thinking_mode":                    nil,
		"effort_level":                     nil,
		"enabled_monkeys_in_a_barrel":      nil,
	}

	// Update the specified setting
	if _, exists := settings[key]; exists {
		settings[key] = value
		logger.Info(fmt.Sprintf("Updating setting %s to %v", key, value))
	} else {
		return fmt.Errorf("unknown setting key: %s", key)
	}

	// Create request body
	requestBody := map[string]interface{}{
		"settings": settings,
	}

	// Make the request
	resp, err := c.client.R().
		SetHeader("referer", "https://claude.ai/new").
		SetHeader("origin", "https://claude.ai").
		SetHeader("anthropic-client-platform", "web_claude_ai").
		SetHeader("cache-control", "no-cache").
		SetHeader("pragma", "no-cache").
		SetHeader("priority", "u=1, i").
		SetBody(requestBody).
		Put(url)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != 202 {
		return fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, resp.String())
	}

	// logger.Info(fmt.Sprintf("Successfully updated user setting %s: %s", key, resp.String()))
	return nil
}
