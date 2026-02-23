package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailMessagesImportCmd imports a message into the mailbox.
// Unlike insert, import processes the message through Gmail's standard receiving
// pipeline (spam detection, filters, categorization).
type GmailMessagesImportCmd struct {
	Raw      string `name:"raw" help:"Raw RFC 2822 message content (base64url encoded)"`
	RawFile  string `name:"raw-file" help:"File containing raw RFC 2822 message (use '-' for stdin)"`
	ThreadID string `name:"thread-id" help:"Thread ID to associate message with"`
}

func (c *GmailMessagesImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	raw, err := resolveRawMessageInput(c.Raw, c.RawFile)
	if err != nil {
		return err
	}
	if raw == "" {
		return usage("must provide --raw or --raw-file")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	msg := &gmail.Message{
		Raw: raw,
	}
	if c.ThreadID != "" {
		msg.ThreadId = strings.TrimSpace(c.ThreadID)
	}

	imported, err := svc.Users.Messages.Import("me", msg).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("import message: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":       imported.Id,
			"threadId": imported.ThreadId,
		})
	}

	u.Out().Printf("id\t%s", imported.Id)
	if imported.ThreadId != "" {
		u.Out().Printf("thread_id\t%s", imported.ThreadId)
	}
	return nil
}

// GmailMessagesInsertCmd inserts a message directly into the mailbox.
// Bypasses most processing - use import for normal Gmail pipeline.
type GmailMessagesInsertCmd struct {
	Raw      string `name:"raw" help:"Raw RFC 2822 message content (base64url encoded)"`
	RawFile  string `name:"raw-file" help:"File containing raw RFC 2822 message (use '-' for stdin)"`
	ThreadID string `name:"thread-id" help:"Thread ID to associate message with"`
}

func (c *GmailMessagesInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	raw, err := resolveRawMessageInput(c.Raw, c.RawFile)
	if err != nil {
		return err
	}
	if raw == "" {
		return usage("must provide --raw or --raw-file")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	msg := &gmail.Message{
		Raw: raw,
	}
	if c.ThreadID != "" {
		msg.ThreadId = strings.TrimSpace(c.ThreadID)
	}

	inserted, err := svc.Users.Messages.Insert("me", msg).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":       inserted.Id,
			"threadId": inserted.ThreadId,
		})
	}

	u.Out().Printf("id\t%s", inserted.Id)
	if inserted.ThreadId != "" {
		u.Out().Printf("thread_id\t%s", inserted.ThreadId)
	}
	return nil
}

// GmailMessagesModifyCmd modifies labels on a message.
type GmailMessagesModifyCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID to modify"`
	Add       string `name:"add" help:"Labels to add (comma-separated, name or ID)"`
	Remove    string `name:"remove" help:"Labels to remove (comma-separated, name or ID)"`
}

func (c *GmailMessagesModifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	msgID := strings.TrimSpace(c.MessageID)
	if msgID == "" {
		return usage("empty messageId")
	}

	addLabels := splitCSV(c.Add)
	removeLabels := splitCSV(c.Remove)
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return usage("must specify --add and/or --remove")
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

	modified, err := svc.Users.Messages.Modify("me", msgID, &gmail.ModifyMessageRequest{
		AddLabelIds:    addIDs,
		RemoveLabelIds: removeIDs,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("modify message: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":            modified.Id,
			"threadId":      modified.ThreadId,
			"addedLabels":   addIDs,
			"removedLabels": removeIDs,
		})
	}

	u.Out().Printf("id\t%s", modified.Id)
	if modified.ThreadId != "" {
		u.Out().Printf("thread_id\t%s", modified.ThreadId)
	}
	u.Err().Printf("Labels modified successfully")
	return nil
}

// GmailMessagesTrashCmd moves a message to trash.
type GmailMessagesTrashCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID to trash"`
}

func (c *GmailMessagesTrashCmd) Run(ctx context.Context, flags *RootFlags) error {
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

// GmailMessagesUntrashCmd removes a message from trash.
type GmailMessagesUntrashCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID to remove from trash"`
}

func (c *GmailMessagesUntrashCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	msg, err := svc.Users.Messages.Untrash("me", msgID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("untrash message: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":        msg.Id,
			"threadId":  msg.ThreadId,
			"untrashed": true,
		})
	}

	u.Err().Printf("Message %s removed from trash", msg.Id)
	return nil
}

// resolveRawMessageInput handles --raw and --raw-file flag resolution.
// Returns base64url-encoded raw message.
func resolveRawMessageInput(raw, rawFile string) (string, error) {
	raw = strings.TrimSpace(raw)
	rawFile = strings.TrimSpace(rawFile)

	if raw != "" && rawFile != "" {
		return "", usage("use only one of --raw or --raw-file")
	}

	if raw != "" {
		// Assume user provides pre-encoded base64url
		return raw, nil
	}

	if rawFile == "" {
		return "", nil
	}

	var data []byte
	var err error

	if rawFile == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(rawFile) //nolint:gosec // user-provided path
	}
	if err != nil {
		return "", fmt.Errorf("read raw file: %w", err)
	}

	// Encode as base64url (no padding)
	return base64.RawURLEncoding.EncodeToString(data), nil
}
