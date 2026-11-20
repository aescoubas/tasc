# Server & MCP Architecture

## Client-Server Model
Tasc can run as a standalone local CLI or as a distributed system.
- **Server**: `cmd/serve.go` starts an HTTP REST API (`internal/server`). On this laptop
  a `systemd --user` unit runs it on port 8081, which means **a process already holds
  `~/.tasc.db` open** — hence `PRAGMA busy_timeout = 10000` in `internal/db/db.go`.
- **Client**: `cmd/root.go` detects `TASC_REMOTE`. If set, `CurrentStore` becomes
  `RemoteClient` instead of `SQLiteStore`.
- **REST boundary**: `serve` defaults to IPv4 loopback. Without a credential,
  the handler also requires a loopback peer and localhost/loopback Host, preventing
  access through DNS rebinding. `--api-token-file` enables one owner bearer token
  across every API route; non-loopback serving requires that token and a TLS
  certificate/key pair. Cross-origin browser requests require exact explicit
  `--allow-origin` entries; valid preflights disclose no API data and do not
  bypass authentication on actual requests. Request gates run before routing,
  redirects or store access. API responses use `no-store`.
- **Remote credentials**: `TASC_API_TOKEN_FILE` supplies the runtime-only token
  through a mode0600/0400 regular file. Authenticated clients require HTTPS or
  loopback HTTP and do not follow redirects. Existing local CLI and MCP authority
  is unchanged; this is a network boundary, not per-task authorization.
- **Store-backed commands**: task commands, timer start/stop, calendar, statistics,
  project listing, and project editing use `Store` in both modes. Statistics aggregate
  a single task snapshot; listing includes completion timestamps for closure rates and
  burndown. Remaining commands that access SQLite directly reject remote mode in the
  root pre-run hook; the README lists them and the `tasc` project tracks the remaining work.
- **Timer stop**: `POST /tasks/stop?id=<id>` stops only the requested active task.
  Omitting `id` stops the current timer. Invalid IDs return HTTP 400. The remote Store
  client preserves the ID instead of silently stopping whichever task is active.
- **Task filters**: `GET /tasks` accepts comma-separated `status` values and
  `include_deleted=true`, alongside project, ID and due-date filters. Status matches
  the displayed dependency state (`blocked` for unfinished tasks with open blockers).
  With no explicit status, the default excludes `done`, `deleted` and `undefined`;
  `include_deleted=true` includes all three. An explicit status still restricts the
  results when that flag is set. Invalid boolean values return HTTP 400.
- Set `exclude_archived_projects=true` on `GET /tasks` or pass
  `exclude_archived_projects: true` to `tasc_list_tasks` to omit tasks assigned
  directly to archived projects. The filter keeps unassigned tasks, tasks whose
  project record is missing, and tasks in active subprojects of archived parents.
  It combines with other filters. To review an archived project's history,
  specify the project and omit the exclusion.

## Model Context Protocol (MCP)
`internal/mcp/server.go` implements an MCP server over `mark3labs/mcp-go`.
- **Transport**: stdio, via `server.ServeStdio`. **Stdout is the protocol** — diagnostics
  go through `log` (stderr by default) and never `fmt.Print`.
- **Resource**: `tasc://tasks`, giving the model the current state in one read.
- **Tools**: 21, covering the `Store` interface.

| Area | Tools |
|---|---|
| Tasks | `tasc_add_task`, `tasc_get_task`, `tasc_update_task`, `tasc_delete_task`, `tasc_list_tasks`, `tasc_search_tasks` |
| Handoffs | `tasc_get_task_handoff`, `tasc_update_task_handoff` |
| Task actions | `tasc_complete_task`, `tasc_start_task`, `tasc_stop_task`, `tasc_get_active_task`, `tasc_batch_update_tasks` |
| Dependencies | `tasc_add_dependency`, `tasc_remove_dependency`, `tasc_get_dependencies` |
| Projects | `tasc_add_project`, `tasc_get_project`, `tasc_update_project`, `tasc_delete_project`, `tasc_list_projects` |

- **Task handoffs**: agents lead a description with the next actor and action.
  The handoff tools use the first nonblank paragraph of the existing description.
  The read returns `id`, `handoff`, `truncated` and `revision` as structured content
  and JSON text. The paragraph preview is capped at 512 UTF-8 bytes.
  A legacy description may open with evidence or context. Use `prepend` to
  preserve that entire body. Use
  `replace` when the complete opening paragraph is already the handoff. Replacement
  preserves the rest of the description, including its original separators.
  The revision covers the entire description. An intervening description edit or
  deletion rejects the write. Read again and reconsider the edit before retrying.
  The remote Store uses `PUT /tasks/{id}/description` through the existing REST
  access controls. SQLite checks the expected body and writes only the description
  in one statement. The full-task tools keep their original contracts, including
  the ability to replace the entire description.
  The local and remote MCP integration fixture returned 1,326 bytes for the
  bounded legacy read versus 7,546 for the full tool result. Two concurrent
  edits from the same snapshot yielded one success and one conflict in each
  mode, with the original evidence preserved byte for byte.
- **Workflow**: the model reads state, then acts. A bulk request ("reschedule
  everything") is a `tasc_list_tasks` to collect ids followed by one
  `tasc_batch_update_tasks`, not a call per task.
- **Consumers**: the bundled skill in `skills/tasc-task-manager`, and the `mcpServers.tasc`
  entry Home Manager writes for Claude Code and Gemini — both of which run the *pinned*
  binary, not the local build. A new tool is invisible to them until homelab's
  `pkgs/tasc/default.nix` rev moves.

Verify tool changes through the real MCP binary in `tests/integration`.
