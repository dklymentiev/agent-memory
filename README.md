# agent-memory

**Persistent memory for AI coding agents.** Single binary, instant full-text search, workspaces, MCP server.

```bash
# Install with Go (requires Go 1.21+ for automatic toolchain management)
go install github.com/steamfoundry/agent-memory@latest

# Or download from GitHub Releases
# https://github.com/steamfoundry/agent-memory/releases
```

## Why agent-memory?

AI coding agents (Claude Code, Cursor, Aider) lose context between sessions. You repeat yourself. The agent forgets decisions, patterns, and project knowledge.

**agent-memory** fixes this:

- **Instant FTS5 search** -- full-text search with BM25 ranking, zero setup
- **Workspaces** -- separate memories per project, team, or role
- **Auto-capture hooks** -- capture important context from tool usage with sensitive data scrubbing
- **MCP server** -- native integration with any MCP-compatible agent
- **Single file storage** -- one SQLite `.db` file, no external databases
- **Content dedup** -- SHA-256 hash prevents duplicate storage

### Dependencies

agent-memory compiles to a single static binary. Build dependencies (resolved automatically by Go):

| Dependency | Purpose |
|-----------|---------|
| `modernc.org/sqlite` | Pure-Go SQLite with FTS5 (no CGO) |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/segmentio/ksuid` | Sortable unique IDs |

No runtime dependencies. No external database. No Docker.

## Quick Start

```bash
# Add a memory
agent-memory add "Always use snake_case for Python functions in this project" \
  -t type:decision -t topic:style

# Search memories
agent-memory search "naming conventions"

# Switch workspace
agent-memory focus my-project

# Get context for current session (pinned + recent + relevant)
agent-memory context

# Pipe content from files
cat ARCHITECTURE.md | agent-memory add -f - -t type:artifact --pin

# Export all memories
agent-memory export --format json
```

## MCP Server

Use agent-memory as an MCP server for Claude Code or any MCP-compatible client:

```bash
# Add to Claude Code
claude mcp add agent-memory -- agent-memory mcp
```

Or add to `.mcp.json`:

```json
{
  "mcpServers": {
    "agent-memory": {
      "command": "agent-memory",
      "args": ["mcp"]
    }
  }
}
```

**Available MCP tools:**

| Tool | Description |
|------|------------|
| `memory_add` | Add a new memory document (max 1MB) |
| `memory_search` | Full-text search with BM25 ranking |
| `memory_context` | Smart context: pinned + recent + relevant |
| `memory_list` | List documents with filters |
| `memory_focus` | Switch workspace |

## Claude Code Hooks

Auto-capture context from your coding sessions. Add to your hooks config:

```json
{
  "hooks": [
    {"event": "PostToolUse", "command": "agent-memory hook post-tool-use", "timeout": 5000},
    {"event": "SessionStart", "command": "agent-memory hook session-start", "timeout": 5000},
    {"event": "Stop", "command": "agent-memory hook stop", "timeout": 5000}
  ]
}
```

**Hook events:**

| Event | What it does |
|-------|-------------|
| `post-tool-use` | Captures tool outputs (with sensitive data scrubbing) |
| `session-start` | Injects relevant context into new sessions |
| `stop` | Session end handler |

## Workspaces

Organize memories by project, team, or role:

```bash
agent-memory focus backend-api        # switch workspace
agent-memory add "..." -w backend-api # add to specific workspace
agent-memory search "Redis" -w ""     # search all workspaces
agent-memory list                     # list in current workspace
```

## Tags

```bash
agent-memory add "Use PostgreSQL" -t type:decision -t topic:database
agent-memory list -t type:decision

# Common prefixes: type: topic: project: source:
```

## Security

- Database and config files use restricted permissions (0600/0700)
- PostToolUse hook scrubs sensitive patterns (passwords, API keys, tokens, JWTs, private keys, URL credentials)
- Session context output is sanitized against prompt injection (XML boundary markers, instruction pattern deny-list)
- FTS5 search queries are sanitized to prevent query injection
- LIKE wildcards are escaped in tag filters
- Content size capped at 1MB per document
- Workspace names are validated (alphanumeric, hyphens, underscores, max 64 chars)

## Comparison

| Feature | agent-memory | claude-mem | beads | engram |
|---------|:----------:|:--------:|:----:|:-----:|
| Single binary | yes | -- | yes | yes |
| Full-text search | FTS5+BM25 | yes | -- | -- |
| Auto-capture hooks | 3 hooks | 6 hooks | 2 hooks | -- |
| MCP server | built-in | -- | Python | -- |
| Workspaces | yes | -- | -- | -- |
| Pinned documents | yes | -- | -- | -- |
| Content dedup | SHA-256 | -- | hash | -- |
| Single-file storage | .db | -- | -- | yes |

## Planned

- [ ] Semantic search via local ONNX embeddings (e5-small, opt-in download)
- [ ] OpenAI/Anthropic embeddings API fallback
- [ ] Hybrid search (FTS5 + vector similarity with configurable weights)
- [ ] Auto-tag inference from similar documents
- [ ] Markdown-aware document chunking for better embedding quality
- [ ] Session summaries on stop
- [ ] Data retention TTL with automatic cleanup
- [ ] `memory delete` CLI command and MCP tool
- [ ] Web UI

## Storage

All data lives in a single SQLite file:

```bash
# Default: ~/.agent-memory/memory.db
# Per-project: .agent-memory/memory.db (via `agent-memory init`)
# Custom: agent-memory --db /path/to/custom.db add "content"
```

## Configuration

`~/.agent-memory/config.json`:

```json
{
  "active_workspace": "default",
  "db_path": ""
}
```

## Building from Source

Requires Go 1.21+ (automatically downloads Go toolchain if needed).

```bash
git clone https://github.com/steamfoundry/agent-memory.git
cd agent-memory
make build    # builds ./agent-memory binary
make test     # runs unit tests
make install  # copies to /usr/local/bin
```

## License

MIT
