package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/calendar/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func listCalendarEvents(ctx context.Context, svc *calendar.Service, calendarID, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)

	call := svc.Events.List(calendarID).
		TimeMin(from).
		TimeMax(to).
		MaxResults(maxResults).
		PageToken(page).
		SingleEvents(true).
		OrderBy("startTime")
	if strings.TrimSpace(query) != "" {
		call = call.Q(query)
	}
	if strings.TrimSpace(privatePropFilter) != "" {
		call = call.PrivateExtendedProperty(privatePropFilter)
	}
	if strings.TrimSpace(sharedPropFilter) != "" {
		call = call.SharedExtendedProperty(sharedPropFilter)
	}
	if strings.TrimSpace(fields) != "" {
		call = call.Fields(gapi.Field(fields))
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"events":        wrapEventsWithCalendar(calendarID, resp.Items),
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No events")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	if showWeekday {
		fmt.Fprintln(w, "ID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
		for _, e := range resp.Items {
			startDay, endDay := eventDaysOfWeek(e)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Id, eventStart(e), startDay, eventEnd(e), endDay, e.Summary)
		}
		printNextPageHint(u, resp.NextPageToken)
		return nil
	}

	fmt.Fprintln(w, "ID\tSTART\tEND\tSUMMARY")
	for _, e := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e), eventEnd(e), e.Summary)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type eventWithCalendar struct {
	*calendar.Event
	CalendarID     string `json:"calendarId"`
	StartDayOfWeek string `json:"startDayOfWeek,omitempty"`
	EndDayOfWeek   string `json:"endDayOfWeek,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	EventTimezone  string `json:"eventTimezone,omitempty"`
	StartLocal     string `json:"startLocal,omitempty"`
	EndLocal       string `json:"endLocal,omitempty"`
}

func (e *eventWithCalendar) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	extras := map[string]any{
		"calendarId":     e.CalendarID,
		"startDayOfWeek": e.StartDayOfWeek,
		"endDayOfWeek":   e.EndDayOfWeek,
		"timezone":       e.Timezone,
		"eventTimezone":  e.EventTimezone,
		"startLocal":     e.StartLocal,
		"endLocal":       e.EndLocal,
	}
	return marshalWrappedEventJSON(e.Event, extras)
}

func listAllCalendarsEvents(ctx context.Context, svc *calendar.Service, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)

	calendars, err := listCalendarList(ctx, svc)
	if err != nil {
		return err
	}

	if len(calendars) == 0 {
		u.Err().Println("No calendars")
		return nil
	}

	ids := make([]string, 0, len(calendars))
	for _, cal := range calendars {
		if cal == nil || strings.TrimSpace(cal.Id) == "" {
			continue
		}
		ids = append(ids, cal.Id)
	}
	if len(ids) == 0 {
		u.Err().Println("No calendars")
		return nil
	}
	return listCalendarIDsEvents(ctx, svc, ids, from, to, maxResults, page, query, privatePropFilter, sharedPropFilter, fields, showWeekday)
}

func listSelectedCalendarsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	return listCalendarIDsEvents(ctx, svc, calendarIDs, from, to, maxResults, page, query, privatePropFilter, sharedPropFilter, fields, showWeekday)
}

func listCalendarIDsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)

	all := []*eventWithCalendar{}
	seen := make(map[string]struct{})
	for _, calID := range calendarIDs {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		call := svc.Events.List(calID).
			TimeMin(from).
			TimeMax(to).
			MaxResults(maxResults).
			PageToken(page).
			SingleEvents(true).
			OrderBy("startTime")
		if strings.TrimSpace(query) != "" {
			call = call.Q(query)
		}
		if strings.TrimSpace(privatePropFilter) != "" {
			call = call.PrivateExtendedProperty(privatePropFilter)
		}
		if strings.TrimSpace(sharedPropFilter) != "" {
			call = call.SharedExtendedProperty(sharedPropFilter)
		}
		if strings.TrimSpace(fields) != "" {
			call = call.Fields(gapi.Field(fields))
		}
		events, err := call.Context(ctx).Do()
		if err != nil {
			u.Err().Printf("calendar %s: %v", calID, err)
			continue
		}
		for _, e := range events.Items {
			key := eventCrossCalendarDedupeKey(e)
			if key != "" {
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
			}
			all = append(all, wrapEventWithCalendar(calID, e))
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"events": all})
	}
	if len(all) == 0 {
		u.Err().Println("No events")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	if showWeekday {
		fmt.Fprintln(w, "CALENDAR\tID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY")
		for _, e := range all {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventStart(e.Event), e.StartDayOfWeek, eventEnd(e.Event), e.EndDayOfWeek, e.Summary)
		}
		return nil
	}

	fmt.Fprintln(w, "CALENDAR\tID\tSTART\tEND\tSUMMARY")
	for _, e := range all {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventStart(e.Event), eventEnd(e.Event), e.Summary)
	}
	return nil
}

func resolveCalendarIDs(ctx context.Context, svc *calendar.Service, inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	calendars, err := listCalendarList(ctx, svc)
	if err != nil {
		return nil, err
	}

	bySummary := make(map[string]string, len(calendars))
	byID := make(map[string]string, len(calendars))
	for _, cal := range calendars {
		if cal == nil {
			continue
		}
		if strings.TrimSpace(cal.Id) != "" {
			byID[strings.ToLower(strings.TrimSpace(cal.Id))] = cal.Id
		}
		if strings.TrimSpace(cal.Summary) != "" {
			bySummary[strings.ToLower(strings.TrimSpace(cal.Summary))] = cal.Id
		}
	}

	out := make([]string, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	var unrecognized []string

	for _, raw := range inputs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if isDigits(value) {
			idx, err := strconv.Atoi(value)
			if err != nil {
				return nil, usagef("invalid calendar index: %s", value)
			}
			if idx < 1 || idx > len(calendars) {
				return nil, usagef("calendar index %d out of range (have %d calendars)", idx, len(calendars))
			}
			cal := calendars[idx-1]
			if cal == nil || strings.TrimSpace(cal.Id) == "" {
				return nil, usagef("calendar index %d has no id", idx)
			}
			appendUniqueCalendarID(&out, seen, cal.Id)
			continue
		}

		key := strings.ToLower(value)
		if id, ok := bySummary[key]; ok {
			appendUniqueCalendarID(&out, seen, id)
			continue
		}
		if id, ok := byID[key]; ok {
			appendUniqueCalendarID(&out, seen, id)
			continue
		}
		unrecognized = append(unrecognized, value)
	}

	if len(unrecognized) > 0 {
		return nil, usagef("unrecognized calendar name(s): %s", strings.Join(unrecognized, ", "))
	}

	return out, nil
}

func listCalendarList(ctx context.Context, svc *calendar.Service) ([]*calendar.CalendarListEntry, error) {
	var (
		items     []*calendar.CalendarListEntry
		pageToken string
	)
	for {
		call := svc.CalendarList.List().MaxResults(250).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		if len(resp.Items) > 0 {
			items = append(items, resp.Items...)
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return items, nil
}

func appendUniqueCalendarID(out *[]string, seen map[string]struct{}, id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if _, ok := seen[id]; ok {
		return
	}
	seen[id] = struct{}{}
	*out = append(*out, id)
}

func wrapEventsWithCalendar(calendarID string, events []*calendar.Event) []*eventWithCalendar {
	if len(events) == 0 {
		return []*eventWithCalendar{}
	}
	out := make([]*eventWithCalendar, 0, len(events))
	for _, event := range events {
		out = append(out, wrapEventWithCalendar(calendarID, event))
	}
	return out
}

func wrapEventWithCalendar(calendarID string, event *calendar.Event) *eventWithCalendar {
	wrapped := wrapEventWithDaysWithTimezone(event, "", nil)
	if wrapped == nil {
		return nil
	}
	return &eventWithCalendar{
		Event:          wrapped.Event,
		CalendarID:     calendarID,
		StartDayOfWeek: wrapped.StartDayOfWeek,
		EndDayOfWeek:   wrapped.EndDayOfWeek,
		Timezone:       wrapped.Timezone,
		EventTimezone:  wrapped.EventTimezone,
		StartLocal:     wrapped.StartLocal,
		EndLocal:       wrapped.EndLocal,
	}
}

func eventCrossCalendarDedupeKey(event *calendar.Event) string {
	if event == nil {
		return ""
	}
	uid := strings.TrimSpace(event.ICalUID)
	if uid == "" {
		return ""
	}
	start := ""
	if event.Start != nil {
		start = strings.TrimSpace(event.Start.DateTime)
		if start == "" {
			start = strings.TrimSpace(event.Start.Date)
		}
	}
	end := ""
	if event.End != nil {
		end = strings.TrimSpace(event.End.DateTime)
		if end == "" {
			end = strings.TrimSpace(event.End.Date)
		}
	}
	return uid + "|" + start + "|" + end
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
