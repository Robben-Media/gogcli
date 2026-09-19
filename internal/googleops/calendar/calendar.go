// Package calendar implements the native Calendar MCP operations.
package calendar

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	calendarapi "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	listDefaultResults = 100
	eventsDefaultRows  = 25
	maxCalendarRows    = 250
	maxFreeBusyDays    = 366
)

var errEmptyTimestamp = errors.New("empty timestamp")

type listInput struct {
	mcpcontract.Selection
	MaxResults int64  `json:"max_results,omitempty"`
	PageToken  string `json:"page_token,omitempty"`
}

type listEventsInput struct {
	mcpcontract.Selection
	CalendarID string `json:"calendar_id"`
	TimeMin    string `json:"time_min"`
	TimeMax    string `json:"time_max"`
	MaxResults int64  `json:"max_results,omitempty"`
	PageToken  string `json:"page_token,omitempty"`
	Query      string `json:"query,omitempty"`
}

type freeBusyInput struct {
	mcpcontract.Selection
	CalendarIDs []string `json:"calendar_ids"`
	TimeMin     string   `json:"time_min"`
	TimeMax     string   `json:"time_max"`
}

type Calendar struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	TimeZone    string `json:"timezone"`
	AccessRole  string `json:"access_role"`
	Primary     bool   `json:"primary"`
	Selected    bool   `json:"selected"`
	Deleted     bool   `json:"deleted"`
	Hidden      bool   `json:"hidden"`
}

type CalendarListData struct {
	Calendars []Calendar `json:"calendars"`
}

type EventTime struct {
	Date     string `json:"date"`
	DateTime string `json:"date_time"`
	TimeZone string `json:"time_zone"`
}

type Event struct {
	ID               string     `json:"id"`
	Summary          string     `json:"summary"`
	Description      string     `json:"description,omitempty"`
	Location         string     `json:"location,omitempty"`
	Status           string     `json:"status"`
	EventType        string     `json:"event_type"`
	Transparency     string     `json:"transparency"`
	Start            EventTime  `json:"start"`
	End              EventTime  `json:"end"`
	AllDay           bool       `json:"all_day"`
	Recurrence       []string   `json:"recurrence,omitempty"`
	RecurringEventID string     `json:"recurring_event_id,omitempty"`
	OriginalStart    *EventTime `json:"original_start,omitempty"`
	Created          string     `json:"created,omitempty"`
	Updated          string     `json:"updated,omitempty"`
	HtmlLink         string     `json:"html_link,omitempty"`
}

type CalendarEventsData struct {
	CalendarID string  `json:"calendar_id"`
	Summary    string  `json:"summary"`
	TimeZone   string  `json:"timezone"`
	AccessRole string  `json:"access_role"`
	Events     []Event `json:"events"`
}

type BusyInterval struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type FreeBusyCalendar struct {
	CalendarID string         `json:"calendar_id"`
	Status     string         `json:"status"`
	Busy       []BusyInterval `json:"busy"`
}

type FreeBusyData struct {
	TimeMin   string             `json:"time_min"`
	TimeMax   string             `json:"time_max"`
	Calendars []FreeBusyCalendar `json:"calendars"`
}

// Operations returns the frozen Calendar tool implementations.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("calendar_list", validateListInput, func(ctx context.Context, id mcpcontract.Identity, in listInput) (mcpcontract.Result[CalendarListData], error) {
			if in.MaxResults == 0 {
				in.MaxResults = listDefaultResults
			}

			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "calendar_list", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[CalendarListData]{}, fmt.Errorf("calendar HTTP client: %w", err)
			}

			svc, err := calendarapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[CalendarListData]{}, fmt.Errorf("create calendar service: %w", err)
			}

			resp, err := svc.CalendarList.List().MaxResults(in.MaxResults).PageToken(in.PageToken).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[CalendarListData]{}, fmt.Errorf("list calendars: %w", err)
			}

			out := mcpcontract.NewResult(id, CalendarListData{Calendars: make([]Calendar, 0, len(resp.Items))})
			for _, item := range resp.Items {
				if item == nil {
					continue
				}
				out.Data.Calendars = append(out.Data.Calendars, Calendar{
					ID: item.Id, Summary: item.Summary, Description: item.Description, TimeZone: item.TimeZone,
					AccessRole: item.AccessRole, Primary: item.Primary, Selected: item.Selected,
					Deleted: item.Deleted, Hidden: item.Hidden,
				})
			}
			out.NextPageToken = resp.NextPageToken

			return out, nil
		}),
		mcpcontract.NewOperation("calendar_list_events", validateListEventsInput, func(ctx context.Context, id mcpcontract.Identity, in listEventsInput) (mcpcontract.Result[CalendarEventsData], error) {
			if in.MaxResults == 0 {
				in.MaxResults = eventsDefaultRows
			}

			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "calendar_list_events", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[CalendarEventsData]{}, fmt.Errorf("calendar HTTP client: %w", err)
			}

			svc, err := calendarapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[CalendarEventsData]{}, fmt.Errorf("create calendar service: %w", err)
			}

			call := svc.Events.List(in.CalendarID).TimeMin(in.TimeMin).TimeMax(in.TimeMax).
				MaxResults(in.MaxResults).PageToken(in.PageToken).SingleEvents(true).OrderBy("startTime")
			if in.Query != "" {
				call = call.Q(in.Query)
			}

			resp, err := call.Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[CalendarEventsData]{}, fmt.Errorf("list calendar events: %w", err)
			}

			out := mcpcontract.NewResult(id, CalendarEventsData{
				CalendarID: in.CalendarID, Summary: resp.Summary, TimeZone: resp.TimeZone, AccessRole: resp.AccessRole,
				Events: make([]Event, 0, len(resp.Items)),
			})
			for _, item := range resp.Items {
				if item == nil {
					continue
				}
				out.Data.Events = append(out.Data.Events, projectEvent(item))
			}
			out.NextPageToken = resp.NextPageToken

			return out, nil
		}),
		mcpcontract.NewOperation("calendar_freebusy", validateFreeBusyInput, func(ctx context.Context, id mcpcontract.Identity, in freeBusyInput) (mcpcontract.Result[FreeBusyData], error) {
			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "calendar_freebusy", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[FreeBusyData]{}, fmt.Errorf("calendar HTTP client: %w", err)
			}

			svc, err := calendarapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[FreeBusyData]{}, fmt.Errorf("create calendar service: %w", err)
			}

			items := make([]*calendarapi.FreeBusyRequestItem, 0, len(in.CalendarIDs))
			for _, calendarID := range in.CalendarIDs {
				items = append(items, &calendarapi.FreeBusyRequestItem{Id: calendarID})
			}

			resp, err := svc.Freebusy.Query(&calendarapi.FreeBusyRequest{
				TimeMin: in.TimeMin, TimeMax: in.TimeMax, Items: items, CalendarExpansionMax: int64(len(items)),
			}).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[FreeBusyData]{}, fmt.Errorf("query free/busy: %w", err)
			}

			out := mcpcontract.NewResult(id, FreeBusyData{
				TimeMin: in.TimeMin, TimeMax: in.TimeMax, Calendars: make([]FreeBusyCalendar, 0, len(in.CalendarIDs)),
			})
			for _, calendarID := range in.CalendarIDs {
				upstream, upstreamExists := resp.Calendars[calendarID]

				calendarOut := FreeBusyCalendar{CalendarID: calendarID, Status: "ok", Busy: []BusyInterval{}}
				if !upstreamExists {
					calendarOut.Status = "error"

					out.PartialFailures = append(out.PartialFailures, mcpcontract.Failure{
						SourceID: calendarID, Category: string(mcpcontract.UpstreamFailure),
						Message: "Google returned no free/busy result for this calendar",
					})
				} else {
					for _, busy := range upstream.Busy {
						if busy == nil {
							continue
						}
						calendarOut.Busy = append(calendarOut.Busy, BusyInterval{Start: busy.Start, End: busy.End})
					}

					if len(upstream.Errors) > 0 {
						calendarOut.Status = "error"

						out.PartialFailures = append(out.PartialFailures, mcpcontract.Failure{
							SourceID: calendarID, Category: calendarFailureCategory(upstream.Errors),
							Message: calendarFailureMessage(upstream.Errors),
						})
					}
				}

				out.Data.Calendars = append(out.Data.Calendars, calendarOut)
			}

			return out, nil
		}),
	}
}

func validateListInput(in listInput) error {
	return validatePagination(in.MaxResults, maxCalendarRows)
}

func validateListEventsInput(in listEventsInput) error {
	if strings.TrimSpace(in.CalendarID) == "" {
		return invalid("calendar_id is required")
	}

	if in.CalendarID != strings.TrimSpace(in.CalendarID) {
		return invalid("calendar_id must not contain surrounding whitespace")
	}

	if err := validatePagination(in.MaxResults, maxCalendarRows); err != nil {
		return err
	}

	return validateTimeRange(in.TimeMin, in.TimeMax)
}

func validateFreeBusyInput(in freeBusyInput) error {
	if len(in.CalendarIDs) == 0 {
		return invalid("calendar_ids must contain at least one calendar")
	}

	if len(in.CalendarIDs) > 50 {
		return invalid("calendar_ids supports at most 50 calendars")
	}

	seen := make(map[string]struct{}, len(in.CalendarIDs))
	for _, calendarID := range in.CalendarIDs {
		if strings.TrimSpace(calendarID) == "" {
			return invalid("calendar_ids must not contain an empty calendar ID")
		}

		if calendarID != strings.TrimSpace(calendarID) {
			return invalid("calendar_ids must not contain surrounding whitespace")
		}

		if _, exists := seen[calendarID]; exists {
			return invalid("calendar_ids must not contain duplicates")
		}
		seen[calendarID] = struct{}{}
	}

	return validateTimeRange(in.TimeMin, in.TimeMax)
}

func validatePagination(value, maxValue int64) error {
	if value < 0 || value > maxValue {
		return invalid("page size must be between 1 and %d when supplied", maxValue)
	}

	return nil
}

func validateTimeRange(minValue, maxValue string) error {
	start, err := parseRFC3339(minValue)
	if err != nil {
		return invalid("time_min must be an RFC3339 timestamp")
	}

	end, err := parseRFC3339(maxValue)
	if err != nil {
		return invalid("time_max must be an RFC3339 timestamp")
	}

	if !end.After(start) {
		return invalid("time_max must be later than time_min")
	}

	if end.Sub(start) > maxFreeBusyDays*24*time.Hour {
		return invalid("the requested interval must be at most 366 days")
	}

	return nil
}

func parseRFC3339(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, errEmptyTimestamp
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse RFC3339 timestamp: %w", err)
	}

	return parsed, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w", mcpcontract.Invalid(fmt.Sprintf(format, args...)))
}

func projectEvent(item *calendarapi.Event) Event {
	out := Event{
		ID: item.Id, Summary: item.Summary, Description: item.Description, Location: item.Location,
		Status: item.Status, EventType: item.EventType, Transparency: item.Transparency,
		Recurrence: append([]string(nil), item.Recurrence...), RecurringEventID: item.RecurringEventId,
		Created: item.Created, Updated: item.Updated, HtmlLink: item.HtmlLink,
	}
	if item.Start != nil {
		out.Start = projectEventTime(item.Start)
		out.AllDay = item.Start.Date != "" && item.Start.DateTime == ""
	}

	if item.End != nil {
		out.End = projectEventTime(item.End)
	}

	if item.OriginalStartTime != nil {
		original := projectEventTime(item.OriginalStartTime)
		out.OriginalStart = &original
	}

	return out
}

func projectEventTime(value *calendarapi.EventDateTime) EventTime {
	return EventTime{Date: value.Date, DateTime: value.DateTime, TimeZone: value.TimeZone}
}

func calendarFailureCategory(upstreamErrors []*calendarapi.Error) string {
	reasons := make(map[string]struct{}, len(upstreamErrors))
	for _, upstream := range upstreamErrors {
		if upstream != nil {
			reasons[upstream.Reason] = struct{}{}
		}
	}

	switch {
	case hasReason(reasons, "notFound"):
		return string(mcpcontract.NotFound)
	case hasReason(reasons, "insufficientPermissions", "forbidden"):
		return string(mcpcontract.Forbidden)
	case hasReason(reasons, "authError"):
		return string(mcpcontract.AuthRequired)
	case hasReason(reasons, "quotaExceeded", "rateLimitExceeded"):
		return string(mcpcontract.QuotaExhausted)
	default:
		return string(mcpcontract.UpstreamFailure)
	}
}

func calendarFailureMessage(upstreamErrors []*calendarapi.Error) string {
	reasons := make([]string, 0, len(upstreamErrors))
	for _, upstream := range upstreamErrors {
		if upstream != nil && upstream.Reason != "" {
			reasons = append(reasons, upstream.Reason)
		}
	}

	sort.Strings(reasons)

	if len(reasons) == 0 {
		return "Google could not compute free/busy data for this calendar"
	}

	return "Google could not compute free/busy data: " + strings.Join(reasons, ", ")
}

func hasReason(reasons map[string]struct{}, values ...string) bool {
	for _, value := range values {
		if _, exists := reasons[value]; exists {
			return true
		}
	}

	return false
}
