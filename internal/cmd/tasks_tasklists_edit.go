package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// TasksListsGetCmd retrieves a single task list by ID.
type TasksListsGetCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
}

func (c *TasksListsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	tl, err := svc.Tasklists.Get(tasklistID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"tasklist": tl})
	}
	u.Out().Printf("id\t%s", tl.Id)
	u.Out().Printf("title\t%s", tl.Title)
	u.Out().Printf("updated\t%s", tl.Updated)
	return nil
}

// TasksListsDeleteCmd permanently deletes a task list and all tasks within it.
type TasksListsDeleteCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
}

func (c *TasksListsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}

	if confErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete task list %s and all its tasks", tasklistID)); confErr != nil {
		return confErr
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	if delErr := svc.Tasklists.Delete(tasklistID).Context(ctx).Do(); delErr != nil {
		return delErr
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":    true,
			"tasklistId": tasklistID,
		})
	}
	u.Err().Printf("Deleted task list %s", tasklistID)
	return nil
}

// TasksListsPatchCmd updates specific fields of a task list (partial update).
type TasksListsPatchCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	Title      string `name:"title" help:"New title for the task list"`
}

func (c *TasksListsPatchCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}

	patch := &tasks.TaskList{}
	changed := false
	if flagProvided(kctx, "title") {
		patch.Title = strings.TrimSpace(c.Title)
		changed = true
	}
	if !changed {
		return usage("no updates provided")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Tasklists.Patch(tasklistID, patch).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"tasklist": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("title\t%s", updated.Title)
	u.Out().Printf("updated\t%s", updated.Updated)
	return nil
}

// TasksListsUpdateCmd replaces a task list entirely (full replace with PUT).
type TasksListsUpdateCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	Title      string `name:"title" required:"" help:"New title for the task list (required)"`
}

func (c *TasksListsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Tasklists.Update(tasklistID, &tasks.TaskList{Title: title}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"tasklist": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("title\t%s", updated.Title)
	u.Out().Printf("updated\t%s", updated.Updated)
	return nil
}
