package embed

import "fmt"

// ONNXEmbedder uses a local ONNX model for embeddings.
// This is a stub for Phase 1.5.
type ONNXEmbedder struct{}

func NewONNXEmbedder(modelPath string) (*ONNXEmbedder, error) {
	return nil, fmt.Errorf("ONNX embeddings not yet available; run 'agent-memory embeddings enable' in a future version")
}

func (e *ONNXEmbedder) Embed(text string) ([]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *ONNXEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (e *ONNXEmbedder) Dimensions() int { return 384 }
func (e *ONNXEmbedder) Close() error    { return nil }
