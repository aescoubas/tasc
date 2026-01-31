# Roadmap

This document outlines the planned future features and improvements for `tasc`.

## Active / Planned Features

### Project Management Enhancements
- [ ] **Cascading Deadlines:** Allow setting top-level project deadlines that automatically update or constrain the due dates of contained tasks.
- [ ] **Dependency-Aware Priority:** Update the priority scoring to factor in the priority of blocked tasks (e.g., blocking a high-priority task increases the blocker's score).

### Stale Task Identification
- [ ] **Age Reporting:** Specifically identify/list tasks that were defined a long time ago and remain incomplete (beyond just priority scoring).
- [ ] **Rescheduling Analysis:** Report on tasks that are constantly being rescheduled to help identify roadblocks.

### Enhanced Productivity Metrics
- [ ] **Advanced Stats:** Expand the `stats` command to provide deeper insights into productivity.
- [ ] **Closure Rates:** Track how many tasks are closed over specific periods.
- [ ] **Estimation Accuracy:** Analyze the mismatch between initial time estimates and the actual time taken to complete tasks.
- [ ] **Flexible Filtering:** Allow filtering of statistics by various attributes.
- [ ] **Time Allocation:** Provide a breakdown of time spent or allocated to each project.

### Remote Server Mode
- [ ] **Client-Server Architecture:** Implement a mode where the executable acts as a client connecting to a central `tasc` server.

### Mobile Integration
- [ ] **Lightweight Android App:** Develop a mobile application interacting with the "Remote Server Mode".

## Completed Features

### Q1 2026

#### Calendar & Visualization
- [x] **Advanced Calendar Views:** Implemented `day`, `week` (columns), `month` (grid), `quarter` (grid), and `year` (grid) views.
- [x] **Calendar Navigation:** Added `--next` / `-n` flag to navigate forward in time.
- [x] **Dependency Visualization:** Implemented `tasc graph` to visualize task dependency trees.
- [x] **Task Inspection:** Implemented `tasc show <id>` to view detailed task information and dependency chains.

#### Project Management
- [x] **Projects Framework:** Implemented `projects` database table and `tasc project` subcommand (list, create, edit, delete).
- [x] **Task Grouping:** Tasks are linked to projects via foreign keys.

#### Core Logic & Priority
- [x] **Advanced Priority Scoring:** Implemented scoring rules based on:
  - **Due Date:** Higher score as deadline approaches/passes.
  - **Schedule Date:** Boosts tasks scheduled for today/past; lowers future tasks.
  - **Estimates:** "Quick wins" (< 30m) get a boost.
  - **Age:** Older tasks get a slight boost (`AgeRule`).
  - **Reschedules:** Tasks moved frequently get a priority boost (`RescheduleRule`).
- [x] **Status Field Refinement:** Standardized statuses (`pending`, `ongoing`, `done`, `blocked`, `deleted`).
- [x] **Recurrence:** Basic support for recurring tasks.

#### User Interface
- [x] **Enhanced Colorscheme:** Implemented configurable ANSI color themes (including 233 grayscale backgrounds).
- [x] **Responsive Layouts:** `tasc list` and calendar views adapt to terminal width/height.