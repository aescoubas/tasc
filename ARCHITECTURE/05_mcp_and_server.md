# Server & MCP Architecture

## Client-Server Model
Tasc can run as a standalone local CLI or as a distributed system.
- **Server**: `cmd/serve.go` starts an HTTP REST API (`internal/server`). On this laptop
  a `systemd --user` unit runs it on port 8081, which means **a process already holds
  `~/.tasc.db` open** — hence `PRAGMA busy_timeout = 10000` in `internal/db/db.go`.
- **Client**: `cmd/root.go` detects `TASC_REMOTE`. If set, `CurrentStore` becomes
  `RemoteClient` instead of `SQLiteStore`.
- **Transparency**: the rest of the CLI is agnostic to the backend, since it only ever
  touches the `Store` interface.
- **Task filters**: `GET /tasks` accepts comma-separated `status` values and
  `include_deleted=true`, alongside project, ID and due-date filters. Status matches
  the displayed dependency state (`blocked` for unfinished tasks with open blockers).
  With no explicit status, the default excludes `done`, `deleted` and `undefined`;
  `include_deleted=true` includes all three. An explicit status still restricts the
  results when that flag is set. Invalid boolean values return HTTP 400.

## Model Context Protocol (MCP)
`internal/mcp/server.go` implements an MCP server over `mark3labs/mcp-go`.
- **Transport**: stdio, via `server.ServeStdio`. **Stdout is the protocol** — diagnostics
  go through `log` (stderr by default) and never `fmt.Print`.
- **Resource**: `tasc://tasks`, giving the model the current state in one read.
- **Tools**: 19, covering the `Store` interface.

| Area | Tools |
|---|---|
| Tasks | `tasc_add_task`, `tasc_get_task`, `tasc_update_task`, `tasc_delete_task`, `tasc_list_tasks`, `tasc_search_tasks` |
| Task actions | `tasc_complete_task`, `tasc_start_task`, `tasc_stop_task`, `tasc_get_active_task`, `tasc_batch_update_tasks` |
| Dependencies | `tasc_add_dependency`, `tasc_remove_dependency`, `tasc_get_dependencies` |
| Projects | `tasc_add_project`, `tasc_get_project`, `tasc_update_project`, `tasc_delete_project`, `tasc_list_projects` |

- **Workflow**: the model reads state, then acts. A bulk request ("reschedule
  everything") is a `tasc_list_tasks` to collect ids followed by one
  `tasc_batch_update_tasks`, not a call per task.
- **Consumers**: the bundled skill in `skills/tasc-task-manager`, and the `mcpServers.tasc`
  entry Home Manager writes for Claude Code and Gemini — both of which run the *pinned*
  binary, not the local build. A new tool is invisible to them until homelab's
  `pkgs/tasc/default.nix` rev moves.

This package has **no tests**. Verify a change by driving the tool.
