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

	"github.com/steamfoundry/agent-memory/internal/config"
	"github.com/steamfoundry/agent-memory/internal/store"
)

const maxContentSize = 1 << 20 // 1MB

// Server is the MCP stdio server.
type Server struct {
	store     *store.SQLiteStore
	workspace string
}

// NewServer creates a new MCP server.
func NewServer(s *store.SQLiteStore, workspace string) *Server {
	return &Server{store: s, workspace: workspace}
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
				"version": "0.1.0",
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
			"description": "Get smart context: pinned + recent + relevant memories",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Optional relevance query"},
					"limit": map[string]any{"type": "integer", "description": "Max docs per section"},
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
	return map[string]string{"id": doc.ID, "status": "created"}, nil
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
	return s.store.Search(p.Query, ws, limit)
}

func (s *Server) toolContext(args json.RawMessage) (any, error) {
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 5
	}

	var sections []string

	pinned := true
	pinnedDocs, _ := s.store.List(store.ListOptions{
		Workspace: s.workspace, Pinned: &pinned, Limit: limit,
	})
	if len(pinnedDocs) > 0 {
		var lines []string
		for _, d := range pinnedDocs {
			lines = append(lines, d.Content)
		}
		sections = append(sections, "## Pinned\n"+strings.Join(lines, "\n---\n"))
	}

	recentDocs, _ := s.store.List(store.ListOptions{
		Workspace: s.workspace, Limit: limit,
	})
	if len(recentDocs) > 0 {
		var lines []string
		for _, d := range recentDocs {
			c := d.Content
			if len(c) > 300 {
				c = c[:300] + "..."
			}
			lines = append(lines, fmt.Sprintf("[%s] %s", d.ID[:8], c))
		}
		sections = append(sections, "## Recent\n"+strings.Join(lines, "\n"))
	}

	if p.Query != "" {
		results, _ := s.store.Search(p.Query, s.workspace, limit)
		if len(results) > 0 {
			var lines []string
			for _, r := range results {
				c := r.Content
				if len(c) > 300 {
					c = c[:300] + "..."
				}
				lines = append(lines, fmt.Sprintf("[%s] %s", r.ID[:8], c))
			}
			sections = append(sections, "## Relevant\n"+strings.Join(lines, "\n"))
		}
	}

	return map[string]string{"context": strings.Join(sections, "\n\n")}, nil
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
