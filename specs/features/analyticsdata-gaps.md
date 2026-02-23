# Analytics Data v1beta -- Gap Coverage Spec

**API**: Analytics Data v1beta (`analyticsdata/v1beta`)
**Current coverage**: 3 methods (properties.getMetadata, properties.runRealtimeReport, properties.runReport)
**Gap**: 8 missing methods
**Service factory**: `newAnalyticsDataService` (existing or to be created)

## Overview

Adding 8 missing methods to the Analytics Data v1beta CLI commands. Covers audience exports (create/get/list/query) and report variants (batch pivot reports, batch reports, check compatibility, pivot report) to achieve full Discovery API parity.

All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Complex JSON inputs (batch and pivot operations) support both inline JSON strings and `@filepath` syntax. List commands include `--max` and `--page` flags for pagination. JSON output includes `nextPageToken`. Text output uses TSV-formatted tables.

---

## Audience Exports

### `gog analytics audience-exports create`

- **API method**: `properties.audienceExports.create`
- **Struct**: `AnalyticsAudienceExportsCreateCmd`
- **Args/Flags**:
  - `parent` (required arg): property resource name (e.g., `properties/123456`)
  - `--audience` (required string): audience resource name (e.g., `properties/123456/audiences/789`)
  - `--dimensions` ([]string): dimension names to include in export (e.g., `deviceId`, `userId`)
  - `--offset` (int64): starting row offset
  - `--limit` (int64): max rows to return
- **Behavior**: Creates a long-running operation. Returns operation metadata with the audience export name. The export runs asynchronously.
- **Output**: JSON object with operation/audienceExport metadata (name, state, audienceDisplayName, dimensions, rowCount)
- **Test**: httptest mock POST `/v1beta/{parent}/audienceExports`, assert audience and dimensions in body

### `gog analytics audience-exports get`

- **API method**: `properties.audienceExports.get`
- **Struct**: `AnalyticsAudienceExportsGetCmd`
- **Args/Flags**:
  - `name` (required arg): audience export resource name (e.g., `properties/123456/audienceExports/789`)
- **Output**: JSON object with export metadata and state; text shows NAME, AUDIENCE, STATE, ROW_COUNT, CREATION_TIME
- **Test**: httptest mock GET `/v1beta/{name}`

### `gog analytics audience-exports list`

- **API method**: `properties.audienceExports.list`
- **Struct**: `AnalyticsAudienceExportsListCmd`
- **Args/Flags**:
  - `parent` (required arg): property resource name
  - `--max` (int64, default 100, alias `limit`): page size
  - `--page` (string): page token
- **Output**: JSON array `{"audienceExports": [...], "nextPageToken": "..."}` ; text table NAME, AUDIENCE, STATE, ROW_COUNT
- **Test**: httptest mock GET `/v1beta/{parent}/audienceExports`, assert pageSize param

### `gog analytics audience-exports query`

- **API method**: `properties.audienceExports.query`
- **Struct**: `AnalyticsAudienceExportsQueryCmd`
- **Args/Flags**:
  - `name` (required arg): audience export resource name
  - `--offset` (int64): starting row offset
  - `--limit` (int64, default 10000): max rows
- **Behavior**: Retrieves the rows from a completed audience export. Must only be called on exports with state=ACTIVE.
- **Output**: JSON object with `audienceExport` metadata and `audienceRows` array; text table with dimension columns
- **Test**: httptest mock POST `/v1beta/{name}:query`, assert offset/limit in body

---

## Properties -- Report Variants

### `gog analytics batch-pivot-reports`

- **API method**: `properties.batchRunPivotReports`
- **Struct**: `AnalyticsBatchPivotReportsCmd`
- **Args/Flags**:
  - `property` (required arg): property resource name
  - `--requests-json` (required string): JSON array of pivot report requests, or @filepath
- **Behavior**: Runs up to 5 pivot reports in a single batch. Each request follows the RunPivotReportRequest schema. Accepts complex JSON input.
- **Output**: JSON object with `pivotReports` array, each containing headers, rows, metadata
- **Test**: httptest mock POST `/v1beta/{property}:batchRunPivotReports`, assert requests in body

### `gog analytics batch-reports`

- **API method**: `properties.batchRunReports`
- **Struct**: `AnalyticsBatchReportsCmd`
- **Args/Flags**:
  - `property` (required arg): property resource name
  - `--requests-json` (required string): JSON array of report requests, or @filepath
- **Behavior**: Runs up to 5 reports in a single batch. Each request follows the RunReportRequest schema.
- **Output**: JSON object with `reports` array
- **Test**: httptest mock POST `/v1beta/{property}:batchRunReports`, assert requests in body

### `gog analytics check-compatibility`

- **API method**: `properties.checkCompatibility`
- **Struct**: `AnalyticsCheckCompatibilityCmd`
- **Args/Flags**:
  - `property` (required arg): property resource name
  - `--dimensions` ([]string): dimension names to check
  - `--metrics` ([]string): metric names to check
  - `--filter-json` (string): JSON dimension filter expression
- **Behavior**: Checks whether the given dimensions and metrics can be used together in a report. Returns compatibility status for each.
- **Output**: JSON object with `dimensionCompatibilities` and `metricCompatibilities` arrays; text table NAME, COMPATIBLE (yes/no)
- **Test**: httptest mock POST `/v1beta/{property}:checkCompatibility`, assert dimensions and metrics in body

### `gog analytics pivot-report`

- **API method**: `properties.runPivotReport`
- **Struct**: `AnalyticsPivotReportCmd`
- **Args/Flags**:
  - `property` (required arg): property resource name
  - `--dimensions` (required []string): dimension names
  - `--metrics` (required []string): metric names
  - `--pivots-json` (required string): JSON array of pivot definitions, or @filepath
  - `--date-from` (string): start date (YYYY-MM-DD or "7daysAgo", "30daysAgo", "yesterday", "today")
  - `--date-to` (string): end date
  - `--filter-json` (string): dimension filter expression JSON
  - `--keep-empty` (bool): include rows with all zero metric values
  - `--currency-code` (string): currency for monetary metrics
- **Behavior**: Pivot reports reorganize report data by pivot dimensions. Each pivot specifies fieldNames, orderBys, offset, and limit. The pivots-json must be an array of Pivot objects.
- **Output**: JSON object with `pivotHeaders`, `dimensionHeaders`, `metricHeaders`, `rows`, `metadata`; text output as formatted table with pivot headers
- **Test**: httptest mock POST `/v1beta/{property}:runPivotReport`, assert dimensions, metrics, pivots in body

---

## Implementation Notes

1. **Pivot reports**: The pivot request structure is significantly different from regular reports. Each pivot has `fieldNames` (from dimensions), `orderBys`, `offset`, and `limit`. Multiple pivots can be specified. Use JSON input for the complex pivot definitions.
2. **Batch operations**: Both batchRunReports and batchRunPivotReports accept up to 5 requests. The JSON input should be an array of request objects matching the individual report request schema.
3. **Audience exports**: These are asynchronous. The create command returns immediately with an operation. Users should poll with `get` until state=ACTIVE, then use `query` to retrieve rows.
4. **checkCompatibility**: Useful for debugging report errors. Returns whether dimension/metric combinations are valid before running a report.
5. **Property resource names**: Always in the format `properties/{numericId}`. The numeric ID comes from the GA4 property.
6. **Complex JSON inputs**: For batch and pivot operations, support both inline JSON strings and `@filepath` syntax to read from a file. These request bodies are too complex for individual CLI flags.
7. Total new test count: minimum 8 test functions plus edge cases for malformed JSON input.
