# Draw Project Roadmap
*Technomancer Standard v1.0*

## 1. Context Profile
*The Agent MUST read these files at the start of every session to understand the project state.*

### Static Context
*   `README.md`
*   `GEMINI.md`
*   `go.mod`
*   `web/package.json`

### Dynamic Context (Source Code)
*   `cmd/` (CLI Commands)
*   `internal/` (Core Logic)
*   `web/src/` (Frontend Application)

---

## 2. Execution Phases

### Phase 1: Core Task & Project Management
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** None

**Objective:**
Establish the core data model and basic task lifecycle management capabilities.

**Tasks:**
- [x] **Core Data Model:** Implement SQLite schema for Tasks (Description, Status, Priority, Dates) and Projects.
- [x] **Project Management:** Dedicated table and CLI commands (`tasc project`) for managing project metadata.
- [x] **Task Lifecycle:** CRUD operations (`add`, `edit`, `done`, `delete`) with status tracking (`backlog`, `ongoing`, `done`).
- [x] **Recurrence:** Support for recurring tasks to automate repetitive work.
- [x] **Input Handling:** Robust parsing of dates and natural language inputs.

**Verification:**
*   [x] `tasc add` successfully creates tasks in the database.
*   [x] `tasc list` retrieves and displays tasks correctly.
*   [x] Recurring tasks generate next instances upon completion.

---

### Phase 2: Visualization & Inspection
**Status:** DONE
**Token Budget:** High
**Prerequisites:** Phase 1

**Objective:**
Implement advanced visualization modes for tasks, including calendar views and dependency graphs.

**Tasks:**
- [x] **Advanced Calendar:** Implement `day`, `week`, `month`, `quarter`, and `year` views.
- [x] **Time Navigation:** Add flags (`--next`) to traverse calendar periods easily.
- [x] **Deep Inspection:** Create `tasc show` for detailed task views and dependency chains.
- [x] **Dependency Graph:** Visual tree rendering (`tasc graph`) for blocking relationships.
- [x] **Adaptive UI:** Terminal-responsive layouts and ANSI color themes.

**Verification:**
*   [x] `tasc calendar` renders the correct grid for the current month.
*   [x] `tasc graph` visually connects blocking tasks to blocked tasks.

---

### Phase 3: Advanced Logic & Prioritization
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 1

**Objective:**
Implement smart scoring algorithms to automatically prioritize tasks based on deadlines, age, and dependencies.

**Tasks:**
- [x] **Smart Scoring:** Priority algorithms based on Due Date, Schedule Date, Age, and Task Estimates.
- [x] **Dynamic Status:** Auto-calculated statuses and "Blocked" state handling.
- [x] **Anti-Procrastination:** Priority boosting for older tasks (`AgeRule`) and "Quick Wins" (`EstimateRule`).
- [x] **Reschedule Tracking:** Track and weight tasks based on how often they are rescheduled (`RescheduleRule`).

**Verification:**
*   [x] Tasks with imminent deadlines appear at the top of the list.
*   [x] Blocked tasks are visually distinct and prioritized correctly.

---

### Phase 4: UI Consistency & Polish
**Status:** DONE
**Token Budget:** Low
**Prerequisites:** Phase 2, Phase 3

**Objective:**
Ensure a unified visual language and consistent behavior across all CLI commands.

**Tasks:**
- [x] **Unified Color Model:** Centralize task coloring logic for `calendar`, `graph`, and `show`.
- [x] **Visual Consistency:** Ensure task representations are identical across all views.
- [x] **Contextual Highlighting:** Ensure high-priority or blocked tasks retain urgency in all views.

**Verification:**
*   [x] Task ID #123 has the same color in `list`, `graph`, and `calendar`.

---

### Phase 5: Project Management Deep Dive
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 1

**Objective:**
Enhance project capabilities with hierarchies, cascading properties, and bulk operations.

**Tasks:**
- [x] **Subprojects:** Implement hierarchical structures (Parent -> Child projects).
- [x] **Cascading Deadlines:** Logic where project deadlines constrain contained tasks.
- [x] **Dependency-Aware Priority:** Propagate priority from high-value blocked tasks to blockers.
- [x] **Bulk Project Ops:** Commands to archive, move, or delete entire project trees.
- [x] **Progress Tracking:** Show completion percentage in project list.

**Verification:**
*   [x] Creating a subproject correctly links it to the parent.
*   [x] Deleting a project prompts to delete or move its tasks.

---

### Phase 6: Productivity & Insights
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 1

**Objective:**
Provide analytics and insights into user productivity and task friction.

**Tasks:**
- [x] **Stale Task Analysis:** Reports identifying "rotting" tasks.
- [x] **Friction Analysis:** Identify tasks with high reschedule counts.
- [x] **Estimation Accuracy:** Track "Actual" time vs "Estimated".
- [x] **Velocity Metrics:** Advanced stats for task closure rates.

**Verification:**
*   [x] `tasc stats` returns calculation of velocity and stale tasks.

---

### Phase 7: Distributed Architecture
**Status:** DONE
**Token Budget:** High
**Prerequisites:** Phase 3

**Objective:**
Refactor the application into a client-server architecture to support remote access and multiple frontends.

**Tasks:**
- [x] **Client-Server Separation:** Refactor core logic into a headless server/daemon.
- [x] **API Layer:** Expose functionality via REST or gRPC.
- [x] **Remote CLI:** Enable `tasc` binary to connect to remote server.
- [x] **Multi-Device Sync:** Enable centralized data management.

**Verification:**
*   [x] Server process starts and handles requests.
*   [x] CLI can connect to `localhost` port to perform operations.

---

### Phase 8: Mobile Applications
**Status:** PENDING
**Token Budget:** High
**Prerequisites:** Phase 7, Phase 10

**Objective:**
Develop mobile interfaces (Android/iOS) reusing the web frontend logic.

**Tasks:**
- [ ] **Unified Architecture:** Adopt a shared React-based framework for Web and Mobile.
- [ ] **Hybrid Packaging:** Use Capacitor to package the React app as a native Android app.
- [ ] **Secure Connectivity:** Integrate with Cloudflare Tunnel (cloudflared).

**Verification:**
- [ ] App builds and installs on Android emulator.
- [ ] App connects to backend via secure tunnel.

---

### Phase 9: External Integrations
**Status:** PENDING
**Token Budget:** Medium
**Prerequisites:** Phase 1

**Objective:**
Integrate with external services, primarily Google Calendar, for bidirectional synchronization.

**Tasks:**
- [ ] **Google OAuth2:** Implement secure authentication flow.
- [ ] **Event Mapping:** Logic to translate Tasc projects/priorities into GCal events.
- [ ] **Bidirectional Sync:** Sync engine for updates, deletions, and conflicts.
- [ ] **Recurrence Translation:** Map `tasc` recurrence rules to GCal RRULEs.

**Verification:**
- [ ] Adding a task in `tasc` creates an event in Google Calendar.
- [ ] Modifying an event in Calendar updates the task in `tasc`.

---

### Phase 10: Web Dashboard
**Status:** DONE
**Token Budget:** High
**Prerequisites:** Phase 7

**Objective:**
Build a responsive web interface for planning and visualization using React.

**Tasks:**
- [x] **React Dashboard:** Build a responsive web interface.
- [x] **Visual Planning:** Implement drag-and-drop Kanban and grid views.

**Verification:**
*   [x] Web UI serves correctly on the server port.
*   [x] Kanban board updates task status on drag-and-drop.

---

### Phase 11: Distribution & Packaging
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 1

**Objective:**
Streamline installation and updates for various platforms.

**Tasks:**
- [x] **Install Script:** Shell script for automated local build/install.
- [x] **Linux Packages:** Generate `.deb` and `.rpm` packages.
- [x] **Homebrew Tap:** Create Homebrew formula for macOS.
- [x] **Release Automation:** Script to build and publish releases via GitHub.
- [x] **Systemd Service:** Unit file to manage backend server process.

**Verification:**
*   [x] `install.sh` completes without errors on a fresh environment.
*   [x] Releases appear on GitHub with correct assets.

---

### Phase 12: Technical Consolidation & Security
**Status:** DONE
**Token Budget:** High
**Prerequisites:** Phase 1

**Objective:**
Harden the codebase with tests, refactoring, and security best practices.

**Tasks:**
- [x] **Unit Testing:** Implement comprehensive unit tests.
- [x] **Integration Testing:** Develop integration tests for reliability.
- [x] **Codebase Consistency:** Audit and refactor code.
- [x] **Upgrade Strategy:** Automated schema migrations.

**Verification:**
*   [x] Test suite passes with high coverage.
*   [x] Database migrations run automatically on version upgrade.

---

### Phase 13: AI Integration (Gemini via MCP)
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 7

**Objective:**
Implement the Model Context Protocol (MCP) to allow LLMs to interact with the task database.

**Tasks:**
- [x] **MCP Server Implementation:** Implement MCP server interface.
- [x] **Tool Exposure:** Expose `add`, `list`, `done`, `edit`, `projects` as tools.
- [x] **Resource Exposure:** Expose task lists as MCP resources.
- [x] **Gemini CLI Config:** Document configuration.
- [x] **Natural Language Interaction:** Enable conversational task management.

**Verification:**
*   [x] LLM can successfully call tools to add and query tasks.

---

### Phase 14: Advanced MCP & Batch Operations
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 13

**Objective:**
Enhance MCP capabilities for complex queries and bulk updates.

**Tasks:**
- [x] **Complex Query Support:** Enable Gemini to perform batch modifications.
- [x] **Batch Update Tool:** Expose tools for efficient bulk changes.

**Verification:**
*   [x] "Reschedule all Project X tasks to Monday" works via LLM.

---

### Phase 15: Smart Scheduling & Time Management
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 3

**Objective:**
Improve scheduling precision and automation.

**Tasks:**
- [x] **Seamless Rescheduling:** Streamline workflow for moving tasks.
- [x] **Time Specificity:** Support for exact times (HH:MM).
- [x] **Smart Auto-Schedule:** Logic to automatically arrange daily tasks.
- [x] **Dynamic Reordering:** Adapting schedule when priorities shift.
- [x] **Default Estimate:** Set default time estimate for new tasks.

**Verification:**
*   [x] Tasks with specific times are sorted correctly in the daily view.

---

### Phase 16: UI Refinements & Configuration
**Status:** DONE
**Token Budget:** Low
**Prerequisites:** Phase 4

**Objective:**
Polish the user interface with better formatting options.

**Tasks:**
- [x] **Relative Time Display:** Configurable relative time formatting for dates.

**Verification:**
*   [x] Dates show as "2d" or "5h" when configured.

---

### Phase 17: Data Model Evolution
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 1

**Objective:**
Refine the data schema to support richer content.

**Tasks:**
- [x] **Schema Refactor:** Rename `description` to `title`.
- [x] **Rich Descriptions:** Add multi-line `description` field.
- [x] **View Updates:** Display title/description appropriately in views.
- [x] **Editor Integration:** Open system editor for multi-line edits.

**Verification:**
*   [x] `tasc edit` opens `vim`/`nano` with the task description.

---

### Phase 18: Time Blocking & Scheduling Improvements
**Status:** DONE
**Token Budget:** Medium
**Prerequisites:** Phase 15

**Objective:**
Implement time-blocking methodology for task scheduling.

**Tasks:**
- [x] **Configurable Time Blocks:** Support user-defined daily blocks (morning, afternoon).
- [x] **Default Configuration:** Set defaults (06:00-12:00, 13:00-18:00).
- [x] **Block-Based Scheduling:** Default new tasks to specific blocks.
- [x] **CLI Integration:** Add `--block` option to commands.
- [x] **Autocomplete:** Shell completion for blocks.
- [x] **Calendar Grid View:** Grid view with hour labels.

**Verification:**
*   [x] `tasc add "Task" --block morning` schedules the task in the morning slot.

---

### Phase 19: Agent Skill Packaging
**Status:** DONE
**Token Budget:** Low
**Prerequisites:** Phase 13

**Objective:**
Bundle a reusable agent skill for Tasc MCP workflows and provide a supported installation path from the `tasc` CLI.

**Tasks:**
- [x] **Bundled Skill:** Add a reusable `SKILL.md`-based task management skill with Tasc MCP workflow guidance.
- [x] **Installer Command:** Add a `tasc skill install` command to install the bundled skill into a skills directory.
- [x] **Documentation:** Document the bundled skill, MCP expectations, and install flow in the README.

**Verification:**
*   [x] `make test` passes with installer coverage.
*   [x] `go run -tags fts5 . skill install --dest /tmp/<dir>` writes the bundled skill files successfully.
