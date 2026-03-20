package embed

import (
	"math"
	"testing"
)

func TestCosineSimilarityIdentical(t *testing.T) {
	a := []float32{1, 2, 3}
	sim := CosineSimilarity(a, a)
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("identical vectors: got %f, want 1.0", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim) > 1e-6 {
		t.Errorf("orthogonal vectors: got %f, want 0.0", sim)
	}
}

func TestCosineSimilarityOpposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim+1.0) > 1e-6 {
		t.Errorf("opposite vectors: got %f, want -1.0", sim)
	}
}

func TestCosineSimilarityEmpty(t *testing.T) {
	sim := CosineSimilarity(nil, nil)
	if sim != 0 {
		t.Errorf("empty vectors: got %f, want 0", sim)
	}
}

func TestTopK(t *testing.T) {
	query := []float32{1, 0, 0}
	vectors := [][]float32{
		{0, 1, 0},   // orthogonal
		{1, 0, 0},   // identical
		{0.5, 0.5, 0}, // partial
		{-1, 0, 0},  // opposite
	}

	indices := TopK(query, vectors, 2)
	if len(indices) != 2 {
		t.Fatalf("got %d indices, want 2", len(indices))
	}
	if indices[0] != 1 {
		t.Errorf("top result index = %d, want 1", indices[0])
	}
	if indices[1] != 2 {
		t.Errorf("second result index = %d, want 2", indices[1])
	}
}
