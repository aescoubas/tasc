# Roadmap

## Phase 1: Core Task & Project Management
- [x] **Core Data Model:** Implement SQLite schema for Tasks (Description, Status, Priority, Dates) and Projects.
- [x] **Project Management:** Dedicated table and CLI commands (`tasc project`) for managing project metadata.
- [x] **Task Lifecycle:** CRUD operations (`add`, `edit`, `done`, `delete`) with status tracking (`backlog`, `ongoing`, `done`).
- [x] **Recurrence:** Support for recurring tasks to automate repetitive work.
- [x] **Input Handling:** Robust parsing of dates and natural language inputs.

## Phase 2: Visualization & Inspection
- [x] **Advanced Calendar:** Implement `day`, `week` (columns), `month` (grid), `quarter` (grid), and `year` (grid) views.
- [x] **Time Navigation:** Add flags (`--next`) to traverse calendar periods easily.
- [x] **Deep Inspection:** Create `tasc show` for detailed task views and dependency chains.
- [x] **Dependency Graph:** Visual tree rendering (`tasc graph`) for blocking relationships.
- [x] **Adaptive UI:** Terminal-responsive layouts (truncation, wrapping) and ANSI color themes.

## Phase 3: Advanced Logic & Prioritization
- [x] **Smart Scoring:** Priority algorithms based on Due Date, Schedule Date, Age, and Task Estimates.
- [x] **Dynamic Status:** Auto-calculated statuses and "Blocked" state handling.
- [x] **Anti-Procrastination:** Priority boosting for older tasks (`AgeRule`) and "Quick Wins" (`EstimateRule`).
- [x] **Reschedule Tracking:** Track and weight tasks based on how often they are rescheduled (`RescheduleRule`).

## Phase 4: UI Consistency & Polish
- [x] **Unified Color Model:** Centralize task coloring logic so `calendar`, `graph`, and `show` match the "official" colorscheme used in `tasc list`.
- [x] **Visual Consistency:** Ensure task representations (IDs, Project names, Status indicators) are identical across all views.
- [x] **Contextual Highlighting:** Ensure high-priority or blocked tasks retain their visual urgency in Calendar and Graph views.

## Phase 5: Project Management Deep Dive
- [x] **Subprojects:** Implement hierarchical structures (Parent -> Child projects) for better organization.
- [x] **Cascading Deadlines:** Logic where project deadlines automatically constrain or set defaults for contained tasks.
- [x] **Dependency-Aware Priority:** Update scoring to propagate priority from high-value blocked tasks to their blockers.
- [x] **Bulk Project Ops:** Commands to archive, move, or delete entire project trees safely.
- [x] **Progress Tracking:** Show completion percentage in project list.

## Phase 6: Productivity & Insights
- [x] **Stale Task Analysis:** Reports specifically identifying tasks that are "rotting" (old and untouched).
- [x] **Friction Analysis:** Identify tasks with high reschedule counts to highlight roadblocks.
- [x] **Estimation Accuracy:** Track "Actual" time vs "Estimated" to improve future planning.
- [x] **Velocity Metrics:** Advanced stats for task closure rates and "Burn-down" charts.

## Phase 7: Distributed Architecture
- [x] **Client-Server Separation:** Refactor core logic into a headless server/daemon.
- [x] **API Layer:** Expose functionality via REST or gRPC.
- [x] **Remote CLI:** Enable the `tasc` binary to connect to a remote server instance.
- [x] **Multi-Device Sync:** Enable centralized data management for access across machines.

## Phase 8: Mobile Applications
- [ ] **Unified Architecture:** Adopt a shared React-based framework to maximize code reuse between Web and Mobile interfaces.
- [ ] **Hybrid Packaging:** Implement a build pipeline using Capacitor to package the React web application as a native Android app.
- [ ] **Secure Connectivity:** Integrate with Cloudflare Tunnel (cloudflared) to securely expose the homelab server without open ports.

## Phase 9: External Integrations
- [ ] **Google OAuth2:** Implement secure authentication flow to connect `tasc` with Google Calendar API.
- [ ] **Event Mapping:** Define logic to translate Tasc projects/priorities into GCal colors and task durations into event blocks.
- [ ] **Bidirectional Sync:** Develop a sync engine to handle updates, deletions, and conflicts between Tasc and Google Calendar.
- [ ] **Recurrence Translation:** accurately map `tasc` recurrence rules to GCal RRULEs.

## Phase 10: Web Dashboard
- [x] **React Dashboard:** Build a responsive web interface for high-level planning and visualization.
- [x] **Visual Planning:** Implement drag-and-drop Kanban and grid views sharing components with the mobile build.

## Phase 11: Distribution & Packaging
- [x] **Install Script:** Simple shell script (`install.sh`) for automated local build and install.
- [x] **Linux Packages:** Generate `.deb` and `.rpm` packages for easy installation on Debian/Ubuntu and Fedora/RHEL.
- [x] **Homebrew Tap:** Create a Homebrew formula for easy installation on macOS (`brew install aescoubas/tasc/tasc`).
- [x] **Release Automation:** Local shell script (`release.sh`) to build and publish releases via GitHub CLI.
- [x] **Systemd Service:** Create a systemd unit file to manage the backend server process and update `install.sh` to configure it.

## Phase 12: Technical Consolidation & Security
- [x] **Unit Testing:** Implement comprehensive unit tests aiming for full code coverage.
- [x] **Integration Testing:** Develop integration tests to verify system reliability and full coverage.
- [x] **Codebase Consistency:** Audit and refactor code for consistency, style, and maintainability.
- [x] **Upgrade Strategy:** Implement robust processes for upgrades, including automated schema migrations.

## Phase 13: AI Integration (Gemini via MCP)

- [x] **MCP Server Implementation:** Implement the Model Context Protocol (MCP) server interface within `tasc` (e.g., `tasc mcp`).
- [x] **Tool Exposure:** Expose core functionalities (`add`, `list`, `done`, `edit`, `projects`) as MCP tools for the LLM.
- [x] **Resource Exposure:** Expose task lists and details as MCP resources for providing context to the LLM.
- [x] **Gemini CLI Config:** Document how to configure Gemini CLI to connect to `tasc`'s MCP server.
- [x] **Natural Language Interaction:** Verify that Gemini can "Add a task to buy milk", "List my high priority tasks", etc.

## Phase 14: Advanced MCP & Batch Operations
- [x] **Complex Query Support:** Enable Gemini to perform batch modifications (e.g., "reschedule all my laserbiolabs tasks to next week").
- [x] **Batch Update Tool:** Expose a dedicated MCP tool or enhance `update` to handle multiple IDs or filter-based updates to support efficient bulk changes.

## Phase 15: Smart Scheduling & Time Management
- [x] **Seamless Rescheduling:** Streamline the workflow for moving tasks to new dates or times.
- [x] **Time Specificity:** Add support for specifying exact times (HH:MM) for task scheduling and deadlines.
- [x] **Smart Auto-Schedule:** Implement logic to automatically arrange daily tasks starting at 07:00, 20m duration each, strictly following priority order.
- [x] **Dynamic Reordering:** Ensure the daily schedule automatically adapts and reorders when task priorities shift.
- [x] **Default Estimate:** Set the default time estimate to 20 minutes for new tasks.

## Phase 16: UI Refinements & Configuration
- [x] **Relative Time Display:** Implement configurable relative time formatting (e.g., "3d", "10m", "-5h") for "Due" and "Scheduled" columns.

## Phase 17: Data Model Evolution
- [x] **Schema Refactor:** Rename the existing `description` field to `title`.
- [x] **Rich Descriptions:** Add a new optional, multi-line `description` field for detailed notes.
- [x] **View Updates:** Display `title` in `tasc list` and the full `description` only in `tasc show`.
- [x] **Editor Integration:** Ensure `tasc edit` opens the multi-line description in the system's preferred text editor for easy modification.
