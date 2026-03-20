package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/dklymentiev/agent-memory/internal/chunker"
	"github.com/dklymentiev/agent-memory/internal/common"
)

var updateCmd = &cobra.Command{
	Use:   "update <id> [content]",
	Short: "Update a memory document",
	Long:  "Update document content and/or tags. Content can be passed as argument or piped via stdin.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runUpdate,
}

var updateTags []string

func init() {
	updateCmd.Flags().StringSliceVarP(&updateTags, "tag", "t", nil, "replace tags (repeatable)")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	id := args[0]

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	doc, err := s.Get(id)
	if err != nil {
		return fmt.Errorf("document not found: %s", id)
	}

	// Resolve new content if provided
	var newContent string
	if len(args) > 1 {
		newContent = strings.Join(args[1:], " ")
	} else {
		// Try stdin if not a terminal
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, common.MaxContentSize+1))
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			newContent = string(data)
		}
	}

	if newContent != "" {
		if len(newContent) > common.MaxContentSize {
			return fmt.Errorf("content too large (%d bytes, max %d)", len(newContent), common.MaxContentSize)
		}
		doc.Content = newContent
	}

	if updateTags != nil {
		doc.Tags = updateTags
	}

	if err := s.Update(doc); err != nil {
		return err
	}

	// Re-chunk if content was updated and is long enough
	if newContent != "" && len(newContent) > 800 {
		chunks := chunker.Chunk(newContent, chunker.DefaultTargetSize, chunker.DefaultOverlap, chunker.DefaultMinSize)
		if len(chunks) > 1 {
			if err := s.AddChunks(doc.ID, chunks); err != nil {
				fmt.Fprintf(os.Stderr, "agent-memory: add chunks: %v\n", err)
			}
		}
	}

	fmt.Println("updated")
	return nil
}
