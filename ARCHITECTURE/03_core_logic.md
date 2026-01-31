# Core Logic Architecture

## Priority Calculation (`internal/priority`)
Tasc uses a dynamic scoring system to calculate "Urgency". The `Calculator` applies a set of `Rule` functions, summing their outputs.

### Rules
1.  **`ScheduledRule`**:
    *   Future scheduled date: Penalty (-5).
    *   Past/Today scheduled date: Boost (+15).
2.  **`RescheduleRule`**: +2 per reschedule event (highlights friction).
3.  **`AgeRule`**: +0.1 per day (prevents starvation of old tasks).
4.  **`DueRule`**:
    *   Overdue: +20 (+2/day).
    *   Today/Tomorrow: +15.
    *   This week: +5.
    *   Later: -5.
5.  **`EstimateRule`**: "Quick Wins" (<30m) get a small boost (+3).

### Dependency Propagation
In `cmd/list.go`, priority scores are propagated through the dependency graph. A task blocking a high-priority task inherits a portion of that priority (relaxation algorithm), ensuring blockers bubble up.

## Recurrence (`internal/recurrence`)
Handles generating the next task instance.
- **Trigger**: When a recurring task is marked `done` or `deleted` (with user choice).
- **Logic**: Parses natural language rules like "daily", "every 2 weeks", "first monday of month".
- **Implementation**: Calculates the next `ScheduledAt` date based on the *previous* `ScheduledAt` (or `DueAt`/`Now`). It creates a *new* task row for the next instance, preserving history.

## Date Parsing (`internal/parse`)
Robust parsing combining standard layouts (ISO 8601) and natural language processing (`go-naturaldate`).
- Preprocessing step handles shorthand inputs like "2d" -> "in 2 days" to ensure intuitive CLI usage.
