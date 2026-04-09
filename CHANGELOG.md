# Changelog

## [0.2.0] - 2026-04-08

### Added
- Local ONNX embeddings with all-MiniLM-L6-v2 (384 dimensions, ~0.4s/query)
- Zero-setup experience: ONNX Runtime and model auto-download on first use
- `embeddings enable --local` flag for local ONNX, `--openai` for OpenAI API
- Provider-agnostic embedder factory (`embed.NewEmbedder`) replacing hardcoded OpenAI
- Platform support: Linux x64, macOS (arm64/x64), Windows x64
- `ONNXRUNTIME_LIB` environment variable for custom library path
- `embeddings status` now shows vector dimensions
- 13 new tests: edge cases (empty input, long text, unicode, CJK), factory, tokenizer, batch consistency

### Changed
- Search and MCP server use embedder factory instead of direct OpenAI calls
- Semantic search works without API keys when local embeddings are enabled
- `embeddings enable` without flags auto-detects: OpenAI if key is set, otherwise prompts for `--local` or `--openai`

### Dependencies
- Added `github.com/yalue/onnxruntime_go v1.22.0`

## [0.1.0] - 2026-03-20

### Added
- 16 CLI commands: add, search, list, focus, context, delete, update, export, timeline, prompt save/get/list, embeddings enable/disable/run/status, init, health, version, mcp, hook
- 14 MCP tools for integration with Claude Code, Cursor, and other MCP-compatible agents
- 5 Claude Code hooks with automatic sensitive data scrubbing
- Full-text search with SQLite FTS5 and BM25 ranking
- Optional semantic search via OpenAI embeddings (hybrid: 30% FTS + 70% cosine)
- Workspace isolation for organizing memories by project or role
- Auto-tagging from similar documents
- Auto-chunking with markdown-aware splitting
- Content deduplication via SHA-256
- Progressive disclosure context system (pinned + recent + relevant)
- Prompt template storage and retrieval
- Timeline view with date range filtering
- JSON and Markdown export
- Prompt injection protection for session context
- Per-project initialization with .gitignore integration
