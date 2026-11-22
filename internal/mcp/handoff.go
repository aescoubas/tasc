package mcp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
)

const handoffMaxBytes = 512

type taskHandoff struct {
	ID        int64  `json:"id"`
	Handoff   string `json:"handoff"`
	Truncated bool   `json:"truncated"`
	Revision  string `json:"revision"`
}

func (ms *MCPServer) registerHandoffTools() {
	ms.server.AddTool(mcp.NewTool("tasc_get_task_handoff",
		mcp.WithDescription("Read the opening paragraph of a task description (up to 512 bytes) and its edit revision"),
		mcp.WithNumber("id", mcp.Required(), mcp.Min(1), mcp.MultipleOf(1), mcp.Description("Task ID")),
		mcp.WithOutputSchema[taskHandoff](),
	), ms.handleGetTaskHandoff)
	ms.server.AddTool(mcp.NewTool("tasc_update_task_handoff",
		mcp.WithDescription("Prepend or replace an opening handoff paragraph if the task description still matches the read revision"),
		mcp.WithNumber("id", mcp.Required(), mcp.Min(1), mcp.MultipleOf(1), mcp.Description("Task ID")),
		mcp.WithString("revision", mcp.Required(), mcp.Description("Revision returned by tasc_get_task_handoff")),
		mcp.WithString("mode", mcp.Required(), mcp.Enum("prepend", "replace"),
			mcp.Description("Prepend preserves the full description. Replace edits a complete opening paragraph you have identified as the handoff.")),
		mcp.WithString("handoff", mcp.Required(), mcp.Description("One paragraph, up to 512 UTF-8 bytes, naming the next actor and action")),
		mcp.WithOutputSchema[taskHandoff](),
	), ms.handleUpdateTaskHandoff)
}

// openingParagraph returns the byte range of the first nonblank paragraph,
// leaving leading blank lines and the paragraph separator outside that range.
func openingParagraph(description string) (start, end int) {
	offset := 0
	for rest := description; rest != ""; {
		line, next, _ := strings.Cut(rest, "\n")
		if strings.TrimSpace(line) == "" {
			if end > start {
				break
			}
		} else {
			if end == start {
				start = offset
			}
			end = offset + len(strings.TrimSuffix(line, "\r"))
		}
		offset += len(line) + 1
		rest = next
	}
	return start, end
}

func handoffRevision(description string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(description)))
}

func handoffResult(task models.Task) (*mcp.CallToolResult, error) {
	start, end := openingParagraph(task.Description)
	paragraph := task.Description[start:end]
	truncated := len(paragraph) > handoffMaxBytes
	if truncated {
		boundary := handoffMaxBytes
		for boundary > 0 && !utf8.RuneStart(paragraph[boundary]) {
			boundary--
		}
		paragraph = paragraph[:boundary]
	}
	return mcp.NewToolResultJSON(taskHandoff{
		ID: task.ID, Handoff: paragraph, Truncated: truncated, Revision: handoffRevision(task.Description),
	})
}

func (ms *MCPServer) taskForHandoff(request mcp.CallToolRequest) (models.Task, error) {
	id, err := taskID(request, "id")
	if err != nil {
		return models.Task{}, err
	}
	task, err := ms.store.GetTask(id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && task.Status == models.StatusDeleted) {
		return models.Task{}, errors.New("task not found")
	}
	return task, err
}

func (ms *MCPServer) handleGetTaskHandoff(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	task, err := ms.taskForHandoff(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return handoffResult(task)
}

func (ms *MCPServer) handleUpdateTaskHandoff(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	handoff := strings.TrimSpace(request.GetString("handoff", ""))
	start, end := openingParagraph(handoff)
	if handoff == "" || len(handoff) > handoffMaxBytes || start != 0 || end != len(handoff) {
		return mcp.NewToolResultError("handoff must be one nonblank paragraph of at most 512 UTF-8 bytes"), nil
	}

	task, err := ms.taskForHandoff(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	const conflict = "task description changed or is unavailable. Read the handoff again before retrying"
	if request.GetString("revision", "") != handoffRevision(task.Description) {
		return mcp.NewToolResultError(conflict), nil
	}

	description := handoff
	switch request.GetString("mode", "") {
	case "prepend":
		if task.Description != "" {
			description += "\n\n" + task.Description
		}
	case "replace":
		start, end := openingParagraph(task.Description)
		if start == end || end-start > handoffMaxBytes {
			return mcp.NewToolResultError("replace requires a complete opening paragraph. Use prepend to preserve the description"), nil
		}
		description = task.Description[:start] + handoff + task.Description[end:]
	default:
		return mcp.NewToolResultError("mode must be prepend or replace"), nil
	}

	if err := ms.caller(ctx).UpdateTaskDescription(task.ID, task.Description, description); err != nil {
		if errors.Is(err, store.ErrTaskDescriptionConflict) {
			return mcp.NewToolResultError(conflict), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("updating task handoff: %v", err)), nil
	}
	task.Description = description
	return handoffResult(task)
}
