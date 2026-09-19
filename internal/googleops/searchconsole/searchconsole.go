// Package searchconsole implements the native Search Console MCP operations.
package searchconsole

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"regexp"
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

var (
	hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`)
	errInvalidDate  = errors.New("invalid date")
)

var allowedDimensions = map[string]struct{}{
	"query": {}, "page": {}, "country": {}, "device": {}, "date": {},
}

type listSitesInput struct {
	mcpcontract.Selection
}

type queryInput struct {
	mcpcontract.Selection
	SiteURL    string   `json:"site_url"`
	StartDate  string   `json:"start_date"`
	EndDate    string   `json:"end_date"`
	Dimensions []string `json:"dimensions,omitempty"`
	Filters    []filter `json:"filters,omitempty"`
	RowLimit   int64    `json:"row_limit,omitempty"`
	StartRow   int64    `json:"start_row,omitempty"`
}

type filter struct {
	Dimension  string `json:"dimension"`
	Operator   string `json:"operator"`
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
				RowLimit: in.RowLimit, StartRow: in.StartRow,
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

			for _, row := range resp.Rows {
				if row == nil {
					continue
				}
				out.Data.Rows = append(out.Data.Rows, QueryRow{
					Keys: append([]string(nil), row.Keys...), Clicks: row.Clicks, Impressions: row.Impressions,
					CTR: row.Ctr, Position: row.Position,
				})
			}

			if int64(len(resp.Rows)) == in.RowLimit {
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

	seenDimensions := make(map[string]struct{}, len(in.Dimensions)+len(in.Filters))
	for _, dimension := range in.Dimensions {
		normalized := strings.ToLower(dimension)
		if _, ok := allowedDimensions[normalized]; !ok {
			return invalid("dimensions supports query, page, country, device, and date")
		}

		if _, exists := seenDimensions[normalized]; exists {
			return invalid("dimensions must be unique")
		}
		seenDimensions[normalized] = struct{}{}
	}

	for _, item := range in.Filters {
		normalized := strings.ToLower(item.Dimension)
		if _, ok := allowedDimensions[normalized]; !ok {
			return invalid("filter dimension must be query, page, country, device, or date")
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

	if in.StartRow > math.MaxInt64-in.RowLimit {
		return invalid("start_row is too large")
	}

	return nil
}

func validateSiteURL(value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return invalid("site_url is required and must not contain surrounding whitespace")
	}

	if strings.ContainsAny(value, "\\\r\n\t") || strings.Contains(strings.ToLower(value), "%2f") || strings.Contains(strings.ToLower(value), "%5c") {
		return invalid("site_url contains an unsupported escape or path separator")
	}

	if domain, ok := strings.CutPrefix(value, "sc-domain:"); ok {
		if !hostnamePattern.MatchString(domain) || strings.Contains(domain, "/") {
			return invalid("domain properties must use sc-domain:{registrable-domain} without a path")
		}

		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() {
		return invalid("site_url must be an http(s) URL-prefix property or sc-domain:{registrable-domain}")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return invalid("URL-prefix properties must use http or https")
	}

	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" || parsed.Port() != "" {
		return invalid("URL-prefix properties must not include credentials, a port, query, or fragment")
	}

	if !hostnamePattern.MatchString(parsed.Hostname()) {
		return invalid("URL-prefix properties must use a valid hostname")
	}

	if parsed.Path == "" {
		return nil
	}

	if strings.Contains(parsed.Path, "%") || strings.Contains(parsed.Path, ";") || strings.Contains(parsed.Path, "//") {
		return invalid("URL-prefix property paths may not contain encoded or repeated path separators")
	}

	cleanPath := path.Clean(parsed.Path)
	if cleanPath == "." || cleanPath == ".." || parsed.Path != cleanPath && parsed.Path != cleanPath+"/" {
		return invalid("URL-prefix property paths must be canonical")
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
