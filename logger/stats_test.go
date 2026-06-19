package logger

import (
	"testing"
	"time"
)

func TestGetStatsBySessionIncludesTokenAndRateLimitProfile(t *testing.T) {
	requestLogger := NewRequestLogger(20)
	now := time.Date(2026, 6, 19, 20, 0, 0, 0, time.Local)

	requestLogger.LogRequest(RequestLog{
		Timestamp:    now,
		Success:      true,
		SessionIdx:   0,
		InputTokens:  100,
		OutputTokens: 50,
	})
	requestLogger.LogRequest(RequestLog{
		Timestamp:    now.Add(time.Minute),
		Success:      true,
		SessionIdx:   0,
		InputTokens:  120,
		OutputTokens: 80,
	})
	requestLogger.LogRequest(RequestLog{
		Timestamp:  now.Add(2 * time.Minute),
		Success:    false,
		ErrorType:  "限流",
		Error:      "rate limit exceeded",
		SessionIdx: 0,
	})

	stats := requestLogger.GetStatsBySession()[0]
	if stats.SuccessRequests != 2 {
		t.Fatalf("expected 2 successes, got %d", stats.SuccessRequests)
	}
	if stats.RateLimitRequests != 1 {
		t.Fatalf("expected 1 rate limit, got %d", stats.RateLimitRequests)
	}
	if stats.TotalTokens != 350 {
		t.Fatalf("expected 350 total tokens, got %d", stats.TotalTokens)
	}
	if stats.AvgTokensPerSuccess != 175 {
		t.Fatalf("expected avg tokens per success 175, got %f", stats.AvgTokensPerSuccess)
	}
	if stats.AvgSuccessesBeforeRateLimit != 2 {
		t.Fatalf("expected avg successes before rate limit 2, got %f", stats.AvgSuccessesBeforeRateLimit)
	}
}

func TestGetStatsBySessionKeepsCumulativeStatsAfterLogTrim(t *testing.T) {
	requestLogger := NewRequestLogger(2)
	now := time.Date(2026, 6, 19, 20, 0, 0, 0, time.Local)

	for i := 0; i < 3; i++ {
		requestLogger.LogRequest(RequestLog{
			Timestamp:    now.Add(time.Duration(i) * time.Minute),
			Success:      true,
			SessionIdx:   0,
			InputTokens:  10,
			OutputTokens: 5,
		})
	}

	if requestLogger.GetTotalCount() != 2 {
		t.Fatalf("expected retained log count 2, got %d", requestLogger.GetTotalCount())
	}

	stats := requestLogger.GetStatsBySession()[0]
	if stats.SuccessRequests != 3 {
		t.Fatalf("expected cumulative successes 3, got %d", stats.SuccessRequests)
	}
	if stats.TotalTokens != 45 {
		t.Fatalf("expected cumulative total tokens 45, got %d", stats.TotalTokens)
	}
}
