package googleauth

import (
	"strings"
	"testing"
)

func TestParseService(t *testing.T) {
	tests := []struct {
		in   string
		want Service
	}{
		{"gmail", ServiceGmail},
		{"GMAIL", ServiceGmail},
		{"calendar", ServiceCalendar},
		{"chat", ServiceChat},
		{"classroom", ServiceClassroom},
		{"drive", ServiceDrive},
		{"docs", ServiceDocs},
		{"contacts", ServiceContacts},
		{"tasks", ServiceTasks},
		{"people", ServicePeople},
		{"sheets", ServiceSheets},
		{"groups", ServiceGroups},
		{"keep", ServiceKeep},
	}
	for _, tt := range tests {
		got, err := ParseService(tt.in)
		if err != nil {
			t.Fatalf("ParseService(%q) err: %v", tt.in, err)
		}

		if got != tt.want {
			t.Fatalf("ParseService(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseService_Invalid(t *testing.T) {
	if _, err := ParseService("nope"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestExtractCodeAndState(t *testing.T) {
	code, state, err := extractCodeAndState("http://localhost:1/?code=abc&state=xyz")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if code != "abc" || state != "xyz" {
		t.Fatalf("unexpected: code=%q state=%q", code, state)
	}
}

func TestExtractCodeAndState_Errors(t *testing.T) {
	if _, _, err := extractCodeAndState("not a url"); err == nil {
		t.Fatalf("expected error")
	}

	if _, _, err := extractCodeAndState("http://localhost:1/?state=xyz"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAllServices(t *testing.T) {
	svcs := AllServices()
	if len(svcs) != 18 {
		t.Fatalf("unexpected: %v", svcs)
	}
	seen := make(map[Service]bool)

	for _, s := range svcs {
		seen[s] = true
	}

	for _, want := range []Service{ServiceGmail, ServiceCalendar, ServiceChat, ServiceClassroom, ServiceDrive, ServiceDocs, ServiceContacts, ServiceTasks, ServicePeople, ServiceSheets, ServiceGroups, ServiceKeep, ServiceYoutube, ServiceBigquery, ServiceAnalytics, ServiceSearchConsole, ServiceTagManager, ServiceBusinessProfile} {
		if !seen[want] {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestUserServices(t *testing.T) {
	svcs := UserServices()
	if len(svcs) != 16 {
		t.Fatalf("unexpected: %v", svcs)
	}

	seenDocs := false

	for _, s := range svcs {
		switch s {
		case ServiceDocs:
			seenDocs = true
		case ServiceKeep:
			t.Fatalf("unexpected keep in user services")
		}
	}

	if !seenDocs {
		t.Fatalf("missing docs in user services")
	}
}

func TestUserServiceCSV(t *testing.T) {
	want := "gmail,calendar,chat,classroom,drive,docs,contacts,tasks,sheets,people,youtube,bigquery,analytics,searchconsole,tagmanager,businessprofile"
	if got := UserServiceCSV(); got != want {
		t.Fatalf("unexpected user services csv: %q", got)
	}
}

func TestServiceOrderCoverage(t *testing.T) {
	seen := make(map[Service]bool)
	for _, svc := range serviceOrder {
		seen[svc] = true

		if _, ok := serviceInfoByService[svc]; !ok {
			t.Fatalf("missing info for %q", svc)
		}
	}

	for svc := range serviceInfoByService {
		if !seen[svc] {
			t.Fatalf("service %q missing from order", svc)
		}
	}
}

func TestServicesInfo_Metadata(t *testing.T) {
	infos := ServicesInfo()
	if len(infos) != len(serviceOrder) {
		t.Fatalf("unexpected services info length: %d", len(infos))
	}

	docsInfo, foundDocs := findServiceInfo(infos, ServiceDocs)

	if !foundDocs {
		t.Fatalf("missing docs info")
	}

	if len(docsInfo.APIs) == 0 {
		t.Fatalf("docs APIs missing")
	}

	for _, want := range []string{
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/documents",
	} {
		if !containsScope(docsInfo.Scopes, want) {
			t.Fatalf("docs missing scope %q", want)
		}
	}

	if markdown := ServicesMarkdown(infos); markdown == "" {
		t.Fatalf("expected markdown output")
	}
}

func findServiceInfo(infos []ServiceInfo, svc Service) (ServiceInfo, bool) {
	for _, info := range infos {
		if info.Service == svc {
			return info, true
		}
	}

	return ServiceInfo{}, false
}

func containsScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}

	return false
}

func TestScopesForServices_UnionSorted(t *testing.T) {
	scopes, err := ScopesForServices([]Service{ServiceContacts, ServiceGmail, ServiceTasks, ServicePeople, ServiceContacts})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(scopes) < 3 {
		t.Fatalf("unexpected scopes: %v", scopes)
	}
	// Ensure stable sorting.
	for i := 1; i < len(scopes); i++ {
		if scopes[i-1] > scopes[i] {
			t.Fatalf("not sorted: %v", scopes)
		}
	}
	// Ensure expected scopes are included.
	want := []string{
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.settings.basic",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
		"https://www.googleapis.com/auth/contacts",
		"https://www.googleapis.com/auth/contacts.other.readonly",
		"https://www.googleapis.com/auth/directory.readonly",
		"https://www.googleapis.com/auth/tasks",
		"profile",
	}
	for _, w := range want {
		found := false

		for _, s := range scopes {
			if s == w {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("missing scope %q in %v", w, scopes)
		}
	}
}

func TestScopesForManageWithOptions_Readonly(t *testing.T) {
	scopes, err := ScopesForManageWithOptions([]Service{ServiceGmail, ServiceDrive, ServiceCalendar, ServiceContacts, ServiceTasks, ServiceSheets, ServiceDocs, ServicePeople}, ScopeOptions{
		Readonly:   true,
		DriveScope: DriveScopeFull,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	want := []string{
		scopeOpenID,
		scopeEmail,
		scopeUserinfoEmail,
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/calendar.readonly",
		"https://www.googleapis.com/auth/contacts.readonly",
		"https://www.googleapis.com/auth/tasks.readonly",
		"https://www.googleapis.com/auth/spreadsheets.readonly",
		"https://www.googleapis.com/auth/documents.readonly",
		"profile",
	}
	for _, w := range want {
		if !containsScope(scopes, w) {
			t.Fatalf("missing %q in %v", w, scopes)
		}
	}

	notWant := []string{
		"https://mail.google.com/",
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.settings.basic",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/calendar",
		"https://www.googleapis.com/auth/contacts",
		"https://www.googleapis.com/auth/tasks",
		"https://www.googleapis.com/auth/spreadsheets",
		"https://www.googleapis.com/auth/documents",
	}
	for _, nw := range notWant {
		if containsScope(scopes, nw) {
			t.Fatalf("unexpected %q in %v", nw, scopes)
		}
	}
}

func TestScopesForManageWithOptions_GmailScopeReadonly(t *testing.T) {
	scopes, err := ScopesForManageWithOptions([]Service{ServiceGmail, ServiceDrive}, ScopeOptions{
		GmailScope: GmailScopeReadonly,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !containsScope(scopes, "https://www.googleapis.com/auth/gmail.readonly") {
		t.Fatalf("missing gmail.readonly in %v", scopes)
	}

	for _, nw := range []string{
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.settings.basic",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
	} {
		if containsScope(scopes, nw) {
			t.Fatalf("unexpected %q in %v", nw, scopes)
		}
	}

	if !containsScope(scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("missing drive in %v", scopes)
	}
}

func TestScopes_ServiceKeep_DefaultIsReadonly(t *testing.T) {
	scopes, err := Scopes(ServiceKeep)
	if err != nil {
		t.Fatalf("Scopes: %v", err)
	}

	if len(scopes) != 1 || scopes[0] != "https://www.googleapis.com/auth/keep.readonly" {
		t.Fatalf("unexpected keep scopes: %#v", scopes)
	}
}

func TestScopesForServiceWithOptions_ServiceKeep_Readonly(t *testing.T) {
	scopes, err := scopesForServiceWithOptions(ServiceKeep, ScopeOptions{Readonly: true})
	if err != nil {
		t.Fatalf("scopesForServiceWithOptions: %v", err)
	}

	if len(scopes) != 1 || scopes[0] != "https://www.googleapis.com/auth/keep.readonly" {
		t.Fatalf("unexpected keep readonly scopes: %#v", scopes)
	}
}

func TestScopesForServiceWithOptions_TagManagerManageUsers(t *testing.T) {
	writeScopes, err := scopesForServiceWithOptions(ServiceTagManager, ScopeOptions{})
	if err != nil {
		t.Fatalf("write scopes: %v", err)
	}

	if !containsScope(writeScopes, "https://www.googleapis.com/auth/tagmanager.manage.users") {
		t.Fatalf("missing tagmanager.manage.users in %v", writeScopes)
	}

	readonlyScopes, err := scopesForServiceWithOptions(ServiceTagManager, ScopeOptions{Readonly: true})
	if err != nil {
		t.Fatalf("readonly scopes: %v", err)
	}

	if len(readonlyScopes) != 1 || readonlyScopes[0] != "https://www.googleapis.com/auth/tagmanager.readonly" {
		t.Fatalf("unexpected readonly scopes: %v", readonlyScopes)
	}
}

func TestScopesForServiceWithOptions_AnalyticsManageUsers(t *testing.T) {
	writeScopes, err := scopesForServiceWithOptions(ServiceAnalytics, ScopeOptions{})
	if err != nil {
		t.Fatalf("write scopes: %v", err)
	}

	if !containsScope(writeScopes, "https://www.googleapis.com/auth/analytics.manage.users") {
		t.Fatalf("missing analytics.manage.users in %v", writeScopes)
	}

	readonlyScopes, err := scopesForServiceWithOptions(ServiceAnalytics, ScopeOptions{Readonly: true})
	if err != nil {
		t.Fatalf("readonly scopes: %v", err)
	}

	if !containsScope(readonlyScopes, "https://www.googleapis.com/auth/analytics.readonly") ||
		!containsScope(readonlyScopes, "https://www.googleapis.com/auth/analytics.manage.users.readonly") {
		t.Fatalf("unexpected readonly scopes: %v", readonlyScopes)
	}
}

func TestScopesForManageWithOptions_DriveScopeFile(t *testing.T) {
	scopes, err := ScopesForManageWithOptions([]Service{ServiceDrive, ServiceDocs}, ScopeOptions{
		DriveScope: DriveScopeFile,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !containsScope(scopes, "https://www.googleapis.com/auth/drive.file") {
		t.Fatalf("missing drive.file in %v", scopes)
	}

	if containsScope(scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("unexpected drive in %v", scopes)
	}

	if !containsScope(scopes, "https://www.googleapis.com/auth/documents") {
		t.Fatalf("missing documents scope in %v", scopes)
	}
}

func TestScopesForManageWithOptions_InvalidDriveScope(t *testing.T) {
	if _, err := ScopesForManageWithOptions([]Service{ServiceDrive}, ScopeOptions{DriveScope: DriveScopeMode("nope")}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestScopesForManageWithOptions_SheetsReadonlyIncludesDriveReadonly(t *testing.T) {
	scopes, err := ScopesForManageWithOptions([]Service{ServiceSheets}, ScopeOptions{Readonly: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !containsScope(scopes, "https://www.googleapis.com/auth/spreadsheets.readonly") {
		t.Fatalf("missing spreadsheets.readonly in %v", scopes)
	}

	if !containsScope(scopes, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("missing drive.readonly in %v", scopes)
	}
}

func TestScopesForManageWithOptions_SheetsHonorsDriveScopeMode(t *testing.T) {
	tests := []struct {
		name      string
		opts      ScopeOptions
		wantDrive string
		wantSheet string
	}{
		{
			name:      "default",
			opts:      ScopeOptions{},
			wantDrive: "https://www.googleapis.com/auth/drive",
			wantSheet: "https://www.googleapis.com/auth/spreadsheets",
		},
		{
			name:      "drive_readonly",
			opts:      ScopeOptions{DriveScope: DriveScopeReadonly},
			wantDrive: "https://www.googleapis.com/auth/drive.readonly",
			wantSheet: "https://www.googleapis.com/auth/spreadsheets",
		},
		{
			name:      "drive_file",
			opts:      ScopeOptions{DriveScope: DriveScopeFile},
			wantDrive: "https://www.googleapis.com/auth/drive.file",
			wantSheet: "https://www.googleapis.com/auth/spreadsheets",
		},
		{
			name:      "readonly",
			opts:      ScopeOptions{Readonly: true, DriveScope: DriveScopeFull},
			wantDrive: "https://www.googleapis.com/auth/drive.readonly",
			wantSheet: "https://www.googleapis.com/auth/spreadsheets.readonly",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scopes, err := ScopesForManageWithOptions([]Service{ServiceSheets}, tc.opts)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			if !containsScope(scopes, tc.wantDrive) {
				t.Fatalf("missing %q in %v", tc.wantDrive, scopes)
			}

			if !containsScope(scopes, tc.wantSheet) {
				t.Fatalf("missing %q in %v", tc.wantSheet, scopes)
			}
		})
	}
}

func TestScopes_DocsIncludesDriveAndDocsScopes(t *testing.T) {
	scopes, err := Scopes(ServiceDocs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for _, want := range []string{
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/documents",
	} {
		found := false

		for _, scope := range scopes {
			if scope == want {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("missing %q in %v", want, scopes)
		}
	}
}

func TestScopes_GmailIncludesSettingsSharing(t *testing.T) {
	scopes, err := Scopes(ServiceGmail)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for _, want := range []string{
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.settings.basic",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
	} {
		if !containsScope(scopes, want) {
			t.Fatalf("missing %q in %v", want, scopes)
		}
	}
}

func TestScopes_UnknownService(t *testing.T) {
	if _, err := Scopes(Service("nope")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServiceUsageIDs(t *testing.T) {
	tests := []struct {
		svc  Service
		want []string
	}{
		{ServiceGmail, []string{"gmail.googleapis.com"}},
		{ServiceCalendar, []string{"calendar-json.googleapis.com"}},
		{ServiceDocs, []string{"docs.googleapis.com", "drive.googleapis.com"}},
		{ServiceSheets, []string{"sheets.googleapis.com", "drive.googleapis.com"}},
		{ServiceAnalytics, []string{"analyticsadmin.googleapis.com", "analyticsdata.googleapis.com"}},
		{ServiceBusinessProfile, []string{"mybusinessaccountmanagement.googleapis.com", "mybusinessbusinessinformation.googleapis.com"}},
	}

	for _, tt := range tests {
		got, err := ServiceUsageIDs(tt.svc)
		if err != nil {
			t.Fatalf("ServiceUsageIDs(%q): %v", tt.svc, err)
		}

		set := make(map[string]bool, len(got))
		for _, id := range got {
			set[id] = true
		}

		if len(got) != len(tt.want) {
			t.Fatalf("ServiceUsageIDs(%q)=%v want %v", tt.svc, got, tt.want)
		}

		for _, id := range tt.want {
			if !set[id] {
				t.Fatalf("ServiceUsageIDs(%q) missing %q in %v", tt.svc, id, got)
			}
		}
	}
}

func TestServiceUsageIDsForServices_DedupAndSort(t *testing.T) {
	got, err := ServiceUsageIDsForServices([]Service{ServiceDocs, ServiceSheets, ServiceDrive})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	want := []string{"docs.googleapis.com", "drive.googleapis.com", "sheets.googleapis.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestServiceUsageIDsForServices_EmptyUsesUserServices(t *testing.T) {
	got, err := ServiceUsageIDsForServices(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(got) == 0 {
		t.Fatalf("expected non-empty usage ids for user services")
	}

	for _, svc := range UserServices() {
		ids, usageErr := ServiceUsageIDs(svc)
		if usageErr != nil {
			t.Fatalf("ServiceUsageIDs(%q): %v", svc, usageErr)
		}

		if len(ids) == 0 {
			t.Fatalf("service %q has no usage ids", svc)
		}
	}

	ids, keepErr := ServiceUsageIDs(ServiceKeep)
	if keepErr != nil || len(ids) == 0 {
		t.Fatalf("keep usage ids: %v %v", ids, keepErr)
	}
}

func TestServiceUsageIDs_AllServicesPresent(t *testing.T) {
	for _, svc := range AllServices() {
		ids, err := ServiceUsageIDs(svc)
		if err != nil {
			t.Fatalf("%q: %v", svc, err)
		}

		if len(ids) == 0 {
			t.Fatalf("%q: empty service usage ids", svc)
		}

		for _, id := range ids {
			if id == "" || !strings.Contains(id, ".googleapis.com") {
				t.Fatalf("%q: invalid id %q", svc, id)
			}
		}
	}
}

func TestServicesInfo_IncludesServiceUsageIDs(t *testing.T) {
	infos := ServicesInfo()
	if len(infos) == 0 {
		t.Fatalf("empty infos")
	}

	for _, info := range infos {
		if len(info.ServiceUsageIDs) == 0 {
			t.Fatalf("%s missing ServiceUsageIDs", info.Service)
		}
	}
}
