package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailThreadDeleteCmd permanently deletes a thread and all its messages.
// This is a destructive operation that requires confirmation.
type GmailThreadDeleteCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID to permanently delete"`
}

func (c *GmailThreadDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	threadID := strings.TrimSpace(c.ThreadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Confirmation for destructive operation
	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("permanently delete thread %s and ALL messages in it", threadID)); confirmErr != nil {
		return confirmErr
	}

	err = svc.Users.Threads.Delete("me", threadID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("delete thread: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":      threadID,
			"deleted": true,
		})
	}

	u.Err().Printf("Thread %s permanently deleted", threadID)
	return nil
}

// GmailThreadTrashCmd moves a thread and all its messages to trash.
type GmailThreadTrashCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID to move to trash"`
}

func (c *GmailThreadTrashCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	threadID := strings.TrimSpace(c.ThreadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	thread, err := svc.Users.Threads.Trash("me", threadID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("trash thread: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":       thread.Id,
			"threadId": thread.Id,
			"trashed":  true,
		})
	}

	u.Err().Printf("Thread %s moved to trash", thread.Id)
	return nil
}

// GmailThreadUntrashCmd removes a thread and all its messages from trash.
type GmailThreadUntrashCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID to remove from trash"`
}

func (c *GmailThreadUntrashCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	threadID := strings.TrimSpace(c.ThreadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	thread, err := svc.Users.Threads.Untrash("me", threadID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("untrash thread: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":        thread.Id,
			"threadId":  thread.Id,
			"untrashed": true,
		})
	}

	u.Err().Printf("Thread %s removed from trash", thread.Id)
	return nil
}
