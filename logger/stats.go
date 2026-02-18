package logger

import (
	"sync"
	"time"
)

// RequestLog represents a single API request log entry
type RequestLog struct {
	Timestamp   time.Time `json:"timestamp"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Model       string    `json:"model"`
	StatusCode  int       `json:"status_code"`
	Duration    int64     `json:"duration_ms"` // milliseconds
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	SessionIdx  int       `json:"session_idx"`
	IsStreaming bool      `json:"is_streaming"`
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

// RequestLogger manages request logging and statistics
type RequestLogger struct {
	logs     []RequestLog
	mu       sync.RWMutex
	maxLogs  int
	startTime time.Time
}

var GlobalRequestLogger *RequestLogger

func init() {
	GlobalRequestLogger = NewRequestLogger(10000) // Keep last 10000 requests
}

// NewRequestLogger creates a new request logger
func NewRequestLogger(maxLogs int) *RequestLogger {
	return &RequestLogger{
		logs:      make([]RequestLog, 0, maxLogs),
		maxLogs:   maxLogs,
		startTime: time.Now(),
	}
}

// LogRequest adds a new request log entry
func (rl *RequestLogger) LogRequest(log RequestLog) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Add log
	rl.logs = append(rl.logs, log)

	// Trim if exceeds max
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
}
