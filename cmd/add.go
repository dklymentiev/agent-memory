package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/dklymentiev/agent-memory/internal/chunker"
	"github.com/dklymentiev/agent-memory/internal/common"
	"github.com/dklymentiev/agent-memory/internal/embed"
	"github.com/dklymentiev/agent-memory/internal/store"
	"github.com/dklymentiev/agent-memory/internal/tagger"
)

var addCmd = &cobra.Command{
	Use:   "add [content]",
	Short: "Add a memory document",
	Long:  "Add a new document to memory. Content can be passed as argument, via --file, or piped via stdin.",
	RunE:  runAdd,
}

var (
	addTags   []string
	addSource string
	addPinned bool
	addFile   string
)

func init() {
	addCmd.Flags().StringSliceVarP(&addTags, "tag", "t", nil, "tags (repeatable, e.g. -t type:note -t topic:dns)")
	addCmd.Flags().StringVarP(&addSource, "source", "s", "cli", "source (cli, hook, mcp)")
	addCmd.Flags().BoolVar(&addPinned, "pin", false, "pin this document")
	addCmd.Flags().StringVarP(&addFile, "file", "f", "", "read content from file (use - for stdin)")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	content, err := resolveContent(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	doc := &store.Document{
		Content:   content,
		Tags:      addTags,
		Workspace: workspace,
		Source:    addSource,
		Pinned:    addPinned,
	}

	if err := s.Add(doc); err != nil {
		return err
	}

	// Auto-tag: infer tags from similar documents and merge with user tags
	inferred := tagger.InferTags(s, content, workspace, 5)
	if len(inferred) > 0 {
		merged := common.MergeTags(doc.Tags, inferred)
		if len(merged) > len(doc.Tags) {
			doc.Tags = merged
			if err := s.Update(doc); err != nil {
				fmt.Fprintf(os.Stderr, "agent-memory: auto-tag update: %v\n", err)
			}
		}
	}

	// Chunking: if content is long enough, split and store chunks
	if len(content) > 800 {
		chunks := chunker.Chunk(content, chunker.DefaultTargetSize, chunker.DefaultOverlap, chunker.DefaultMinSize)
		if len(chunks) > 1 {
			if err := s.AddChunks(doc.ID, chunks); err != nil {
				fmt.Fprintf(os.Stderr, "agent-memory: add chunks: %v\n", err)
			}
		}
	}

	// Embed chunks if embeddings are enabled
	embedChunksForDoc(s, doc.ID)

	fmt.Println(doc.ID)
	return nil
}

// embedChunksForDoc embeds any unembedded chunks for a document if embeddings are enabled.
func embedChunksForDoc(s *store.SQLiteStore, docID string) {
	if cfg.EmbeddingProvider == "" {
		return
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return
	}
	embedder, err := embed.NewOpenAIEmbedder(apiKey, cfg.EmbeddingModel)
	if err != nil {
		return
	}
	defer embedder.Close()

	// Get unembedded chunks for this doc
	docChunks, err := s.GetUnembeddedChunksByDoc(docID, 1000)
	if err != nil || len(docChunks) == 0 {
		return
	}

	texts := make([]string, len(docChunks))
	for i, c := range docChunks {
		texts[i] = c.ChunkText
	}

	embeddings, err := embedder.EmbedBatch(texts)
	if err != nil {
		return
	}

	for i, emb := range embeddings {
		if err := s.UpdateChunkEmbedding(docChunks[i].ID, emb); err != nil {
			fmt.Fprintf(os.Stderr, "agent-memory: update chunk embedding: %v\n", err)
		}
	}
}

func resolveContent(args []string) (string, error) {
	var content string

	if addFile == "-" {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, common.MaxContentSize+1))
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		content = string(data)
	} else if addFile != "" {
		data, err := os.ReadFile(addFile)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		content = string(data)
	} else if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		// Try stdin if not a terminal
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, common.MaxContentSize+1))
			if err != nil {
				return "", fmt.Errorf("read stdin: %w", err)
			}
			content = string(data)
		} else {
			return "", fmt.Errorf("provide content as argument, --file, or pipe via stdin")
		}
	}

	if len(content) > common.MaxContentSize {
		return "", fmt.Errorf("content too large (%d bytes, max %d)", len(content), common.MaxContentSize)
	}
	return content, nil
}
