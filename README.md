# Tasc

**Tasc** is a modern, snappy, and powerful CLI task manager built for the terminal. It combines the speed of the command line with the interactivity of a Text User Interface (TUI), all backed by a robust SQLite database.

## Features

*   **Fast & Efficient:** Instant startup and operations using SQLite.
*   **Fuzzy Search:** Powerful FTS5-based fuzzy search to find tasks instantly.
*   **Interactive TUI:** A beautiful, keyboard-driven UI powered by Bubble Tea for managing tasks.
*   **Dependency Management:** Create and visualize task dependencies (graphs).
*   **AI Integration:** Built-in integration with Google Gemini for natural language queries and advice.
*   **Smart Scheduling:** Track Due Dates and Estimates.
*   **Priority System:** Dynamic priority scoring to highlight what matters most.
*   **Time Tracking:** Built-in timer to track how long tasks take.
*   **Calendar View:** Visualize your schedule in the terminal.

## Installation

### Homebrew (macOS)

```bash
brew install aescoubas/tasc/tasc
```

### Linux Packages (Debian/Ubuntu/Fedora/RHEL)

Download the latest `.deb` or `.rpm` release from the [Releases Page](https://github.com/aescoubas/tasc/releases).

**Debian/Ubuntu:**
```bash
sudo dpkg -i tasc_*.deb
```

**Fedora/RHEL:**
```bash
sudo rpm -i tasc_*.rpm
```

### Quick Install (Script)

You can also install Tasc quickly using the provided install script:

```bash
git clone https://github.com/aescoubas/tasc.git
cd tasc
./install.sh
```

### Build from Source

**Prerequisites:** Go 1.24 or higher.

**Important:** Tasc requires the SQLite `fts5` extension. You *must* include the `-tags fts5` flag when building.

```bash
git clone https://github.com/aescoubas/tasc.git
cd tasc
go build -tags fts5 -o tasc
```

Move the binary to your path:
```bash
sudo mv tasc /usr/local/bin/
```

## Usage

### Basic Commands

*   **Add a task:**
    ```bash
    tasc add "Finish the report"
    tasc add "Buy milk" --due "tomorrow"
    ```

*   **Log a completed task:**
    ```bash
    tasc log "Fixed that bug"
    tasc log "Meeting with team" --project "Work"
    ```

*   **List tasks:**
    ```bash
    tasc list
    tasc list --sort due      # Sort by due date
    tasc list --sort age      # Sort by task age
    tasc list --sort duration # Sort by time spent
    tasc list --sort score -d # Sort by priority score descending
    ```

*   **Mark as done:**
    ```bash
    tasc done <task_id>
    ```

*   **Edit a task:**
    ```bash
    tasc edit <task_id>        # Opens task in your default $EDITOR
    tasc modify <task_id> "New description" --due "next friday"
    ```

*   **Delete a task:**
    ```bash
    tasc delete <task_id>
    ```

*   **Undo last action:**
    ```bash
    tasc undo
    tasc undo --yes
    ```

*   **Renumber tasks:**
    Renumber open task IDs from `0` while preserving current order.
    Done/deleted tasks are moved to negative IDs so they are clearly distinct:
    ```bash
    tasc renumber
    tasc renumber --yes
    ```

### Advanced Features

*   **Interactive UI:**
    Launch the full TUI:
    ```bash
    tasc ui
    ```

*   **Search:**
    Fuzzy search your tasks:
    ```bash
    tasc search "milk"
    ```

*   **Inspect Task:**
    View detailed information about a task:
    ```bash
    tasc show <task_id>
    ```

*   **Dependencies:**
    Link tasks together:
    ```bash
    tasc dep <parent_id> <child_id>
    ```
    View dependency graph:
    ```bash
    tasc graph
    ```

*   **AI Assistant:**
    Ask Gemini for help with your tasks:
    ```bash
    tasc ask "How should I prioritize my day?"
    ```

*   **AI Integration (MCP):**
    Run an MCP server to let AI agents (like Gemini) manage your tasks:
    ```bash
    tasc mcp
    ```
    **Configuration:**
    To use with Gemini CLI, add this to your config:
    ```yaml
    mcpServers:
      tasc:
        command: "tasc"
        args: ["mcp"]
    ```

*   **Calendar:**
    View upcoming tasks on a calendar:
    ```bash
    tasc calendar
    ```

*   **Time Tracking:**
    Track time spent on tasks:
    ```bash
    tasc start <task_id>       # Start the timer for a task
    tasc stop                  # Stop the currently active timer
    ```

*   **Statistics:**
    View task counts by status:
    ```bash
    tasc stats
    ```

*   **Task Refinement:**
    Mark vague tasks that need more clarity:
    ```bash
    tasc vague <task_id>
    ```

*   **Project Management:**
    Manage project metadata (descriptions, goals):
    ```bash
    tasc project list
    tasc project add "Work" --desc "Professional tasks"
    tasc project show "Work"
    ```

*   **Data Management:**
    Import from TaskWarrior:
    ```bash
    task export | tasc import-tw
    ```
    Backup and Restore your database:
    ```bash
    tasc backup > tasc_backup.json
    tasc restore < tasc_backup.json
    ```
    Seed with dummy data (for testing):
    ```bash
    tasc seed
    ```

### Web Dashboard

Tasc comes with a modern React-based web dashboard for visual planning.

1.  **Start the API Server:**
    ```bash
    tasc serve
    ```

2.  **Start the Web UI:**
    ```bash
    cd web
    npm install
    npm run dev
    ```
    Open `http://localhost:5173` to view your Kanban board and task lists.

### Distributed Mode (Client-Server)

Tasc can run in a distributed mode where a central server manages the database, and clients connect to it remotely.

**1. Start the Server:**

You can use the provided start script to build and launch the server:

```bash
./start-server.sh
```

Or manually:
```bash
tasc serve --port 8081
```

**2. Configure the Client:**

Set the `TASC_REMOTE` environment variable to point to your server:

```bash
export TASC_REMOTE=http://localhost:8081
```

Now, all `tasc` commands (add, list, done, etc.) will operate on the remote server instead of the local database.

You can also specify the remote URL per command using the `--remote` flag:
```bash
tasc list --remote http://localhost:8081
```

## Contributing

Contributions are welcome!

1.  Fork the repository.
2.  Create a feature branch (`git checkout -b feature/amazing-feature`).
3.  Commit your changes (`git commit -m 'Add amazing feature'`).
4.  Push to the branch (`git push origin feature/amazing-feature`).
5.  Open a Pull Request.

## License

This project is licensed under the **Mozilla Public License 2.0 (MPL-2.0)**.

*   **Commercial Use:** You may use this software in commercial applications (including proprietary ones).
*   **Modifications:** If you modify the source code of this project, you must make those modifications available under the MPL-2.0 license.

See the [LICENSE](LICENSE) file for details.
