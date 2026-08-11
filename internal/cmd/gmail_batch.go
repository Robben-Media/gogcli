package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailBatchCmd struct {
	Delete GmailBatchDeleteCmd `cmd:"" name:"delete" help:"Permanently delete multiple messages"`
	Modify GmailBatchModifyCmd `cmd:"" name:"modify" help:"Modify labels on multiple messages"`
}

type GmailBatchDeleteCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs"`
}

func (c *GmailBatchDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if len(c.MessageIDs) == 0 {
		return usage("at least one message ID is required")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("permanently delete %d gmail message(s)", len(c.MessageIDs))); confirmErr != nil {
		return confirmErr
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Messages.BatchDelete("me", &gmail.BatchDeleteMessagesRequest{
		Ids: c.MessageIDs,
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"deleted": c.MessageIDs,
			"count":   len(c.MessageIDs),
		})
	}

	if outfmt.IsPlain(ctx) {
		return writeGmailBatchPlainReceipt(ctx, "delete", len(c.MessageIDs), nil, nil)
	}

	u.Out().Printf("Deleted %d messages", len(c.MessageIDs))
	return nil
}

type GmailBatchModifyCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs"`
	Add        string   `name:"add" help:"Labels to add (comma-separated, name or ID)"`
	Remove     string   `name:"remove" help:"Labels to remove (comma-separated, name or ID)"`
}

func (c *GmailBatchModifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	addLabels := splitCSV(c.Add)
	removeLabels := splitCSV(c.Remove)
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return errors.New("must specify --add and/or --remove")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}

	addIDs := resolveLabelIDs(addLabels, idMap)
	removeIDs := resolveLabelIDs(removeLabels, idMap)

	err = svc.Users.Messages.BatchModify("me", &gmail.BatchModifyMessagesRequest{
		Ids:            c.MessageIDs,
		AddLabelIds:    addIDs,
		RemoveLabelIds: removeIDs,
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"modified":      c.MessageIDs,
			"count":         len(c.MessageIDs),
			"addedLabels":   addIDs,
			"removedLabels": removeIDs,
		})
	}

	if outfmt.IsPlain(ctx) {
		return writeGmailBatchPlainReceipt(ctx, "modify", len(c.MessageIDs), addIDs, removeIDs)
	}

	u.Out().Printf("Modified %d messages", len(c.MessageIDs))
	return nil
}

func writeGmailBatchPlainReceipt(ctx context.Context, action string, count int, addedLabels, removedLabels []string) error {
	w, flush := tableWriter(ctx)
	defer flush()
	writeTableRow(ctx, w, []string{"ACTION", "COUNT", "ADDED_LABELS", "REMOVED_LABELS"})
	writeTableRow(ctx, w, []string{
		action,
		strconv.Itoa(count),
		joinCSV(addedLabels),
		joinCSV(removedLabels),
	})
	return nil
}
