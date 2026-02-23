package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DriveChannelsCmd is the parent command for channel operations
type DriveChannelsCmd struct {
	Stop DriveChannelsStopCmd `cmd:"" name:"stop" help:"Stop watching resources via webhook"`
}

// DriveChannelsStopCmd stops watching a resource via webhook
type DriveChannelsStopCmd struct {
	ChannelID  string `name:"channel-id" help:"Channel ID to stop (required)"`
	ResourceID string `name:"resource-id" help:"Resource ID to stop watching (required)"`
}

func (c *DriveChannelsStopCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	channelID := strings.TrimSpace(c.ChannelID)
	resourceID := strings.TrimSpace(c.ResourceID)

	if channelID == "" {
		return usage("--channel-id is required")
	}
	if resourceID == "" {
		return usage("--resource-id is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	channel := &drive.Channel{
		Id:         channelID,
		ResourceId: resourceID,
	}

	if err := svc.Channels.Stop(channel).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"stopped":    true,
			"channelId":  channelID,
			"resourceId": resourceID,
		})
	}

	u.Out().Printf("stopped\ttrue")
	u.Out().Printf("channel_id\t%s", channelID)
	u.Out().Printf("resource_id\t%s", resourceID)
	return nil
}
