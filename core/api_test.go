package core

import (
	"strings"
	"testing"
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
