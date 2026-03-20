package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/embed"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search memory documents",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSearch,
}

var (
	searchLimit    int
	searchJSON     bool
	searchSemantic bool
	searchFTS      bool
)

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "max results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")
	searchCmd.Flags().BoolVar(&searchSemantic, "semantic", false, "semantic search only (requires embeddings)")
	searchCmd.Flags().BoolVar(&searchFTS, "fts", false, "FTS search only (ignore embeddings)")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	var results []store.SearchResult

	// Determine search mode
	useEmbeddings := !searchFTS && cfg.EmbeddingProvider != "" && os.Getenv("OPENAI_API_KEY") != ""

	if searchSemantic || (useEmbeddings && !searchFTS) {
		// Need to embed the query
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			if searchSemantic {
				return fmt.Errorf("OPENAI_API_KEY required for semantic search")
			}
			// Fallback to FTS
			useEmbeddings = false
		} else {
			embedder, err := embed.NewOpenAIEmbedder(apiKey, cfg.EmbeddingModel)
			if err != nil {
				if searchSemantic {
					return err
				}
				useEmbeddings = false
			} else {
				defer embedder.Close()

				queryEmb, err := embedder.Embed(query)
				if err != nil {
					if searchSemantic {
						return fmt.Errorf("embed query: %w", err)
					}
					useEmbeddings = false
				} else {
					if searchSemantic {
						results, err = s.SearchSemantic(queryEmb, workspace, searchLimit)
					} else {
						results, err = s.HybridSearch(query, queryEmb, workspace, searchLimit)
					}
					if err != nil {
						return err
					}
				}
			}
		}
	}

	// FTS fallback or explicit FTS mode
	if results == nil {
		results, err = s.Search(query, workspace, searchLimit)
		if err != nil {
			return err
		}
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	if searchJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	for i, r := range results {
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Printf("id: %s\n", r.ID)
		if len(r.Tags) > 0 {
			fmt.Printf("tags: %s\n", strings.Join(r.Tags, ", "))
		}
		fmt.Printf("workspace: %s\n", r.Workspace)
		fmt.Printf("created: %s\n", r.CreatedAt.Format("2006-01-02 15:04"))
		content := r.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("\n%s\n", content)
	}

	return nil
}
