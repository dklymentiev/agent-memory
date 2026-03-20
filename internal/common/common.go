package common

// MaxContentSize is the maximum allowed document content size (1MB).
const MaxContentSize = 1 << 20 // 1MB

// MergeTags merges user-provided and inferred tags; user tags take priority.
func MergeTags(userTags, inferred []string) []string {
	seen := make(map[string]bool, len(userTags))
	for _, t := range userTags {
		seen[t] = true
	}
	merged := append([]string{}, userTags...)
	for _, t := range inferred {
		if !seen[t] {
			merged = append(merged, t)
			seen[t] = true
		}
	}
	return merged
}
