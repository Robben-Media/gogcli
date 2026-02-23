package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// CalendarChannelsCmd is the parent command for channel operations.
// Channels are used for watching resources for changes.
type CalendarChannelsCmd struct {
	Stop CalendarChannelsStopCmd `cmd:"" name:"stop" help:"Stop watching a resource"`
}

// CalendarChannelsStopCmd stops watching a resource through a channel.
// This is used to stop receiving notifications for a previously created watch.
type CalendarChannelsStopCmd struct {
	ChannelID  string `name:"channel-id" required:"" help:"Channel ID from the watch response"`
	ResourceID string `name:"resource-id" required:"" help:"Resource ID from the watch response"`
}

func (c *CalendarChannelsStopCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	channelID := strings.TrimSpace(c.ChannelID)
	if channelID == "" {
		return usage("--channel-id is required")
	}
	resourceID := strings.TrimSpace(c.ResourceID)
	if resourceID == "" {
		return usage("--resource-id is required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	channel := &calendar.Channel{
		Id:         channelID,
		ResourceId: resourceID,
	}

	if err := svc.Channels.Stop(channel).Do(); err != nil {
		return fmt.Errorf("stop channel: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"stopped":    true,
			"channelId":  channelID,
			"resourceId": resourceID,
		})
	}

	u.Out().Printf("stopped\ttrue")
	u.Out().Printf("channelId\t%s", channelID)
	u.Out().Printf("resourceId\t%s", resourceID)
	return nil
}
