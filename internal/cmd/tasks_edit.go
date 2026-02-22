package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// TasksMoveCmd moves a task within a task list (repositioning and nesting).
type TasksMoveCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID to move"`
	Parent     string `name:"parent" help:"New parent task ID (makes it a subtask; omit to move to top level)"`
	Previous   string `name:"previous" help:"Task ID to insert after (omit to place at beginning)"`
}

func (c *TasksMoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Tasks.Move(tasklistID, taskID)
	if strings.TrimSpace(c.Parent) != "" {
		call = call.Parent(strings.TrimSpace(c.Parent))
	}
	if strings.TrimSpace(c.Previous) != "" {
		call = call.Previous(strings.TrimSpace(c.Previous))
	}

	moved, err := call.Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"task": moved})
	}
	u.Out().Printf("id\t%s", moved.Id)
	u.Out().Printf("title\t%s", moved.Title)
	if strings.TrimSpace(moved.Parent) != "" {
		u.Out().Printf("parent\t%s", moved.Parent)
	}
	if strings.TrimSpace(moved.Position) != "" {
		u.Out().Printf("position\t%s", moved.Position)
	}
	return nil
}

// TasksReplaceCmd replaces a task entirely (full replace with PUT semantics).
// Unlike the update command which uses PATCH for partial updates, this uses PUT
// and clears any fields not explicitly provided.
type TasksReplaceCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
	Title      string `name:"title" required:"" help:"Task title (required)"`
	Notes      string `name:"notes" help:"Task notes/description"`
	Status     string `name:"status" help:"Task status: needsAction|completed"`
	Due        string `name:"due" help:"Due date (RFC3339 or YYYY-MM-DD; time may be ignored by Google Tasks)"`
}

func (c *TasksReplaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	status := strings.TrimSpace(c.Status)
	if status != "" && status != taskStatusNeedsAction && status != taskStatusCompleted {
		return usage("invalid --status (expected needsAction or completed)")
	}

	warnTasksDueTime(u, c.Due)
	dueValue, err := normalizeTaskDue(c.Due)
	if err != nil {
		return err
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	task := &tasks.Task{
		Title:  title,
		Notes:  strings.TrimSpace(c.Notes),
		Status: status,
		Due:    dueValue,
	}

	updated, err := svc.Tasks.Update(tasklistID, taskID, task).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"task": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("title\t%s", updated.Title)
	if strings.TrimSpace(updated.Status) != "" {
		u.Out().Printf("status\t%s", updated.Status)
	}
	if strings.TrimSpace(updated.Due) != "" {
		u.Out().Printf("due\t%s", updated.Due)
	}
	if strings.TrimSpace(updated.WebViewLink) != "" {
		u.Out().Printf("link\t%s", updated.WebViewLink)
	}
	return nil
}
