# tasc

A terminal task manager: a Cobra CLI and a Bubble Tea TUI over SQLite, with a REST
server, an MCP surface, and a frozen React dashboard in `web/`. Active work addresses
the core implementation gaps tracked in Phase 20.

`~/.claude/CLAUDE.md` holds the standing rules (gates, evidence, git, shell hygiene) and
is not repeated here. `CLAUDE.md` and `GEMINI.md` are symlinks to this file.
`ARCHITECTURE/` is five short notes, worth reading whole before a change in the area
each one covers.

## The gate

```sh
nix develop --command make check    # fmt-check, vet, test — the whole gate
nix develop --command make test     # the suite alone, while iterating
```

**The dev shell is not optional.** `mattn/go-sqlite3` is cgo and this laptop has no
`gcc` on `PATH`; outside the shell every SQLite package drops out of the build. That is
why `check` depends on `require-cc`, which fails with the fix rather than letting a
compile-nothing run report green. The tree is `gofmt` clean and `go vet -tags fts5`
clean as of 2026-11-05.

| Change | Gate |
|---|---|
| Go | `make check` from the dev shell |
| Anything touching search, migrations, or the store | `make check` — the integration suite is the only place the real binary is exercised end to end |
| Concurrency, the server, the timer | `make test-race` |
| Nix | `nix fmt`, then `nix flake check` — builds the CLI *and* the web dashboard |
| `go.mod` | a new `vendorHash` in **both** `nix/package.nix` and homelab's `pkgs/tasc/default.nix` |
| A new or renamed MCP tool | `skills/tasc-task-manager` describes the tools; update it in the same commit |

## Hard rules

1. **`~/.tasc.db` is live data, and any command migrates it in place.** `cmd/root.go`
   calls `db.InitDB()`, which runs `RunMigrations` unconditionally — so merely listing
   tasks with a dev build carrying a newer schema upgrades the owner's real database,
   under a `tasc serve` user unit that still has the old binary mapped. Never run a local
   build against the default path:
   ```sh
   TASC_DB_PATH=$(mktemp -d)/tasc.db HOME=$(mktemp -d) ./tasc <args>
   ```
   That file is ~7 MB and written daily. A `tasc-backup.timer` user unit now copies it
   to `~/.config/tasc/backups/` nightly, so there is usually a copy from last midnight —
   check its mtime rather than assuming, and never assume it covers today's edits.

2. **`-tags fts5` or the search tests are theatre.** `rebuildTaskSearchIndex` probes for
   the module (`sqliteHasFTS5`), and when it is missing it *drops* `tasks_fts`, logs
   "using LIKE-based search fallback", and carries on. So `go test ./...` without the tag
   passes `internal/db` and `internal/store/sqlite` green while never touching the search
   path that ships — verified 2026-11-05. The Makefile puts the tag on every target for
   this reason; don't invoke `go test` by hand. The same mechanism means **running a
   non-fts5 binary against a real database degrades it**: the index is dropped on the way
   through.

3. **The `tasc` on `PATH` is not your build.** It is
   `/etc/profiles/per-user/alex/bin/tasc`, built by Home Manager from homelab's
   `pkgs/tasc/default.nix`, which pins a GitHub `rev`. Check the distance with
   `git rev-list --count <rev>..HEAD` before believing anything about the running
   version — it was 4 on 2026-11-05. `make build` produces `./tasc` and changes nothing
   the shell or the MCP client sees. Shipping is: commit here → push → bump `rev` and
   `hash` in `homelab/pkgs/tasc/default.nix` → rebuild the **system**. Home Manager runs
   as a NixOS module here (`nixos/hosts/thinkpad_perso/configuration.nix` imports
   `home/alex-laptop.nix`), there is no `home-manager` CLI on `PATH`, and the deploy is
   ```sh
   sudo nixos-rebuild switch --flake "git+file:///home/alex/git_repos/github/aescoubas/homelab?rev=<sha>#thinkpad_perso"
   ```
   Name the committed `rev` rather than using `path:`, which copies the working tree and
   would deploy whatever else is uncommitted in homelab. A local change is invisible to
   the agent skill and to the `serve` unit until that pin moves.

4. **Migrations are append-only.** Six of them in `internal/db/migrate.go`; never edit a
   landed one, since every existing database has already run it. Migration 6 installs the
   audit triggers that `tasc undo` reads — a schema change that adds or renames a column
   on `tasks` or `projects` must extend those trigger bodies too, or undo silently
   restores a row with a missing field.

5. **`~/.tasc.yaml` is a read-only symlink into `/nix/store`.** Home Manager owns it,
   from `homelab/home/alex-laptop.nix`. A config change belongs in that repo; nothing
   here may expect to write it. Test config handling with `HOME` pointed at a temp dir,
   the way `tests/integration` does.

6. **`skills/tasc-task-manager/` is the source of truth for the installed skill.**
   The directory is `go:embed`ed (`skills/bundle.go`), so the *binary* carries it and
   `tasc skill install` unpacks from there, not from this checkout — a doc edit reaches
   the installed skill only once the homelab pin moves. Mind the destination:
   `--client` defaults to **codex** (`~/.agents/skills`), so the Claude copy needs
   `tasc skill install --client claude --force`. Editing the installed copy loses the
   change at the next install; edit it here.

7. **In `tasc mcp`, stdout is the protocol.** `internal/mcp` runs `server.ServeStdio`.
   Diagnostics go through `log`, which is stderr by default — keep it that way, and never
   `fmt.Print` in that path. A corrupted JSON-RPC stream surfaces as a client error that
   does not mention tasc.

## Where things live

| Path | What | Notes |
|---|---|---|
| `cmd/` | one file per CLI subcommand | `root.go` wires config, store, and undo tracking |
| `internal/store/store.go` | the `Store` interface | **the seam** — `sqlite` and `remote` both implement it, so a new operation goes here first or the remote backend silently lacks it |
| `internal/db/` | connection, the six migrations, the FTS5 probe | rules 1, 2 and 4 live here |
| `internal/priority/` | the urgency rules and their weights | calibrated against the owner's real task set; a weight change reorders every list — propose it |
| `internal/recurrence/`, `parse/` | natural-language rules and dates | the richest unit tests in the repo; extend them rather than adding a new parser |
| `internal/ui/` | Bubble Tea model, styles, tables | completion regressions; also verify by running the TUI against a temp DB |
| `internal/mcp/server.go` | all 19 MCP tools, 816 lines | no tests, and it is what the agent skill drives |
| `internal/server/` | the REST API behind `tasc serve` | `TASC_REMOTE` makes the CLI a client of it |
| `tests/integration/` | builds the real binary and drives it | already isolates `TASC_DB_PATH` and `HOME`; copy that pattern |
| `web/` | frozen React dashboard, built by `nix/web.nix` | off limits; excluded from the Go package `src` |

**No package-local tests:** `internal/mcp`, `internal/store/remote`. Integration tests
drive both through the real binary and HTTP server. The first is the agent-facing
surface, so a change there is verified by driving the tool, not by reasoning about it.

## The roadmap

`ROADMAP.md` uses Technomancer Standard v1.0 and retains historical phase numbers.
Mobile applications and external integrations were removed from scope on 2026-11-10.
Phase 10 (Web Dashboard) is frozen and off limits: do not change or iterate on the
dashboard unless the owner explicitly reopens it. Phase 20 tracks the active core
implementation gaps. Finishing work means ticking the phase checklist and its
Verification block, and a verification line is ticked against a command that was run.

## Deliberate gaps

Not oversights — don't quietly close them as a side effect:

- **No `golangci-lint`.** `go vet` is the linter. Adding the full suite to a
  feature-complete codebase would make the gate red on arrival; that is its own piece of
  work, not a prerequisite for an unrelated change.
- **`ARCHITECTURE/01`, `03` and `04` are thin and predate several phases** — orientation,
  not truth. `02_data_model.md` and `05_mcp_and_server.md` were corrected on 2026-11-05:
  the first still described the pre-migration-2 schema, where `description` was the task
  title and the undo tables did not exist; the second listed 6 MCP tools out of 18.

## Stop and ask

A seventh migration or any change to what is stored. A change to the priority weights.
Anything that writes to `~/.tasc.db`, `~/.tasc.yaml`, or `~/.claude/skills/`. Bumping the
homelab pin — that is a deploy to the machine the owner is working on.
