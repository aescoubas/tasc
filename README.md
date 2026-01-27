# Tasc

**Tasc** is a modern, snappy, and powerful CLI task manager built for the terminal. It combines the speed of the command line with the interactivity of a Text User Interface (TUI), all backed by a robust SQLite database.

## Features

*   **Fast & Efficient:** Instant startup and operations using SQLite.
*   **Fuzzy Search:** Powerful FTS5-based fuzzy search to find tasks instantly.
*   **Interactive TUI:** A beautiful, keyboard-driven UI powered by Bubble Tea for managing tasks.
*   **Dependency Management:** Create and visualize task dependencies (graphs).
*   **AI Integration:** Built-in integration with Google Gemini for natural language queries and advice.
*   **Smart Scheduling:** Track Due Dates, Scheduled Dates, and Estimates.
*   **Priority System:** Dynamic priority scoring to highlight what matters most.
*   **Calendar View:** Visualize your schedule in the terminal.

## Installation

### Prerequisites

*   Go 1.24 or higher.

### Build from Source

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
    tasc add "Buy milk" --due "tomorrow" --priority 5
    ```

*   **List tasks:**
    ```bash
    tasc list
    tasc list --sort due      # Sort by due date
    tasc list --sort score -d # Sort by priority score descending
    ```

*   **Mark as done:**
    ```bash
    tasc done <task_id>
    ```

*   **Delete a task:**
    ```bash
    tasc delete <task_id>
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

*   **Dependencies:**
    Link tasks together:
    ```bash
    tasc dep link <parent_id> <child_id>
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

*   **Calendar:**
    View upcoming tasks on a calendar:
    ```bash
    tasc calendar
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
