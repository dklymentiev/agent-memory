// Package tagger implements auto-tag inference from similar documents.
package tagger

import (
	"sort"
	"strings"

	"github.com/dklymentiev/agent-memory/internal/store"
)

// InferTags suggests tags based on similar documents found via FTS5.
// It looks at the most common tags among the top N search results.
func InferTags(s *store.SQLiteStore, content string, workspace string, maxTags int) []string {
	if maxTags <= 0 {
		maxTags = 5
	}

	// Use first 200 chars as search query
	query := content
	if len(query) > 200 {
		query = query[:200]
	}
	// Clean query for FTS5
	query = cleanForFTS(query)
	if query == "" {
		return nil
	}

	results, err := s.Search(query, workspace, 10)
	if err != nil || len(results) == 0 {
		return nil
	}

	// Count tag frequencies
	freq := make(map[string]int)
	for _, r := range results {
		for _, tag := range r.Tags {
			// Skip source/date tags -- they're not useful for inference
			if strings.HasPrefix(tag, "source:") || strings.HasPrefix(tag, "date:") {
				continue
			}
			freq[tag]++
		}
	}

	// Sort by frequency
	type tagFreq struct {
		tag   string
		count int
	}
	var sorted []tagFreq
	for tag, count := range freq {
		if count >= 2 { // must appear in at least 2 docs
			sorted = append(sorted, tagFreq{tag, count})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var tags []string
	for i, tf := range sorted {
		if i >= maxTags {
			break
		}
		tags = append(tags, tf.tag)
	}
	return tags
}

func cleanForFTS(s string) string {
	// Remove special characters that break FTS5 queries
	replacer := strings.NewReplacer(
		"\"", "", "'", "", "(", "", ")", "",
		"[", "", "]", "", "{", "", "}", "",
		"*", "", ":", " ", "/", " ", "\\", " ",
		"\n", " ", "\r", " ", "\t", " ",
	)
	result := replacer.Replace(s)
	// Collapse multiple spaces
	parts := strings.Fields(result)
	if len(parts) > 20 {
		parts = parts[:20]
	}
	return strings.Join(parts, " ")
}
