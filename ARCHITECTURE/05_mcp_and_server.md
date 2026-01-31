# Server & MCP Architecture

## Client-Server Model
Tasc can run as a standalone local CLI or as a distributed system.
- **Server**: `cmd/serve.go` starts a HTTP REST API (`internal/server`).
- **Client**: `cmd/root.go` detects if `TASC_REMOTE` is set. If so, it initializes `CurrentStore` with `RemoteClient` instead of `SQLiteStore`.
- **Transparency**: The rest of the CLI commands are agnostic to the backend, as they only interact with the `Store` interface.

## Model Context Protocol (MCP)
Tasc implements an MCP Server (`internal/mcp`) to integrate with LLMs (like Gemini).
- **Transport**: Stdio (Standard Input/Output).
- **Tools**: Exposes core `Store` methods as tools for the LLM:
    - `tasc_add_task`
    - `tasc_list_tasks` (with filters)
    - `tasc_complete_task`
    - `tasc_update_task`
    - `tasc_list_projects`
- **Resources**: Exposes `tasc://tasks` as a resource to give the LLM full context of the current state.
- **Workflow**: The LLM uses tools to read state and perform actions. Complex requests (e.g. "reschedule all") are currently handled by the LLM making multiple tool calls or are planned for future batch-tool enhancements.
