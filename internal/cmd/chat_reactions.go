package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ChatReactionsCmd contains subcommands for reaction management.
type ChatReactionsCmd struct {
	List   ChatReactionsListCmd   `cmd:"" name:"list" help:"List reactions on a message"`
	Create ChatReactionsCreateCmd `cmd:"" name:"create" help:"Add a reaction to a message"`
	Delete ChatReactionsDeleteCmd `cmd:"" name:"delete" help:"Remove a reaction"`
}

// ChatReactionsListCmd lists reactions on a message.
type ChatReactionsListCmd struct {
	Parent string `arg:"" name:"parent" help:"Message resource name (spaces/.../messages/...)"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results (default: 100)" default:"100"`
	Page   string `name:"page" help:"Page token"`
	Filter string `name:"filter" help:"Filter reactions"`
}

func (c *ChatReactionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("required: parent")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Spaces.Messages.Reactions.List(parent).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	if c.Filter != "" {
		call = call.Filter(c.Filter)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Emoji    string `json:"emoji"`
			User     string `json:"user"`
		}
		items := make([]item, 0, len(resp.Reactions))
		for _, r := range resp.Reactions {
			if r == nil {
				continue
			}
			items = append(items, item{
				Resource: r.Name,
				Emoji:    reactionEmoji(r),
				User:     reactionUser(r),
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"reactions":     items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Reactions) == 0 {
		u.Err().Println("No reactions")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tEMOJI\tUSER")
	for _, r := range resp.Reactions {
		if r == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			r.Name,
			sanitizeTab(reactionEmoji(r)),
			sanitizeTab(reactionUser(r)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// ChatReactionsCreateCmd adds a reaction to a message.
type ChatReactionsCreateCmd struct {
	Parent string `arg:"" name:"parent" help:"Message resource name (spaces/.../messages/...)"`
	Emoji  string `name:"emoji" required:"" help:"Unicode emoji string or custom emoji resource name (customEmojis/...)"`
}

func (c *ChatReactionsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("required: parent")
	}

	emoji := strings.TrimSpace(c.Emoji)
	if emoji == "" {
		return usage("required: --emoji")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	reaction := &chat.Reaction{
		Emoji: &chat.Emoji{},
	}
	if strings.HasPrefix(emoji, "customEmojis/") {
		reaction.Emoji.CustomEmoji = &chat.CustomEmoji{Name: emoji}
	} else {
		reaction.Emoji.Unicode = emoji
	}

	resp, err := svc.Spaces.Messages.Reactions.Create(parent, reaction).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"reaction": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	u.Out().Printf("emoji\t%s", reactionEmoji(resp))
	return nil
}

// ChatReactionsDeleteCmd removes a reaction.
type ChatReactionsDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Reaction resource name (spaces/.../messages/.../reactions/...)"`
}

func (c *ChatReactionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove reaction %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.Spaces.Messages.Reactions.Delete(name).Do()
	if err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, fmt.Sprintf("reaction %s", name))
}

// reactionEmoji extracts the emoji display string from a reaction.
func reactionEmoji(r *chat.Reaction) string {
	if r == nil || r.Emoji == nil {
		return ""
	}
	if r.Emoji.Unicode != "" {
		return r.Emoji.Unicode
	}
	if r.Emoji.CustomEmoji != nil {
		return r.Emoji.CustomEmoji.Uid
	}
	return ""
}

// reactionUser extracts the user display string from a reaction.
func reactionUser(r *chat.Reaction) string {
	if r == nil || r.User == nil {
		return ""
	}
	if r.User.DisplayName != "" {
		return r.User.DisplayName
	}
	return r.User.Name
}
