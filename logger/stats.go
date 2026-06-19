package logger

import (
	"strings"
	"sync"
	"time"
)

// RequestLog represents a single API request log entry
type RequestLog struct {
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	Model        string    `json:"model"`
	StatusCode   int       `json:"status_code"`
	Duration     int64     `json:"duration_ms"` // milliseconds
	Success      bool      `json:"success"`
	Error        string    `json:"error,omitempty"`
	ErrorType    string    `json:"error_type,omitempty"`
	SessionIdx   int       `json:"session_idx"`
	SessionLabel string    `json:"session_label,omitempty"`
	IsStreaming  bool      `json:"is_streaming"`
	ContextCount int       `json:"context_count"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
}

// Stats represents aggregated statistics
type Stats struct {
	TotalRequests   int64   `json:"total_requests"`
	SuccessRequests int64   `json:"success_requests"`
	FailedRequests  int64   `json:"failed_requests"`
	SuccessRate     float64 `json:"success_rate"`
	RPM             float64 `json:"rpm"` // requests per minute
	AvgDuration     float64 `json:"avg_duration_ms"`
}

// SessionStats represents aggregated statistics for a single session.
type SessionStats struct {
	TotalRequests               int64   `json:"total_requests"`
	SuccessRequests             int64   `json:"success_requests"`
	FailedRequests              int64   `json:"failed_requests"`
	RateLimitRequests           int64   `json:"rate_limit_requests"`
	InputTokens                 int64   `json:"input_tokens"`
	OutputTokens                int64   `json:"output_tokens"`
	TotalTokens                 int64   `json:"total_tokens"`
	AvgTokensPerSuccess         float64 `json:"avg_tokens_per_success"`
	AvgSuccessesBeforeRateLimit float64 `json:"avg_successes_before_rate_limit"`
	SuccessRate                 float64 `json:"success_rate"`
}

// RequestLogger manages request logging and statistics
type RequestLogger struct {
	logs                          []RequestLog
	mu                            sync.RWMutex
	maxLogs                       int
	startTime                     time.Time
	sessionStats                  map[int]SessionStats
	successesSinceRateLimit       map[int]int64
	successesBeforeRateLimitTotal map[int]int64
}

var GlobalRequestLogger *RequestLogger

func init() {
	GlobalRequestLogger = NewRequestLogger(1000) // Keep last 1000 requests by default
}

// NewRequestLogger creates a new request logger
func NewRequestLogger(maxLogs int) *RequestLogger {
	return &RequestLogger{
		logs:                          make([]RequestLog, 0, maxLogs),
		maxLogs:                       maxLogs,
		startTime:                     time.Now(),
		sessionStats:                  make(map[int]SessionStats),
		successesSinceRateLimit:       make(map[int]int64),
		successesBeforeRateLimitTotal: make(map[int]int64),
	}
}

// LogRequest adds a new request log entry
func (rl *RequestLogger) LogRequest(log RequestLog) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Add log
	rl.logs = append(rl.logs, log)
	rl.updateSessionStats(log)

	// Trim if exceeds max
	if len(rl.logs) > rl.maxLogs {
		rl.logs = rl.logs[len(rl.logs)-rl.maxLogs:]
	}
}

func (rl *RequestLogger) updateSessionStats(log RequestLog) {
	if log.SessionIdx < 0 {
		return
	}

	stats := rl.sessionStats[log.SessionIdx]
	stats.TotalRequests++
	stats.InputTokens += int64(log.InputTokens)
	stats.OutputTokens += int64(log.OutputTokens)
	stats.TotalTokens += int64(log.InputTokens + log.OutputTokens)
	if log.Success {
		stats.SuccessRequests++
		rl.successesSinceRateLimit[log.SessionIdx]++
	} else {
		stats.FailedRequests++
		if isRateLimitLog(log) {
			stats.RateLimitRequests++
			rl.successesBeforeRateLimitTotal[log.SessionIdx] += rl.successesSinceRateLimit[log.SessionIdx]
			rl.successesSinceRateLimit[log.SessionIdx] = 0
		}
	}

	finalizeSessionStats(&stats, rl.successesBeforeRateLimitTotal[log.SessionIdx])
	rl.sessionStats[log.SessionIdx] = stats
}

// SetMaxLogs updates the retention limit and immediately trims old entries.
func (rl *RequestLogger) SetMaxLogs(maxLogs int) {
	if maxLogs <= 0 {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.maxLogs = maxLogs
	if len(rl.logs) > rl.maxLogs {
		rl.logs = rl.logs[len(rl.logs)-rl.maxLogs:]
	}
}

// GetStats returns aggregated statistics
func (rl *RequestLogger) GetStats() Stats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	var successCount, failedCount int64
	var totalDuration int64

	for _, log := range rl.logs {
		if log.Success {
			successCount++
		} else {
			failedCount++
		}
		totalDuration += log.Duration
	}

	total := successCount + failedCount
	var successRate, rpm, avgDuration float64

	if total > 0 {
		successRate = float64(successCount) / float64(total) * 100
		avgDuration = float64(totalDuration) / float64(total)
	}

	// Calculate RPM (requests per minute)
	elapsed := time.Since(rl.startTime).Minutes()
	if elapsed > 0 {
		rpm = float64(total) / elapsed
	}

	return Stats{
		TotalRequests:   total,
		SuccessRequests: successCount,
		FailedRequests:  failedCount,
		SuccessRate:     successRate,
		RPM:             rpm,
		AvgDuration:     avgDuration,
	}
}

// GetRecentLogs returns the most recent logs
func (rl *RequestLogger) GetRecentLogs(limit int) []RequestLog {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if limit <= 0 || limit > len(rl.logs) {
		limit = len(rl.logs)
	}

	// Return last N logs in reverse order (newest first)
	result := make([]RequestLog, limit)
	for i := 0; i < limit; i++ {
		result[i] = rl.logs[len(rl.logs)-1-i]
	}
	return result
}

// GetLogsWithPagination returns logs with pagination support
// page is 1-indexed, pageSize is the number of items per page
func (rl *RequestLogger) GetLogsWithPagination(page, pageSize int) ([]RequestLog, int, bool) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	total := len(rl.logs)
	if total == 0 {
		return []RequestLog{}, 0, false
	}

	// Default values
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Calculate total pages
	totalPages := (total + pageSize - 1) / pageSize
	if page > totalPages {
		page = totalPages
	}

	// Calculate start and end indices (logs are stored oldest first, but we want newest first)
	startIdx := total - page*pageSize
	endIdx := total - (page-1)*pageSize

	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > total {
		endIdx = total
	}

	// Extract logs in reverse order (newest first)
	result := make([]RequestLog, 0, endIdx-startIdx)
	for i := endIdx - 1; i >= startIdx; i-- {
		result = append(result, rl.logs[i])
	}

	hasMore := page < totalPages

	return result, total, hasMore
}

// GetTotalCount returns the total number of logs
func (rl *RequestLogger) GetTotalCount() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.logs)
}

// GetLastSuccessBySession returns the latest successful request time for each session index.
func (rl *RequestLogger) GetLastSuccessBySession() map[int]time.Time {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	result := make(map[int]time.Time)
	for _, log := range rl.logs {
		if !log.Success || log.SessionIdx < 0 {
			continue
		}
		if last, ok := result[log.SessionIdx]; !ok || log.Timestamp.After(last) {
			result[log.SessionIdx] = log.Timestamp
		}
	}
	return result
}

// GetLastErrorBySession returns the latest failed request log for each session index.
func (rl *RequestLogger) GetLastErrorBySession() map[int]RequestLog {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	result := make(map[int]RequestLog)
	for _, log := range rl.logs {
		if log.Success || log.SessionIdx < 0 {
			continue
		}
		if last, ok := result[log.SessionIdx]; !ok || log.Timestamp.After(last.Timestamp) {
			result[log.SessionIdx] = log
		}
	}
	return result
}

// GetStatsBySession returns request statistics grouped by session index.
func (rl *RequestLogger) GetStatsBySession() map[int]SessionStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	result := make(map[int]SessionStats)
	for sessionIdx, stats := range rl.sessionStats {
		result[sessionIdx] = stats
	}

	return result
}

func finalizeSessionStats(stats *SessionStats, successesBeforeRateLimitTotal int64) {
	if stats.TotalRequests > 0 {
		stats.SuccessRate = float64(stats.SuccessRequests) / float64(stats.TotalRequests) * 100
	}
	if stats.SuccessRequests > 0 {
		stats.AvgTokensPerSuccess = float64(stats.TotalTokens) / float64(stats.SuccessRequests)
	}
	if stats.RateLimitRequests > 0 {
		stats.AvgSuccessesBeforeRateLimit = float64(successesBeforeRateLimitTotal) / float64(stats.RateLimitRequests)
	}
}

func isRateLimitLog(log RequestLog) bool {
	errorType := strings.ToLower(strings.TrimSpace(log.ErrorType))
	errorText := strings.ToLower(log.Error)
	return strings.Contains(errorType, "限流") ||
		strings.Contains(errorType, "rate limit") ||
		strings.Contains(errorText, "rate limit") ||
		strings.Contains(errorText, "too many requests") ||
		strings.Contains(errorText, "retry-after") ||
		strings.Contains(errorText, "429")
}

// GetLogsByTimeRange returns logs within a time range
func (rl *RequestLogger) GetLogsByTimeRange(start, end time.Time) []RequestLog {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	var result []RequestLog
	for _, log := range rl.logs {
		if log.Timestamp.After(start) && log.Timestamp.Before(end) {
			result = append(result, log)
		}
	}
	return result
}

// Clear clears all logs
func (rl *RequestLogger) Clear() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.logs = make([]RequestLog, 0)
	rl.startTime = time.Now()
	rl.sessionStats = make(map[int]SessionStats)
	rl.successesSinceRateLimit = make(map[int]int64)
	rl.successesBeforeRateLimitTotal = make(map[int]int64)
}
