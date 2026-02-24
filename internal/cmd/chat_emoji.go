package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ChatEmojiCmd contains subcommands for custom emoji management.
type ChatEmojiCmd struct {
	List   ChatEmojiListCmd   `cmd:"" name:"list" help:"List custom emojis"`
	Get    ChatEmojiGetCmd    `cmd:"" name:"get" help:"Get a custom emoji"`
	Create ChatEmojiCreateCmd `cmd:"" name:"create" help:"Create a custom emoji"`
	Delete ChatEmojiDeleteCmd `cmd:"" name:"delete" help:"Delete a custom emoji"`
}

// ChatEmojiListCmd lists custom emojis.
type ChatEmojiListCmd struct {
	Max    int64  `name:"max" aliases:"limit" help:"Max results (default: 25, max: 200)" default:"25"`
	Page   string `name:"page" help:"Page token"`
	Filter string `name:"filter" help:"Filter by creator (e.g., 'creator.users/me' for current user)"`
}

func (c *ChatEmojiListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.CustomEmojis.List().PageSize(c.Max)
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
			Name     string `json:"name,omitempty"`
			UID      string `json:"uid,omitempty"`
			ImageURI string `json:"imageUri,omitempty"`
		}
		items := make([]item, 0, len(resp.CustomEmojis))
		for _, emoji := range resp.CustomEmojis {
			if emoji == nil {
				continue
			}
			items = append(items, item{
				Resource: emoji.Name,
				Name:     emoji.EmojiName,
				UID:      emoji.Uid,
				ImageURI: emoji.TemporaryImageUri,
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"customEmojis":  items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.CustomEmojis) == 0 {
		u.Err().Println("No custom emojis")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tUID")
	for _, emoji := range resp.CustomEmojis {
		if emoji == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			emoji.Name,
			sanitizeTab(emoji.EmojiName),
			sanitizeTab(emoji.Uid),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// ChatEmojiGetCmd gets a specific custom emoji.
type ChatEmojiGetCmd struct {
	Name string `arg:"" name:"name" help:"Custom emoji resource name (customEmojis/...)"`
}

func (c *ChatEmojiGetCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	// Normalize resource name if needed
	if !strings.HasPrefix(name, "customEmojis/") {
		name = "customEmojis/" + name
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	emoji, err := svc.CustomEmojis.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"customEmoji": emoji})
	}

	if emoji.Name != "" {
		u.Out().Printf("resource\t%s", emoji.Name)
	}
	if emoji.EmojiName != "" {
		u.Out().Printf("name\t%s", emoji.EmojiName)
	}
	if emoji.Uid != "" {
		u.Out().Printf("uid\t%s", emoji.Uid)
	}
	if emoji.TemporaryImageUri != "" {
		u.Out().Printf("imageUri\t%s", emoji.TemporaryImageUri)
	}
	return nil
}

// ChatEmojiCreateCmd creates a custom emoji.
type ChatEmojiCreateCmd struct {
	EmojiName string `name:"name" help:"Emoji name (must start and end with :, lowercase, alphanumeric, hyphens, underscores)"`
	File      string `name:"file" short:"f" help:"Path to image file (PNG, JPG, or GIF; use - for stdin)"`
}

func (c *ChatEmojiCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	emojiName := strings.TrimSpace(c.EmojiName)
	if emojiName == "" {
		return usage("required: --name")
	}

	// Validate emoji name format
	if !isValidEmojiName(emojiName) {
		return usage("emoji name must start and end with :, be lowercase, and contain only alphanumeric characters, hyphens, and underscores (e.g., :my-emoji:)")
	}

	filePath := strings.TrimSpace(c.File)
	if filePath == "" {
		return usage("required: --file")
	}

	// Read the image file
	var imageData []byte
	if filePath == "-" {
		imageData, err = io.ReadAll(os.Stdin)
	} else {
		filePath, err = config.ExpandPath(filePath)
		if err != nil {
			return fmt.Errorf("expanding file path: %w", err)
		}
		imageData, err = os.ReadFile(filePath) //nolint:gosec // user-provided path
	}
	if err != nil {
		return fmt.Errorf("reading image file: %w", err)
	}

	if len(imageData) == 0 {
		return usage("image file is empty")
	}

	// Get filename from path
	var filename string
	if filePath != "-" {
		filename = filePath[strings.LastIndex(filePath, "/")+1:]
		if filename == "" {
			filename = "emoji.png"
		}
	} else {
		filename = "emoji.png"
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	// Build the custom emoji request
	req := &chat.CustomEmoji{
		EmojiName: emojiName,
		Payload: &chat.CustomEmojiPayload{
			FileContent: base64.StdEncoding.EncodeToString(imageData),
			Filename:    filename,
		},
	}

	resp, err := svc.CustomEmojis.Create(req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"customEmoji": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	if resp.EmojiName != "" {
		u.Out().Printf("name\t%s", resp.EmojiName)
	}
	return nil
}

// ChatEmojiDeleteCmd deletes a custom emoji.
type ChatEmojiDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Custom emoji resource name (customEmojis/...)"`
}

func (c *ChatEmojiDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	// Normalize resource name if needed
	if !strings.HasPrefix(name, "customEmojis/") {
		name = "customEmojis/" + name
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete custom emoji %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.CustomEmojis.Delete(name).Do()
	if err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, fmt.Sprintf("custom emoji %s", name))
}

// isValidEmojiName validates that an emoji name follows the required format.
func isValidEmojiName(name string) bool {
	if len(name) < 3 {
		return false
	}
	if name[0] != ':' || name[len(name)-1] != ':' {
		return false
	}
	// Check characters between colons
	for i := 1; i < len(name)-1; i++ {
		c := name[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
