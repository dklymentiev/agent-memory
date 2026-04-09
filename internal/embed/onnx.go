package embed

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	onnxDimensions = 384
	maxSeqLength   = 128

	// WordPiece special tokens
	clsToken    = "[CLS]"
	sepToken    = "[SEP]"
	unkToken    = "[UNK]"
	subwordPfx  = "##"

	defaultModelName = "all-MiniLM-L6-v2"
)

// OnnxEmbedder uses a local ONNX model for embeddings.
type OnnxEmbedder struct {
	session       *ort.AdvancedSession
	inputIDs      *ort.Tensor[int64]
	attentionMask *ort.Tensor[int64]
	tokenTypeIDs  *ort.Tensor[int64]
	output        *ort.Tensor[float32]
	vocab         map[string]int64
	clsID         int64
	sepID         int64
	unkID         int64
}

// NewOnnxEmbedder creates a new ONNX embedder with persistent session.
func NewOnnxEmbedder(modelPath, vocabPath string) (*OnnxEmbedder, error) {
	// Set library path before initialization
	libPath := findOnnxLib()
	if libPath != "" {
		ort.SetSharedLibraryPath(libPath)
	}

	if err := ort.InitializeEnvironment(); err != nil {
		if !strings.Contains(err.Error(), "already") {
			return nil, fmt.Errorf("init ONNX runtime: %w", err)
		}
	}

	vocab, err := loadVocab(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("load vocab: %w", err)
	}

	e := &OnnxEmbedder{
		vocab: vocab,
		clsID: vocab[clsToken],
		sepID: vocab[sepToken],
		unkID: vocab[unkToken],
	}

	// Create persistent tensors
	shape := ort.NewShape(1, int64(maxSeqLength))
	outShape := ort.NewShape(1, int64(maxSeqLength), int64(onnxDimensions))

	e.inputIDs, err = ort.NewEmptyTensor[int64](shape)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	e.attentionMask, err = ort.NewEmptyTensor[int64](shape)
	if err != nil {
		e.destroyTensors()
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	e.tokenTypeIDs, err = ort.NewEmptyTensor[int64](shape)
	if err != nil {
		e.destroyTensors()
		return nil, fmt.Errorf("create token_type_ids tensor: %w", err)
	}
	e.output, err = ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		e.destroyTensors()
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	// Create persistent ONNX session
	e.session, err = ort.NewAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.Value{e.inputIDs, e.attentionMask, e.tokenTypeIDs},
		[]ort.Value{e.output},
		nil,
	)
	if err != nil {
		e.destroyTensors()
		return nil, fmt.Errorf("create ONNX session: %w", err)
	}

	return e, nil
}

// Embed generates an embedding for a single text.
func (e *OnnxEmbedder) Embed(text string) ([]float32, error) {
	results, err := e.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *OnnxEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := e.embedSingle(text)
		if err != nil {
			return nil, fmt.Errorf("embed text %d: %w", i, err)
		}
		results[i] = vec
	}
	return results, nil
}

// Dimensions returns the embedding vector dimensions (384 for MiniLM).
func (e *OnnxEmbedder) Dimensions() int { return onnxDimensions }

// Close releases ONNX session resources.
func (e *OnnxEmbedder) Close() error {
	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}
	e.destroyTensors()
	return nil
}

func (e *OnnxEmbedder) destroyTensors() {
	if e.inputIDs != nil {
		e.inputIDs.Destroy()
	}
	if e.attentionMask != nil {
		e.attentionMask.Destroy()
	}
	if e.tokenTypeIDs != nil {
		e.tokenTypeIDs.Destroy()
	}
	if e.output != nil {
		e.output.Destroy()
	}
}

func (e *OnnxEmbedder) embedSingle(text string) ([]float32, error) {
	tokens := e.tokenize(text)

	// Truncate to maxSeqLength - 2 (for CLS and SEP)
	if len(tokens) > maxSeqLength-2 {
		tokens = tokens[:maxSeqLength-2]
	}

	// Build input: [CLS] tokens... [SEP] [PAD]...
	seqLen := len(tokens) + 2

	inputData := e.inputIDs.GetData()
	attData := e.attentionMask.GetData()
	tokData := e.tokenTypeIDs.GetData()

	// Zero all
	for i := range inputData {
		inputData[i] = 0
		attData[i] = 0
		tokData[i] = 0
	}

	// Fill
	inputData[0] = e.clsID
	attData[0] = 1
	for i, tok := range tokens {
		inputData[i+1] = tok
		attData[i+1] = 1
	}
	inputData[seqLen-1] = e.sepID
	attData[seqLen-1] = 1

	// Run inference
	if err := e.session.Run(); err != nil {
		return nil, fmt.Errorf("ONNX inference: %w", err)
	}

	// Mean pooling over non-padding tokens (positions with attention_mask=1)
	outData := e.output.GetData()
	embedding := make([]float32, onnxDimensions)
	for i := 0; i < seqLen; i++ {
		offset := i * onnxDimensions
		for j := 0; j < onnxDimensions; j++ {
			embedding[j] += outData[offset+j]
		}
	}

	// Average
	n := float32(seqLen)
	for j := range embedding {
		embedding[j] /= n
	}

	// L2 normalize
	var norm float64
	for _, v := range embedding {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for j := range embedding {
			embedding[j] = float32(float64(embedding[j]) / norm)
		}
	}

	return embedding, nil
}

// tokenize performs WordPiece tokenization.
func (e *OnnxEmbedder) tokenize(text string) []int64 {
	text = strings.ToLower(text)
	words := splitWords(text)

	var tokens []int64
	for _, word := range words {
		wordTokens := e.wordPiece(word)
		tokens = append(tokens, wordTokens...)
	}
	return tokens
}

// wordPiece performs greedy longest-match WordPiece tokenization on a single word.
func (e *OnnxEmbedder) wordPiece(word string) []int64 {
	if len(word) == 0 {
		return nil
	}

	if id, ok := e.vocab[word]; ok {
		return []int64{id}
	}

	var tokens []int64
	start := 0
	runes := []rune(word)

	for start < len(runes) {
		end := len(runes)
		found := false

		for end > start {
			substr := string(runes[start:end])
			if start > 0 {
				substr = subwordPfx + substr
			}

			if id, ok := e.vocab[substr]; ok {
				tokens = append(tokens, id)
				start = end
				found = true
				break
			}
			end--
		}

		if !found {
			tokens = append(tokens, e.unkID)
			start++
		}
	}

	return tokens
}

// splitWords splits text into words on whitespace and non-alphanumeric boundaries.
func splitWords(text string) []string {
	var words []string
	var current []rune

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			if len(current) > 0 {
				words = append(words, string(current))
				current = current[:0]
			}
			if !unicode.IsSpace(r) {
				words = append(words, string(r))
			}
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

// loadVocab reads a vocab.txt file (one token per line).
func loadVocab(path string) (map[string]int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	vocab := make(map[string]int64)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			vocab[line] = int64(i)
		}
	}
	return vocab, nil
}

// ModelDir returns the directory where ONNX models are stored.
func ModelDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-memory", "models", defaultModelName)
}

// DefaultModelPath returns the path to the default ONNX model file.
func DefaultModelPath() string {
	return filepath.Join(ModelDir(), "model.onnx")
}

// DefaultVocabPath returns the path to the default vocab file.
func DefaultVocabPath() string {
	return filepath.Join(ModelDir(), "vocab.txt")
}

// ModelExists checks if the ONNX model files are present.
func ModelExists() bool {
	_, err1 := os.Stat(DefaultModelPath())
	_, err2 := os.Stat(DefaultVocabPath())
	return err1 == nil && err2 == nil
}

// findOnnxLib searches for the ONNX Runtime shared library.
func findOnnxLib() string {
	// Check env var first
	if p := os.Getenv("ONNXRUNTIME_LIB"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Check our own lib dir (~/.agent-memory/lib/)
	home, _ := os.UserHomeDir()
	localLib := filepath.Join(home, ".agent-memory", "lib", ortLibName())
	if _, err := os.Stat(localLib); err == nil {
		return localLib
	}

	// Check system locations
	var candidates []string
	switch runtime.GOOS {
	case "linux":
		candidates = []string{
			"/usr/local/lib/libonnxruntime.so",
			"/usr/lib/libonnxruntime.so",
			"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
		}
	case "darwin":
		candidates = []string{
			"/opt/homebrew/lib/libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.dylib",
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
