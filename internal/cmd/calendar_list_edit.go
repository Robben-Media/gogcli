package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Calendar list management commands - manage the user's calendar list.
// These commands operate on CalendarListEntry resources (calendars the user has added to their list).

// CalendarCalendarsEditCmd is the parent command for calendar list operations.
type CalendarCalendarsEditCmd struct {
	Get    CalendarCalendarsGetCmd    `cmd:"" name:"get" help:"Get a calendar from the calendar list"`
	Insert CalendarCalendarsInsertCmd `cmd:"" name:"insert" help:"Add an existing calendar to the user's calendar list"`
	Update CalendarCalendarsUpdateCmd `cmd:"" name:"update" help:"Update a calendar entry in the calendar list"`
	Patch  CalendarCalendarsPatchCmd  `cmd:"" name:"patch" help:"Patch a calendar entry in the calendar list"`
	Delete CalendarCalendarsDeleteCmd `cmd:"" name:"delete" help:"Remove a calendar from the calendar list"`
	Watch  CalendarCalendarsWatchCmd  `cmd:"" name:"watch" help:"Watch for changes to the calendar list"`
}

// CalendarCalendarsGetCmd retrieves a calendar from the user's calendar list.
type CalendarCalendarsGetCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
}

func (c *CalendarCalendarsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	entry, err := svc.CalendarList.Get(calendarID).Do()
	if err != nil {
		return fmt.Errorf("get calendar list entry: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"calendar": entry})
	}

	u.Out().Printf("id\t%s", entry.Id)
	u.Out().Printf("summary\t%s", entry.Summary)
	if entry.Description != "" {
		u.Out().Printf("description\t%s", entry.Description)
	}
	u.Out().Printf("access_role\t%s", entry.AccessRole)
	u.Out().Printf("primary\t%v", entry.Primary)
	if entry.TimeZone != "" {
		u.Out().Printf("timezone\t%s", entry.TimeZone)
	}
	u.Out().Printf("hidden\t%v", entry.Hidden)
	if entry.BackgroundColor != "" {
		u.Out().Printf("background_color\t%s", entry.BackgroundColor)
	}
	if entry.ForegroundColor != "" {
		u.Out().Printf("foreground_color\t%s", entry.ForegroundColor)
	}
	return nil
}

// CalendarCalendarsInsertCmd adds an existing calendar to the user's calendar list.
type CalendarCalendarsInsertCmd struct {
	CalendarID           string `arg:"" name:"calendarId" help:"Calendar ID to add"`
	Summary              string `name:"summary" help:"Override summary"`
	Hidden               bool   `name:"hidden" help:"Hide this calendar"`
	Selected             bool   `name:"selected" help:"Show calendar in UI (default true)"`
	Unselected           bool   `name:"unselected" help:"Hide calendar in UI"`
	BackgroundColor      string `name:"background-color" help:"Hex background color (#RRGGBB)"`
	ForegroundColor      string `name:"foreground-color" help:"Hex foreground color (#RRGGBB)"`
	DefaultRemindersJSON string `name:"default-reminders" help:"JSON array of default reminders"`
}

func (c *CalendarCalendarsInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	entry := &calendar.CalendarListEntry{
		Id: calendarID,
	}

	if c.Summary != "" {
		entry.SummaryOverride = c.Summary
	}
	if c.Hidden {
		entry.Hidden = true
	}
	if c.Unselected {
		entry.Selected = false
	} else if c.Selected {
		entry.Selected = true
	}
	if c.BackgroundColor != "" {
		entry.BackgroundColor = c.BackgroundColor
	}
	if c.ForegroundColor != "" {
		entry.ForegroundColor = c.ForegroundColor
	}
	if c.DefaultRemindersJSON != "" {
		var reminders []*calendar.EventReminder
		if unmarshalErr := json.Unmarshal([]byte(c.DefaultRemindersJSON), &reminders); unmarshalErr != nil {
			return usagef("invalid default-reminders JSON: %v", unmarshalErr)
		}
		entry.DefaultReminders = reminders
	}

	created, err := svc.CalendarList.Insert(entry).Do()
	if err != nil {
		return fmt.Errorf("insert calendar to list: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"calendar": created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("summary\t%s", created.Summary)
	if created.SummaryOverride != "" {
		u.Out().Printf("summary_override\t%s", created.SummaryOverride)
	}
	return nil
}

// CalendarCalendarsUpdateCmd updates a calendar entry in the calendar list (full replace).
type CalendarCalendarsUpdateCmd struct {
	CalendarID           string `arg:"" name:"calendarId" help:"Calendar ID"`
	Summary              string `name:"summary" help:"Override summary"`
	ClearSummaryOverride bool   `name:"clear-summary-override" help:"Clear the summary override"`
	Hidden               bool   `name:"hidden" help:"Hide this calendar"`
	Visible              bool   `name:"visible" help:"Show this calendar (opposite of hidden)"`
	Selected             bool   `name:"selected" help:"Show calendar in UI"`
	Unselected           bool   `name:"unselected" help:"Hide calendar in UI"`
	BackgroundColor      string `name:"background-color" help:"Hex background color (#RRGGBB)"`
	ForegroundColor      string `name:"foreground-color" help:"Hex foreground color (#RRGGBB)"`
	ClearColors          bool   `name:"clear-colors" help:"Clear custom colors"`
	DefaultRemindersJSON string `name:"default-reminders" help:"JSON array of default reminders"`
	ClearReminders       bool   `name:"clear-reminders" help:"Clear default reminders"`
}

func (c *CalendarCalendarsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	// First get the existing entry
	existing, err := svc.CalendarList.Get(calendarID).Do()
	if err != nil {
		return fmt.Errorf("get calendar list entry: %w", err)
	}

	// Update fields
	if c.Summary != "" {
		existing.SummaryOverride = c.Summary
	}
	if c.ClearSummaryOverride {
		existing.SummaryOverride = ""
	}
	if c.Hidden {
		existing.Hidden = true
	}
	if c.Visible {
		existing.Hidden = false
	}
	if c.Unselected {
		existing.Selected = false
	} else if c.Selected {
		existing.Selected = true
	}
	if c.BackgroundColor != "" {
		existing.BackgroundColor = c.BackgroundColor
	}
	if c.ForegroundColor != "" {
		existing.ForegroundColor = c.ForegroundColor
	}
	if c.ClearColors {
		existing.BackgroundColor = ""
		existing.ForegroundColor = ""
	}
	if c.DefaultRemindersJSON != "" {
		var reminders []*calendar.EventReminder
		if unmarshalErr := json.Unmarshal([]byte(c.DefaultRemindersJSON), &reminders); unmarshalErr != nil {
			return usagef("invalid default-reminders JSON: %v", unmarshalErr)
		}
		existing.DefaultReminders = reminders
	}
	if c.ClearReminders {
		existing.DefaultReminders = nil
	}

	updated, err := svc.CalendarList.Update(calendarID, existing).Do()
	if err != nil {
		return fmt.Errorf("update calendar list entry: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"calendar": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("summary\t%s", updated.Summary)
	if updated.SummaryOverride != "" {
		u.Out().Printf("summary_override\t%s", updated.SummaryOverride)
	}
	u.Out().Printf("hidden\t%v", updated.Hidden)
	return nil
}

// CalendarCalendarsPatchCmd patches a calendar entry in the calendar list (partial update).
type CalendarCalendarsPatchCmd struct {
	CalendarID           string `arg:"" name:"calendarId" help:"Calendar ID"`
	Summary              string `name:"summary" help:"Override summary"`
	ClearSummaryOverride bool   `name:"clear-summary-override" help:"Clear the summary override"`
	Hidden               bool   `name:"hidden" help:"Hide this calendar"`
	Visible              bool   `name:"visible" help:"Show this calendar (opposite of hidden)"`
	Selected             bool   `name:"selected" help:"Show calendar in UI"`
	Unselected           bool   `name:"unselected" help:"Hide calendar in UI"`
	BackgroundColor      string `name:"background-color" help:"Hex background color (#RRGGBB)"`
	ForegroundColor      string `name:"foreground-color" help:"Hex foreground color (#RRGGBB)"`
	ClearColors          bool   `name:"clear-colors" help:"Clear custom colors"`
	DefaultRemindersJSON string `name:"default-reminders" help:"JSON array of default reminders"`
	ClearReminders       bool   `name:"clear-reminders" help:"Clear default reminders"`
}

func (c *CalendarCalendarsPatchCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	// Check if any updates provided
	hasUpdates := c.Summary != "" ||
		c.ClearSummaryOverride ||
		c.Hidden ||
		c.Visible ||
		c.Selected ||
		c.Unselected ||
		c.BackgroundColor != "" ||
		c.ForegroundColor != "" ||
		c.ClearColors ||
		c.DefaultRemindersJSON != "" ||
		c.ClearReminders ||
		flagProvided(kctx, "hidden") ||
		flagProvided(kctx, "visible") ||
		flagProvided(kctx, "selected") ||
		flagProvided(kctx, "unselected")

	if !hasUpdates {
		return usage("no updates provided")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	entry := &calendar.CalendarListEntry{}

	if c.Summary != "" {
		entry.SummaryOverride = c.Summary
	}
	if c.ClearSummaryOverride {
		entry.SummaryOverride = ""
	}
	if flagProvided(kctx, "hidden") {
		entry.Hidden = c.Hidden
	}
	if flagProvided(kctx, "visible") {
		entry.Hidden = !c.Visible
	}
	if flagProvided(kctx, "selected") {
		entry.Selected = c.Selected
	}
	if flagProvided(kctx, "unselected") {
		entry.Selected = !c.Unselected
	}
	if c.BackgroundColor != "" {
		entry.BackgroundColor = c.BackgroundColor
	}
	if c.ForegroundColor != "" {
		entry.ForegroundColor = c.ForegroundColor
	}
	if c.ClearColors {
		entry.BackgroundColor = ""
		entry.ForegroundColor = ""
	}
	if c.DefaultRemindersJSON != "" {
		var reminders []*calendar.EventReminder
		if unmarshalErr := json.Unmarshal([]byte(c.DefaultRemindersJSON), &reminders); unmarshalErr != nil {
			return usagef("invalid default-reminders JSON: %v", unmarshalErr)
		}
		entry.DefaultReminders = reminders
	}
	if c.ClearReminders {
		entry.DefaultReminders = nil
	}

	updated, err := svc.CalendarList.Patch(calendarID, entry).Do()
	if err != nil {
		return fmt.Errorf("patch calendar list entry: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"calendar": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("summary\t%s", updated.Summary)
	if updated.SummaryOverride != "" {
		u.Out().Printf("summary_override\t%s", updated.SummaryOverride)
	}
	u.Out().Printf("hidden\t%v", updated.Hidden)
	return nil
}

// CalendarCalendarsDeleteCmd removes a calendar from the user's calendar list.
type CalendarCalendarsDeleteCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID to remove from list"`
}

func (c *CalendarCalendarsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	// Confirm destructive action
	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove calendar %s from your calendar list", calendarID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.CalendarList.Delete(calendarID).Do(); err != nil {
		return fmt.Errorf("delete calendar from list: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":      calendarID,
			"deleted": true,
		})
	}

	u.Err().Printf("Calendar %q removed from your list", calendarID)
	return nil
}

// CalendarCalendarsWatchCmd watches for changes to the calendar list.
type CalendarCalendarsWatchCmd struct {
	WebhookURL string `name:"webhook-url" required:"" help:"Webhook URL to receive notifications"`
	ChannelID  string `name:"channel-id" help:"Unique channel ID (auto-generated if not provided)"`
	AuthToken  string `name:"auth-token" help:"Token sent with each notification"`
}

func (c *CalendarCalendarsWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resp, err := svc.CalendarList.Watch(channel).Do()
	if err != nil {
		return fmt.Errorf("watch calendar list: %w", err)
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
