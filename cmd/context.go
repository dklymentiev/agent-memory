package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/dklymentiev/agent-memory/internal/store"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Get smart context for current session",
	Long:  "Assembles relevant context: pinned docs + recent + project-relevant memories.",
	RunE:  runContext,
}

var (
	contextLimit  int
	contextQuery  string
	contextBudget int
)

func init() {
	contextCmd.Flags().IntVarP(&contextLimit, "limit", "n", 5, "max documents per section")
	contextCmd.Flags().StringVarP(&contextQuery, "query", "q", "", "additional search query")
	contextCmd.Flags().IntVar(&contextBudget, "budget", 1000, "character budget for context output")
	rootCmd.AddCommand(contextCmd)
}

// firstLineOf returns the first non-empty line of s.
func firstLineOf(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	r := []rune(s)
	if len(r) > 100 {
		return string(r[:100]) + "..."
	}
	return s
}

func runContext(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	budget := contextBudget
	if budget <= 0 {
		budget = 1000
	}
	used := 0

	var sections []string

	// Layer 1: Pinned documents (titles/first line only)
	pinned := true
	pinnedDocs, err := s.List(store.ListOptions{
		Workspace: workspace,
		Pinned:    &pinned,
		Limit:     contextLimit,
	})
	if err == nil && len(pinnedDocs) > 0 {
		var lines []string
		for _, d := range pinnedDocs {
			line := firstLineOf(d.Content)
			entry := fmt.Sprintf("- [short] %s", line)
			if used+len(entry) > budget {
				break
			}
			lines = append(lines, entry)
			used += len(entry) + 1
		}
		if len(lines) > 0 {
			sections = append(sections, "## Pinned\n"+strings.Join(lines, "\n"))
		}
	}

	// Layer 2: Recent document summaries (first 100 chars each)
	recentDocs, err := s.List(store.ListOptions{
		Workspace: workspace,
		Limit:     contextLimit,
	})
	if err == nil && len(recentDocs) > 0 && used < budget {
		var lines []string
		for _, d := range recentDocs {
			content := d.Content
			if r := []rune(content); len(r) > 100 {
				content = string(r[:100]) + "..."
			}

			content = strings.ReplaceAll(content, "\n", " ")
			entry := fmt.Sprintf("- [%s] %s", d.CreatedAt.Format("2006-01-02"), content)
			if used+len(entry) > budget {
				break
			}
			lines = append(lines, entry)
			used += len(entry) + 1
		}
		if len(lines) > 0 {
			sections = append(sections, "## Recent (last 24h)\n"+strings.Join(lines, "\n"))
		}
	}

	// Layer 3: Relevant documents (full content if budget allows)
	if contextQuery == "" {
		cwd, _ := os.Getwd()
		if cwd != "" {
			parts := strings.Split(cwd, string(os.PathSeparator))
			if len(parts) > 0 {
				contextQuery = parts[len(parts)-1]
			}
		}
	}
	if contextQuery != "" && used < budget {
		results, err := s.Search(contextQuery, workspace, contextLimit)
		if err == nil && len(results) > 0 {
			var lines []string
			for _, r := range results {
				content := r.Content
				maxLen := 200
				remaining := budget - used
				if remaining < maxLen {
					maxLen = remaining
				}
				if maxLen <= 0 {
					break
				}
				if len(content) > maxLen {
					content = content[:maxLen] + "..."
				}
				content = strings.ReplaceAll(content, "\n", " ")
				entry := fmt.Sprintf("- [%s] %s", r.ID, content)
				lines = append(lines, entry)
				used += len(entry) + 1
			}
			if len(lines) > 0 {
				sections = append(sections, "## Relevant to: "+contextQuery+"\n"+strings.Join(lines, "\n"))
			}
		}
	}

	if len(sections) == 0 {
		fmt.Println("No context available.")
		return nil
	}

	fmt.Print(strings.Join(sections, "\n\n"))
	return nil
}
