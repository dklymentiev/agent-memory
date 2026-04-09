package embed

import (
	"math"
	"strings"
	"testing"
)

// skipIfNoModel skips the test if the ONNX model hasn't been downloaded.
func skipIfNoModel(t *testing.T) {
	t.Helper()
	if !ModelExists() {
		t.Skip("ONNX model not downloaded; run 'agent-memory embeddings enable --local' first")
	}
}

// newTestEmbedder creates an OnnxEmbedder for testing.
func newTestEmbedder(t *testing.T) *OnnxEmbedder {
	t.Helper()
	skipIfNoModel(t)
	e, err := NewOnnxEmbedder(DefaultModelPath(), DefaultVocabPath())
	if err != nil {
		t.Fatalf("create embedder: %v", err)
	}
	return e
}

func TestWordPieceTokenizer(t *testing.T) {
	skipIfNoModel(t)

	vocab, err := loadVocab(DefaultVocabPath())
	if err != nil {
		t.Fatalf("load vocab: %v", err)
	}

	e := &OnnxEmbedder{
		vocab: vocab,
		clsID: vocab[clsToken],
		sepID: vocab[sepToken],
		unkID: vocab[unkToken],
	}

	t.Run("basic words", func(t *testing.T) {
		tokens := e.tokenize("hello world")
		if len(tokens) == 0 {
			t.Fatal("expected tokens, got none")
		}
	})

	t.Run("subword splitting", func(t *testing.T) {
		tokens := e.tokenize("embeddings")
		if len(tokens) == 0 {
			t.Fatal("expected tokens for 'embeddings', got none")
		}
		// "embeddings" should be split into subwords, not a single UNK
		for _, tok := range tokens {
			if tok == e.unkID {
				t.Error("'embeddings' produced UNK token; expected subword split")
			}
		}
	})

	t.Run("common words no UNK", func(t *testing.T) {
		for _, tok := range e.tokenize("the quick brown fox") {
			if tok == e.unkID {
				t.Error("unexpected UNK token for common English words")
			}
		}
	})

	t.Run("empty string", func(t *testing.T) {
		tokens := e.tokenize("")
		if len(tokens) != 0 {
			t.Errorf("expected 0 tokens for empty string, got %d", len(tokens))
		}
	})

	t.Run("punctuation", func(t *testing.T) {
		tokens := e.tokenize("hello, world! how are you?")
		if len(tokens) == 0 {
			t.Fatal("expected tokens for punctuated text")
		}
	})

	t.Run("numbers", func(t *testing.T) {
		tokens := e.tokenize("the year is 2024 and pi is 3.14")
		if len(tokens) == 0 {
			t.Fatal("expected tokens for text with numbers")
		}
	})
}

func TestSplitWords(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"hello, world!", []string{"hello", ",", "world", "!"}},
		{"", nil},
		{"   spaces   ", []string{"spaces"}},
		{"one", []string{"one"}},
		{"a-b", []string{"a", "-", "b"}},
		{"don't", []string{"don", "'", "t"}},
	}
	for _, tc := range tests {
		got := splitWords(tc.input)
		if len(got) != len(tc.expected) {
			t.Errorf("splitWords(%q) = %v, want %v", tc.input, got, tc.expected)
			continue
		}
		for i := range got {
			if got[i] != tc.expected[i] {
				t.Errorf("splitWords(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.expected[i])
			}
		}
	}
}

func TestOnnxEmbed(t *testing.T) {
	e := newTestEmbedder(t)
	defer e.Close()

	t.Run("dimensions", func(t *testing.T) {
		if e.Dimensions() != 384 {
			t.Errorf("expected 384 dimensions, got %d", e.Dimensions())
		}
	})

	t.Run("basic embedding", func(t *testing.T) {
		vec, err := e.Embed("I graduated with a degree in Computer Science")
		if err != nil {
			t.Fatalf("embed: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("expected 384-dim vector, got %d", len(vec))
		}
	})

	t.Run("L2 normalized", func(t *testing.T) {
		vec, err := e.Embed("test normalization")
		if err != nil {
			t.Fatalf("embed: %v", err)
		}
		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)
		if math.Abs(norm-1.0) > 0.01 {
			t.Errorf("vector not L2-normalized: norm = %f", norm)
		}
	})

	t.Run("semantic similarity", func(t *testing.T) {
		vec1, _ := e.Embed("I graduated with a degree in Computer Science")
		vec2, _ := e.Embed("What university degree did you earn")
		vec3, _ := e.Embed("The weather is nice today")

		sim12 := CosineSimilarity(vec1, vec2)
		sim13 := CosineSimilarity(vec1, vec3)

		t.Logf("similarity(CS degree, university degree) = %.4f", sim12)
		t.Logf("similarity(CS degree, weather) = %.4f", sim13)

		if sim12 <= sim13 {
			t.Errorf("expected CS/university similarity (%.4f) > CS/weather similarity (%.4f)", sim12, sim13)
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		vec1, _ := e.Embed("deterministic test input")
		vec2, _ := e.Embed("deterministic test input")
		sim := CosineSimilarity(vec1, vec2)
		if math.Abs(sim-1.0) > 0.0001 {
			t.Errorf("same input produced different vectors: cosine = %.6f", sim)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		vec, err := e.Embed("")
		if err != nil {
			t.Fatalf("embed empty string: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("expected 384-dim vector for empty string, got %d", len(vec))
		}
	})

	t.Run("very long text", func(t *testing.T) {
		// Text with more than 128 tokens (should be truncated, not error)
		long := strings.Repeat("the quick brown fox jumps over the lazy dog ", 50)
		vec, err := e.Embed(long)
		if err != nil {
			t.Fatalf("embed long text: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("expected 384-dim vector for long text, got %d", len(vec))
		}
	})

	t.Run("unicode text", func(t *testing.T) {
		vec, err := e.Embed("Kubernetes est une plateforme d'orchestration")
		if err != nil {
			t.Fatalf("embed unicode: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("expected 384-dim vector for unicode, got %d", len(vec))
		}
	})

	t.Run("CJK text", func(t *testing.T) {
		vec, err := e.Embed("machine learning")
		if err != nil {
			t.Fatalf("embed CJK: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("expected 384-dim for CJK, got %d", len(vec))
		}
	})

	t.Run("special characters", func(t *testing.T) {
		vec, err := e.Embed("hello@world.com https://example.com/path?q=1&b=2")
		if err != nil {
			t.Fatalf("embed special chars: %v", err)
		}
		if len(vec) != 384 {
			t.Fatalf("expected 384-dim for special chars, got %d", len(vec))
		}
	})
}

func TestOnnxEmbedBatch(t *testing.T) {
	e := newTestEmbedder(t)
	defer e.Close()

	t.Run("multiple texts", func(t *testing.T) {
		texts := []string{
			"Machine learning is a subset of artificial intelligence",
			"The stock market closed higher today",
			"Neural networks process information in layers",
		}

		results, err := e.EmbedBatch(texts)
		if err != nil {
			t.Fatalf("embed batch: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}

		// ML and neural networks should be more similar than ML and stock market
		simML_NN := CosineSimilarity(results[0], results[2])
		simML_Stock := CosineSimilarity(results[0], results[1])

		t.Logf("similarity(ML, neural networks) = %.4f", simML_NN)
		t.Logf("similarity(ML, stock market) = %.4f", simML_Stock)

		if simML_NN <= simML_Stock {
			t.Errorf("expected ML/NN similarity (%.4f) > ML/stock similarity (%.4f)", simML_NN, simML_Stock)
		}
	})

	t.Run("empty batch", func(t *testing.T) {
		results, err := e.EmbedBatch(nil)
		if err != nil {
			t.Fatalf("embed empty batch: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for nil batch, got %d", len(results))
		}
	})

	t.Run("single item batch", func(t *testing.T) {
		results, err := e.EmbedBatch([]string{"hello"})
		if err != nil {
			t.Fatalf("embed single batch: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if len(results[0]) != 384 {
			t.Errorf("expected 384-dim, got %d", len(results[0]))
		}
	})

	t.Run("batch matches individual", func(t *testing.T) {
		text := "consistency check between batch and individual"
		individual, _ := e.Embed(text)
		batch, _ := e.EmbedBatch([]string{text})

		sim := CosineSimilarity(individual, batch[0])
		if math.Abs(sim-1.0) > 0.0001 {
			t.Errorf("batch and individual embeddings differ: cosine = %.6f", sim)
		}
	})
}

func TestOnnxEmbedderInterface(t *testing.T) {
	e := newTestEmbedder(t)
	defer e.Close()

	// Verify OnnxEmbedder implements Embedder interface
	var _ Embedder = e
}
