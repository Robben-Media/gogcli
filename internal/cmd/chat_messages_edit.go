package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ChatMessagesGetCmd gets details about a message.
type ChatMessagesGetCmd struct {
	Name string `arg:"" name:"name" help:"Message resource name (spaces/.../messages/...)"`
}

func (c *ChatMessagesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Messages.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"message": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	sender := chatMessageSender(resp)
	if sender != "" {
		u.Out().Printf("sender\t%s", sender)
	}
	text := chatMessageText(resp)
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	if text != "" {
		u.Out().Printf("text\t%s", text)
	}
	if resp.CreateTime != "" {
		u.Out().Printf("createTime\t%s", resp.CreateTime)
	}
	return nil
}

// ChatMessagesDeleteCmd deletes a message.
type ChatMessagesDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Message resource name (spaces/.../messages/...)"`
}

func (c *ChatMessagesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete message %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.Spaces.Messages.Delete(name).Do()
	if err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, fmt.Sprintf("message %s", name))
}

// ChatMessagesPatchCmd patches a message (partial update).
type ChatMessagesPatchCmd struct {
	Name    string `arg:"" name:"name" help:"Message resource name (spaces/.../messages/...)"`
	Text    string `name:"text" help:"New message text"`
	CardsV2 string `name:"cards-v2" help:"Cards v2 JSON (raw JSON string)"`
}

func (c *ChatMessagesPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	var fields []string
	msg := &chat.Message{}

	if flagProvided(kctx, "text") {
		fields = append(fields, "text")
		msg.Text = c.Text
	}
	if flagProvided(kctx, "cards-v2") {
		fields = append(fields, "cardsV2")
		msg.CardsV2 = nil // raw JSON handled by API; set field for update mask
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Messages.Patch(name, msg).UpdateMask(strings.Join(fields, ",")).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"message": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	sender := chatMessageSender(resp)
	if sender != "" {
		u.Out().Printf("sender\t%s", sender)
	}
	text := chatMessageText(resp)
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	if text != "" {
		u.Out().Printf("text\t%s", text)
	}
	return nil
}

// ChatMessagesUpdateCmd performs a full update on a message.
type ChatMessagesUpdateCmd struct {
	Name    string `arg:"" name:"name" help:"Message resource name (spaces/.../messages/...)"`
	Text    string `name:"text" help:"New message text" required:""`
	CardsV2 string `name:"cards-v2" help:"Cards v2 JSON (raw JSON string)"`
}

func (c *ChatMessagesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	msg := &chat.Message{
		Text: c.Text,
	}

	fields := []string{"text"}
	if strings.TrimSpace(c.CardsV2) != "" {
		fields = append(fields, "cardsV2")
	}
	mask := strings.Join(fields, ",")

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Messages.Update(name, msg).UpdateMask(mask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"message": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	sender := chatMessageSender(resp)
	if sender != "" {
		u.Out().Printf("sender\t%s", sender)
	}
	text := chatMessageText(resp)
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	if text != "" {
		u.Out().Printf("text\t%s", text)
	}
	return nil
}

// ChatAttachmentsGetCmd gets details about a message attachment.
type ChatAttachmentsGetCmd struct {
	Name string `arg:"" name:"name" help:"Attachment resource name (spaces/.../messages/.../attachments/...)"`
}

func (c *ChatAttachmentsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Messages.Attachments.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"attachment": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.ContentName != "" {
		u.Out().Printf("contentName\t%s", resp.ContentName)
	}
	if resp.ContentType != "" {
		u.Out().Printf("contentType\t%s", resp.ContentType)
	}
	if resp.DownloadUri != "" {
		u.Out().Printf("downloadUri\t%s", resp.DownloadUri)
	}
	if resp.Source != "" {
		u.Out().Printf("source\t%s", resp.Source)
	}
	return nil
}
