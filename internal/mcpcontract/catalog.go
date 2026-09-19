package mcpcontract

// Definition is the immutable public-name to policy/scopes/retry mapping.
type Definition struct {
	Name        string
	Description string
	Actions     []string
	Scopes      []string
	Retry       RetryClass
	Local       bool
	// AnyAction means discovery requires any listed action; calls still authorize all selected actions.
	AnyAction bool
}

const (
	GmailReadScope         = "https://www.googleapis.com/auth/gmail.readonly"
	DriveReadScope         = "https://www.googleapis.com/auth/drive.readonly"
	DocsReadScope          = "https://www.googleapis.com/auth/documents.readonly"
	CalendarReadScope      = "https://www.googleapis.com/auth/calendar.readonly"
	AnalyticsReadScope     = "https://www.googleapis.com/auth/analytics.readonly"
	SearchConsoleReadScope = "https://www.googleapis.com/auth/webmasters.readonly"
	SheetsReadScope        = "https://www.googleapis.com/auth/spreadsheets.readonly"
)

var catalog = []Definition{
	{Name: "accounts_list", Description: "List Google connections granted to this caller and their capabilities.", Local: true},
	{Name: "gmail_search", Description: "Search messages in the selected mailbox, returning bounded metadata and a page token.", Actions: []string{"gmail:messages.search"}, Scopes: []string{GmailReadScope}, Retry: SafeRead},
	{Name: "gmail_get_message", Description: "Read one message with bounded MIME text in the selected mailbox.", Actions: []string{"gmail:get"}, Scopes: []string{GmailReadScope}, Retry: SafeRead},
	{Name: "gmail_get_thread", Description: "Read an ordered email thread with bounded message bodies.", Actions: []string{"gmail:thread.get"}, Scopes: []string{GmailReadScope}, Retry: SafeRead},
	{Name: "drive_search", Description: "Search Drive file metadata, including shared drives, with pagination.", Actions: []string{"drive:search"}, Scopes: []string{DriveReadScope}, Retry: SafeRead},
	{Name: "drive_get_file", Description: "Read Drive file metadata without downloading binary content.", Actions: []string{"drive:get"}, Scopes: []string{DriveReadScope}, Retry: SafeRead},
	{Name: "docs_get_text", Description: "Extract bounded document text, preserving sources and truncation.", Actions: []string{"docs:cat"}, Scopes: []string{DocsReadScope}, Retry: SafeRead},
	{Name: "calendar_list", Description: "List calendars with timezone and access metadata.", Actions: []string{"calendar:calendars.list"}, Scopes: []string{CalendarReadScope}, Retry: SafeRead},
	{Name: "calendar_list_events", Description: "Read calendar events in a bounded interval, preserving all-day and timezone values.", Actions: []string{"calendar:events"}, Scopes: []string{CalendarReadScope}, Retry: SafeRead},
	{Name: "calendar_freebusy", Description: "Read busy intervals and per-calendar failures; failed calendars are not free.", Actions: []string{"calendar:freebusy"}, Scopes: []string{CalendarReadScope}, Retry: SafeRead},
	{Name: "analytics_list_properties", Description: "List accessible GA4 properties with pagination.", Actions: []string{"analytics:properties"}, Scopes: []string{AnalyticsReadScope}, Retry: SafeRead},
	{Name: "analytics_metadata", AnyAction: true, Description: "Read dimensions, metrics, or both; each projection requires its own policy grant.", Actions: []string{"analytics:dimensions", "analytics:metrics"}, Scopes: []string{AnalyticsReadScope}, Retry: SafeRead},
	{Name: "analytics_report", Description: "Read a bounded GA4 report with property timezone, totals, and pagination.", Actions: []string{"analytics:report"}, Scopes: []string{AnalyticsReadScope}, Retry: SafeRead},
	{Name: "searchconsole_list_sites", Description: "List Search Console domain and URL-prefix properties.", Actions: []string{"searchconsole:sites.list"}, Scopes: []string{SearchConsoleReadScope}, Retry: SafeRead},
	{Name: "searchconsole_query", Description: "Query bounded Search Console rows using inclusive dates in Pacific time.", Actions: []string{"searchconsole:query"}, Scopes: []string{SearchConsoleReadScope}, Retry: SafeRead},
	{Name: "sheets_get_metadata", Description: "Read spreadsheet titles, sheet dimensions, locale, and timezone.", Actions: []string{"sheets:metadata"}, Scopes: []string{SheetsReadScope}, Retry: SafeRead},
	{Name: "sheets_read_range", Description: "Read a bounded A1 range, preserving sparse rows and explicit render options.", Actions: []string{"sheets:get"}, Scopes: []string{SheetsReadScope}, Retry: SafeRead},
}

func Catalog() []Definition {
	out := make([]Definition, len(catalog))
	for i, d := range catalog {
		d.Actions = append([]string(nil), d.Actions...)
		d.Scopes = append([]string(nil), d.Scopes...)
		out[i] = d
	}

	return out
}

func Lookup(name string) (Definition, bool) {
	for _, d := range Catalog() {
		if d.Name == name {
			return d, true
		}
	}

	return Definition{}, false
}
