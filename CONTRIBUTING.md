# Contributing to agent-memory

Thank you for your interest in agent-memory! This guide covers how to set up a development environment, run tests, and submit changes.

## Development Setup

```bash
# Clone the repository
git clone https://github.com/dklymentiev/agent-memory.git
cd agent-memory

# Build (requires Go 1.21+; automatically downloads Go 1.25 toolchain)
make build

# Run tests
make test

# Verify
./agent-memory version
```

## Project Structure

```
agent-memory/
  cmd/                   # CLI command handlers (one file per command)
    root.go              # Root command, global flags
    add.go               # add command
    search.go            # search command
    mcp.go               # MCP server command
    hook.go              # Claude Code hooks
    ...
  internal/              # Internal packages
    store/               # SQLite storage, FTS5, CRUD
    mcp/                 # MCP server (JSON-RPC, tool handlers)
    chunker/             # Markdown-aware content splitting
    embed/               # OpenAI embeddings client
    common/              # Shared constants and utilities
    hooks/               # Hook event handlers
    tagger/              # Auto-tagging from similar documents
  docs/                  # Documentation
    reference.md         # Technical reference (all types, functions, schemas)
    guide.md             # User guide (13 sections, install to security)
  main.go                # Entry point
  go.mod                 # Go module definition
  Makefile               # Build, test, install targets
  hooks-example.json     # Example Claude Code hooks config
```

## Running Tests

```bash
# All tests
make test

# Verbose output
make test-verbose

# Single package
go test ./internal/store/... -v -count=1

# With coverage
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- No external linter required -- `go vet` catches issues
- Add tests for new functionality in `*_test.go` files

```bash
# Run linter
make lint
```

## Making Changes

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint`
6. Commit with a descriptive message (see below)
7. Open a pull request

## Commit Messages

Format: `type: short description`

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Examples:
- `feat: add timeline filtering by tag`
- `fix: handle empty content in chunker`
- `docs: update MCP tool reference`

## MCP Tool Guidelines

MCP tool handlers live in `internal/mcp/server.go`. When adding or modifying tools:

- Define tool schema in `listTools()` with clear descriptions
- Handle the tool call in `callTool()` switch
- Validate all required parameters before processing
- Return errors as JSON `{"error": "message"}`, not Go errors
- Add the tool to `docs/reference.md` and `README.md`

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
