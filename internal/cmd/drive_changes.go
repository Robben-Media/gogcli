package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/google/uuid"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const allDrivesCorpora = "allDrives"

// DriveChangesCmd is the parent command for changes subcommands
type DriveChangesCmd struct {
	List            DriveChangesListCmd            `cmd:"" name:"list" help:"List changes to files"`
	GetStartPageTok DriveChangesGetStartPageTokCmd `cmd:"" name:"get-start-page-token" help:"Get the starting page token for changes"`
	Watch           DriveChangesWatchCmd           `cmd:"" name:"watch" help:"Watch for changes via webhook"`
}

// DriveChangesGetStartPageTokCmd gets the starting page token for watching changes
type DriveChangesGetStartPageTokCmd struct {
	// No arguments needed
}

func (c *DriveChangesGetStartPageTokCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Changes.GetStartPageToken().
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"startPageToken": resp.StartPageToken,
			"kind":           resp.Kind,
		})
	}

	u.Out().Printf("start_page_token\t%s", resp.StartPageToken)
	return nil
}

// DriveChangesListCmd lists changes to files
type DriveChangesListCmd struct {
	PageToken string `arg:"" name:"pageToken" help:"Page token to start from (use get-start-page-token to get initial token)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	NextToken string `name:"next" help:"Next page token (alternative to positional argument)"`
	Include   string `name:"include" help:"Include items from: allDrives|domain|user" default:""`
}

func (c *DriveChangesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	pageToken := strings.TrimSpace(c.PageToken)
	if pageToken == "" {
		pageToken = strings.TrimSpace(c.NextToken)
	}
	if pageToken == "" {
		return usage("pageToken is required (use 'drive changes get-start-page-token' to get initial token)")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	includeCorpus := strings.TrimSpace(c.Include)
	if includeCorpus == "" {
		includeCorpus = allDrivesCorpora
	}

	call := svc.Changes.List(pageToken).
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(includeCorpus == allDrivesCorpora).
		IncludeCorpusRemovals(true).
		Fields("nextPageToken, newStartPageToken, changes(type, time, removed, fileId, file(id, name, mimeType, size, modifiedTime, parents, webViewLink, trashed))").
		Context(ctx)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"changes":           resp.Changes,
			"changeCount":       len(resp.Changes),
			"nextPageToken":     resp.NextPageToken,
			"newStartPageToken": resp.NewStartPageToken,
		})
	}

	if len(resp.Changes) == 0 {
		u.Err().Println("No changes")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "FILE_ID\tNAME\tTYPE\tACTION\tMODIFIED")
	for _, change := range resp.Changes {
		var fileID, name, mimeType, action, modified string

		if change.Removed {
			action = "removed"
		} else {
			action = "modified"
		}

		if change.File != nil {
			fileID = change.File.Id
			name = change.File.Name
			mimeType = driveType(change.File.MimeType)
			if change.File.Trashed {
				action = "trashed"
			}
			modified = formatDateTime(change.File.ModifiedTime)
		} else {
			fileID = change.FileId
			name = "-"
			mimeType = "-"
			modified = formatDateTime(change.Time)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", fileID, name, mimeType, action, modified)
	}

	if resp.NewStartPageToken != "" {
		u.Err().Printf("New start page token: %s\n", resp.NewStartPageToken)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// DriveChangesWatchCmd watches for changes via webhook
type DriveChangesWatchCmd struct {
	PageToken string `arg:"" name:"pageToken" help:"Page token to start from"`
	Address   string `name:"address" help:"Webhook callback URL (required)"`
	ChannelID string `name:"channel-id" help:"Channel ID (auto-generated if not specified)"`
	Token     string `name:"token" help:"Verification token for webhook"`
	NextToken string `name:"next" help:"Next page token (alternative to positional argument)"`
}

func (c *DriveChangesWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	pageToken := strings.TrimSpace(c.PageToken)
	if pageToken == "" {
		pageToken = strings.TrimSpace(c.NextToken)
	}
	if pageToken == "" {
		return usage("pageToken is required (use 'drive changes get-start-page-token' to get initial token)")
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

	resp, err := svc.Changes.Watch(pageToken, channel).
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"channel":  resp,
			"resource": resp.ResourceId,
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
