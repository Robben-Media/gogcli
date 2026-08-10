package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/calendar/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// multiCalendarPageCursorPrefix marks opaque aggregate page tokens for multi-calendar
// event listing. Single-calendar mode continues to use raw Google page tokens.
const multiCalendarPageCursorPrefix = "mcal1."

// multiCalendarPageCursor is the decoded form of an aggregate multi-calendar page token.
// Selection is the exact calendar set the cursor was issued for.
// Next maps unfinished calendars to their Google page token; an empty value means the
// calendar still needs its first page (used to retry first-page failures without
// treating the calendar as exhausted).
type multiCalendarPageCursor struct {
	Selection []string
	Next      map[string]string
}

type multiCalendarPageCursorPayload struct {
	Selection []string          `json:"s"`
	Next      map[string]string `json:"n"`
}

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
			"events":        wrapEventsWithDays(resp.Items),
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
	CalendarID     string
	StartDayOfWeek string `json:"startDayOfWeek,omitempty"`
	EndDayOfWeek   string `json:"endDayOfWeek,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	StartLocal     string `json:"startLocal,omitempty"`
	EndLocal       string `json:"endLocal,omitempty"`
}

type calendarEventError struct {
	CalendarID string `json:"calendarId"`
	Error      string `json:"error"`
}

func listAllCalendarsEvents(ctx context.Context, svc *calendar.Service, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	calendars, err := listCalendarList(ctx, svc)
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(calendars))
	for _, cal := range calendars {
		if cal == nil || strings.TrimSpace(cal.Id) == "" {
			continue
		}
		ids = append(ids, cal.Id)
	}
	return listCalendarIDsEvents(ctx, svc, ids, from, to, maxResults, page, query, privatePropFilter, sharedPropFilter, fields, showWeekday)
}

func listSelectedCalendarsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	return listCalendarIDsEvents(ctx, svc, calendarIDs, from, to, maxResults, page, query, privatePropFilter, sharedPropFilter, fields, showWeekday)
}

func listCalendarIDsEvents(ctx context.Context, svc *calendar.Service, calendarIDs []string, from, to string, maxResults int64, page, query, privatePropFilter, sharedPropFilter, fields string, showWeekday bool) error {
	u := ui.FromContext(ctx)

	selected := make([]string, 0, len(calendarIDs))
	selectedSet := make(map[string]struct{}, len(calendarIDs))
	for _, calID := range calendarIDs {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		if _, ok := selectedSet[calID]; ok {
			continue
		}
		selectedSet[calID] = struct{}{}
		selected = append(selected, calID)
	}

	cursor, decodeErr := decodeMultiCalendarPageCursor(page)
	if decodeErr != nil {
		return decodeErr
	}
	if compatibilityErr := ensureMultiCalendarCursorCompatible(selected, selectedSet, cursor); compatibilityErr != nil {
		return compatibilityErr
	}

	all := []*eventWithCalendar{}
	failures := []calendarEventError{}
	nextTokens := map[string]string{}
	for _, calID := range selected {
		pageToken := ""
		if cursor != nil {
			token, ok := cursor.Next[calID]
			if !ok {
				// Exhausted on a previous aggregate page; do not restart.
				continue
			}
			pageToken = token
		}

		call := svc.Events.List(calID).
			TimeMin(from).
			TimeMax(to).
			MaxResults(maxResults).
			PageToken(pageToken).
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
		events, listErr := call.Context(ctx).Do()
		if listErr != nil {
			failure := calendarEventError{CalendarID: calID, Error: listErr.Error()}
			failures = append(failures, failure)
			u.Err().Printf("calendar %s: %s", failure.CalendarID, failure.Error)
			// Keep the failed request retryable without restarting exhausted calendars.
			nextTokens[calID] = pageToken
			continue
		}
		if strings.TrimSpace(events.NextPageToken) != "" {
			nextTokens[calID] = events.NextPageToken
		}
		for _, e := range events.Items {
			startDay, endDay := eventDaysOfWeek(e)
			evTimezone := eventTimezone(e)
			startLocal := formatEventLocal(e.Start, nil)
			endLocal := formatEventLocal(e.End, nil)
			all = append(all, &eventWithCalendar{
				Event:          e,
				CalendarID:     calID,
				StartDayOfWeek: startDay,
				EndDayOfWeek:   endDay,
				Timezone:       evTimezone,
				StartLocal:     startLocal,
				EndLocal:       endLocal,
			})
		}
	}

	nextPageToken, err := encodeMultiCalendarPageCursor(selected, nextTokens)
	if err != nil {
		return err
	}

	var resultErr error
	if len(failures) > 0 {
		resultErr = fmt.Errorf("failed to fetch events from %d calendar(s)", len(failures))
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(os.Stdout, map[string]any{
			"events":        all,
			"errors":        failures,
			"complete":      len(failures) == 0,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		return resultErr
	}
	if len(all) == 0 && len(failures) == 0 {
		u.Err().Println("No events")
		printNextPageHint(u, nextPageToken)
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	header := []string{"TYPE", "CALENDAR", "ID", "START", "END", "SUMMARY", "ERROR"}
	if showWeekday {
		header = []string{"TYPE", "CALENDAR", "ID", "START", "START_DOW", "END", "END_DOW", "SUMMARY", "ERROR"}
	}
	writeTableRow(ctx, w, header)

	for _, e := range all {
		row := []string{"event", e.CalendarID, e.Id, eventStart(e.Event), eventEnd(e.Event), e.Summary, ""}
		if showWeekday {
			row = []string{"event", e.CalendarID, e.Id, eventStart(e.Event), e.StartDayOfWeek, eventEnd(e.Event), e.EndDayOfWeek, e.Summary, ""}
		}
		writeTableRow(ctx, w, row)
	}
	for _, failure := range failures {
		row := make([]string, len(header))
		row[0] = "calendar_error"
		row[1] = failure.CalendarID
		row[len(row)-1] = sanitizeTab(failure.Error)
		writeTableRow(ctx, w, row)
	}
	printNextPageHint(u, nextPageToken)
	return resultErr
}

func encodeMultiCalendarPageCursor(selection []string, next map[string]string) (string, error) {
	if len(next) == 0 {
		return "", nil
	}
	cleanSelection := make([]string, 0, len(selection))
	seen := make(map[string]struct{}, len(selection))
	for _, calID := range selection {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		if _, ok := seen[calID]; ok {
			continue
		}
		seen[calID] = struct{}{}
		cleanSelection = append(cleanSelection, calID)
	}
	if len(cleanSelection) == 0 {
		return "", nil
	}

	cleanNext := make(map[string]string, len(next))
	for calID, token := range next {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		if _, ok := seen[calID]; !ok {
			return "", fmt.Errorf("internal error: unfinished calendar %q not in selection", calID)
		}
		// Preserve empty tokens: they mean retry the first page.
		cleanNext[calID] = strings.TrimSpace(token)
	}
	if len(cleanNext) == 0 {
		return "", nil
	}

	raw, err := json.Marshal(multiCalendarPageCursorPayload{
		Selection: cleanSelection,
		Next:      cleanNext,
	})
	if err != nil {
		return "", err
	}
	return multiCalendarPageCursorPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeMultiCalendarPageCursor(page string) (*multiCalendarPageCursor, error) {
	page = strings.TrimSpace(page)
	if page == "" {
		// A nil cursor marks the first aggregate page.
		return nil, nil //nolint:nilnil
	}
	if !strings.HasPrefix(page, multiCalendarPageCursorPrefix) {
		return nil, usage("invalid multi-calendar page token")
	}
	encoded := strings.TrimPrefix(page, multiCalendarPageCursorPrefix)
	if encoded == "" {
		return nil, usage("invalid multi-calendar page token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, usagef("invalid multi-calendar page token: %v", err)
	}
	var payload multiCalendarPageCursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, usagef("invalid multi-calendar page token: %v", err)
	}

	selection := make([]string, 0, len(payload.Selection))
	selectedSet := make(map[string]struct{}, len(payload.Selection))
	for _, calID := range payload.Selection {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			return nil, usage("invalid multi-calendar page token: empty calendar id in selection")
		}
		if _, ok := selectedSet[calID]; ok {
			continue
		}
		selectedSet[calID] = struct{}{}
		selection = append(selection, calID)
	}
	if len(selection) == 0 {
		return nil, usage("invalid multi-calendar page token: empty selection")
	}

	next := make(map[string]string)
	for calID, token := range payload.Next {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			return nil, usage("invalid multi-calendar page token: empty calendar id")
		}
		if _, ok := selectedSet[calID]; !ok {
			return nil, usagef("invalid multi-calendar page token: unfinished calendar not in selection: %s", calID)
		}
		// Empty token is valid and means retry first page.
		next[calID] = strings.TrimSpace(token)
	}
	if len(next) == 0 {
		return nil, usage("invalid multi-calendar page token: no unfinished calendars")
	}
	return &multiCalendarPageCursor{Selection: selection, Next: next}, nil
}

func ensureMultiCalendarCursorCompatible(selected []string, selectedSet map[string]struct{}, cursor *multiCalendarPageCursor) error {
	if cursor == nil {
		return nil
	}
	if len(cursor.Selection) != len(selectedSet) {
		return usage("multi-calendar page token selection does not match requested calendars")
	}
	for _, calID := range cursor.Selection {
		if _, ok := selectedSet[calID]; !ok {
			return usagef("multi-calendar page token selection does not match requested calendars: %s", calID)
		}
	}
	// selected is already unique; require exact set equality with cursor.Selection.
	if len(selected) != len(cursor.Selection) {
		return usage("multi-calendar page token selection does not match requested calendars")
	}
	for calID := range cursor.Next {
		if _, ok := selectedSet[calID]; !ok {
			return usagef("multi-calendar page token includes calendar not in selection: %s", calID)
		}
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
