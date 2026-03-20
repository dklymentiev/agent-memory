package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Get smart context for current session",
	Long:  "Assembles relevant context: pinned docs + recent + project-relevant memories.",
	RunE:  runContext,
}

var (
	contextLimit int
	contextQuery string
)

func init() {
	contextCmd.Flags().IntVarP(&contextLimit, "limit", "n", 5, "max documents per section")
	contextCmd.Flags().StringVarP(&contextQuery, "query", "q", "", "additional search query")
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	var sections []string

	// 1. Pinned documents
	pinned := true
	pinnedDocs, err := s.List(store.ListOptions{
		Workspace: workspace,
		Pinned:    &pinned,
		Limit:     contextLimit,
	})
	if err == nil && len(pinnedDocs) > 0 {
		var b strings.Builder
		b.WriteString("## Pinned\n\n")
		for _, d := range pinnedDocs {
			b.WriteString(d.Content)
			b.WriteString("\n\n")
		}
		sections = append(sections, b.String())
	}

	// 2. Recent documents
	recentDocs, err := s.List(store.ListOptions{
		Workspace: workspace,
		Limit:     contextLimit,
	})
	if err == nil && len(recentDocs) > 0 {
		var b strings.Builder
		b.WriteString("## Recent\n\n")
		for _, d := range recentDocs {
			content := d.Content
			if len(content) > 300 {
				content = content[:300] + "..."
			}
			b.WriteString(fmt.Sprintf("- [%s] %s\n", d.ID, strings.ReplaceAll(content, "\n", " ")))
		}
		sections = append(sections, b.String())
	}

	// 3. Project-relevant (search by CWD basename)
	if contextQuery == "" {
		cwd, _ := os.Getwd()
		if cwd != "" {
			parts := strings.Split(cwd, string(os.PathSeparator))
			if len(parts) > 0 {
				contextQuery = parts[len(parts)-1]
			}
		}
	}
	if contextQuery != "" {
		results, err := s.Search(contextQuery, workspace, contextLimit)
		if err == nil && len(results) > 0 {
			var b strings.Builder
			b.WriteString("## Relevant\n\n")
			for _, r := range results {
				content := r.Content
				if len(content) > 300 {
					content = content[:300] + "..."
				}
				b.WriteString(fmt.Sprintf("- [%s] %s\n", r.ID, strings.ReplaceAll(content, "\n", " ")))
			}
			sections = append(sections, b.String())
		}
	}

	if len(sections) == 0 {
		fmt.Println("No context available.")
		return nil
	}

	fmt.Print(strings.Join(sections, "\n"))
	return nil
}
