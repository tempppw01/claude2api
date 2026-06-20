package core

import (
	"claude2api/config"
	"strings"
	"testing"
	"time"
)

func TestCitationCollectorExtractsDeduplicatedSources(t *testing.T) {
	collector := newCitationCollector()

	collector.AddFrom(map[string]interface{}{
		"type": "content_block_delta",
		"delta": map[string]interface{}{
			"citations": []interface{}{
				map[string]interface{}{
					"title": "Example [Docs]",
					"url":   "https://example.com/docs",
				},
				map[string]interface{}{
					"name": "Duplicate",
					"href": "https://example.com/docs",
				},
				map[string]interface{}{
					"source_url": "https://news.example/article",
				},
			},
		},
	})

	annotations := collector.Annotations()
	if len(annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(annotations))
	}

	markdown := collector.Markdown()
	if !strings.Contains(markdown, "1. [Example \\[Docs\\]](https://example.com/docs)") {
		t.Fatalf("expected escaped titled citation in markdown, got %q", markdown)
	}
	if !strings.Contains(markdown, "2. [news.example](https://news.example/article)") {
		t.Fatalf("expected host fallback citation in markdown, got %q", markdown)
	}
}

func TestParseRateLimitResetValueFromRetryAfterSeconds(t *testing.T) {
	now := time.Date(2026, 6, 18, 19, 30, 0, 0, time.Local)

	resetAt, ok := parseRateLimitResetValue("120", now, true)
	if !ok {
		t.Fatal("expected retry-after seconds to parse")
	}
	if !resetAt.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("expected reset at %s, got %s", now.Add(2*time.Minute), resetAt)
	}
}

func TestParseRateLimitResetValueFromClockText(t *testing.T) {
	now := time.Date(2026, 6, 18, 19, 30, 0, 0, time.Local)

	resetAt, ok := parseRateLimitResetValue("You are out of free messages until 10:40 PM", now, false)
	if !ok {
		t.Fatal("expected clock reset text to parse")
	}

	expected := time.Date(2026, 6, 18, 22, 40, 0, 0, time.Local)
	if !resetAt.Equal(expected) {
		t.Fatalf("expected reset at %s, got %s", expected, resetAt)
	}
}

func TestFormatRateLimitResetAtUsesChinaTime(t *testing.T) {
	resetAt := time.Date(2026, 6, 19, 12, 7, 34, 0, time.UTC)

	got := formatRateLimitResetAt(resetAt)
	want := "2026-06-19 20:07:34 中国时间"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestIsUsableRateLimitResetRejectsCurrentOrTooCloseTime(t *testing.T) {
	now := time.Date(2026, 6, 20, 21, 45, 57, 0, time.Local)

	if isUsableRateLimitReset(now, now) {
		t.Fatal("expected current request time to be unusable")
	}
	if isUsableRateLimitReset(now.Add(config.MinRateLimitResetWindow), now) {
		t.Fatal("expected reset exactly at minimum window to be unusable")
	}
	if !isUsableRateLimitReset(now.Add(config.MinRateLimitResetWindow+time.Second), now) {
		t.Fatal("expected future reset beyond minimum window to be usable")
	}
}
