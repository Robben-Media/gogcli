# Search Console v1 -- Gap Coverage Spec

**API**: Search Console v1 (`searchconsole/v1`, discovery name `webmasters/v3` for some endpoints)
**Current coverage**: 5 methods (searchanalytics.query, sitemaps.list/submit, sites.list, urlInspection.index.inspect)
**Gap**: 6 missing methods
**Service factory**: `newSearchConsoleService` (existing or to be created)

## Overview

Adding 6 missing methods to the Search Console CLI commands. Covers mobile-friendly testing, sitemap management (delete/get), and site management (add/delete/get) to achieve full Discovery API parity.

**Pagination**: The existing `sitemaps list` command includes `--max` (default 10) and `--page` flags for pagination. JSON output includes `nextPageToken`. The new gap commands (sites, sitemaps get/delete, mobile-friendly test) are single-resource operations and do not require pagination.

**Output format**: Commands support `--format` flag with `json` (default) and `text` (TSV-aligned table) output. Text output displays labeled columns (e.g., SITE_URL, PERMISSION_LEVEL for sites get).

**Error handling**: All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete operations require `confirmDestructive()`.

---

## URL Testing Tools

### `gog searchconsole mobile-friendly-test`

- **API method**: `urlTestingTools.mobileFriendlyTest.run`
- **Struct**: `SearchConsoleMobileFriendlyTestCmd`
- **Args/Flags**:
  - `--url` (required string): URL to test
  - `--request-screenshot` (bool): include a screenshot in the response
- **Behavior**: Runs a mobile-friendly test on the given URL. Returns test status, mobile-friendly issues, and optionally a screenshot (base64-encoded).
- **Output**: JSON object with `testStatus` (COMPLETE/INTERNAL_ERROR/PAGE_UNREACHABLE), `mobileFriendliness` (MOBILE_FRIENDLY/NOT_MOBILE_FRIENDLY), `mobileFriendlyIssues` array, `resourceIssues`, and optionally `screenshot`; text shows URL, STATUS, MOBILE_FRIENDLY (yes/no), ISSUES (comma-separated)
- **Test**: httptest mock POST `/v1/urlTestingTools/mobileFriendlyTest:run`, assert url and requestScreenshot in body

---

## Sitemaps

### `gog searchconsole sitemaps delete`

- **API method**: `sitemaps.delete`
- **Struct**: `SearchConsoleSitemapsDeleteCmd`
- **Args/Flags**:
  - `siteUrl` (required arg): site URL (e.g., `https://example.com/` or `sc-domain:example.com`)
  - `feedpath` (required arg): sitemap URL (e.g., `https://example.com/sitemap.xml`)
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Deletes a sitemap from Search Console (does not delete the actual file).
- **Output**: Empty on success; stderr shows confirmation
- **Test**: httptest mock DELETE `/webmasters/v3/sites/{siteUrl}/sitemaps/{feedpath}`

### `gog searchconsole sitemaps get`

- **API method**: `sitemaps.get`
- **Struct**: `SearchConsoleSitemapsGetCmd`
- **Args/Flags**:
  - `siteUrl` (required arg): site URL
  - `feedpath` (required arg): sitemap URL
- **Output**: JSON object with `path`, `lastSubmitted`, `isPending`, `isSitemapsIndex`, `type`, `lastDownloaded`, `warnings`, `errors`, `contents` array; text shows PATH, TYPE, LAST_SUBMITTED, LAST_DOWNLOADED, WARNINGS, ERRORS
- **Test**: httptest mock GET `/webmasters/v3/sites/{siteUrl}/sitemaps/{feedpath}`

---

## Sites

### `gog searchconsole sites add`

- **API method**: `sites.add`
- **Struct**: `SearchConsoleSitesAddCmd`
- **Args/Flags**:
  - `siteUrl` (required arg): site URL to add (e.g., `https://example.com/` or `sc-domain:example.com`)
- **Behavior**: Adds a site to the user's Search Console. The user still needs to verify ownership separately.
- **Output**: Empty on success (HTTP 204); stderr shows "Site added: {siteUrl}. Verify ownership to access data."
- **Test**: httptest mock PUT `/webmasters/v3/sites/{siteUrl}` (note: this is PUT, not POST)

### `gog searchconsole sites delete`

- **API method**: `sites.delete`
- **Struct**: `SearchConsoleSitesDeleteCmd`
- **Args/Flags**:
  - `siteUrl` (required arg): site URL to remove
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Removes the site from the user's Search Console. Does not affect the actual website.
- **Output**: Empty on success; stderr shows confirmation
- **Test**: httptest mock DELETE `/webmasters/v3/sites/{siteUrl}`

### `gog searchconsole sites get`

- **API method**: `sites.get`
- **Struct**: `SearchConsoleSitesGetCmd`
- **Args/Flags**:
  - `siteUrl` (required arg): site URL
- **Output**: JSON object with `siteUrl` and `permissionLevel`; text shows SITE_URL, PERMISSION_LEVEL
- **Test**: httptest mock GET `/webmasters/v3/sites/{siteUrl}`

---

## Implementation Notes

1. **URL encoding**: Site URLs contain special characters (`:`, `/`, `.`). The siteUrl arg must be URL-encoded when constructing API paths. The Google client library handles this, but tests should verify correct encoding.
2. **Webmasters prefix**: Some endpoints use the `/webmasters/v3/` path prefix in the Discovery API even though they are part of Search Console. The Go client library abstracts this, but mock test URLs must use the correct prefix.
3. **sites.add uses PUT**: Unlike most create operations, `sites.add` uses PUT (idempotent). No request body is needed.
4. **sc-domain: prefix**: Domain properties use the `sc-domain:example.com` format. URL-prefix properties use `https://example.com/`. Both formats are valid for siteUrl.
5. **mobileFriendlyTest**: This is a standalone endpoint, not tied to a site property. It can test any public URL. The screenshot is base64 PNG data.
6. Total new test count: minimum 6 test functions.
