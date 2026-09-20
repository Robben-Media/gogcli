// Package searchconsole implements the native Search Console MCP operations.
package searchconsole

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/option"
	searchconsoleapi "google.golang.org/api/searchconsole/v1"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	queryDefaultRows = 100
	queryMaxRows     = 1000
	pacificTimezone  = "America/Los_Angeles"
)

var errInvalidDate = errors.New("invalid date")

var allowedGroupDimensions = map[string]struct{}{
	"query": {}, "page": {}, "country": {}, "device": {}, "date": {}, "search_appearance": {},
}

var allowedFilterDimensions = map[string]struct{}{
	"query": {}, "page": {}, "country": {}, "device": {}, "search_appearance": {},
}

type listSitesInput struct {
	mcpcontract.Selection
}

type queryInput struct {
	mcpcontract.Selection
	SiteURL    string   `json:"site_url" jsonschema:"Exact opaque property identifier returned by searchconsole_list_sites"`
	StartDate  string   `json:"start_date"`
	EndDate    string   `json:"end_date"`
	Dimensions []string `json:"dimensions,omitempty" jsonschema:"Grouping dimensions: query, page, country, device, date, or search_appearance"`
	Filters    []filter `json:"filters,omitempty" jsonschema:"Filters ANDed together; dimensions support query, page, country, device, and search_appearance, and operators support equals, contains, and not_contains"`
	RowLimit   int64    `json:"row_limit,omitempty" jsonschema:"Maximum rows returned; one additional row is requested only to detect truncation"`
	StartRow   int64    `json:"start_row,omitempty"`
}

type filter struct {
	Dimension  string `json:"dimension" jsonschema:"query, page, country, device, or search_appearance"`
	Operator   string `json:"operator" jsonschema:"equals, contains, or not_contains"`
	Expression string `json:"expression"`
}

type Site struct {
	SiteURL         string `json:"site_url"`
	SiteType        string `json:"site_type"`
	PermissionLevel string `json:"permission_level"`
}

type SitesData struct {
	Sites []Site `json:"sites"`
}

type QueryRow struct {
	Keys        []string `json:"keys"`
	Clicks      float64  `json:"clicks"`
	Impressions float64  `json:"impressions"`
	CTR         float64  `json:"ctr"`
	Position    float64  `json:"position"`
}

type QueryData struct {
	SiteURL                 string     `json:"site_url"`
	SiteType                string     `json:"site_type"`
	StartDate               string     `json:"start_date"`
	EndDate                 string     `json:"end_date"`
	TimeZone                string     `json:"timezone"`
	Dimensions              []string   `json:"dimensions"`
	Rows                    []QueryRow `json:"rows"`
	FirstIncompleteDate     string     `json:"first_incomplete_date,omitempty"`
	ResponseAggregationType string     `json:"response_aggregation_type,omitempty"`
}

// Operations returns the frozen Search Console tool implementations.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("searchconsole_list_sites", func(listSitesInput) error { return nil }, func(ctx context.Context, id mcpcontract.Identity, in listSitesInput) (mcpcontract.Result[SitesData], error) {
			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "searchconsole_list_sites", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[SitesData]{}, fmt.Errorf("search Console HTTP client: %w", err)
			}

			svc, err := searchconsoleapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[SitesData]{}, fmt.Errorf("create Search Console service: %w", err)
			}

			resp, err := svc.Sites.List().Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[SitesData]{}, fmt.Errorf("list Search Console sites: %w", err)
			}
			out := mcpcontract.NewResult(id, SitesData{Sites: []Site{}})

			for _, site := range resp.SiteEntry {
				if site == nil {
					continue
				}
				out.Data.Sites = append(out.Data.Sites, Site{
					SiteURL: site.SiteUrl, SiteType: siteType(site.SiteUrl), PermissionLevel: site.PermissionLevel,
				})
			}

			return out, nil
		}),
		mcpcontract.NewOperation("searchconsole_query", validateQueryInput, func(ctx context.Context, id mcpcontract.Identity, in queryInput) (mcpcontract.Result[QueryData], error) {
			if in.RowLimit == 0 {
				in.RowLimit = queryDefaultRows
			}

			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "searchconsole_query", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[QueryData]{}, fmt.Errorf("search Console HTTP client: %w", err)
			}

			svc, err := searchconsoleapi.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[QueryData]{}, fmt.Errorf("create Search Console service: %w", err)
			}

			req := &searchconsoleapi.SearchAnalyticsQueryRequest{
				StartDate: in.StartDate, EndDate: in.EndDate, Dimensions: make([]string, 0, len(in.Dimensions)),
				RowLimit: in.RowLimit + 1, StartRow: in.StartRow,
			}
			for _, dimension := range in.Dimensions {
				req.Dimensions = append(req.Dimensions, strings.ToUpper(dimension))
			}

			if len(in.Filters) > 0 {
				apiFilters := make([]*searchconsoleapi.ApiDimensionFilter, 0, len(in.Filters))
				for _, item := range in.Filters {
					apiFilters = append(apiFilters, &searchconsoleapi.ApiDimensionFilter{
						Dimension: strings.ToUpper(item.Dimension), Operator: filterOperator(item.Operator), Expression: item.Expression,
					})
				}
				req.DimensionFilterGroups = []*searchconsoleapi.ApiDimensionFilterGroup{{GroupType: "AND", Filters: apiFilters}}
			}

			resp, err := svc.Searchanalytics.Query(in.SiteURL, req).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[QueryData]{}, fmt.Errorf("query Search Console: %w", err)
			}

			dimensions := append([]string(nil), in.Dimensions...)

			out := mcpcontract.NewResult(id, QueryData{
				SiteURL: in.SiteURL, SiteType: siteType(in.SiteURL), StartDate: in.StartDate, EndDate: in.EndDate,
				TimeZone: pacificTimezone, Dimensions: dimensions, Rows: make([]QueryRow, 0, len(resp.Rows)),
				ResponseAggregationType: resp.ResponseAggregationType,
			})
			if resp.Metadata != nil {
				out.Data.FirstIncompleteDate = resp.Metadata.FirstIncompleteDate
			}

			upstreamRows := resp.Rows

			hasDefiniteNextPage := int64(len(upstreamRows)) > in.RowLimit
			if hasDefiniteNextPage {
				upstreamRows = upstreamRows[:in.RowLimit]
			}

			for _, row := range upstreamRows {
				if row == nil {
					continue
				}
				out.Data.Rows = append(out.Data.Rows, QueryRow{
					Keys: append([]string(nil), row.Keys...), Clicks: row.Clicks, Impressions: row.Impressions,
					CTR: row.Ctr, Position: row.Position,
				})
			}

			if hasDefiniteNextPage {
				next := in.StartRow + in.RowLimit
				out.NextPageToken = strconv.FormatInt(next, 10)
				out.Truncated = true
			}

			return out, nil
		}),
	}
}

func validateQueryInput(in queryInput) error {
	if err := validateSiteURL(in.SiteURL); err != nil {
		return err
	}

	if err := validateDateRange(in.StartDate, in.EndDate); err != nil {
		return err
	}

	seenGroupDimensions := make(map[string]struct{}, len(in.Dimensions))
	for _, dimension := range in.Dimensions {
		normalized := strings.ToLower(dimension)
		if _, ok := allowedGroupDimensions[normalized]; !ok {
			return invalid("dimensions supports query, page, country, device, date, and search_appearance")
		}

		if _, exists := seenGroupDimensions[normalized]; exists {
			return invalid("dimensions must be unique")
		}
		seenGroupDimensions[normalized] = struct{}{}
	}

	for _, item := range in.Filters {
		normalized := strings.ToLower(item.Dimension)
		if _, ok := allowedFilterDimensions[normalized]; !ok {
			return invalid("filter dimension must be query, page, country, device, or search_appearance")
		}

		switch item.Operator {
		case "equals", "contains", "not_contains":
		default:
			return invalid("filter operator must be equals, contains, or not_contains")
		}

		if item.Expression == "" {
			return invalid("filter expression is required")
		}
	}

	if in.RowLimit < 0 || in.RowLimit > queryMaxRows {
		return invalid("row_limit must be between 1 and 1000 when supplied")
	}

	if in.StartRow < 0 {
		return invalid("start_row must be a non-negative integer")
	}

	rowLimit := in.RowLimit
	if rowLimit == 0 {
		rowLimit = queryDefaultRows
	}

	if in.StartRow > math.MaxInt64-rowLimit {
		return invalid("start_row is too large")
	}

	return nil
}

func validateSiteURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return invalid("site_url is required and must not contain surrounding whitespace")
	}

	if len(value) > 2048 {
		return invalid("site_url must be at most 2048 characters")
	}

	if domain, ok := strings.CutPrefix(value, "sc-domain:"); ok {
		if strings.TrimSpace(domain) == "" {
			return invalid("sc-domain property identifiers must include the domain")
		}

		return nil
	}

	if rest, ok := strings.CutPrefix(value, "https://"); ok && strings.TrimSpace(rest) != "" {
		return nil
	}

	if rest, ok := strings.CutPrefix(value, "http://"); ok && strings.TrimSpace(rest) != "" {
		return nil
	}

	return invalid("site_url must be an http(s) URL-prefix property or sc-domain:{domain}")
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

func invalid(message string) error {
	return fmt.Errorf("%w", mcpcontract.Invalid(message))
}

func filterOperator(value string) string {
	switch value {
	case "equals":
		return "EQUALS"
	case "contains":
		return "CONTAINS"
	case "not_contains":
		return "NOT_CONTAINS"
	default:
		return ""
	}
}

func siteType(value string) string {
	if strings.HasPrefix(value, "sc-domain:") {
		return "domain"
	}

	return "url_prefix"
}
