// Package analytics implements the native GA4 MCP operations.
package analytics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	adminapi "google.golang.org/api/analyticsadmin/v1beta"
	dataapi "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	propertiesDefaultSize = 50
	propertiesMaxSize     = 100
	reportDefaultRows     = 100
	reportMaxRows         = 1000
)

var (
	analyticsNamePattern = regexp.MustCompile(`^[A-Za-z0-9_:]+$`)
	errInvalidDate       = errors.New("invalid date")
)

type listPropertiesInput struct {
	mcpcontract.Selection
	PageSize  int64  `json:"page_size,omitempty"`
	PageToken string `json:"page_token,omitempty"`
}

type metadataInput struct {
	mcpcontract.Selection
	Property string `json:"property"`
	Kind     string `json:"kind"`
}

type reportInput struct {
	mcpcontract.Selection
	Property   string   `json:"property"`
	Metrics    []string `json:"metrics"`
	Dimensions []string `json:"dimensions,omitempty"`
	StartDate  string   `json:"start_date"`
	EndDate    string   `json:"end_date"`
	Limit      int64    `json:"limit,omitempty"`
	Offset     int64    `json:"offset,omitempty"`
}

type Property struct {
	ResourceName        string `json:"resource_name"`
	DisplayName         string `json:"display_name"`
	PropertyType        string `json:"property_type"`
	Parent              string `json:"parent"`
	AccountResourceName string `json:"account_resource_name"`
	AccountDisplayName  string `json:"account_display_name"`
}

type PropertiesData struct {
	Properties []Property `json:"properties"`
}

type DimensionMetadata struct {
	APIName            string   `json:"api_name"`
	UIName             string   `json:"ui_name"`
	Description        string   `json:"description"`
	Category           string   `json:"category"`
	CustomDefinition   bool     `json:"custom_definition"`
	DeprecatedAPINames []string `json:"deprecated_api_names,omitempty"`
}

type MetricMetadata struct {
	APIName            string   `json:"api_name"`
	UIName             string   `json:"ui_name"`
	Description        string   `json:"description"`
	Category           string   `json:"category"`
	Type               string   `json:"type"`
	Expression         string   `json:"expression"`
	CustomDefinition   bool     `json:"custom_definition"`
	DeprecatedAPINames []string `json:"deprecated_api_names,omitempty"`
	BlockedReasons     []string `json:"blocked_reasons,omitempty"`
}

type MetadataData struct {
	Property   string              `json:"property"`
	Kind       string              `json:"kind"`
	Dimensions []DimensionMetadata `json:"dimensions,omitempty"`
	Metrics    []MetricMetadata    `json:"metrics,omitempty"`
}

type DimensionHeader struct {
	Name string `json:"name"`
}

type MetricHeader struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ReportRow struct {
	Dimensions []string `json:"dimensions"`
	Metrics    []string `json:"metrics"`
}

type ReportData struct {
	Property           string            `json:"property"`
	TimeZone           string            `json:"timezone"`
	CurrencyCode       string            `json:"currency_code"`
	StartDate          string            `json:"start_date"`
	EndDate            string            `json:"end_date"`
	DimensionHeaders   []DimensionHeader `json:"dimension_headers"`
	MetricHeaders      []MetricHeader    `json:"metric_headers"`
	Rows               []ReportRow       `json:"rows"`
	Totals             []ReportRow       `json:"totals"`
	RowCount           int64             `json:"row_count"`
	Offset             int64             `json:"offset"`
	EmptyReason        string            `json:"empty_reason,omitempty"`
	DataLossOtherRow   bool              `json:"data_loss_other_row"`
	SubjectToThreshold bool              `json:"subject_to_threshold"`
}

// Operations returns the frozen GA4 tool implementations.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("analytics_list_properties", validateListPropertiesInput, func(ctx context.Context, id mcpcontract.Identity, in listPropertiesInput) (mcpcontract.Result[PropertiesData], error) {
			if in.PageSize == 0 {
				in.PageSize = propertiesDefaultSize
			}

			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "analytics_list_properties", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[PropertiesData]{}, fmt.Errorf("analytics HTTP client: %w", err)
			}

			svc, err := adminapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[PropertiesData]{}, fmt.Errorf("create analytics admin service: %w", err)
			}

			resp, err := svc.AccountSummaries.List().PageSize(in.PageSize).PageToken(in.PageToken).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[PropertiesData]{}, fmt.Errorf("list GA4 properties: %w", err)
			}
			out := mcpcontract.NewResult(id, PropertiesData{Properties: []Property{}})

			for _, account := range resp.AccountSummaries {
				if account == nil {
					continue
				}

				for _, property := range account.PropertySummaries {
					if property == nil {
						continue
					}
					out.Data.Properties = append(out.Data.Properties, Property{
						ResourceName: property.Property, DisplayName: property.DisplayName, PropertyType: property.PropertyType,
						Parent: property.Parent, AccountResourceName: account.Account, AccountDisplayName: account.DisplayName,
					})
				}
			}
			out.NextPageToken = resp.NextPageToken

			return out, nil
		}),
		mcpcontract.NewOperation("analytics_metadata", validateMetadataInput, func(ctx context.Context, id mcpcontract.Identity, in metadataInput) (mcpcontract.Result[MetadataData], error) {
			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "analytics_metadata", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[MetadataData]{}, fmt.Errorf("analytics HTTP client: %w", err)
			}

			svc, err := dataapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[MetadataData]{}, fmt.Errorf("create analytics data service: %w", err)
			}
			property := normalizeProperty(in.Property)

			resp, err := svc.Properties.GetMetadata(property + "/metadata").Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[MetadataData]{}, fmt.Errorf("read GA4 metadata: %w", err)
			}

			out := mcpcontract.NewResult(id, MetadataData{Property: property, Kind: in.Kind})
			if in.Kind == "dimensions" || in.Kind == "both" {
				out.Data.Dimensions = make([]DimensionMetadata, 0, len(resp.Dimensions))
				for _, item := range resp.Dimensions {
					if item == nil {
						continue
					}
					out.Data.Dimensions = append(out.Data.Dimensions, DimensionMetadata{
						APIName: item.ApiName, UIName: item.UiName, Description: item.Description, Category: item.Category,
						CustomDefinition: item.CustomDefinition, DeprecatedAPINames: append([]string(nil), item.DeprecatedApiNames...),
					})
				}
			}

			if in.Kind == "metrics" || in.Kind == "both" {
				out.Data.Metrics = make([]MetricMetadata, 0, len(resp.Metrics))
				for _, item := range resp.Metrics {
					if item == nil {
						continue
					}
					out.Data.Metrics = append(out.Data.Metrics, MetricMetadata{
						APIName: item.ApiName, UIName: item.UiName, Description: item.Description, Category: item.Category,
						Type: item.Type, Expression: item.Expression, CustomDefinition: item.CustomDefinition,
						DeprecatedAPINames: append([]string(nil), item.DeprecatedApiNames...),
						BlockedReasons:     append([]string(nil), item.BlockedReasons...),
					})
				}
			}

			return out, nil
		}),
		mcpcontract.NewOperation("analytics_report", validateReportInput, func(ctx context.Context, id mcpcontract.Identity, in reportInput) (mcpcontract.Result[ReportData], error) {
			if in.Limit == 0 {
				in.Limit = reportDefaultRows
			}

			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "analytics_report", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[ReportData]{}, fmt.Errorf("analytics HTTP client: %w", err)
			}

			svc, err := dataapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[ReportData]{}, fmt.Errorf("create analytics data service: %w", err)
			}
			property := normalizeProperty(in.Property)

			req := &dataapi.RunReportRequest{
				Dimensions: make([]*dataapi.Dimension, 0, len(in.Dimensions)),
				Metrics:    make([]*dataapi.Metric, 0, len(in.Metrics)),
				DateRanges: []*dataapi.DateRange{{StartDate: in.StartDate, EndDate: in.EndDate}},
				Limit:      in.Limit, Offset: in.Offset, MetricAggregations: []string{"TOTAL"},
			}
			for _, name := range in.Dimensions {
				req.Dimensions = append(req.Dimensions, &dataapi.Dimension{Name: name})
			}

			for _, name := range in.Metrics {
				req.Metrics = append(req.Metrics, &dataapi.Metric{Name: name})
			}

			resp, err := svc.Properties.RunReport(property, req).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[ReportData]{}, fmt.Errorf("run GA4 report: %w", err)
			}

			out := mcpcontract.NewResult(id, ReportData{
				Property: property, StartDate: in.StartDate, EndDate: in.EndDate, Offset: in.Offset,
				DimensionHeaders: make([]DimensionHeader, 0, len(resp.DimensionHeaders)),
				MetricHeaders:    make([]MetricHeader, 0, len(resp.MetricHeaders)),
				Rows:             make([]ReportRow, 0, len(resp.Rows)),
				Totals:           make([]ReportRow, 0, len(resp.Totals)),
			})
			if resp.Metadata != nil {
				out.Data.TimeZone = resp.Metadata.TimeZone
				out.Data.CurrencyCode = resp.Metadata.CurrencyCode
				out.Data.EmptyReason = resp.Metadata.EmptyReason
				out.Data.DataLossOtherRow = resp.Metadata.DataLossFromOtherRow
				out.Data.SubjectToThreshold = resp.Metadata.SubjectToThresholding
			}

			for _, header := range resp.DimensionHeaders {
				if header != nil {
					out.Data.DimensionHeaders = append(out.Data.DimensionHeaders, DimensionHeader{Name: header.Name})
				}
			}

			for _, header := range resp.MetricHeaders {
				if header != nil {
					out.Data.MetricHeaders = append(out.Data.MetricHeaders, MetricHeader{Name: header.Name, Type: header.Type})
				}
			}

			for _, row := range resp.Rows {
				if row != nil {
					out.Data.Rows = append(out.Data.Rows, projectReportRow(row))
				}
			}

			for _, row := range resp.Totals {
				if row != nil {
					out.Data.Totals = append(out.Data.Totals, projectReportRow(row))
				}
			}

			out.Data.RowCount = resp.RowCount
			if end := in.Offset + in.Limit; end < resp.RowCount {
				out.NextPageToken = strconv.FormatInt(end, 10)
				out.Truncated = true
			}

			return out, nil
		}),
	}
}

func validateListPropertiesInput(in listPropertiesInput) error {
	if in.PageSize < 0 || in.PageSize > propertiesMaxSize {
		return invalid("page_size must be between 1 and 100 when supplied")
	}

	return nil
}

func validateMetadataInput(in metadataInput) error {
	if err := validateProperty(in.Property); err != nil {
		return err
	}

	switch in.Kind {
	case "dimensions", "metrics", "both":
		return nil
	default:
		return invalid("kind must be dimensions, metrics, or both")
	}
}

func validateReportInput(in reportInput) error {
	if err := validateProperty(in.Property); err != nil {
		return err
	}

	if err := validateNames("metrics", in.Metrics, 10); err != nil {
		return err
	}

	if err := validateNames("dimensions", in.Dimensions, 9); err != nil {
		return err
	}

	if err := validateDateRange(in.StartDate, in.EndDate); err != nil {
		return err
	}

	if in.Limit < 0 || in.Limit > reportMaxRows {
		return invalid("limit must be between 1 and 1000 when supplied")
	}

	if in.Offset < 0 {
		return invalid("offset must be a non-negative integer")
	}

	if in.Offset > math.MaxInt64-in.Limit {
		return invalid("offset is too large")
	}

	return nil
}

func validateProperty(value string) error {
	property := normalizeProperty(value)
	if property == "properties/" {
		return invalid("property must be a numeric GA4 property ID or properties/{property_id}")
	}

	id := strings.TrimPrefix(property, "properties/")
	if id == "" || strings.ContainsAny(id, "/:") || !isDecimal(id) {
		return invalid("property must be a numeric GA4 property ID or properties/{property_id}")
	}

	return nil
}

func normalizeProperty(value string) string {
	value = strings.TrimSpace(value)
	if value != "" && !strings.HasPrefix(value, "properties/") {
		return "properties/" + value
	}

	return value
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}

	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

func validateNames(kind string, values []string, maximum int) error {
	if kind == "metrics" && len(values) == 0 {
		return invalid("metrics must contain at least one metric name")
	}

	if len(values) > maximum {
		return invalid("%s supports at most %d names", kind, maximum)
	}

	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !analyticsNamePattern.MatchString(value) {
			return invalid("%s names may contain only letters, digits, underscores, and colons", kind)
		}

		if _, exists := seen[value]; exists {
			return invalid("%s names must be unique", kind)
		}
		seen[value] = struct{}{}
	}

	return nil
}

func validateDateRange(startValue, endValue string) error {
	start, err := parseDate(startValue)
	if err != nil {
		return invalid("start_date must use YYYY-MM-DD")
	}

	end, err := parseDate(endValue)
	if err != nil {
		return invalid("end_date must use YYYY-MM-DD")
	}

	if end.Before(start) {
		return invalid("end_date must not precede start_date")
	}

	return nil
}

func parseDate(value string) (time.Time, error) {
	if len(value) != 10 || value[4] != '-' || value[7] != '-' {
		return time.Time{}, errInvalidDate
	}

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date: %w", err)
	}

	return parsed, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w", mcpcontract.Invalid(fmt.Sprintf(format, args...)))
}

func projectReportRow(row *dataapi.Row) ReportRow {
	out := ReportRow{Dimensions: []string{}, Metrics: []string{}}

	for _, value := range row.DimensionValues {
		if value != nil {
			out.Dimensions = append(out.Dimensions, value.Value)
		}
	}

	for _, value := range row.MetricValues {
		if value != nil {
			out.Metrics = append(out.Metrics, value.Value)
		}
	}

	return out
}
