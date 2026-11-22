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
- **Store-backed commands** use the selected backend for everyday task and project
  operations, timers, calendar, statistics, and the terminal UI. Statistics aggregate
  a single task snapshot with completion timestamps for closure rates and burndown.
  The root pre-run hook rejects the SQLite administration commands listed in the README
  when the client is in remote mode.
- **Task history and status** use `POST /tasks/log` and `POST /tasks/{id}/vague`.
  Logging inserts completed history without creating or reopening a project.
  Marking a task vague updates its status without rewriting its other fields.
- **Project actions** use `POST /projects/create`, `POST /projects/{name}/archive`,
  and `POST /projects/{name}/delete`. CLI creation checks that a supplied parent
  exists and leaves the short name unset. Archive updates the status in one write.
  CLI deletion applies the chosen empty, unlink, or cascade behavior in a single
  transaction. Dedicated routes make older servers reject operations whose
  semantics they cannot honor. Existing REST and MCP project operations keep
  their original contracts.
- **Timer stop**: `POST /tasks/stop?id=<id>` stops only the requested active task.
  Omitting `id` stops the current timer. A malformed ID returns HTTP 400, and a
  missing target returns HTTP 404. An explicit `-1` addresses that task ID. The
  remote Store sends it to `POST /tasks/-1/stop` so older servers cannot mistake
  it for an omitted target.
- **Task errors** use HTTP 400 for validation, 404 for missing targets, and 409
  for assignment and description conflicts. The remote Store preserves the error
  categories. MCP returns tool errors and the CLI exits nonzero for failed task
  requests. Batch counts use distinct IDs and are reported only after success.
- **Task edits with dependencies** use `POST /tasks/with-links` and
  `POST /tasks/batch-update/with-links`. The task changes and links commit together.
  The response reports the number of edges added. Older servers reject these routes
  before applying any changes.
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
- **User priority**: `GET /tasks?sort=priority` orders matching tasks by user
  priority, then ID. Use `POST /tasks/batch-update` with an
  `updates.user_priority` integer from 1 to 5, or `null` to clear it. Full task
  updates preserve the stored priority. Remote clients send priority-bearing
  batches to `POST /tasks/batch-update/priority` so older servers reject the request.

## Model Context Protocol (MCP)
`internal/mcp/server.go` implements an MCP server over `mark3labs/mcp-go`.
- **Transport**: stdio, via `server.ServeStdio`. **Stdout is the protocol** — diagnostics
  go through `log` (stderr by default) and never `fmt.Print`.
- **Resource**: `tasc://tasks`, giving the model the current state in one read.
- **Tools**: 25, covering the `Store` interface.

| Area | Tools |
|---|---|
| Tasks | `tasc_add_task`, `tasc_get_task`, `tasc_update_task`, `tasc_delete_task`, `tasc_list_tasks`, `tasc_search_tasks` |
| Assignments | `tasc_list_assignments`, `tasc_claim_tasks`, `tasc_release_tasks`, `tasc_end_run` |
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
- **User priority**: `tasc_list_tasks` accepts `sort: "priority"`.
  `tasc_update_task` and `tasc_batch_update_tasks` accept
  `user_priority` or `clear_user_priority: true`.
- **Consumers**: the bundled skill in `skills/tasc-task-manager`, and the `mcpServers.tasc`
  entry Home Manager writes for Claude Code and Gemini — both of which run the *pinned*
  binary, not the local build. A new tool is invisible to them until homelab's
  `pkgs/tasc/default.nix` rev moves.

Verify tool changes through the real MCP binary in `tests/integration`.

Agent goal writes use a UUID bound to their stdio connection until the explicit
end of that goal. The client must end the run when yielding an idle session.
Disconnect stops the heartbeat and preserves recovery information. Use a
separate stdio connection per concurrent goal. Clients sharing a server process
can use Linux PID-bound CLI claims for independent goals.

REST serves `GET /assignments` and `POST /assignments`. Scoped requests use
`/actors/{run-id}/...` (or `guest`) under the same access checks as other REST
calls. Older servers reject those paths. The server's boot and clock determine
expiry for both local and remote writers.

### Assignment verification on Linux, 2026-11-22

`TestAssignmentLifecycle` drives two real stdio clients and the operator CLI
against isolated databases in local and REST-backed modes. It checks a
partially available batch, same-label runs, stale writes, recurring successors,
release on completion, owner takeover, recovery context, and handoff to Alex.
`TestConcurrentClaimsAndAwakeExpiry` races separate SQLite connections and
checks the exact expiry boundary and boot change with a controlled clock.

In a separate laptop check the CLI worker renewed after 60.1 awake seconds
and exited when its watched PID exited. An MCP connection kept open for 62
seconds after its watched PID exited renewed zero times. After setting that
fixture's deadline to the current monotonic time, a fresh connection reclaimed
both tasks. The old session returned `Error completing task: task assignment conflict`.
The TUI rendered `Review operator handoff · Alex` against a separate fixture.
