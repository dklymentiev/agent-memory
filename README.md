# agent-memory

[![CI](https://github.com/dklymentiev/agent-memory/actions/workflows/ci.yml/badge.svg)](https://github.com/dklymentiev/agent-memory/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dklymentiev/agent-memory)](https://goreportcard.com/report/github.com/dklymentiev/agent-memory)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Persistent memory for AI coding agents.** Single binary. Zero setup. Instant search.

AI agents forget everything between sessions. You repeat context, re-explain decisions, re-describe architecture. **agent-memory** fixes this -- it gives your agent a persistent, searchable memory backed by a single SQLite file.

```bash
go install github.com/dklymentiev/agent-memory@latest
```

## The Problem

Every time you start a new Claude Code session, your agent starts from scratch:

- "We decided to use PostgreSQL for this" -- you've said it 5 times
- "Don't deploy on Fridays" -- the agent doesn't know
- "The auth flow uses JWT refresh tokens" -- explained it last week, gone today

## The Solution

```bash
# Save a decision once
agent-memory add "Auth uses JWT refresh tokens, 15min access / 7d refresh" \
  -t type:decision -t topic:auth

# Agent finds it when relevant
agent-memory search "authentication tokens"

# Or get smart context automatically at session start
agent-memory context
```

One binary. One SQLite file. No Docker. No external database. No configuration required.

## Key Features

**Search that works** -- FTS5 full-text search with BM25 ranking. Optional hybrid search with OpenAI embeddings (30% keyword + 70% semantic). Finds what you need even with different wording.

**Workspaces** -- isolate memories per project. `agent-memory focus backend-api` and everything stays separate.

**MCP server** -- 14 tools for Claude Code, Cursor, or any MCP-compatible agent. One command to set up: `claude mcp add agent-memory -- agent-memory mcp`

**Auto-capture hooks** -- automatically saves tool outputs and user prompts from Claude Code sessions. Sensitive data (passwords, API keys, tokens, JWTs) is scrubbed before storage.

**Smart context** -- progressive disclosure system assembles the right context for each session: pinned memories first, then recent, then search results -- all within a token budget.

Also: auto-tagging, markdown-aware chunking, content dedup (SHA-256), prompt templates, timeline view, JSON/Markdown export, prompt injection protection.

## Quick Start

```bash
# Add memories
agent-memory add "Use snake_case for Python, camelCase for TypeScript" \
  -t type:decision -t topic:style

# Search
agent-memory search "naming conventions"

# Switch workspace
agent-memory focus my-project

# Get session context (pinned + recent + relevant)
agent-memory context

# Pipe files into memory
cat ARCHITECTURE.md | agent-memory add -f - -t type:artifact --pin
```

## MCP Server (Claude Code Integration)

```bash
# One-liner setup
claude mcp add agent-memory -- agent-memory mcp
```

Or add to `.mcp.json` in your project:

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

14 MCP tools available: `memory_add`, `memory_search`, `memory_context`, `memory_list`, `memory_focus`, `memory_delete`, `memory_update`, `memory_stats`, `memory_timeline`, `memory_save_prompt`, `memory_get_prompt`, `memory_suggest_tags`, `memory_session_start`, `memory_session_end`.

See [docs/guide.md](docs/guide.md) for details on each tool.

## Claude Code Hooks

Auto-capture context from coding sessions:

```json
{
  "hooks": [
    {"event": "PostToolUse", "command": "agent-memory hook post-tool-use", "timeout": 5000},
    {"event": "SessionStart", "command": "agent-memory hook session-start", "timeout": 5000},
    {"event": "UserPromptSubmit", "command": "agent-memory hook user-prompt-submit", "timeout": 5000},
    {"event": "SessionEnd", "command": "agent-memory hook session-end", "timeout": 5000}
  ]
}
```

Hooks scrub sensitive data (passwords, API keys, tokens, JWTs, private keys) and protect against prompt injection.

## How It Compares

| | agent-memory | mem0 | Zep | ChromaDB |
|---|---|---|---|---|
| Setup | `go install`, done | Python + API key | Docker + Postgres | Python + server |
| Dependencies | None (single binary) | Python, OpenAI | Docker, Postgres, Redis | Python, multiple |
| Storage | Single SQLite file | Cloud or self-hosted | Postgres | Persistent dir |
| MCP support | Built-in (14 tools) | No | No | No |
| Search | FTS5 + optional embeddings | Embeddings only | Embeddings + graph | Embeddings only |
| Works offline | Yes (FTS5 mode) | No | Yes | Yes |
| Binary size | ~11MB | N/A | N/A | N/A |

agent-memory is designed for **personal/small-team use** with AI coding agents. If you need a production vector database for millions of documents, use ChromaDB or Pinecone. If you want something that works in 10 seconds with zero infrastructure, this is it.

## Security

- Database and config files: restricted permissions (0600/0700)
- Hook data: sensitive patterns scrubbed before storage (12 regex rules)
- Session context: prompt injection protection (XML boundaries + 17-pattern deny-list)
- Search queries: FTS5 and LIKE injection sanitized
- Content: 1MB limit, SHA-256 dedup, workspace name validation

See [SECURITY.md](SECURITY.md) for full details.

## Storage

All data in one file:

```
~/.agent-memory/memory.db          # default (global)
.agent-memory/memory.db            # per-project (via agent-memory init)
agent-memory --db /path/to/my.db   # custom path
```

## Documentation

- [User Guide](docs/guide.md) -- installation, all features, configuration
- [Technical Reference](docs/reference.md) -- all types, functions, schemas, CLI flags
- [Architecture](ARCHITECTURE.md) -- internal design, data flow, package structure
- [Contributing](CONTRIBUTING.md) -- development setup, code style, PR guidelines
- [Security](SECURITY.md) -- threat model, protections, vulnerability reporting

## Planned

- [ ] Local embeddings via ONNX (e5-small) -- no OpenAI dependency
- [ ] Data retention TTL with automatic cleanup
- [ ] Web UI for browsing and searching memories

## Building from Source

```bash
git clone https://github.com/dklymentiev/agent-memory.git
cd agent-memory
make build      # builds ./agent-memory
make test       # runs tests
make install    # copies to /usr/local/bin
```

Requires Go 1.21+ (automatically downloads Go 1.25 toolchain).

## License

MIT
