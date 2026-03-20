package embed

import "fmt"

// OpenAIEmbedder uses the OpenAI API for embeddings.
// This is a stub for Phase 1.5.
type OpenAIEmbedder struct{}

func NewOpenAIEmbedder(apiKey string) (*OpenAIEmbedder, error) {
	return nil, fmt.Errorf("OpenAI embeddings not yet available; will be implemented in a future version")
}

func (e *OpenAIEmbedder) Embed(text string) ([]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *OpenAIEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *OpenAIEmbedder) Dimensions() int { return 1536 }
func (e *OpenAIEmbedder) Close() error    { return nil }
