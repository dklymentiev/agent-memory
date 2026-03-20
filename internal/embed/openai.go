package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	openaiEmbeddingsURL = "https://api.openai.com/v1/embeddings"
	defaultOpenAIModel  = "text-embedding-3-small"
	openaiDimensions    = 1536
)

// OpenAIEmbedder uses the OpenAI API for embeddings.
type OpenAIEmbedder struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIEmbedder creates a new OpenAI embedder.
// apiKey must be a valid OpenAI API key.
// model can be empty to use the default (text-embedding-3-small).
func NewOpenAIEmbedder(apiKey string, model string) (*OpenAIEmbedder, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required for OpenAI embeddings")
	}
	if len(apiKey) < 20 || !strings.HasPrefix(apiKey, "sk-") {
		return nil, fmt.Errorf("invalid API key format")
	}
	if model == "" {
		model = defaultOpenAIModel
	}
	return &OpenAIEmbedder{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Model returns the model name.
func (e *OpenAIEmbedder) Model() string {
	return e.model
}

type openaiRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openaiResponse struct {
	Data  []openaiEmbeddingData `json:"data"`
	Error *openaiError          `json:"error,omitempty"`
}

type openaiEmbeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Embed generates an embedding for a single text.
func (e *OpenAIEmbedder) Embed(text string) ([]float32, error) {
	results, err := e.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return results[0], nil
}

// EmbedBatch generates embeddings for multiple texts in one API call.
func (e *OpenAIEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	result, err := e.doRequest(texts)
	if err != nil {
		// One retry on failure (rate limit or transient error)
		time.Sleep(1 * time.Second)
		result, err = e.doRequest(texts)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (e *OpenAIEmbedder) doRequest(texts []string) ([][]float32, error) {
	reqBody := openaiRequest{
		Input: texts,
		Model: e.model,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", openaiEmbeddingsURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiResp openaiResponse
		json.Unmarshal(respBody, &apiResp)
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if apiResp.Error != nil {
			msg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, apiResp.Error.Message)
		}
		return nil, fmt.Errorf("OpenAI API error: %s", msg)
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", apiResp.Error.Message)
	}

	// Order results by index
	results := make([][]float32, len(texts))
	for _, d := range apiResp.Data {
		if d.Index < len(results) {
			results[d.Index] = d.Embedding
		}
	}

	// Verify all slots filled
	for i, r := range results {
		if r == nil {
			return nil, fmt.Errorf("missing embedding for input %d", i)
		}
	}

	return results, nil
}

// Dimensions returns the embedding vector dimensions.
func (e *OpenAIEmbedder) Dimensions() int { return openaiDimensions }

// Close is a no-op for OpenAI (no local resources).
func (e *OpenAIEmbedder) Close() error { return nil }
