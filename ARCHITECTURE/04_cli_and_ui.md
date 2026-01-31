# CLI & UI Architecture

## CLI (Cobra)
The Command Line Interface is built with `spf13/cobra`.
- **Structure**: `cmd/root.go` initializes the global state (Config, DB/Remote connection).
- **Execution**: Subcommands (`cmd/*.go`) perform specific actions.
- **Output**: Most commands output formatted text or tables (`text/tabwriter`) directly to stdout.

## TUI (Bubbletea)
Interactive mode (`tasc ui`) is built with the ELM architecture using `charmbracelet/bubbletea`.
- **Model**: `internal/ui/model.go` holds state (list of tasks).
- **Update**: Handles keyboard events (navigation, marking done).
- **View**: Renders the UI using `lipgloss` for styling.

## Visualization
- **Calendar**: `cmd/calendar.go` implements custom rendering logic for Day, Week, Month, Quarter, and Year grids.
- **Graph**: `cmd/graph.go` renders the dependency tree visually using ASCII connectors.
- **Styling**: `internal/ui/style.go` centrally defines colors and visual cues (Urgency Tiers) based on user configuration (`.tasc.yaml`).
