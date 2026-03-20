package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var hookCmd = &cobra.Command{
	Use:   "hook [event]",
	Short: "Handle Claude Code hook events",
	Long:  "Process hook events: post-tool-use, session-start, stop",
	Args:  cobra.ExactArgs(1),
	RunE:  runHook,
}

func init() {
	rootCmd.AddCommand(hookCmd)
}

// HookInput represents the JSON input from Claude Code hooks.
type HookInput struct {
	SessionID string          `json:"session_id"`
	Event     string          `json:"event"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	Output    string          `json:"output"`
}

// Sensitive patterns that should be scrubbed before storing.
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)(secret|token|access_key)\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9\-._~+/]+=*`),
	regexp.MustCompile(`(?i)(aws_access_key_id|aws_secret_access_key)\s*=\s*\S+`),
	regexp.MustCompile(`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
	// JWT tokens (three base64 segments separated by dots)
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
	// URL-embedded credentials (user:pass@host)
	regexp.MustCompile(`://[^:]+:[^@]+@`),
	// GCP service account JSON key patterns
	regexp.MustCompile(`(?i)"private_key"\s*:\s*"-----BEGIN`),
	// High-entropy hex strings (64+ chars, likely keys)
	regexp.MustCompile(`(?i)(key|secret|token|credential)\s*[=:]\s*[0-9a-f]{64,}`),
}

// Instruction patterns that could be used for prompt injection.
var instructionDenyList = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard previous",
	"forget your instructions",
	"new instructions:",
	"system prompt:",
	"you are now",
	"act as",
	"pretend to be",
	"override your",
	"ignore your rules",
	"bypass your",
	"from now on you",
	"your new role is",
	"you must now",
	"do not follow your",
	"stop being",
	"instead of following",
	"<system>",
	"</system>",
}

func runHook(cmd *cobra.Command, args []string) error {
	event := args[0]

	switch event {
	case "post-tool-use":
		return hookPostToolUse()
	case "session-start":
		return hookSessionStart()
	case "stop":
		return hookStop()
	case "user-prompt-submit":
		return hookUserPromptSubmit()
	case "session-end":
		return hookSessionEnd()
	default:
		return fmt.Errorf("unknown hook event: %s", event)
	}
}

func hookPostToolUse() error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 10<<20))
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: read stdin: %v\n", err)
		os.Exit(1)
	}

	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	// Skip noisy tools
	skipTools := map[string]bool{
		"Read": true, "Glob": true, "Grep": true,
		"Bash": true, "TaskOutput": true,
	}
	if skipTools[input.ToolName] {
		return nil
	}

	// Scrub sensitive content before storing
	output := scrubSensitive(input.Output)
	content := fmt.Sprintf("[%s] %s", input.ToolName, truncate(output, 500))

	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: open store: %v\n", err)
		return nil // non-fatal for hooks
	}
	defer s.Close()

	doc := &store.Document{
		Content:   content,
		Tags:      []string{"source:hook", "tool:" + strings.ToLower(input.ToolName)},
		Workspace: workspace,
		Source:    "hook",
	}
	s.Add(doc)
	return nil
}

func hookSessionStart() error {
	s, err := openStore()
	if err != nil {
		return nil
	}
	defer s.Close()

	pinned := true
	pinnedDocs, _ := s.List(store.ListOptions{
		Workspace: workspace,
		Pinned:    &pinned,
		Limit:     5,
	})

	recentDocs, _ := s.List(store.ListOptions{
		Workspace: workspace,
		Limit:     5,
	})

	if len(pinnedDocs) == 0 && len(recentDocs) == 0 {
		return nil
	}

	// Use structured XML boundary markers for safe context injection
	fmt.Println("<agent-memory-context type=\"data\" readonly=\"true\">")
	if len(pinnedDocs) > 0 {
		fmt.Println("## Pinned Memories")
		for _, d := range pinnedDocs {
			fmt.Printf("- %s\n", sanitizeContextOutput(truncate(strings.ReplaceAll(d.Content, "\n", " "), 200)))
		}
	}
	if len(recentDocs) > 0 {
		fmt.Println("## Recent Memories")
		for _, d := range recentDocs {
			fmt.Printf("- [%s] %s\n", d.CreatedAt.Format("01-02"),
				sanitizeContextOutput(truncate(strings.ReplaceAll(d.Content, "\n", " "), 200)))
		}
	}
	fmt.Println("</agent-memory-context>")
	return nil
}

func hookStop() error {
	return nil
}

func hookUserPromptSubmit() error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 10<<20))
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: read stdin: %v\n", err)
		os.Exit(1)
	}

	var input struct {
		SessionID string `json:"session_id"`
		Prompt    string `json:"prompt"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	if strings.TrimSpace(input.Prompt) == "" {
		return nil
	}

	content := scrubSensitive(input.Prompt)
	content = truncate(content, 1000)

	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: open store: %v\n", err)
		return nil
	}
	defer s.Close()

	doc := &store.Document{
		Content:   content,
		Tags:      []string{"source:hook", "type:user-prompt"},
		Workspace: workspace,
		Source:    "hook",
	}
	s.Add(doc)
	return nil
}

func hookSessionEnd() error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 10<<20))
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: read stdin: %v\n", err)
		os.Exit(1)
	}

	var input struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory hook: open store: %v\n", err)
		return nil
	}
	defer s.Close()

	// Get the last 5 recent documents to build a summary
	recentDocs, err := s.List(store.ListOptions{
		Workspace: workspace,
		Limit:     5,
	})
	if err != nil || len(recentDocs) == 0 {
		return nil
	}

	var summaryLines []string
	for _, d := range recentDocs {
		line := d.Content
		// Take first line or truncate
		if idx := strings.Index(line, "\n"); idx > 0 {
			line = line[:idx]
		}
		line = truncate(line, 100)
		summaryLines = append(summaryLines, "- "+line)
	}
	summary := "Session summary:\n" + strings.Join(summaryLines, "\n")

	doc := &store.Document{
		Content:   summary,
		Tags:      []string{"type:session-summary", "source:hook"},
		Workspace: workspace,
		Source:    "hook",
	}
	s.Add(doc)

	// Also close the session in the sessions table if session_id provided
	if input.SessionID != "" {
		_ = s.SessionEnd(input.SessionID, summary)
	}

	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// scrubSensitive removes sensitive patterns from content before storage.
func scrubSensitive(s string) string {
	for _, pat := range sensitivePatterns {
		s = pat.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// sanitizeContextOutput strips content that could be used for prompt injection
// when memory content is injected into agent session context.
func sanitizeContextOutput(s string) string {
	// Strip XML/HTML-like tags
	tagRe := regexp.MustCompile(`<[^>]+>`)
	s = tagRe.ReplaceAllString(s, "")
	// Remove code fences that could break context boundaries
	s = strings.ReplaceAll(s, "```", "")
	// Check for instruction injection patterns and redact
	lower := strings.ToLower(s)
	for _, pattern := range instructionDenyList {
		if strings.Contains(lower, pattern) {
			return "[REDACTED: potential prompt injection]"
		}
	}
	return s
}
