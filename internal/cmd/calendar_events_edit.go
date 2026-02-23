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

// Calendar events operations - watch, import, instances, quick-add, move.
// These are additional event operations beyond the basic list/get/create/update/delete.

// CalendarEventsWatchCmd sets up a webhook to watch for event changes.
type CalendarEventsWatchCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID (default: primary)"`
	WebhookURL string `name:"webhook-url" required:"" help:"Webhook URL to receive notifications"`
	ChannelID  string `name:"channel-id" help:"Unique channel ID (auto-generated if not provided)"`
	AuthToken  string `name:"auth-token" help:"Token sent with each notification"`
}

func (c *CalendarEventsWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		calendarID = calendarIDPrimary
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

	resp, err := svc.Events.Watch(calendarID, channel).Do()
	if err != nil {
		return fmt.Errorf("watch events: %w", err)
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

// CalendarEventsImportCmd imports an event from another source (e.g., an iCal event).
// This preserves the event's original ID and metadata.
type CalendarEventsImportCmd struct {
	CalendarID     string   `arg:"" name:"calendarId" help:"Calendar ID (default: primary)"`
	EventID        string   `arg:"" name:"eventId" help:"Event ID (iCal UID)"`
	Summary        string   `name:"summary" required:"" help:"Event summary/title"`
	From           string   `name:"from" required:"" help:"Start time (RFC3339)"`
	To             string   `name:"to" required:"" help:"End time (RFC3339)"`
	Description    string   `name:"description" help:"Description"`
	Location       string   `name:"location" help:"Location"`
	AllDay         bool     `name:"all-day" help:"All-day event (use date-only in --from/--to)"`
	Timezone       string   `name:"timezone" help:"Event timezone (default: calendar timezone)"`
	ConferenceData string   `name:"conference-data" help:"Conference data JSON"`
	ICalUID        string   `name:"ical-uid" help:"iCal UID (defaults to eventId)"`
	Organizer      string   `name:"organizer" help:"Organizer email"`
	Recurrence     []string `name:"rrule" sep:"none" help:"Recurrence rules (e.g., 'RRULE:FREQ=MONTHLY;BYMONTHDAY=11')"`
	Status         string   `name:"status" help:"Event status: confirmed, tentative, cancelled"`
	Visibility     string   `name:"visibility" help:"Event visibility: default, public, private, confidential"`
}

func (c *CalendarEventsImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		calendarID = calendarIDPrimary
	}
	eventID := strings.TrimSpace(c.EventID)
	if eventID == "" {
		return usage("eventId is required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	event := &calendar.Event{
		Id:          eventID,
		Summary:     strings.TrimSpace(c.Summary),
		Description: strings.TrimSpace(c.Description),
		Location:    strings.TrimSpace(c.Location),
	}

	// Set iCal UID
	icalUID := strings.TrimSpace(c.ICalUID)
	if icalUID == "" {
		icalUID = eventID
	}
	event.ICalUID = icalUID

	// Set times
	allDay := c.AllDay
	if strings.TrimSpace(c.Timezone) == "" {
		tz, _, _ := getCalendarLocation(ctx, svc, calendarID)
		event.Start = buildEventDateTimeWithTimezone(c.From, allDay, tz)
		event.End = buildEventDateTimeWithTimezone(c.To, allDay, tz)
	} else {
		event.Start = buildEventDateTimeWithTimezone(c.From, allDay, c.Timezone)
		event.End = buildEventDateTimeWithTimezone(c.To, allDay, c.Timezone)
	}

	// Set optional fields
	if strings.TrimSpace(c.Organizer) != "" {
		event.Organizer = &calendar.EventOrganizer{Email: strings.TrimSpace(c.Organizer)}
	}
	if len(c.Recurrence) > 0 {
		event.Recurrence = buildRecurrence(c.Recurrence)
	}
	if strings.TrimSpace(c.Status) != "" {
		event.Status = strings.TrimSpace(c.Status)
	}
	if strings.TrimSpace(c.Visibility) != "" {
		visibility, vErr := validateVisibility(c.Visibility)
		if vErr != nil {
			return vErr
		}
		event.Visibility = visibility
	}

	call := svc.Events.Import(calendarID, event)
	if c.ConferenceData != "" {
		call = call.ConferenceDataVersion(1)
	}

	imported, err := call.Do()
	if err != nil {
		return fmt.Errorf("import event: %w", err)
	}

	tz, loc, _ := getCalendarLocation(ctx, svc, calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(imported, tz, loc)})
	}
	printCalendarEventWithTimezone(u, imported, tz, loc)
	return nil
}

// CalendarEventsInstancesCmd retrieves instances of a recurring event.
type CalendarEventsInstancesCmd struct {
	CalendarID    string `arg:"" name:"calendarId" help:"Calendar ID (default: primary)"`
	EventID       string `arg:"" name:"eventId" help:"Recurring event ID"`
	From          string `name:"from" help:"Start time for instances (RFC3339)"`
	To            string `name:"to" help:"End time for instances (RFC3339)"`
	Max           int64  `name:"max" aliases:"limit" help:"Max results" default:"250"`
	Page          string `name:"page" help:"Page token"`
	ShowDeleted   bool   `name:"show-deleted" help:"Include deleted instances"`
	Timezone      string `name:"timezone" help:"Timezone for response"`
	OriginalStart string `name:"original-start" help:"Filter by original start time"`
}

func (c *CalendarEventsInstancesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		calendarID = calendarIDPrimary
	}
	eventID := strings.TrimSpace(c.EventID)
	if eventID == "" {
		return usage("eventId is required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Events.Instances(calendarID, eventID).
		MaxResults(c.Max).
		PageToken(c.Page).
		ShowDeleted(c.ShowDeleted)

	if strings.TrimSpace(c.From) != "" {
		call = call.TimeMin(strings.TrimSpace(c.From))
	}
	if strings.TrimSpace(c.To) != "" {
		call = call.TimeMax(strings.TrimSpace(c.To))
	}
	if strings.TrimSpace(c.Timezone) != "" {
		call = call.TimeZone(strings.TrimSpace(c.Timezone))
	}
	if strings.TrimSpace(c.OriginalStart) != "" {
		call = call.OriginalStart(strings.TrimSpace(c.OriginalStart))
	}

	resp, err := call.Do()
	if err != nil {
		return fmt.Errorf("get event instances: %w", err)
	}

	tz, _, _ := getCalendarLocation(ctx, svc, calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"instances":     resp.Items,
			"nextPageToken": resp.NextPageToken,
			"timeZone":      tz,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No instances")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tSUMMARY\tSTART\tEND\tSTATUS")
	for _, inst := range resp.Items {
		start := formatEventTimeWithTimezone(inst.Start)
		end := formatEventTimeWithTimezone(inst.End)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", inst.Id, inst.Summary, start, end, inst.Status)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// CalendarEventsQuickAddCmd creates an event from a simple text string.
// Example: "Meeting with John tomorrow at 3pm"
type CalendarEventsQuickAddCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID (default: primary)"`
	Text       string `arg:"" name:"text" help:"Event description (e.g., 'Meeting tomorrow at 3pm')"`
	SendUpdate string `name:"send-updates" help:"Notification mode: all, externalOnly, none (default: all)"`
}

func (c *CalendarEventsQuickAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		calendarID = calendarIDPrimary
	}
	text := strings.TrimSpace(c.Text)
	if text == "" {
		return usage("text is required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Events.QuickAdd(calendarID, text)

	sendUpdates, err := validateSendUpdates(c.SendUpdate)
	if err != nil {
		return err
	}
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}

	created, err := call.Do()
	if err != nil {
		return fmt.Errorf("quick add event: %w", err)
	}

	tz, loc, _ := getCalendarLocation(ctx, svc, calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(created, tz, loc)})
	}
	printCalendarEventWithTimezone(u, created, tz, loc)
	return nil
}

// CalendarEventsMoveCmd moves an event from one calendar to another.
type CalendarEventsMoveCmd struct {
	SourceCalendarID      string `arg:"" name:"sourceCalendarId" help:"Source calendar ID (default: primary)"`
	EventID               string `arg:"" name:"eventId" help:"Event ID"`
	DestinationCalendarID string `arg:"" name:"destinationCalendarId" help:"Destination calendar ID"`
	SendUpdate            string `name:"send-updates" help:"Notification mode: all, externalOnly, none (default: all)"`
}

func (c *CalendarEventsMoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	sourceCalendarID := strings.TrimSpace(c.SourceCalendarID)
	if sourceCalendarID == "" {
		sourceCalendarID = calendarIDPrimary
	}
	eventID := strings.TrimSpace(c.EventID)
	if eventID == "" {
		return usage("eventId is required")
	}
	destinationCalendarID := strings.TrimSpace(c.DestinationCalendarID)
	if destinationCalendarID == "" {
		return usage("destinationCalendarId is required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Events.Move(sourceCalendarID, eventID, destinationCalendarID)

	sendUpdates, err := validateSendUpdates(c.SendUpdate)
	if err != nil {
		return err
	}
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}

	moved, err := call.Do()
	if err != nil {
		return fmt.Errorf("move event: %w", err)
	}

	tz, loc, _ := getCalendarLocation(ctx, svc, destinationCalendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"event":          wrapEventWithDaysWithTimezone(moved, tz, loc),
			"sourceCalendar": sourceCalendarID,
			"destCalendar":   destinationCalendarID,
		})
	}

	u.Out().Printf("moved\ttrue")
	u.Out().Printf("eventId\t%s", moved.Id)
	u.Out().Printf("calendar\t%s", destinationCalendarID)
	u.Out().Printf("summary\t%s", moved.Summary)
	return nil
}

// buildEventDateTimeWithTimezone creates an EventDateTime with explicit timezone.
func buildEventDateTimeWithTimezone(value string, allDay bool, timezone string) *calendar.EventDateTime {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	if allDay || (!strings.Contains(value, "T") && len(value) == 10) {
		return &calendar.EventDateTime{
			Date:     value,
			TimeZone: timezone,
		}
	}

	return &calendar.EventDateTime{
		DateTime: value,
		TimeZone: timezone,
	}
}

// formatEventTimeWithTimezone formats an EventDateTime for display.
func formatEventTimeWithTimezone(dt *calendar.EventDateTime) string {
	if dt == nil {
		return ""
	}
	if dt.Date != "" {
		return dt.Date
	}
	if dt.DateTime != "" {
		// Just return the datetime string, formatting handled elsewhere
		return dt.DateTime
	}
	return ""
}
