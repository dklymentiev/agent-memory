# agent-memory User Guide

**Version 0.1.0**

Persistent memory for AI coding agents. agent-memory gives any AI agent (Claude Code,
Cursor, Aider) persistent context across sessions -- instant full-text search, workspaces,
auto-capture hooks, and an MCP server, all in a single binary backed by one SQLite file.

---

## 1. Getting Started

### Install

```bash
# Option A: Install with Go (requires Go 1.21+; auto-downloads Go 1.25 toolchain)
go install github.com/dklymentiev/agent-memory@latest

# Option B: Download from GitHub Releases
# https://github.com/dklymentiev/agent-memory/releases

# Option C: Build from source
git clone https://github.com/dklymentiev/agent-memory.git
cd agent-memory
make build      # produces ./agent-memory binary
make install    # copies to /usr/local/bin
```

### First memory

```bash
# Add your first memory
agent-memory add "This project uses PostgreSQL 16 with pgvector extension" \
  -t type:decision -t topic:database

# Output: 2mPQ4k7a9B1cD3eF5gH6j  (KSUID -- your document ID)
```

### First search

```bash
agent-memory search "database"
# Output:
# id: 2mPQ4k7a9B1cD3eF5gH6j
# tags: type:decision, topic:database
# workspace: default
# created: 2026-03-20 14:30
#
# This project uses PostgreSQL 16 with pgvector extension
```

### Verify installation

```bash
agent-memory version    # prints: agent-memory 0.1.0
agent-memory health     # prints: ok
```

---

## 2. Adding Memories

### From command-line argument

```bash
agent-memory add "API rate limit is 100 req/min per user"
```

### From a file

```bash
agent-memory add -f ARCHITECTURE.md -t type:artifact -t topic:architecture
```

### From stdin (pipe)

```bash
cat README.md | agent-memory add -f - -t type:artifact
echo "Deploy requires Docker 24+" | agent-memory add
```

### With tags

Tags use a `key:value` convention. You can repeat `-t` for multiple tags:

```bash
agent-memory add "Use snake_case for all Python functions" \
  -t type:decision \
  -t topic:style \
  -t project:backend-api
```

Common tag prefixes:
- `type:` -- decision, note, artifact, worklog, prompt
- `topic:` -- dns, database, auth, deploy
- `project:` -- backend-api, frontend, infra
- `source:` -- cli, hook, mcp (set automatically)

### Pinning documents

Pinned documents appear in every `context` call and every `session-start` hook:

```bash
agent-memory add "CRITICAL: Never deploy on Fridays" --pin -t type:decision
```

### Content limits

- Maximum content size: 1 MB per document
- Empty content is rejected
- Duplicate content within the same workspace is rejected (SHA-256 dedup)

### Auto-tagging

When you add a document, agent-memory searches for similar existing documents and
infers tags from them. Tags appearing in 2+ of the top 10 similar results are
automatically merged with your provided tags. Your explicit tags always take priority.

### Auto-chunking

Documents longer than 800 characters are automatically split into overlapping chunks
for better search granularity. Chunking is markdown-aware: it splits at headers, list
items, paragraphs, and sentence boundaries (in that priority order). Chunks are stored
alongside the document and used for semantic search when embeddings are enabled.

---

## 3. Searching

### Basic full-text search (FTS5 + BM25)

```bash
agent-memory search "database migration"
agent-memory search "deploy process" -n 5        # limit to 5 results
agent-memory search "auth tokens" --json          # JSON output
```

### Semantic search (requires embeddings)

```bash
# Semantic only -- finds conceptually similar docs even without exact keyword matches
agent-memory search "how do we handle user authentication" --semantic

# FTS only -- ignores embeddings even if enabled
agent-memory search "auth" --fts
```

### Hybrid search (automatic)

When embeddings are enabled and `OPENAI_API_KEY` is set, search automatically uses
hybrid mode: FTS5 BM25 (30% weight) + semantic cosine similarity (70% weight). This
gives you both keyword precision and conceptual recall.

### Search across all workspaces

```bash
agent-memory search "Redis" -w ""     # empty workspace = search all
```

### Output format

Default output shows id, tags, workspace, created date, and content (truncated to 200 chars). Use `--json` for machine-readable output with full content.

---

## 4. Workspaces

Workspaces isolate memories by project, team, or role. Each document belongs to exactly one workspace.

### Switch workspace

```bash
agent-memory focus backend-api
# Output: Switched to workspace: backend-api

agent-memory focus
# Output: Active workspace: backend-api
```

### Add to a specific workspace

```bash
agent-memory add "Redis cache TTL is 300s" -w backend-api
```

### List across workspaces

```bash
agent-memory list                     # current workspace only
agent-memory list -w backend-api      # specific workspace
agent-memory search "Redis" -w ""     # search all workspaces
```

### Workspace naming rules

- 1-64 characters
- Alphanumeric, hyphens, underscores
- Must start with a letter or number
- Examples: `default`, `backend-api`, `infra_2026`, `myProject`

### Workspace persistence

The active workspace is saved in `~/.agent-memory/config.json` and persists across
sessions. You can also override it per-command with the `-w` flag.

---

## 5. Context and Progressive Disclosure

The `context` command assembles relevant context for the current session using a
budget-based progressive disclosure system.

### Basic usage

```bash
agent-memory context                          # default: 1000 char budget
agent-memory context --budget 2000            # larger budget
agent-memory context -q "authentication"      # focus on a topic
agent-memory context -n 10                    # more docs per section
```

### How it works

Context is assembled in three layers, each consuming from the character budget:

1. **Pinned** (Layer 1) -- First line of each pinned document. Always included first.
2. **Recent** (Layer 2) -- First 100 chars of recent documents. Included if budget remains.
3. **Relevant** (Layer 3) -- FTS search results (up to 200 chars each). Query defaults to
   the current directory name if not specified.

Output format:

```
## Pinned
- [short] CRITICAL: Never deploy on Fridays

## Recent (last 24h)
- [2026-03-20] API rate limit is 100 req/min per user...

## Relevant to: backend-api
- [2mPQ4k7a] This project uses PostgreSQL 16 with pgvector extension...
```

---

## 6. Prompt Templates

Save and reuse prompt templates across sessions.

### Save a prompt

```bash
agent-memory prompt save code-review "Review this code for:
1. Security vulnerabilities
2. Performance issues
3. Error handling gaps
4. Missing tests"

# Or pipe from a file
cat prompts/review.txt | agent-memory prompt save code-review
```

### Retrieve a prompt

```bash
agent-memory prompt get code-review
# Output: Review this code for: ...
```

### List all prompts

```bash
agent-memory prompt list
# Output:
# 2mPQ4k7a  code-review          Review this code for: 1. Security vuln...
# 2mPR8j2b  debug-checklist      When debugging: 1. Check logs first...
```

### How prompts work

Prompts are stored as regular documents with special tags (`type:prompt`, `prompt:<name>`).
They follow all the same rules as regular documents: workspace scoping, dedup, etc.

---

## 7. Timeline

View documents chronologically for a date range.

### Basic usage

```bash
agent-memory timeline                                    # last 7 days
agent-memory timeline --from 2026-03-01 --to 2026-03-20  # specific range
agent-memory timeline --from 2026-03-15 -n 50            # more results
agent-memory timeline --json                              # JSON output
```

### Default behavior

- `--from` defaults to 7 days ago
- `--to` defaults to today
- Results are ordered by creation date (newest first)
- Limited to 20 results by default

---

## 8. MCP Server

agent-memory includes a built-in Model Context Protocol (MCP) server for integration
with Claude Code and other MCP-compatible AI agents.

### Setup with Claude Code

```bash
# One-liner setup
claude mcp add agent-memory -- agent-memory mcp
```

### Setup via .mcp.json

Create or edit `.mcp.json` in your project root:

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

### With a specific workspace

```json
{
  "mcpServers": {
    "agent-memory": {
      "command": "agent-memory",
      "args": ["mcp", "-w", "my-project"]
    }
  }
}
```

### With a per-project database

```json
{
  "mcpServers": {
    "agent-memory": {
      "command": "agent-memory",
      "args": ["mcp", "--db", ".agent-memory/memory.db"]
    }
  }
}
```

### Available MCP tools

The MCP server exposes 14 tools:

| Tool | Description |
|------|-------------|
| `memory_add` | Add a new memory document (max 1MB). Auto-tags, chunks, and embeds. |
| `memory_search` | Search with FTS5. Auto-upgrades to hybrid when embeddings are enabled. |
| `memory_context` | Smart context: pinned + recent + relevant, layered by character budget. |
| `memory_list` | List documents with workspace/tag filters. |
| `memory_focus` | Switch active workspace for this session. |
| `memory_delete` | Delete a document by ID. |
| `memory_update` | Update content and/or tags. Re-chunks if content is long. |
| `memory_stats` | Get document count, workspace breakdown, DB size. |
| `memory_timeline` | Get documents for a date range. |
| `memory_save_prompt` | Save a reusable prompt template by name. |
| `memory_get_prompt` | Retrieve a saved prompt template. |
| `memory_suggest_tags` | Suggest tags for content based on similar documents. |
| `memory_session_start` | Start a session for tracking agent work. |
| `memory_session_end` | End a session and record a summary. |

### Protocol details

- Transport: stdio (stdin/stdout)
- Protocol: JSON-RPC 2.0 (newline-delimited)
- Protocol version: `2024-11-05`
- Graceful shutdown on SIGTERM/SIGINT

---

## 9. Claude Code Hooks

Hooks auto-capture context from Claude Code sessions. They run as shell commands
triggered by Claude Code events.

### Setup

Copy `hooks-example.json` to your Claude Code hooks config, or add manually:

```json
{
  "hooks": [
    {"event": "PostToolUse", "command": "agent-memory hook post-tool-use", "timeout": 5000},
    {"event": "SessionStart", "command": "agent-memory hook session-start", "timeout": 5000},
    {"event": "Stop", "command": "agent-memory hook stop", "timeout": 5000},
    {"event": "UserPromptSubmit", "command": "agent-memory hook user-prompt-submit", "timeout": 5000},
    {"event": "SessionEnd", "command": "agent-memory hook session-end", "timeout": 5000}
  ]
}
```

### Hook events

**PostToolUse** -- Captures tool outputs after each tool call.
- Receives JSON on stdin: `{"session_id", "event", "tool_name", "tool_input", "output"}`
- Skips noisy tools: Read, Glob, Grep, Bash, TaskOutput
- Scrubs sensitive data (passwords, API keys, tokens, JWTs, private keys)
- Stores output truncated to 500 chars with tags `source:hook`, `tool:<name>`

**SessionStart** -- Injects relevant context into new sessions.
- Outputs up to 5 pinned documents and 5 recent documents
- Content wrapped in `<agent-memory-context type="data" readonly="true">` XML tags
- Sanitized against prompt injection (instruction pattern deny-list)

**Stop** -- No-op placeholder for future use.

**UserPromptSubmit** -- Captures user prompts.
- Receives JSON on stdin: `{"session_id", "prompt"}`
- Scrubs sensitive data
- Stores prompt truncated to 1000 chars with tags `source:hook`, `type:user-prompt`

**SessionEnd** -- Creates a session summary.
- Builds summary from the last 5 recent documents (first line of each)
- Stores with tag `type:session-summary`
- Closes the session record in the sessions table if `session_id` is provided

### Sensitive data scrubbing

The PostToolUse and UserPromptSubmit hooks automatically scrub these patterns before
storing content:

- `password=...`, `api_key=...`, `secret=...`, `token=...`
- Bearer tokens
- AWS access key ID / secret access key
- PEM private keys (`-----BEGIN PRIVATE KEY-----`)
- JWT tokens (three dot-separated base64 segments)
- URL-embedded credentials (`://user:pass@host`)
- GCP service account JSON keys
- High-entropy hex strings (64+ chars after key/secret/token prefix)

All matches are replaced with `[REDACTED]`.

### Prompt injection protection

Session context output (from `session-start`) is protected against prompt injection:

- XML/HTML tags are stripped from memory content
- Code fences (```) are removed
- Content is checked against an instruction deny-list (17 patterns like
  "ignore previous instructions", "you are now", "system prompt:", etc.)
- Matching content is replaced with `[REDACTED: potential prompt injection]`
- Context is wrapped in read-only XML boundary markers

---

## 10. Semantic Search / Embeddings

agent-memory supports optional semantic search using OpenAI embeddings. This enables
finding conceptually similar documents even when they don't share exact keywords.

### Enable embeddings

```bash
export OPENAI_API_KEY="sk-..."
agent-memory embeddings enable
# Output: Embeddings enabled: openai / text-embedding-3-small
```

### Generate embeddings for existing documents

```bash
agent-memory embeddings run
# Output:
# Embedding 42 chunks...
#   100/42
# Done. Embedded 42 chunks.
```

To re-embed all chunks (e.g., after changing models):

```bash
agent-memory embeddings run --all
```

### Check status

```bash
agent-memory embeddings status
# Output:
# Embeddings: enabled
# Provider:   openai
# Model:      text-embedding-3-small
# Chunks:     42 total, 42 with embeddings
```

### Disable embeddings

```bash
agent-memory embeddings disable
```

### How it works

1. Documents >800 chars are automatically split into overlapping chunks (target: 800 chars, overlap: 150 chars).
2. Each chunk is embedded via the OpenAI `text-embedding-3-small` model (1536 dimensions).
3. Embeddings are stored as raw little-endian float32 bytes in the `chunks` table.
4. Search computes cosine similarity between the query embedding and all chunk embeddings.
5. Best chunk score per document is used for ranking.

### Hybrid search

When embeddings are enabled, `search` and the `memory_search` MCP tool automatically use
hybrid search:
- FTS5 BM25 score (normalized to 0..1) with weight 0.3
- Semantic cosine similarity with weight 0.7
- Combined score determines final ranking

You can force a specific mode:
```bash
agent-memory search "auth" --fts        # FTS only
agent-memory search "auth" --semantic   # semantic only
```

### Embedding on add

When embeddings are enabled, new documents are automatically chunked and embedded
at add time. This applies to both CLI `add` and the `memory_add` MCP tool.

### Costs

Embeddings use the OpenAI API which has per-token costs. `text-embedding-3-small` is
the most cost-effective model. Batch processing (100 chunks per API call) minimizes
request overhead. One automatic retry with 1-second delay on transient failures.

---

## 11. Import / Export

### Export to JSON

```bash
# Export current workspace
agent-memory export --format json > backup.json

# Export all workspaces
agent-memory export --format json --all > full-backup.json

# Export only specific tags
agent-memory export --format json -t type:decision > decisions.json
```

### Export to Markdown

```bash
agent-memory export --format markdown > memories.md
agent-memory export --format md --all > all-memories.md
```

Markdown format:

```markdown
## 2mPQ4k7a9B1cD3eF5gH6j

**Tags:** type:decision, topic:database

**Workspace:** default | **Created:** 2026-03-20 14:30

This project uses PostgreSQL 16 with pgvector extension

---

## 2mPR8j2bK4lM6nP8qR0s

...
```

### Import

To import from a JSON backup, pipe each document through `add`:

```bash
# Re-add from exported JSON (using jq to extract content)
cat backup.json | jq -r '.[].content' | while read -r line; do
  agent-memory add "$line"
done
```

---

## 12. Configuration

### Config file

Location: `~/.agent-memory/config.json`

```json
{
  "active_workspace": "default",
  "embedding_provider": "openai",
  "embedding_model": "text-embedding-3-small",
  "db_path": ""
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `active_workspace` | `"default"` | Current workspace (changed by `focus` command) |
| `embedding_provider` | `""` | Embedding provider (`""` = disabled, `"openai"` = enabled) |
| `embedding_model` | `""` | Embedding model name |
| `db_path` | `""` | Custom database path (empty = default location) |

### Environment variables

| Variable | Purpose |
|----------|---------|
| `OPENAI_API_KEY` | Required for semantic search and embeddings |
| `XDG_DATA_HOME` | Override data directory (default: `~/.agent-memory`) |
| `XDG_CONFIG_HOME` | Override config directory (default: `~/.agent-memory`) |

### Per-project initialization

```bash
cd /path/to/my-project
agent-memory init
# Output: Initialized agent-memory in /path/to/my-project/.agent-memory
```

This creates:
- `.agent-memory/memory.db` -- local project database
- Appends `.agent-memory/` to `.gitignore` (if `.gitignore` exists)

When using a per-project database, pass the `--db` flag:

```bash
agent-memory --db .agent-memory/memory.db add "project-specific note"
```

### Database location priority

1. `--db` CLI flag (highest priority)
2. `db_path` in config.json
3. `~/.agent-memory/memory.db` (default)

### XDG compliance

agent-memory respects XDG Base Directory Specification:

```bash
# Data (database)
$XDG_DATA_HOME/agent-memory/memory.db      # if XDG_DATA_HOME is set
~/.agent-memory/memory.db                   # fallback

# Config
$XDG_CONFIG_HOME/agent-memory/config.json   # if XDG_CONFIG_HOME is set
~/.agent-memory/config.json                 # fallback
```

---

## 13. Security

### File permissions

- Database file: `0600` (owner read/write only)
- Config directory: `0700` (owner access only)
- Config file: `0600` (owner read/write only)

### Sensitive data scrubbing

All hook-captured content is scrubbed before storage. The following patterns are
detected and replaced with `[REDACTED]`:

- Password, API key, secret, and token assignments
- Bearer authorization tokens
- AWS access key ID and secret access key
- PEM private keys
- JWT tokens
- URL-embedded credentials (`://user:pass@host`)
- GCP service account JSON keys
- High-entropy hex strings (64+ chars) following key/secret/token words

### Prompt injection protection

When memory content is injected into agent sessions (via the `session-start` hook),
it is sanitized against prompt injection attacks:

1. **XML boundary markers** -- Context is wrapped in `<agent-memory-context type="data" readonly="true">` tags that signal to the LLM this is data, not instructions.
2. **Tag stripping** -- All XML/HTML tags within memory content are removed.
3. **Code fence removal** -- Triple backtick code fences are stripped to prevent context boundary manipulation.
4. **Instruction deny-list** -- Content containing any of 17 known injection patterns (like "ignore previous instructions", "system prompt:", "you are now") is replaced with `[REDACTED: potential prompt injection]`.

### Query sanitization

- **FTS5 queries** are sanitized: quotes, parentheses, brackets, asterisks, colons, slashes, and other special characters are stripped. Queries are limited to 30 words.
- **LIKE wildcards** (`%` and `_`) are escaped in tag filter queries to prevent SQL wildcard injection.
- **Date inputs** for timeline queries are validated against `YYYY-MM-DD` format.

### Content deduplication

SHA-256 content hashing prevents duplicate documents within the same workspace.
The same content can exist in different workspaces.

### Workspace name validation

Workspace names are validated against `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$` to prevent
directory traversal, injection, and other naming attacks.

### API key validation

OpenAI API keys are validated for minimum length (20 chars) and required `sk-` prefix
before any API calls are made.
