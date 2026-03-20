# Changelog

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
