package embed

import "math"

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// TopK returns indices of the top-k most similar vectors to query.
func TopK(query []float32, vectors [][]float32, k int) []int {
	type scored struct {
		index int
		score float64
	}

	var scores []scored
	for i, v := range vectors {
		scores = append(scores, scored{i, CosineSimilarity(query, v)})
	}

	// Simple selection sort for top-k (fine for small k)
	for i := 0; i < k && i < len(scores); i++ {
		maxIdx := i
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[maxIdx].score {
				maxIdx = j
			}
		}
		scores[i], scores[maxIdx] = scores[maxIdx], scores[i]
	}

	n := k
	if n > len(scores) {
		n = len(scores)
	}
	indices := make([]int, n)
	for i := 0; i < n; i++ {
		indices[i] = scores[i].index
	}
	return indices
}
