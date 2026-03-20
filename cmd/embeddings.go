package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/dklymentiev/agent-memory/internal/embed"
)

var embeddingsCmd = &cobra.Command{
	Use:   "embeddings",
	Short: "Manage embedding generation for semantic search",
}

var embeddingsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show embeddings status",
	RunE:  runEmbeddingsStatus,
}

var embeddingsEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable embeddings (requires OPENAI_API_KEY)",
	RunE:  runEmbeddingsEnable,
}

var embeddingsDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable embeddings",
	RunE:  runEmbeddingsDisable,
}

var embeddingsRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Generate embeddings for unembedded chunks",
	RunE:  runEmbeddingsRun,
}

var embedRunAll bool

func init() {
	embeddingsRunCmd.Flags().BoolVar(&embedRunAll, "all", false, "re-embed all chunks (not just unembedded)")
	embeddingsCmd.AddCommand(embeddingsStatusCmd)
	embeddingsCmd.AddCommand(embeddingsEnableCmd)
	embeddingsCmd.AddCommand(embeddingsDisableCmd)
	embeddingsCmd.AddCommand(embeddingsRunCmd)
	rootCmd.AddCommand(embeddingsCmd)
}

func runEmbeddingsStatus(cmd *cobra.Command, args []string) error {
	provider := cfg.EmbeddingProvider
	model := cfg.EmbeddingModel

	if provider == "" {
		fmt.Println("Embeddings: disabled")
		fmt.Println("Enable with: agent-memory embeddings enable")
	} else {
		fmt.Printf("Embeddings: enabled\n")
		fmt.Printf("Provider:   %s\n", provider)
		fmt.Printf("Model:      %s\n", model)
	}

	// Show chunk stats if we can open the store
	s, err := openStore()
	if err != nil {
		return nil // not an error -- just no stats
	}
	defer s.Close()

	total, withEmb, err := s.ChunkStats()
	if err != nil {
		return nil
	}
	fmt.Printf("Chunks:     %d total, %d with embeddings\n", total, withEmb)
	if total > 0 && withEmb < total {
		fmt.Printf("Pending:    %d chunks need embedding\n", total-withEmb)
	}

	return nil
}

func runEmbeddingsEnable(cmd *cobra.Command, args []string) error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	// Validate we can create the embedder
	_, err := embed.NewOpenAIEmbedder(apiKey, "")
	if err != nil {
		return err
	}

	cfg.EmbeddingProvider = "openai"
	cfg.EmbeddingModel = "text-embedding-3-small"
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Println("Embeddings enabled: openai / text-embedding-3-small")
	fmt.Println("Run 'agent-memory embeddings run' to generate embeddings for existing chunks.")
	return nil
}

func runEmbeddingsDisable(cmd *cobra.Command, args []string) error {
	cfg.EmbeddingProvider = ""
	cfg.EmbeddingModel = ""
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Println("Embeddings disabled.")
	return nil
}

func runEmbeddingsRun(cmd *cobra.Command, args []string) error {
	if cfg.EmbeddingProvider == "" {
		return fmt.Errorf("embeddings not enabled; run 'agent-memory embeddings enable' first")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	embedder, err := embed.NewOpenAIEmbedder(apiKey, cfg.EmbeddingModel)
	if err != nil {
		return err
	}
	defer embedder.Close()

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	var chunks []chunkForEmbed

	if embedRunAll {
		records, err := s.GetAllChunks(0)
		if err != nil {
			return fmt.Errorf("get all chunks: %w", err)
		}
		for _, r := range records {
			chunks = append(chunks, chunkForEmbed{id: r.ID, text: r.ChunkText})
		}
	} else {
		records, err := s.GetUnembeddedChunks(0)
		if err != nil {
			return fmt.Errorf("get unembedded chunks: %w", err)
		}
		for _, r := range records {
			chunks = append(chunks, chunkForEmbed{id: r.ID, text: r.ChunkText})
		}
	}

	if len(chunks) == 0 {
		fmt.Println("No chunks to embed.")
		return nil
	}

	fmt.Printf("Embedding %d chunks...\n", len(chunks))

	// Process in batches of 100 (OpenAI limit is 2048 but 100 is safe)
	const batchSize = 100
	embedded := 0

	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		texts := make([]string, len(batch))
		for j, c := range batch {
			texts[j] = c.text
		}

		embeddings, err := embedder.EmbedBatch(texts)
		if err != nil {
			return fmt.Errorf("embed batch at offset %d: %w", i, err)
		}

		for j, emb := range embeddings {
			if err := s.UpdateChunkEmbedding(batch[j].id, emb); err != nil {
				return fmt.Errorf("store embedding for chunk %d: %w", batch[j].id, err)
			}
			embedded++
		}

		if len(chunks) > batchSize {
			fmt.Printf("  %d/%d\n", embedded, len(chunks))
		}
	}

	fmt.Printf("Done. Embedded %d chunks.\n", embedded)
	return nil
}

type chunkForEmbed struct {
	id   int64
	text string
}
