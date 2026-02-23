package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const calendarIDPrimary = "primary"

type CalendarCmd struct {
	Calendars       CalendarCalendarsCmd       `cmd:"" name:"calendars" help:"Manage calendar list"`
	ACL             CalendarAclCmd             `cmd:"" name:"acl" help:"Manage calendar ACL"`
	Settings        CalendarSettingsCmd        `cmd:"" name:"settings" help:"Manage calendar settings"`
	Events          CalendarEventsCmd          `cmd:"" name:"events" aliases:"list" help:"List events from a calendar or all calendars"`
	Event           CalendarEventCmd           `cmd:"" name:"event" aliases:"get" help:"Get event"`
	Create          CalendarCreateCmd          `cmd:"" name:"create" help:"Create an event"`
	Update          CalendarUpdateCmd          `cmd:"" name:"update" help:"Update an event"`
	Delete          CalendarDeleteCmd          `cmd:"" name:"delete" help:"Delete an event"`
	FreeBusy        CalendarFreeBusyCmd        `cmd:"" name:"freebusy" help:"Get free/busy"`
	Respond         CalendarRespondCmd         `cmd:"" name:"respond" help:"Respond to an event invitation"`
	ProposeTime     CalendarProposeTimeCmd     `cmd:"" name:"propose-time" help:"Generate URL to propose a new meeting time (browser-only feature)"`
	Colors          CalendarColorsCmd          `cmd:"" name:"colors" help:"Show calendar colors"`
	Conflicts       CalendarConflictsCmd       `cmd:"" name:"conflicts" help:"Find conflicts"`
	Search          CalendarSearchCmd          `cmd:"" name:"search" help:"Search events"`
	Time            CalendarTimeCmd            `cmd:"" name:"time" help:"Show server time"`
	Users           CalendarUsersCmd           `cmd:"" name:"users" help:"List workspace users (use their email as calendar ID)"`
	Team            CalendarTeamCmd            `cmd:"" name:"team" help:"Show events for all members of a Google Group"`
	FocusTime       CalendarFocusTimeCmd       `cmd:"" name:"focus-time" help:"Create a Focus Time block"`
	OOO             CalendarOOOCmd             `cmd:"" name:"out-of-office" aliases:"ooo" help:"Create an Out of Office event"`
	WorkingLocation CalendarWorkingLocationCmd `cmd:"" name:"working-location" aliases:"wl" help:"Set working location (home/office/custom)"`
	// Event operations
	EventsWatch     CalendarEventsWatchCmd     `cmd:"" name:"events-watch" help:"Watch for event changes"`
	EventsImport    CalendarEventsImportCmd    `cmd:"" name:"events-import" help:"Import an event from another source"`
	EventsInstances CalendarEventsInstancesCmd `cmd:"" name:"events-instances" help:"Get instances of a recurring event"`
	EventsQuickAdd  CalendarEventsQuickAddCmd  `cmd:"" name:"events-quick-add" help:"Create an event from text"`
	EventsMove      CalendarEventsMoveCmd      `cmd:"" name:"events-move" help:"Move an event to another calendar"`
	// Channel operations
	Channels CalendarChannelsCmd `cmd:"" name:"channels" help:"Manage notification channels"`
}

// CalendarCalendarsCmd is the parent command for calendar list operations.
type CalendarCalendarsCmd struct {
	List   CalendarCalendarsListCmd   `cmd:"" name:"list" help:"List calendars"`
	Get    CalendarCalendarsGetCmd    `cmd:"" name:"get" help:"Get a calendar from the list"`
	Insert CalendarCalendarsInsertCmd `cmd:"" name:"insert" help:"Add a calendar to the list"`
	Update CalendarCalendarsUpdateCmd `cmd:"" name:"update" help:"Update a calendar in the list"`
	Patch  CalendarCalendarsPatchCmd  `cmd:"" name:"patch" help:"Patch a calendar in the list"`
	Delete CalendarCalendarsDeleteCmd `cmd:"" name:"delete" help:"Remove a calendar from the list"`
	Watch  CalendarCalendarsWatchCmd  `cmd:"" name:"watch" help:"Watch for changes to the calendar list"`
}

// CalendarCalendarsListCmd lists calendars in the user's calendar list.
type CalendarCalendarsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
}

func (c *CalendarCalendarsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.CalendarList.List().MaxResults(c.Max).PageToken(c.Page).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"calendars":     resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No calendars")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tROLE")
	for _, cal := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\n", cal.Id, cal.Summary, cal.AccessRole)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// CalendarAclCmd is defined in calendar_acl.go with subcommands for ACL management.

type CalendarEventsCmd struct {
	CalendarID        string   `arg:"" name:"calendarId" optional:"" help:"Calendar ID (default: primary)"`
	Cal               []string `name:"cal" help:"Calendar ID or name (can be repeated)"`
	Calendars         string   `name:"calendars" help:"Comma-separated calendar IDs, names, or indices from 'calendar calendars'"`
	From              string   `name:"from" help:"Start time (RFC3339, date, or relative: today, tomorrow, monday)"`
	To                string   `name:"to" help:"End time (RFC3339, date, or relative)"`
	Today             bool     `name:"today" help:"Today only (timezone-aware)"`
	Tomorrow          bool     `name:"tomorrow" help:"Tomorrow only (timezone-aware)"`
	Week              bool     `name:"week" help:"This week (uses --week-start, default Mon)"`
	Days              int      `name:"days" help:"Next N days (timezone-aware)" default:"0"`
	WeekStart         string   `name:"week-start" help:"Week start day for --week (sun, mon, ...)" default:""`
	Max               int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page              string   `name:"page" help:"Page token"`
	Query             string   `name:"query" help:"Free text search"`
	All               bool     `name:"all" help:"Fetch events from all calendars"`
	PrivatePropFilter string   `name:"private-prop-filter" help:"Filter by private extended property (key=value)"`
	SharedPropFilter  string   `name:"shared-prop-filter" help:"Filter by shared extended property (key=value)"`
	Fields            string   `name:"fields" help:"Comma-separated fields to return"`
	Weekday           bool     `name:"weekday" help:"Include start/end day-of-week columns" default:"${calendar_weekday}"`
}

func (c *CalendarEventsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarID := strings.TrimSpace(c.CalendarID)
	calInputs := append([]string{}, c.Cal...)
	if strings.TrimSpace(c.Calendars) != "" {
		calInputs = append(calInputs, splitCSV(c.Calendars)...)
	}
	if c.All && (calendarID != "" || len(calInputs) > 0) {
		return usage("calendarId or --cal/--calendars not allowed with --all flag")
	}
	if calendarID != "" && len(calInputs) > 0 {
		return usage("calendarId not allowed with --cal/--calendars")
	}
	if !c.All && calendarID == "" && len(calInputs) == 0 {
		calendarID = calendarIDPrimary
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	// Use timezone-aware time resolution
	timeRange, err := ResolveTimeRange(ctx, svc, TimeRangeFlags{
		From:      c.From,
		To:        c.To,
		Today:     c.Today,
		Tomorrow:  c.Tomorrow,
		Week:      c.Week,
		Days:      c.Days,
		WeekStart: c.WeekStart,
	})
	if err != nil {
		return err
	}

	from, to := timeRange.FormatRFC3339()

	if c.All {
		return listAllCalendarsEvents(ctx, svc, from, to, c.Max, c.Page, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
	}
	if len(calInputs) > 0 {
		ids, err := resolveCalendarIDs(ctx, svc, calInputs)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return usage("no calendars specified")
		}
		return listSelectedCalendarsEvents(ctx, svc, ids, from, to, c.Max, c.Page, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
	}
	return listCalendarEvents(ctx, svc, calendarID, from, to, c.Max, c.Page, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
}

type CalendarEventCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
}

func (c *CalendarEventCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	eventID := strings.TrimSpace(c.EventID)
	if calendarID == "" {
		return usage("empty calendarId")
	}
	if eventID == "" {
		return usage("empty eventId")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	event, err := svc.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return err
	}
	tz, loc, _ := getCalendarLocation(ctx, svc, calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(event, tz, loc)})
	}
	printCalendarEventWithTimezone(u, event, tz, loc)
	return nil
}
