package googleapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type readOnlyTestTransport struct{ calls int }

func (t *readOnlyTestTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
}

func TestReadOnlyTransportBlocksMutationsAndPermitsReads(t *testing.T) {
	base := &readOnlyTestTransport{}
	transport := readOnlyTransportFromContext(WithReadOnly(context.Background(), true), base)

	for _, tc := range []struct {
		method string
		url    string
		allow  bool
	}{
		{http.MethodGet, "https://www.googleapis.com/drive/v3/files", true},
		{http.MethodHead, "https://www.googleapis.com/drive/v3/files", true},
		{http.MethodPost, "https://www.googleapis.com/calendar/v3/freeBusy", true},
		{http.MethodPost, "https://www.googleapis.com/gmail/v1/users/me/messages/send", false},
		{http.MethodDelete, "https://www.googleapis.com/drive/v3/files/id", false},
	} {
		req, err := http.NewRequest(tc.method, tc.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = transport.RoundTrip(req)
		if tc.allow && err != nil {
			t.Errorf("%s %s: %v", tc.method, tc.url, err)
		}
		if !tc.allow && !errors.Is(err, ErrReadOnly) {
			t.Errorf("%s %s error = %v, want ErrReadOnly", tc.method, tc.url, err)
		}
	}
	if base.calls != 3 {
		t.Fatalf("base calls = %d, want 3", base.calls)
	}
}

func TestReadOnlyPOSTRegistryRejectsNearMatchesAndOverrides(t *testing.T) {
	for _, raw := range []string{
		"https://www.googleapis.com/calendar/v3/freeBusy",
		"https://searchconsole.googleapis.com/webmasters/v3/sites/example/searchAnalytics/query",
		"https://sheets.googleapis.com/v4/spreadsheets/id/values:batchGetByDataFilter",
	} {
		req, err := http.NewRequest(http.MethodPost, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !ReadOnlyRequestAllowed(req) {
			t.Errorf("ReadOnlyRequestAllowed(%q) = false, want true", raw)
		}
	}
	for _, raw := range []string{
		"http://www.googleapis.com/calendar/v3/freeBusy",
		"https://example.test/calendar/v3/freeBusy",
		"https://www.googleapis.com/v2/activity:query",
		"https://sheets.googleapis.com/v4/spreadsheets/id:batchUpdate",
	} {
		req, err := http.NewRequest(http.MethodPost, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if ReadOnlyRequestAllowed(req) {
			t.Errorf("ReadOnlyRequestAllowed(%q) = true, want false", raw)
		}
	}
	req, err := http.NewRequest(http.MethodPost, "https://www.googleapis.com/calendar/v3/freeBusy", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-HTTP-Method-Override", http.MethodDelete)
	if ReadOnlyRequestAllowed(req) {
		t.Fatal("request with method override unexpectedly allowed")
	}
}
