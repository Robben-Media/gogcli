package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Calendar settings commands - manage user calendar settings.

// CalendarSettingsCmd is the parent command for settings operations.
type CalendarSettingsCmd struct {
	List  CalendarSettingsListCmd  `cmd:"" name:"list" help:"List all calendar settings"`
	Get   CalendarSettingsGetCmd   `cmd:"" name:"get" help:"Get a specific calendar setting"`
	Watch CalendarSettingsWatchCmd `cmd:"" name:"watch" help:"Watch for changes to calendar settings"`
}

// CalendarSettingsListCmd lists all user settings for the calendar.
type CalendarSettingsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
}

func (c *CalendarSettingsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Settings.List().MaxResults(c.Max).PageToken(c.Page).Do()
	if err != nil {
		return fmt.Errorf("list settings: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"settings":      resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No settings")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tVALUE")
	for _, s := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\n", s.Id, s.Value)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// CalendarSettingsGetCmd retrieves a specific calendar setting.
type CalendarSettingsGetCmd struct {
	SettingID string `arg:"" name:"settingId" help:"Setting ID (e.g., 'timezone', 'weekStart', 'locale')"`
}

func (c *CalendarSettingsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	settingID := strings.TrimSpace(c.SettingID)
	if settingID == "" {
		return usage("settingId required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	setting, err := svc.Settings.Get(settingID).Do()
	if err != nil {
		return fmt.Errorf("get setting: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"setting": setting})
	}

	u.Out().Printf("id\t%s", setting.Id)
	u.Out().Printf("value\t%s", setting.Value)
	return nil
}

// CalendarSettingsWatchCmd watches for changes to calendar settings.
type CalendarSettingsWatchCmd struct {
	WebhookURL string `name:"webhook-url" required:"" help:"Webhook URL to receive notifications"`
	ChannelID  string `name:"channel-id" help:"Unique channel ID (auto-generated if not provided)"`
	AuthToken  string `name:"auth-token" help:"Token sent with each notification"`
}

func (c *CalendarSettingsWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.WebhookURL) == "" {
		return usage("--webhook-url is required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	// Generate channel ID if not provided
	channelID := c.ChannelID
	if channelID == "" {
		channelID = uuid.New().String()
	}

	channel := &calendar.Channel{
		Id:      channelID,
		Type:    "web_hook",
		Address: c.WebhookURL,
	}

	if c.AuthToken != "" {
		channel.Token = c.AuthToken
	}

	resp, err := svc.Settings.Watch(channel).Do()
	if err != nil {
		return fmt.Errorf("watch settings: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"channel": resp})
	}

	u.Out().Printf("channel_id\t%s", resp.Id)
	u.Out().Printf("resource_id\t%s", resp.ResourceId)
	u.Out().Printf("resource_uri\t%s", resp.ResourceUri)
	if resp.Expiration > 0 {
		u.Out().Printf("expiration\t%s", formatUnixMillis(resp.Expiration))
	}
	return nil
}
