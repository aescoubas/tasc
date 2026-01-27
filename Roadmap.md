# Roadmap

This document outlines the planned future features and improvements for `tasc`.

Completed features are listed in the **Completed** section at the bottom of this file, grouped by the quarter in which they were finished.

## Stale Task Identification
- **Age Tracking:** Identify tasks that were defined a long time ago and remain incomplete.
- **Rescheduling Analysis:** Identify tasks that are constantly being rescheduled, indicating potential roadblocks or lack of clarity.

## Enhanced Productivity Metrics
- **Advanced Stats:** Expand the `stats` command to provide deeper insights into productivity.
- **Closure Rates:** Track how many tasks are closed over specific periods.
- **Estimation Accuracy:** Analyze the mismatch between initial time estimates and the actual time taken to complete tasks.
- **Flexible Filtering:** Allow filtering of statistics by various attributes, such as project, tag, or priority.
- **Time Allocation:** Provide a breakdown of time spent or allocated to each project.

## Advanced Priority Scoring
- **Dynamic Scoring:** Improve the priority algorithm to consider additional data points, such as proximity to due dates and other task metadata.

## User Interface & Experience
- **Enhanced Colorscheme:** Improve the visual presentation of the CLI.
- **Background Color Support:** Add support for background colors in task listings (e.g., red on black) to improve readability and aesthetic appeal.

## Remote Server Mode
- **Client-Server Architecture:** Implement a mode where the executable acts as a client connecting to a central `tasc` server, enabling data synchronization and access across multiple devices.

## Mobile Integration
- **Lightweight Android App:** Develop a mobile application with a modern interface to allow task management and updates on the go, likely interacting with the "Remote Server Mode".

## Project Management Framework
- **Projects & Subprojects:** Implement efficient grouping of tasks into projects and subprojects.
- **Cascading Deadlines:** Allow setting top-level project deadlines that automatically update or constrain the due dates of contained tasks.
- **Dependency Visualization:** Provide tools to quickly visualize and update task dependencies.
- **Dependency-Aware Priority:** Update the priority scoring to factor in the priority of blocked tasks or the number of blocked tasks (e.g., blocking a high-priority task increases the blocker's score).

## Completed (Q1 2026)

### Status Field Refinement
- **Expanded Status Values:** Formalize a status field with values: `backlog`, `ongoing`, `done`, `blocked`, and `undefined`.
- **Refactor Existing Statuses:** Migrate existing `pending` to `backlog` or `ongoing`, `completed` to `done`, and `poorly_defined` to `undefined`.
- **Explicit Blocked State:** Evaluate making `blocked` an explicit status for easier filtering, while maintaining the dynamic dependency logic.
