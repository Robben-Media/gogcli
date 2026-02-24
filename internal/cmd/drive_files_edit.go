package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/google/uuid"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DriveFilesCmd is the parent command for file operations
type DriveFilesCmd struct {
	Watch       DriveFilesWatchCmd       `cmd:"" name:"watch" help:"Watch a file for changes via webhook"`
	GenerateIds DriveFilesGenerateIdsCmd `cmd:"" name:"generate-ids" help:"Generate IDs for file creation"`
	EmptyTrash  DriveFilesEmptyTrashCmd  `cmd:"" name:"empty-trash" help:"Permanently delete all trashed files"`
}

// DriveFilesWatchCmd watches a specific file for changes via webhook
type DriveFilesWatchCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID to watch"`
	Address   string `name:"address" help:"Webhook callback URL (required)"`
	ChannelID string `name:"channel-id" help:"Channel ID (auto-generated if not specified)"`
	Token     string `name:"token" help:"Verification token for webhook"`
}

func (c *DriveFilesWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("fileId is required")
	}

	address := strings.TrimSpace(c.Address)
	if address == "" {
		return usage("--address is required for webhook")
	}

	channelID := strings.TrimSpace(c.ChannelID)
	if channelID == "" {
		channelID = uuid.New().String()
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	channel := &drive.Channel{
		Id:      channelID,
		Type:    "web_hook",
		Address: address,
	}
	if c.Token != "" {
		channel.Token = strings.TrimSpace(c.Token)
	}

	resp, err := svc.Files.Watch(fileID, channel).
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"channel":    resp,
			"channelId":  resp.Id,
			"resourceId": resp.ResourceId,
		})
	}

	u.Out().Printf("channel_id\t%s", resp.Id)
	u.Out().Printf("resource_id\t%s", resp.ResourceId)
	u.Out().Printf("address\t%s", address)
	if resp.Expiration > 0 {
		u.Out().Printf("expires\t%d", resp.Expiration)
	}
	return nil
}

// DriveFilesGenerateIdsCmd generates file IDs for batch creation
type DriveFilesGenerateIdsCmd struct {
	Count int64  `name:"count" help:"Number of IDs to generate" default:"10"`
	Space string `name:"space" help:"Space to generate IDs for: drive|appDataFolder|photos" default:"drive"`
	Type  string `name:"type" help:"Type of IDs to generate: files" default:"files"`
}

func (c *DriveFilesGenerateIdsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if c.Count < 1 || c.Count > 1000 {
		return usage("--count must be between 1 and 1000")
	}

	space := strings.TrimSpace(c.Space)
	if space == "" {
		space = "drive"
	}
	validSpaces := map[string]bool{"drive": true, "appDataFolder": true, "photos": true}
	if !validSpaces[space] {
		return usage("invalid --space: must be drive, appDataFolder, or photos")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Files.GenerateIds().
		Count(c.Count).
		Space(space).
		Context(ctx)
	fileType := strings.TrimSpace(c.Type)
	if fileType != "" {
		call = call.Type(fileType)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"ids":   resp.Ids,
			"space": resp.Space,
			"kind":  resp.Kind,
		})
	}

	for _, id := range resp.Ids {
		u.Out().Printf("%s", id)
	}
	return nil
}

// DriveFilesEmptyTrashCmd permanently deletes all trashed files
type DriveFilesEmptyTrashCmd struct {
	// Uses global --force flag from RootFlags
}

func (c *DriveFilesEmptyTrashCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	// Extra-strong warning since this is irreversible
	if confirmErr := confirmDestructive(ctx, flags, "PERMANENTLY DELETE ALL trashed files in Drive (this cannot be undone)"); confirmErr != nil {
		return confirmErr
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Files.EmptyTrash().Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"emptied": true,
		})
	}

	u.Out().Printf("emptied\ttrue")
	u.Err().Println("All trashed files have been permanently deleted")
	return nil
}
