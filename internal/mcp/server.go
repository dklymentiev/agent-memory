// Package mcp implements a Model Context Protocol server over stdio.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/steamfoundry/agent-memory/internal/chunker"
	"github.com/steamfoundry/agent-memory/internal/config"
	"github.com/steamfoundry/agent-memory/internal/embed"
	"github.com/steamfoundry/agent-memory/internal/store"
	"github.com/steamfoundry/agent-memory/internal/tagger"
)

const maxContentSize = 1 << 20 // 1MB

// Server is the MCP stdio server.
type Server struct {
	store     *store.SQLiteStore
	workspace string
	version   string
}

// NewServer creates a new MCP server.
func NewServer(s *store.SQLiteStore, workspace string, version string) *Server {
	if version == "" {
		version = "dev"
	}
	return &Server{store: s, workspace: workspace, version: version}
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Run starts the MCP server with graceful shutdown on SIGTERM/SIGINT.
func (s *Server) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		cancel()
	}()

	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return s.store.Close()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := s.handleRequest(&req)
		if resp != nil {
			encoder.Encode(resp)
		}
	}
}

func (s *Server) handleRequest(req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.respond(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "agent-memory",
				"version": s.version,
			},
		})

	case "tools/list":
		return s.respond(req.ID, map[string]any{
			"tools": s.toolDefinitions(),
		})

	case "tools/call":
		return s.handleToolCall(req)

	case "notifications/initialized":
		return nil

	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) respond(id any, result any) *jsonRPCResponse {
	return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) toolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "memory_add",
			"description": "Add a new memory document",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content":   map[string]any{"type": "string", "description": "Document content (max 1MB)"},
					"tags":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags"},
					"workspace": map[string]any{"type": "string", "description": "Workspace (default: current)"},
				},
				"required": []string{"content"},
			},
		},
		{
			"name":        "memory_search",
			"description": "Search memory documents using full-text search",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":     map[string]any{"type": "string", "description": "Search query"},
					"workspace": map[string]any{"type": "string", "description": "Workspace filter"},
					"limit":     map[string]any{"type": "integer", "description": "Max results (default 10)"},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "memory_context",
			"description": "Get smart context with progressive disclosure: pinned + recent + relevant memories, layered by budget",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":  map[string]any{"type": "string", "description": "Optional relevance query"},
					"limit":  map[string]any{"type": "integer", "description": "Max docs per section"},
					"budget": map[string]any{"type": "integer", "description": "Character budget for context (default 2000)"},
				},
			},
		},
		{
			"name":        "memory_list",
			"description": "List memory documents with filters",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]any{"type": "string"},
					"tag":       map[string]any{"type": "string"},
					"limit":     map[string]any{"type": "integer"},
				},
			},
		},
		{
			"name":        "memory_focus",
			"description": "Switch active workspace",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace": map[string]any{"type": "string", "description": "Workspace name"},
				},
				"required": []string{"workspace"},
			},
		},
		{
			"name":        "memory_delete",
			"description": "Delete a memory document by ID",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Document ID"},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "memory_update",
			"description": "Update a memory document's content and/or tags",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Document ID"},
					"content": map[string]any{"type": "string", "description": "New content (optional)"},
					"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Replace tags (optional)"},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "memory_stats",
			"description": "Get memory statistics: document count, workspaces, DB size",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "memory_timeline",
			"description": "Get documents in chronological order for a date range",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start_date": map[string]any{"type": "string", "description": "Start date (YYYY-MM-DD)"},
					"end_date":   map[string]any{"type": "string", "description": "End date (YYYY-MM-DD, default: today)"},
					"workspace":  map[string]any{"type": "string", "description": "Workspace filter"},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 20)"},
				},
				"required": []string{"start_date"},
			},
		},
		{
			"name":        "memory_save_prompt",
			"description": "Save a reusable prompt template by name",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Prompt template name"},
					"content": map[string]any{"type": "string", "description": "Prompt template content"},
					"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Additional tags"},
				},
				"required": []string{"name", "content"},
			},
		},
		{
			"name":        "memory_get_prompt",
			"description": "Retrieve a saved prompt template by name",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Prompt template name"},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "memory_suggest_tags",
			"description": "Suggest relevant tags for given content based on similar documents",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string", "description": "Content to suggest tags for"},
					"limit":   map[string]any{"type": "integer", "description": "Max tag suggestions (default 5)"},
				},
				"required": []string{"content"},
			},
		},
		{
			"name":        "memory_session_start",
			"description": "Start a new session for tracking agent work",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project":   map[string]any{"type": "string", "description": "Project name"},
					"workspace": map[string]any{"type": "string", "description": "Workspace (default: current)"},
				},
			},
		},
		{
			"name":        "memory_session_end",
			"description": "End a session and record summary",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Session ID"},
					"summary": map[string]any{"type": "string", "description": "Session summary"},
				},
				"required": []string{"id", "summary"},
			},
		},
	}
}

func (s *Server) handleToolCall(req *jsonRPCRequest) *jsonRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{Code: -32602, Message: "invalid params"},
		}
	}

	var result any
	var callErr error

	switch params.Name {
	case "memory_add":
		result, callErr = s.toolAdd(params.Arguments)
	case "memory_search":
		result, callErr = s.toolSearch(params.Arguments)
	case "memory_context":
		result, callErr = s.toolContext(params.Arguments)
	case "memory_list":
		result, callErr = s.toolList(params.Arguments)
	case "memory_focus":
		result, callErr = s.toolFocus(params.Arguments)
	case "memory_delete":
		result, callErr = s.toolDelete(params.Arguments)
	case "memory_update":
		result, callErr = s.toolUpdate(params.Arguments)
	case "memory_stats":
		result, callErr = s.toolStats()
	case "memory_timeline":
		result, callErr = s.toolTimeline(params.Arguments)
	case "memory_save_prompt":
		result, callErr = s.toolSavePrompt(params.Arguments)
	case "memory_get_prompt":
		result, callErr = s.toolGetPrompt(params.Arguments)
	case "memory_suggest_tags":
		result, callErr = s.toolSuggestTags(params.Arguments)
	case "memory_session_start":
		result, callErr = s.toolSessionStart(params.Arguments)
	case "memory_session_end":
		result, callErr = s.toolSessionEnd(params.Arguments)
	default:
		callErr = fmt.Errorf("unknown tool: %s", params.Name)
	}

	if callErr != nil {
		return s.respond(req.ID, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Error: " + callErr.Error()},
			},
			"isError": true,
		})
	}

	text, _ := json.Marshal(result)
	return s.respond(req.ID, map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
	})
}

func (s *Server) toolAdd(args json.RawMessage) (any, error) {
	var p struct {
		Content   string   `json:"content"`
		Tags      []string `json:"tags"`
		Workspace string   `json:"workspace"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if len(p.Content) > maxContentSize {
		return nil, fmt.Errorf("content too large (%d bytes, max %d)", len(p.Content), maxContentSize)
	}
	ws := p.Workspace
	if ws == "" {
		ws = s.workspace
	}
	doc := &store.Document{
		Content:   p.Content,
		Tags:      p.Tags,
		Workspace: ws,
		Source:    "mcp",
	}
	if err := s.store.Add(doc); err != nil {
		return nil, err
	}

	// Auto-tag: infer tags from similar documents and merge with user tags
	inferred := tagger.InferTags(s.store, p.Content, ws, 5)
	if len(inferred) > 0 {
		merged := mergeTags(doc.Tags, inferred)
		if len(merged) > len(doc.Tags) {
			doc.Tags = merged
			_ = s.store.Update(doc)
		}
	}

	// Chunking: if content is long enough, split and store chunks
	if len(p.Content) > 800 {
		chunks := chunker.Chunk(p.Content, chunker.DefaultTargetSize, chunker.DefaultOverlap, chunker.DefaultMinSize)
		if len(chunks) > 1 {
			_ = s.store.AddChunks(doc.ID, chunks)
		}
	}

	// Embed new chunks if embeddings are enabled
	s.embedChunksForDoc(doc.ID)

	return map[string]string{"id": doc.ID, "status": "created"}, nil
}

// embedChunksForDoc embeds any unembedded chunks for a document if embeddings are enabled.
func (s *Server) embedChunksForDoc(docID string) {
	cfg := config.Load()
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

	docChunks, err := s.store.GetUnembeddedChunksByDoc(docID, 1000)
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
		_ = s.store.UpdateChunkEmbedding(docChunks[i].ID, emb)
	}
}

// mergeTags merges user-provided and inferred tags, user tags take priority.
func mergeTags(userTags, inferred []string) []string {
	seen := make(map[string]bool, len(userTags))
	for _, t := range userTags {
		seen[t] = true
	}
	merged := append([]string{}, userTags...)
	for _, t := range inferred {
		if !seen[t] {
			merged = append(merged, t)
			seen[t] = true
		}
	}
	return merged
}

func (s *Server) toolSearch(args json.RawMessage) (any, error) {
	var p struct {
		Query     string `json:"query"`
		Workspace string `json:"workspace"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	ws := p.Workspace
	if ws == "" {
		ws = s.workspace
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 10
	}

	// Auto-detect: use hybrid search if embeddings are enabled
	cfg := config.Load()
	if cfg.EmbeddingProvider != "" {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey != "" {
			embedder, err := embed.NewOpenAIEmbedder(apiKey, cfg.EmbeddingModel)
			if err == nil {
				defer embedder.Close()
				queryEmb, err := embedder.Embed(p.Query)
				if err == nil {
					return s.store.HybridSearch(p.Query, queryEmb, ws, limit)
				}
			}
		}
	}

	// Fallback to FTS
	return s.store.Search(p.Query, ws, limit)
}

func (s *Server) toolContext(args json.RawMessage) (any, error) {
	var p struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Budget int    `json:"budget"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 5
	}
	budget := p.Budget
	if budget <= 0 {
		budget = 2000
	}

	var sections []string
	used := 0

	// Layer 1: Pinned documents (titles/first line only)
	pinned := true
	pinnedDocs, _ := s.store.List(store.ListOptions{
		Workspace: s.workspace, Pinned: &pinned, Limit: limit,
	})
	if len(pinnedDocs) > 0 {
		var lines []string
		for _, d := range pinnedDocs {
			line := firstLine(d.Content)
			entry := fmt.Sprintf("- [short] %s", line)
			if used+len(entry) > budget {
				break
			}
			lines = append(lines, entry)
			used += len(entry) + 1
		}
		if len(lines) > 0 {
			section := "## Pinned\n" + strings.Join(lines, "\n")
			sections = append(sections, section)
		}
	}

	// Layer 2: Recent document summaries (first 100 chars each)
	recentDocs, _ := s.store.List(store.ListOptions{
		Workspace: s.workspace, Limit: limit,
	})
	if len(recentDocs) > 0 && used < budget {
		var lines []string
		for _, d := range recentDocs {
			c := d.Content
			if len(c) > 100 {
				c = c[:100] + "..."
			}
			c = strings.ReplaceAll(c, "\n", " ")
			entry := fmt.Sprintf("- [%s] %s", d.CreatedAt.Format("2006-01-02"), c)
			if used+len(entry) > budget {
				break
			}
			lines = append(lines, entry)
			used += len(entry) + 1
		}
		if len(lines) > 0 {
			section := "## Recent (last 24h)\n" + strings.Join(lines, "\n")
			sections = append(sections, section)
		}
	}

	// Layer 3: Full content of relevant documents (on demand)
	if p.Query != "" && used < budget {
		results, _ := s.store.Search(p.Query, s.workspace, limit)
		if len(results) > 0 {
			var lines []string
			for _, r := range results {
				c := r.Content
				maxLen := 200
				remaining := budget - used
				if remaining < maxLen {
					maxLen = remaining
				}
				if maxLen <= 0 {
					break
				}
				if len(c) > maxLen {
					c = c[:maxLen] + "..."
				}
				c = strings.ReplaceAll(c, "\n", " ")
				idPrefix := r.ID
				if len(idPrefix) > 8 {
					idPrefix = idPrefix[:8]
				}
				entry := fmt.Sprintf("- [%s] %s", idPrefix, c)
				lines = append(lines, entry)
				used += len(entry) + 1
			}
			if len(lines) > 0 {
				section := "## Relevant to: " + p.Query + "\n" + strings.Join(lines, "\n")
				sections = append(sections, section)
			}
		}
	}

	return map[string]string{"context": strings.Join(sections, "\n\n")}, nil
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	if len(s) > 100 {
		return s[:100] + "..."
	}
	return s
}

func (s *Server) toolList(args json.RawMessage) (any, error) {
	var p struct {
		Workspace string `json:"workspace"`
		Tag       string `json:"tag"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	ws := p.Workspace
	if ws == "" {
		ws = s.workspace
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	return s.store.List(store.ListOptions{
		Workspace: ws, Tag: p.Tag, Limit: limit,
	})
}

func (s *Server) toolFocus(args json.RawMessage) (any, error) {
	var p struct {
		Workspace string `json:"workspace"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if err := config.ValidateWorkspace(p.Workspace); err != nil {
		return nil, err
	}
	s.workspace = p.Workspace
	return map[string]string{"workspace": p.Workspace, "status": "switched"}, nil
}

func (s *Server) toolDelete(args json.RawMessage) (any, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if err := s.store.Delete(p.ID); err != nil {
		return nil, err
	}
	return map[string]string{"id": p.ID, "status": "deleted"}, nil
}

func (s *Server) toolUpdate(args json.RawMessage) (any, error) {
	var p struct {
		ID      string   `json:"id"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}

	doc, err := s.store.Get(p.ID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %s", p.ID)
	}

	if p.Content != "" {
		if len(p.Content) > maxContentSize {
			return nil, fmt.Errorf("content too large (%d bytes, max %d)", len(p.Content), maxContentSize)
		}
		doc.Content = p.Content
	}
	if p.Tags != nil {
		doc.Tags = p.Tags
	}

	if err := s.store.Update(doc); err != nil {
		return nil, err
	}

	// Re-chunk if content was updated and is long enough
	if p.Content != "" && len(p.Content) > 800 {
		chunks := chunker.Chunk(p.Content, chunker.DefaultTargetSize, chunker.DefaultOverlap, chunker.DefaultMinSize)
		if len(chunks) > 1 {
			_ = s.store.AddChunks(doc.ID, chunks)
			s.embedChunksForDoc(doc.ID)
		}
	}

	return map[string]string{"id": doc.ID, "status": "updated"}, nil
}

func (s *Server) toolStats() (any, error) {
	return s.store.Stats()
}

func (s *Server) toolTimeline(args json.RawMessage) (any, error) {
	var p struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Workspace string `json:"workspace"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if p.EndDate == "" {
		p.EndDate = time.Now().Format("2006-01-02")
	}
	ws := p.Workspace
	if ws == "" {
		ws = s.workspace
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	return s.store.Timeline(p.StartDate, p.EndDate, ws, limit)
}

func (s *Server) toolSavePrompt(args json.RawMessage) (any, error) {
	var p struct {
		Name    string   `json:"name"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if p.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	if len(p.Content) > maxContentSize {
		return nil, fmt.Errorf("content too large (%d bytes, max %d)", len(p.Content), maxContentSize)
	}

	tags := append([]string{"type:prompt", "prompt:" + p.Name}, p.Tags...)
	doc := &store.Document{
		Content:   p.Content,
		Tags:      tags,
		Workspace: s.workspace,
		Source:    "mcp",
	}
	if err := s.store.Add(doc); err != nil {
		return nil, err
	}
	return map[string]string{"id": doc.ID, "name": p.Name, "status": "saved"}, nil
}

func (s *Server) toolGetPrompt(args json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	docs, err := s.store.List(store.ListOptions{
		Tag:   "prompt:" + p.Name,
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("prompt not found: %s", p.Name)
	}
	return map[string]string{
		"name":    p.Name,
		"content": docs[0].Content,
		"id":      docs[0].ID,
	}, nil
}

func (s *Server) toolSuggestTags(args json.RawMessage) (any, error) {
	var p struct {
		Content string `json:"content"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if p.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 5
	}
	tags := tagger.InferTags(s.store, p.Content, s.workspace, limit)
	if tags == nil {
		tags = []string{}
	}
	return map[string]any{"tags": tags}, nil
}

func (s *Server) toolSessionStart(args json.RawMessage) (any, error) {
	var p struct {
		Project   string `json:"project"`
		Workspace string `json:"workspace"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	ws := p.Workspace
	if ws == "" {
		ws = s.workspace
	}
	id := ksuid.New().String()
	if err := s.store.SessionStart(id, p.Project, ws); err != nil {
		return nil, err
	}
	return map[string]string{"id": id, "status": "started"}, nil
}

func (s *Server) toolSessionEnd(args json.RawMessage) (any, error) {
	var p struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if err := s.store.SessionEnd(p.ID, p.Summary); err != nil {
		return nil, err
	}
	return map[string]string{"id": p.ID, "status": "ended"}, nil
}
