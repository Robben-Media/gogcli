package mcpcontract

import "testing"

func TestScopeGranted(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		granted  []string
		required string
		want     bool
	}{
		{"exact", []string{GmailReadScope}, GmailReadScope, true},
		{"modify covers mail read", []string{"https://www.googleapis.com/auth/gmail.modify"}, GmailReadScope, true},
		{"drive read covers docs", []string{DriveReadScope}, DocsReadScope, true},
		{"drive read covers sheets", []string{DriveReadScope}, SheetsReadScope, true},
		{"full calendar", []string{"https://www.googleapis.com/auth/calendar"}, CalendarReadScope, true},
		{"reverse forbidden", []string{DocsReadScope}, DriveReadScope, false},
		{"limited file scope", []string{"https://www.googleapis.com/auth/drive.file"}, DriveReadScope, false},
		{"prefix is not authority", []string{GmailReadScope + ".extra"}, GmailReadScope, false},
		{"missing", nil, GmailReadScope, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ScopeGranted(tc.granted, tc.required); got != tc.want {
				t.Fatalf("ScopeGranted = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAlternativeScopesDoNotRequireAllOrGrantReverse(t *testing.T) {
	alternatives := Definition{AnyScope: true, Scopes: []string{GmailReadScope, "https://www.googleapis.com/auth/gmail.modify"}}
	if !ScopesSatisfied([]string{GmailReadScope}, alternatives) {
		t.Fatal("one supported alternative must suffice")
	}

	if ScopesSatisfied([]string{GmailReadScope}, Definition{AnyScope: true, Scopes: []string{"https://www.googleapis.com/auth/gmail.modify"}}) {
		t.Fatal("read scope authorized write")
	}

	if ScopesSatisfied([]string{GmailReadScope}, Definition{AnyScope: true}) {
		t.Fatal("empty alternatives authorized")
	}

	if ScopesSatisfied([]string{GmailReadScope}, Definition{Scopes: []string{GmailReadScope, CalendarReadScope}}) {
		t.Fatal("workflow skipped required calendar scope")
	}
}
