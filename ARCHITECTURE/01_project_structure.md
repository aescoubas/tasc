# Project Structure Architecture

Tasc follows the standard Go project layout to ensure maintainability and separation of concerns.

## Directory Layout

### `/cmd`
Contains the main entry points for the application. Each file corresponds to a CLI subcommand (e.g., `cmd/add.go`, `cmd/list.go`).
- **`root.go`**: Defines the root command, global flags, and initialization logic (config loading, DB connection).
- **Subcommands**: Individual files define the logic for specific CLI operations, delegating business logic to the `internal` packages.

### `/internal`
Contains private application code. This code is not importable by other modules, ensuring encapsulation.

- **`config`**: Handles configuration loading (YAML) and default values (colors, paths).
- **`db`**: Database initialization, schema migration, and connection management.
- **`models`**: Core domain entities (`Task`, `Project`) and their JSON/YAML struct tags.
- **`store`**: Defines the `Store` interface for data persistence and provides implementations (`sqlite`, `remote`).
- **`ui`**: TUI components using Bubbletea and Lipgloss for rendering lists, tables, and styling.
- **`parse`**: Utilities for parsing dates and natural language inputs.
- **`priority`**: Algorithms for calculating dynamic task priority scores.
- **`recurrence`**: Logic for handling recurring tasks.
- **`server`**: The REST API server implementation.
- **`mcp`**: The Model Context Protocol (MCP) server implementation for AI integration.

### `/dist` & `/tests`
- **`dist`**: Build artifacts.
- **`tests`**: Integration tests and test fixtures.
