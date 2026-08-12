package googleapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
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
		req, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
		if err != nil {
			t.Fatal(err)
		}

		response, err := transport.RoundTrip(req)
		if response != nil {
			defer response.Body.Close()
		}

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

func TestReadOnlyTransportBlocksBigQueryMutations(t *testing.T) {
	base := &readOnlyTestTransport{}
	transport := readOnlyTransportFromContext(WithReadOnly(context.Background(), true), base)

	for _, query := range []string{
		"DELETE FROM dataset.table WHERE id = 1",
		"UPDATE dataset.table SET enabled = FALSE",
		"CREATE TABLE dataset.created AS SELECT 1",
	} {
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"https://bigquery.googleapis.com/bigquery/v2/projects/project/queries",
			bytes.NewBufferString(`{"query":`+strconv.Quote(query)+`}`),
		)
		if err != nil {
			t.Fatal(err)
		}

		response, err := transport.RoundTrip(req)
		if response != nil {
			defer response.Body.Close()
		}

		if !errors.Is(err, ErrReadOnly) {
			t.Errorf("query %q error = %v, want ErrReadOnly", query, err)
		}
	}

	if base.calls != 0 {
		t.Fatalf("base calls = %d, want 0", base.calls)
	}
}

func TestReadOnlyPOSTRegistryRejectsNearMatchesAndOverrides(t *testing.T) {
	for _, raw := range []string{
		"https://www.googleapis.com/calendar/v3/freeBusy",
		"https://searchconsole.googleapis.com/webmasters/v3/sites/example/searchAnalytics/query",
		"https://searchconsole.googleapis.com/v1/urlTestingTools/mobileFriendlyTest:run",
		"https://sheets.googleapis.com/v4/spreadsheets/id/values:batchGetByDataFilter",
		"https://analyticsdata.googleapis.com/v1beta/properties/123:batchRunPivotReports",
		"https://analyticsdata.googleapis.com/v1beta/properties/123:checkCompatibility",
		"https://analyticsdata.googleapis.com/v1alpha/properties/123/audiences/456:query",
		"https://mybusinessbusinessinformation.googleapis.com/v1/locations:search",
	} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, raw, nil)
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
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, raw, nil)
		if err != nil {
			t.Fatal(err)
		}

		if ReadOnlyRequestAllowed(req) {
			t.Errorf("ReadOnlyRequestAllowed(%q) = true, want false", raw)
		}
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://www.googleapis.com/calendar/v3/freeBusy", nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("X-HTTP-Method-Override", http.MethodDelete)

	if ReadOnlyRequestAllowed(req) {
		t.Fatal("request with method override unexpectedly allowed")
	}
}
