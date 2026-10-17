package businessprofile

// Daily performance metrics come from the Business Profile Performance API.
// Its discovery document lists no scopes; Google documents business.manage.
// Method and scope reference verified 2026-10-17:
//
//   - locations.fetchMultiDailyMetricsTimeSeries requires
//     https://www.googleapis.com/auth/business.manage:
//     https://developers.google.com/my-business/reference/performance/rest/v1/locations/fetchMultiDailyMetricsTimeSeries
//
// The operation is read-only: one GET for one exact location, a fixed metric
// allow-list and a date range of at most 18 months.

import (
	"context"
	"fmt"
	"slices"
	"time"

	businessprofileperformance "google.golang.org/api/businessprofileperformance/v1"
	"google.golang.org/api/option"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	// metricsDateLayout is the input and output form of a calendar day.
	metricsDateLayout = "2006-01-02"
	// metricsMaxRangeMonths matches the 18 months of history Google retains.
	metricsMaxRangeMonths = 18
)

// defaultDailyMetrics are fetched when the caller names none.
var defaultDailyMetrics = []string{"CALL_CLICKS", "BUSINESS_DIRECTION_REQUESTS", "WEBSITE_CLICKS"}

// allowedDailyMetrics is the fixed metric allow-list.
var allowedDailyMetrics = []string{
	"CALL_CLICKS", "BUSINESS_DIRECTION_REQUESTS", "WEBSITE_CLICKS",
	"BUSINESS_IMPRESSIONS_DESKTOP_MAPS", "BUSINESS_IMPRESSIONS_DESKTOP_SEARCH",
	"BUSINESS_IMPRESSIONS_MOBILE_MAPS", "BUSINESS_IMPRESSIONS_MOBILE_SEARCH",
}

type fetchDailyMetricsInput struct {
	mcpcontract.Selection
	Location  string   `json:"location" jsonschema:"Exact opaque location resource name returned by businessprofile_list_locations, in locations/{id} form"`
	StartDate string   `json:"start_date" jsonschema:"First day of the range, YYYY-MM-DD, inclusive"`
	EndDate   string   `json:"end_date" jsonschema:"Last day of the range, YYYY-MM-DD, inclusive; at most 18 months after start_date"`
	Metrics   []string `json:"metrics,omitempty" jsonschema:"Daily metrics to fetch, each at most once: CALL_CLICKS, BUSINESS_DIRECTION_REQUESTS, WEBSITE_CLICKS, BUSINESS_IMPRESSIONS_DESKTOP_MAPS, BUSINESS_IMPRESSIONS_DESKTOP_SEARCH, BUSINESS_IMPRESSIONS_MOBILE_MAPS, BUSINESS_IMPRESSIONS_MOBILE_SEARCH; default CALL_CLICKS, BUSINESS_DIRECTION_REQUESTS, WEBSITE_CLICKS"`
}

// DailyValue is one day's count. Google omits zero values; they project as 0.
type DailyValue struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

// MonthlyValue is the sum of the returned daily values in one calendar month.
type MonthlyValue struct {
	Month string `json:"month"`
	Value int64  `json:"value"`
}

// MetricSeries is one metric's returned days, with monthly and range totals
// summed from those days.
type MetricSeries struct {
	Metric  string         `json:"metric"`
	Total   int64          `json:"total"`
	Monthly []MonthlyValue `json:"monthly"`
	Daily   []DailyValue   `json:"daily"`
}

type DailyMetricsData struct {
	Location  string         `json:"location"`
	StartDate string         `json:"start_date"`
	EndDate   string         `json:"end_date"`
	Metrics   []MetricSeries `json:"metrics"`
}

func dailyMetricsOperation(provider mcpcontract.ClientProvider) mcpcontract.Operation {
	return mcpcontract.NewOperation("businessprofile_fetch_daily_metrics", validateFetchDailyMetrics, func(ctx context.Context, id mcpcontract.Identity, in fetchDailyMetricsInput) (mcpcontract.Result[DailyMetricsData], error) {
		httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "businessprofile_fetch_daily_metrics", Retry: mcpcontract.SafeRead})
		if err != nil {
			return mcpcontract.Result[DailyMetricsData]{}, nativegoogleapi.NativePublicError(err)
		}

		svc, err := businessprofileperformance.NewService(ctx, option.WithHTTPClient(httpClient))
		if err != nil {
			return mcpcontract.Result[DailyMetricsData]{}, nativegoogleapi.NativePublicError(err)
		}

		metrics := in.Metrics
		if len(metrics) == 0 {
			metrics = defaultDailyMetrics
		}

		// Both dates were validated by validateFetchDailyMetrics.
		start, _ := time.Parse(metricsDateLayout, in.StartDate)
		end, _ := time.Parse(metricsDateLayout, in.EndDate)

		resp, err := svc.Locations.FetchMultiDailyMetricsTimeSeries(in.Location).DailyMetrics(metrics...).
			DailyRangeStartDateYear(int64(start.Year())).DailyRangeStartDateMonth(int64(start.Month())).DailyRangeStartDateDay(int64(start.Day())).
			DailyRangeEndDateYear(int64(end.Year())).DailyRangeEndDateMonth(int64(end.Month())).DailyRangeEndDateDay(int64(end.Day())).
			Context(ctx).Do()
		if err != nil {
			return mcpcontract.Result[DailyMetricsData]{}, nativegoogleapi.NativePublicError(err)
		}

		out := mcpcontract.NewResult(id, DailyMetricsData{Location: in.Location, StartDate: in.StartDate, EndDate: in.EndDate, Metrics: []MetricSeries{}})

		for _, group := range resp.MultiDailyMetricTimeSeries {
			if group == nil {
				continue
			}

			for _, series := range group.DailyMetricTimeSeries {
				if series == nil {
					continue
				}

				out.Data.Metrics = append(out.Data.Metrics, projectMetricSeries(series))
			}
		}

		return out, nil
	})
}

func projectMetricSeries(series *businessprofileperformance.DailyMetricTimeSeries) MetricSeries {
	out := MetricSeries{Metric: series.DailyMetric, Monthly: []MonthlyValue{}, Daily: []DailyValue{}}
	if series.TimeSeries == nil {
		return out
	}

	monthIndex := map[string]int{}

	for _, point := range series.TimeSeries.DatedValues {
		if point == nil || point.Date == nil {
			continue
		}

		date := point.Date
		month := fmt.Sprintf("%04d-%02d", date.Year, date.Month)
		out.Daily = append(out.Daily, DailyValue{Date: fmt.Sprintf("%s-%02d", month, date.Day), Value: point.Value})
		out.Total += point.Value

		index, ok := monthIndex[month]
		if !ok {
			index = len(out.Monthly)
			monthIndex[month] = index
			out.Monthly = append(out.Monthly, MonthlyValue{Month: month})
		}

		out.Monthly[index].Value += point.Value
	}

	return out
}

// addMonthsClamped adds months to day, clamping to the last day of the target
// month instead of overflowing into the next one as time.AddDate does
// (2025-08-31 plus 18 months is 2027-02-28, not 2027-03-03).
func addMonthsClamped(day time.Time, months int) time.Time {
	first := time.Date(day.Year(), day.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()

	return first.AddDate(0, 0, min(day.Day(), last)-1)
}

// validateFetchDailyMetrics checks structure only; location IDs stay opaque.
func validateFetchDailyMetrics(in fetchDailyMetricsInput) error {
	if !plainResourceName(in.Location, "locations/") {
		return invalid("location must be the exact locations/{id} resource name returned by businessprofile_list_locations")
	}

	start, err := time.Parse(metricsDateLayout, in.StartDate)
	if err != nil {
		return invalid("start_date must be a calendar day in YYYY-MM-DD form")
	}

	end, err := time.Parse(metricsDateLayout, in.EndDate)
	if err != nil {
		return invalid("end_date must be a calendar day in YYYY-MM-DD form")
	}

	if end.Before(start) {
		return invalid("end_date must not be before start_date")
	}

	if !end.Before(addMonthsClamped(start, metricsMaxRangeMonths)) {
		return invalid("the date range must be shorter than 18 months")
	}

	for i, metric := range in.Metrics {
		if !slices.Contains(allowedDailyMetrics, metric) {
			return invalid("metrics may only contain CALL_CLICKS, BUSINESS_DIRECTION_REQUESTS, WEBSITE_CLICKS and the four BUSINESS_IMPRESSIONS_* metrics")
		}

		if slices.Contains(in.Metrics[:i], metric) {
			return invalid("metrics must not repeat a metric")
		}
	}

	return nil
}
