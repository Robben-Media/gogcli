package businessprofile

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestBusinessProfileFetchDailyMetricsSendsOneRangedReadAndProjectsDays(t *testing.T) {
	t.Parallel()
	operation, provider, recorder := fixture(t, "businessprofile_fetch_daily_metrics", http.StatusOK, `{
		"multiDailyMetricTimeSeries": [
			{"dailyMetricTimeSeries": [
				{"dailyMetric": "CALL_CLICKS", "timeSeries": {"datedValues": [
					{"date": {"year": 2026, "month": 8, "day": 31}, "value": "3"},
					{"date": {"year": 2026, "month": 9, "day": 1}},
					{"date": {"year": 2026, "month": 9, "day": 2}, "value": "12"},
					null
				]}},
				{"dailyMetric": "BUSINESS_DIRECTION_REQUESTS", "timeSeries": {"datedValues": [
					{"date": {"year": 2026, "month": 8, "day": 31}, "value": "1"},
					{"date": {"year": 2026, "month": 9, "day": 1}, "value": "2"},
					{"date": {"year": 2026, "month": 9, "day": 2}}
				]}},
				null
			]},
			{"dailyMetricTimeSeries": [
				{"dailyMetric": "WEBSITE_CLICKS", "timeSeries": {"datedValues": [
					{"date": {"year": 2026, "month": 9, "day": 2}, "value": "40"}
				]}}
			]}
		]
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"a","location":"locations/3912355503421668705","start_date":"2026-08-31","end_date":"2026-09-02"}`, testIdentity("a"))
	if err != nil {
		t.Fatal(err)
	}

	if recorder.calls != 1 || recorder.path != "/v1/locations/3912355503421668705:fetchMultiDailyMetricsTimeSeries" {
		t.Fatalf("calls=%d path=%q", recorder.calls, recorder.path)
	}

	if got := recorder.query["dailyMetrics"]; !slices.Equal(got, []string{"CALL_CLICKS", "BUSINESS_DIRECTION_REQUESTS", "WEBSITE_CLICKS"}) {
		t.Fatalf("dailyMetrics=%v", got)
	}

	for key, want := range map[string]string{
		"dailyRange.startDate.year": "2026", "dailyRange.startDate.month": "8", "dailyRange.startDate.day": "31",
		"dailyRange.endDate.year": "2026", "dailyRange.endDate.month": "9", "dailyRange.endDate.day": "2",
	} {
		if got := recorder.query[key]; !slices.Equal(got, []string{want}) {
			t.Fatalf("%s=%v want %s (query %v)", key, got, want, recorder.query)
		}
	}

	want := mcpcontract.CallOptions{Operation: "businessprofile_fetch_daily_metrics", Retry: mcpcontract.SafeRead}
	if provider.options[0] != want {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}

	got, err := json.Marshal(result.(mcpcontract.Result[DailyMetricsData]).Data)
	if err != nil {
		t.Fatal(err)
	}

	const wantData = `{"location":"locations/3912355503421668705","start_date":"2026-08-31","end_date":"2026-09-02","metrics":[` +
		`{"metric":"CALL_CLICKS","total":15,"monthly":[{"month":"2026-08","value":3},{"month":"2026-09","value":12}],"daily":[{"date":"2026-08-31","value":3},{"date":"2026-09-01","value":0},{"date":"2026-09-02","value":12}]},` +
		`{"metric":"BUSINESS_DIRECTION_REQUESTS","total":3,"monthly":[{"month":"2026-08","value":1},{"month":"2026-09","value":2}],"daily":[{"date":"2026-08-31","value":1},{"date":"2026-09-01","value":2},{"date":"2026-09-02","value":0}]},` +
		`{"metric":"WEBSITE_CLICKS","total":40,"monthly":[{"month":"2026-09","value":40}],"daily":[{"date":"2026-09-02","value":40}]}]}`
	if string(got) != wantData {
		t.Fatalf("data drifted:\n got %s\nwant %s", got, wantData)
	}
}

func TestBusinessProfileFetchDailyMetricsSendsChosenMetricsAndKeepsEmptySeries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		response string
		want     string
	}{
		{
			name: "series_without_values",
			response: `{"multiDailyMetricTimeSeries": [{"dailyMetricTimeSeries": [
				{"dailyMetric": "BUSINESS_IMPRESSIONS_MOBILE_MAPS", "timeSeries": {}},
				{"dailyMetric": "CALL_CLICKS"}
			]}]}`,
			want: `{"location":"locations/55","start_date":"2025-04-01","end_date":"2026-09-30","metrics":[` +
				`{"metric":"BUSINESS_IMPRESSIONS_MOBILE_MAPS","total":0,"monthly":[],"daily":[]},` +
				`{"metric":"CALL_CLICKS","total":0,"monthly":[],"daily":[]}]}`,
		},
		{
			name:     "no_series",
			response: `{}`,
			want:     `{"location":"locations/55","start_date":"2025-04-01","end_date":"2026-09-30","metrics":[]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			operation, _, recorder := fixture(t, "businessprofile_fetch_daily_metrics", http.StatusOK, tc.response)

			result, err := decodeRun(t, operation, `{"account_id":"a","location":"locations/55","start_date":"2025-04-01","end_date":"2026-09-30","metrics":["BUSINESS_IMPRESSIONS_MOBILE_MAPS","CALL_CLICKS"]}`, testIdentity("a"))
			if err != nil {
				t.Fatal(err)
			}

			if got := recorder.query["dailyMetrics"]; recorder.calls != 1 || !slices.Equal(got, []string{"BUSINESS_IMPRESSIONS_MOBILE_MAPS", "CALL_CLICKS"}) {
				t.Fatalf("calls=%d dailyMetrics=%v", recorder.calls, got)
			}

			got, err := json.Marshal(result.(mcpcontract.Result[DailyMetricsData]).Data)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestBusinessProfileFetchDailyMetricsRejectsInvalidInputBeforeUpstream(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"account_id":"a","start_date":"2026-09-01","end_date":"2026-09-30"}`,
		`{"account_id":"a","location":"accounts/1/locations/55","start_date":"2026-09-01","end_date":"2026-09-30"}`,
		`{"account_id":"a","location":"locations/55:getDailyMetricsTimeSeries","start_date":"2026-09-01","end_date":"2026-09-30"}`,
		`{"account_id":"a","location":"locations/55/x","start_date":"2026-09-01","end_date":"2026-09-30"}`,
		`{"account_id":"a","location":"locations/55","end_date":"2026-09-30"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2026-9-1","end_date":"2026-09-30"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2026-02-30","end_date":"2026-03-30"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2026-09-30","end_date":"2026-09-01"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2025-04-01","end_date":"2026-10-01"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2025-08-31","end_date":"2027-02-28"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2025-08-31","end_date":"2027-03-02"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2026-09-01","end_date":"2026-09-30","metrics":["BUSINESS_CONVERSATIONS"]}`,
		`{"account_id":"a","location":"locations/55","start_date":"2026-09-01","end_date":"2026-09-30","metrics":["call_clicks"]}`,
		`{"account_id":"a","location":"locations/55","start_date":"2026-09-01","end_date":"2026-09-30","metrics":["CALL_CLICKS","CALL_CLICKS"]}`,
		`{"account_id":"a","location":"locations/55","start_date":"2026-09-01","end_date":"2026-09-30","daily_range":"x"}`,
	} {
		operation, _, recorder := fixture(t, "businessprofile_fetch_daily_metrics", http.StatusOK, `{}`)
		if _, err := decodeRun(t, operation, raw, testIdentity("a")); err == nil {
			t.Fatalf("accepted %s", raw)
		}

		if recorder.calls != 0 {
			t.Fatalf("%s reached upstream", raw)
		}
	}
}

func TestBusinessProfileFetchDailyMetricsAcceptsRangesShorterThanEighteenMonths(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"account_id":"a","location":"locations/55","start_date":"2026-09-01","end_date":"2026-09-01"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2025-04-01","end_date":"2026-09-30"}`,
		`{"account_id":"a","location":"locations/55","start_date":"2025-08-31","end_date":"2027-02-27"}`,
	} {
		operation, _, recorder := fixture(t, "businessprofile_fetch_daily_metrics", http.StatusOK, `{}`)
		if _, err := decodeRun(t, operation, raw, testIdentity("a")); err != nil || recorder.calls != 1 {
			t.Fatalf("%s: calls=%d err=%v", raw, recorder.calls, err)
		}
	}
}

func TestBusinessProfileFetchDailyMetricsIsReadOnlyDefinition(t *testing.T) {
	t.Parallel()

	def, ok := mcpcontract.Lookup("businessprofile_fetch_daily_metrics")
	if !ok || def.Retry != mcpcontract.SafeRead || !slices.Equal(def.Actions, []string{"businessprofile:performance.fetch"}) || !slices.Equal(def.Scopes, []string{mcpcontract.BusinessManageScope}) {
		t.Fatalf("unexpected definition: %#v", def)
	}
}
