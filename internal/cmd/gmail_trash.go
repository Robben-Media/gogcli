package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailTrashCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID to trash"`
}

func (c *GmailTrashCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	msgID := strings.TrimSpace(c.MessageID)
	if msgID == "" {
		return usage("empty messageId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	msg, err := svc.Users.Messages.Trash("me", msgID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("trash message: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":       msg.Id,
			"threadId": msg.ThreadId,
			"trashed":  true,
		})
	}

	u.Err().Printf("Message %s moved to trash", msg.Id)
	return nil
}
