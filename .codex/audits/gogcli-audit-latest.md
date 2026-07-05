# gogcli Audit Report

Date: 2026-06-28
Branch: `ai-audit/gogcli-latest`
Audited baseline: `origin/main` at `37e1042` (`fix(analytics): use page-token in next-page hints (#34)`)
Mode: Audit report only; no source, test, README, or docs changes performed

## Audit Scope

Reviewed current CLI behavior for missing Google/Workspace/business capabilities, data completeness and pagination, parseable output contracts, exit/error behavior, auth/config safety, mutation safety, verification surfaces, and agent/script operability.

Commands and read-only probes run:

- `git status --short --branch`
- `git fetch origin main`
- `git log origin/main --oneline --decorate --max-count=30`
- `rg -n "nextPageToken|NextPageToken|PageToken|page-token|PageSize|MaxResults|printNextPageHint|next page|Offset|Limit" internal/cmd internal/googleapi`
- `rg -n "dry-run|DryRun|confirm|Confirm|Force|NoInput|no-input|destructive|delete|remove" internal/cmd`
- `rg -n "OptionalOutputPath|OutputPath|out\"|output file|stdout|stderr|plain|json" internal/cmd`
- `nl -ba internal/cmd/analytics.go | sed -n '1,380p'`
- `nl -ba internal/cmd/businessprofile_accounts.go | sed -n '1,270p'`
- `nl -ba internal/cmd/youtube.go | sed -n '1,190p'`
- `nl -ba internal/cmd/flags_output.go | sed -n '1,80p'`
- `nl -ba internal/cmd/output_helpers.go | sed -n '1,60p'`
- `go run ./cmd/gog analytics report --help`
- `go run ./cmd/gog analytics accounts --help`
- `go run ./cmd/gog analytics properties --help`
- `go run ./cmd/gog business-profile accounts list --help`
- `go run ./cmd/gog youtube channels --help`

Web references used for comparison:

- Command Line Interface Guidelines: <https://clig.dev/>
- Google Cloud CLI scripting guidance: <https://docs.cloud.google.com/sdk/docs/scripting-gcloud>
- AWS CLI pagination guidance: <https://docs.aws.amazon.com/cli/v1/userguide/cli-usage-pagination.html>
- AWS CLI command reference pattern for `nextToken` reuse: <https://docs.aws.amazon.com/cli/latest/reference/stepfunctions/list-activities.html>

Current-state notes:

- The prior stale Top 3 have landed on `origin/main`: BigQuery page-token input (#32), Analytics Admin parseable plain mutation output (#33), and Analytics Admin `--page-token` hints (#34).
- The current audit excludes those merged tasks and re-ranks remaining items by functional impact to agent/Jeremy workflows.
- Modern scripting-oriented CLIs prioritize predictable machine output and complete pagination loops. `gogcli` already follows this pattern in many commands; the strongest remaining gaps are places that emit `nextPageToken` or row totals without a matching input flag to continue retrieval.

## 1. Prioritized Improvements

### 1. Analytics report supports `--limit` but not `--offset`

- Priority: High
- Why it matters: GA4 Data API reports can return more rows than a single request. `gog analytics report` exposes `--limit` and JSON includes `rowCount`, but agents cannot request the next row window. This blocks complete SEO/performance exports when `rowCount > len(rows)`.
- Exact location:
  - `internal/cmd/analytics.go:50` through `internal/cmd/analytics.go:57` (`AnalyticsReportCmd` has `Limit` but no `Offset`)
  - `internal/cmd/analytics.go:94` through `internal/cmd/analytics.go:103` (`RunReportRequest` sets `Limit` only)
  - `internal/cmd/analytics.go:115` through `internal/cmd/analytics.go:121` (JSON emits `rowCount` but no continuation context)
  - `internal/cmd/analytics_audience.go:219` through `internal/cmd/analytics_audience.go:241` already shows the local `--offset` + `--limit` pattern for audience export queries.
- What is wrong: The command tells scripts the total row count but does not expose the API's offset-based continuation path.
- Recommended improvement: add `Offset int64 'name:"offset" help:"Starting row offset" default:"0"'` to `AnalyticsReportCmd`, set `req.Offset = c.Offset` when positive, and add a focused request-body test for `--limit 100 --offset 100`.
- Expected impact: complete GA4 report extraction becomes scriptable without changing default first-page behavior.
- Functional impact category: Data completeness
- Workflow improved: `gog --json analytics report --property ... --metrics ... --dimensions ... --limit N` loops that need every row for client reporting.
- Verification proof: `go test ./internal/cmd -run AnalyticsReport` plus `go run ./cmd/gog analytics report --help` showing `--offset`.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Analytics account/property discovery emits page tokens but cannot consume them

- Priority: High
- Why it matters: Account/property discovery is the setup step before GA reporting. Both commands return `nextPageToken` and print a copied next-page hint, but help output only exposes `--page-size`; the hinted `--page` flag does not exist.
- Exact location:
  - `internal/cmd/analytics.go:255` through `internal/cmd/analytics.go:257` (`AnalyticsPropertiesCmd` lacks a page-token field)
  - `internal/cmd/analytics.go:270` through `internal/cmd/analytics.go:283` (`AccountSummaries.List` returns `nextPageToken`)
  - `internal/cmd/analytics.go:301` (`printNextPageHint`)
  - `internal/cmd/analytics.go:307` through `internal/cmd/analytics.go:309` (`AnalyticsAccountsCmd` lacks a page-token field)
  - `internal/cmd/analytics.go:322` through `internal/cmd/analytics.go:335` (`Accounts.List` returns `nextPageToken`)
  - `internal/cmd/analytics.go:351` (`printNextPageHint`)
  - `internal/cmd/output_helpers.go:21` through `internal/cmd/output_helpers.go:25` (shared hint hardcodes `--page`)
- What is wrong: The command output advertises pagination without a matching input path. This is especially brittle for agents that copy the emitted hint.
- Recommended improvement: add `Page string 'name:"page" help:"Page token"'` to both commands and call `.PageToken(strings.TrimSpace(c.Page))` when non-empty. Add tests that assert `pageToken=tok` reaches `/v1beta/accountSummaries` and `/v1beta/accounts`.
- Expected impact: agents can enumerate all GA accounts/properties instead of silently stopping at the first API page.
- Functional impact category: Data completeness
- Workflow improved: GA4 account/property discovery before reporting, audits, and admin checks.
- Verification proof: `go test ./internal/cmd -run 'Analytics(Accounts|Properties)'` and help checks for `analytics accounts --help` / `analytics properties --help`.
- Estimated risk: Low
- Safe to automate: Yes

### 3. Business Profile account list exposes `nextPageToken` without page controls

- Priority: High
- Why it matters: Business Profile account discovery is the entry point for location, admin, invitation, and account-management operations. The command returns `nextPageToken` and prints a next-page hint, but the list command struct has no flags and the API call cannot set page size or page token.
- Exact location:
  - `internal/cmd/businessprofile_accounts.go:201` through `internal/cmd/businessprofile_accounts.go:202` (`BusinessProfileAccountsListCmd struct{}`)
  - `internal/cmd/businessprofile_accounts.go:216` (`svc.Accounts.List().Do()`)
  - `internal/cmd/businessprofile_accounts.go:221` through `internal/cmd/businessprofile_accounts.go:225` (JSON includes `nextPageToken`)
  - `internal/cmd/businessprofile_accounts.go:239` (`printNextPageHint`)
- What is wrong: Account discovery can be incomplete for agency users with multiple accounts/location groups, and the copied next-page hint cannot be executed.
- Recommended improvement: add `Max int64 'name:"max" aliases:"limit" help:"Max results" default:"50"'` and `Page string 'name:"page" help:"Page token"'`, then pass them to `Accounts.List().PageSize(c.Max).PageToken(strings.TrimSpace(c.Page))` where supported by the generated client.
- Expected impact: complete Business Profile account enumeration becomes reliable for scripts and agency workflows.
- Functional impact category: Data completeness
- Workflow improved: `gog business-profile accounts list` before location, admin, invitation, and account-management operations.
- Verification proof: `go test ./internal/cmd -run 'BusinessProfileAccounts'` with request-query assertions; `go run ./cmd/gog business-profile accounts list --help` documents `--page` and `--max`.
- Estimated risk: Low
- Safe to automate: Yes

### 4. YouTube channel listing emits a next page token but lacks `--page`

- Priority: Medium
- Why it matters: YouTube is not the core Workspace path, but it is a real Google business/marketing workflow. `gog youtube channels --mine` can emit `nextPageToken` and a copied `--page` hint while help output exposes only `--mine`, `--id`, and `--max`.
- Exact location:
  - `internal/cmd/youtube.go:30` through `internal/cmd/youtube.go:34` (`YoutubeChannelsCmd` has no `Page` field)
  - `internal/cmd/youtube.go:51` through `internal/cmd/youtube.go:57` (`Channels.List` sets `MaxResults`, `Mine`, and `Id`, but not `PageToken`)
  - `internal/cmd/youtube.go:64` through `internal/cmd/youtube.go:68` (JSON includes `nextPageToken`)
  - `internal/cmd/youtube.go:92` (`printNextPageHint`)
  - `internal/cmd/youtube.go:98` through `internal/cmd/youtube.go:126` shows sibling `youtube videos` already supports `Page`.
- What is wrong: The first page can be incomplete, and the shared hint points to a flag the command does not accept.
- Recommended improvement: add `Page string 'name:"page" help:"Page token"'` and pass it to `Channels.List(...).PageToken(strings.TrimSpace(c.Page))` when non-empty.
- Expected impact: channel enumeration behaves consistently with other YouTube list commands.
- Functional impact category: Data completeness
- Workflow improved: `gog --json youtube channels --mine --max N` channel inventory jobs.
- Verification proof: `go test ./internal/cmd -run YoutubeChannels` with a query assertion for `pageToken=page2`; `go run ./cmd/gog youtube channels --help`.
- Estimated risk: Low
- Safe to automate: Yes

### 5. Shared optional `--out` help text is inaccurate for command-specific outputs

- Priority: Medium
- Why it matters: Output paths are used in document, sheet, slide, Gmail attachment, and Chat media download workflows. The shared help says the default is the gogcli config directory even though several commands derive filenames, use the current directory, or choose command-specific defaults. This can send agents looking in the wrong place after an export/download.
- Exact location:
  - `internal/cmd/flags_output.go:3` through `internal/cmd/flags_output.go:5`
  - Consumers include `internal/cmd/sheets.go`, `internal/cmd/docs.go`, `internal/cmd/slides.go`, `internal/cmd/gmail_attachment.go`, and `internal/cmd/chat_media.go`.
- What is wrong: The help text over-specifies one default for a shared optional flag that has multiple runtime behaviors.
- Recommended improvement: change the shared optional help to `Output file path (default: command-specific)` or split specialized flag structs where a precise default is important.
- Expected impact: fewer file-recovery mistakes after exports/downloads, with no behavior change.
- Functional impact category: Operator speed
- Workflow improved: Drive/Docs/Sheets/Slides exports and Gmail/Chat attachment downloads.
- Verification proof: targeted help checks such as `go run ./cmd/gog sheets export --help` and `go run ./cmd/gog gmail attachment download --help`.
- Estimated risk: Low
- Safe to automate: Yes

### 6. Help rendering still depends on local config and keyring state

- Priority: Low
- Why it matters: Help output is usually expected to be stable anywhere, while top-level help includes dynamic config/keyring diagnostics. This can be useful for operators but makes help snapshots machine-dependent.
- Exact location:
  - `internal/cmd/root.go` (`helpDescription`, config path, keyring backend display)
- What is wrong: Dynamic environment data is mixed into general help instead of an explicit diagnostic/doctor command.
- Recommended improvement: consider moving dynamic diagnostics to a dedicated `config doctor` or status command, or make dynamic help diagnostics opt-in.
- Expected impact: more deterministic help and docs/test fixtures if adopted.
- Functional impact category: Reliability
- Workflow improved: scripted help inspection and reproducible CLI documentation generation.
- Verification proof: compare `gog --help` output under different `GOG_KEYRING_BACKEND` values.
- Estimated risk: Medium
- Safe to automate: No, because the current behavior may be an intentional operator convenience and changes a broad top-level UX surface.

### 7. Retry exhaustion remains HTTP-native rather than a typed outcome

- Priority: Low
- Why it matters: Retry and circuit-breaker behavior is centralized, but exhausted retryable HTTP responses return as ordinary Google API errors. Agents get less explicit information about whether a command failed after retry exhaustion versus a normal non-retryable API error.
- Exact location:
  - `internal/googleapi/transport.go`
  - `internal/googleapi/errors.go`
  - `internal/googleapi/transport_test.go`
- What is wrong: Open circuit has a typed `CircuitBreakerError`, but exhausted 429/5xx retries do not have a typed final outcome.
- Recommended improvement: design whether retry exhaustion should remain HTTP-native or become a typed error with stable user-facing formatting.
- Expected impact: clearer failure classification for automation.
- Functional impact category: Reliability
- Workflow improved: automated retry/recovery wrappers around Google API operations.
- Verification proof: transport tests for repeated 429/5xx responses and final error formatting.
- Estimated risk: Medium
- Safe to automate: No, because it changes cross-service error semantics.

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Add `--offset` to `analytics report`.
- Add `--page` to `analytics accounts` and `analytics properties`.
- Add `--max`/`--page` to `business-profile accounts list`.
- Add `--page` to `youtube channels`.
- Correct shared optional `--out` help text.

### Larger Refactors

- Move dynamic config/keyring details out of top-level help into a diagnostic command.
- Convert retry exhaustion into a typed error across the shared transport.
- Design safe full HTTP request/response logging with redaction.
- Bulk dependency updates across Google APIs, OAuth, Kong, and `x/*` packages.

## 3. Do Not Change List

- Stable command names and aliases: keep `gmail/mail/email`, `analytics/ga/ga4`, `business-profile/gbp/business`, `bigquery/bq`, `youtube/yt`, and existing service command names because user scripts likely call them.
- Existing `--json` shapes: do not rename `nextPageToken`, `rowCount`, `accounts`, `accountSummaries`, `files`, or resource wrapper keys in small automation PRs.
- Plain/text stdout contract: preserve primary data on stdout and progress/hints on stderr. This follows modern CLI scripting guidance and is already a repo pattern.
- Default first-page behavior: adding page/offset flags should not change current default list/report output.
- Shared next-page hint wording: keep the existing `# Next page: --page TOKEN` pattern for commands that expose `--page`; fix commands that do not.
- Destructive-command safeguards: keep `--force`, `--no-input`, and confirmation behavior intact for delete/update flows.
- Auth/config/keyring behavior: do not change token storage, account selection, OAuth client selection, or service-account semantics as part of pagination/output tasks.
- Human-readable default output: keep readable tables/prose unless the task specifically fixes parseability under `--plain` or `--json`.

## 4. Execution Plan

Convert only `Safe to automate: Yes` items into independent PR-sized tasks.

### Task 1

- Title: Add offset paging to Analytics reports
- Why: `analytics report` can limit returned rows but cannot request the next offset even when `rowCount` shows more data exists.
- Files/modules:
  - `internal/cmd/analytics.go`
  - `internal/cmd/analytics_test.go`
- Risk: Low
- Expected impact: complete GA report extraction for larger SEO/business reporting jobs.
- Functional impact category: Data completeness
- Workflow improved: paged `gog --json analytics report` loops.
- Verification proof: request-body test for `limit` and `offset`, plus help output showing `--offset`.
- Steps:
  1. Add an `Offset int64` field to `AnalyticsReportCmd`.
  2. Set `req.Offset = c.Offset` when `c.Offset > 0`.
  3. Extend the analytics test server/test to assert `--limit 100 --offset 100` reaches the API request body.
- Validation:
  - `go test ./internal/cmd -run AnalyticsReport`
  - `go test ./...`
  - `go run ./cmd/gog analytics report --help`
- Do not change:
  - Default first-page behavior.
  - Existing JSON keys.
  - Report metric/dimension parsing.

### Task 2

- Title: Add page-token input to Analytics discovery
- Why: `analytics accounts` and `analytics properties` emit `nextPageToken` and next-page hints but cannot accept the token back.
- Files/modules:
  - `internal/cmd/analytics.go`
  - `internal/cmd/analytics_test.go`
- Risk: Low
- Expected impact: agents can enumerate all GA accounts/properties before running reports or audits.
- Functional impact category: Data completeness
- Workflow improved: GA4 account/property discovery.
- Verification proof: request-query tests showing `pageToken=tok` for both AccountSummaries and Accounts list calls.
- Steps:
  1. Add `Page string` to `AnalyticsPropertiesCmd` and `AnalyticsAccountsCmd`.
  2. Pass trimmed non-empty tokens to `.PageToken(...)`.
  3. Add focused tests for explicit `--page tok` and first-page omission.
- Validation:
  - `go test ./internal/cmd -run 'Analytics(Accounts|Properties)'`
  - `go test ./...`
  - `go run ./cmd/gog analytics accounts --help`
  - `go run ./cmd/gog analytics properties --help`
- Do not change:
  - `--page-size` behavior.
  - Output columns.
  - JSON wrapper keys.

### Task 3

- Title: Add pagination controls to Business Profile accounts
- Why: `business-profile accounts list` can report `nextPageToken` but cannot request that page or tune page size.
- Files/modules:
  - `internal/cmd/businessprofile_accounts.go`
  - `internal/cmd/businessprofile_accounts_test.go` or `internal/cmd/businessprofile_test.go`
- Risk: Low
- Expected impact: complete Business Profile account discovery for agency/location-group workflows.
- Functional impact category: Data completeness
- Workflow improved: Business Profile account discovery before location/admin/invitation operations.
- Verification proof: request-query tests for `pageSize` and `pageToken`; help output showing `--max` and `--page`.
- Steps:
  1. Replace `BusinessProfileAccountsListCmd struct{}` with fields for `Max` and `Page`.
  2. Pass page size and trimmed page token to `Accounts.List()`.
  3. Add or extend list tests to assert pagination query parameters and unchanged JSON shape.
- Validation:
  - `go test ./internal/cmd -run 'BusinessProfileAccounts'`
  - `go test ./...`
  - `go run ./cmd/gog business-profile accounts list --help`
- Do not change:
  - Account create/get/patch behavior.
  - Existing `accounts` and `nextPageToken` JSON keys.
  - Confirmation behavior for account/admin mutations.

### Task 4

- Title: Add page-token input to YouTube channels
- Why: `youtube channels` emits `nextPageToken` and a next-page hint but has no `--page` flag.
- Files/modules:
  - `internal/cmd/youtube.go`
  - `internal/cmd/youtube_test.go`
- Risk: Low
- Expected impact: complete channel enumeration for business/marketing inventory scripts.
- Functional impact category: Data completeness
- Workflow improved: `gog --json youtube channels --mine --max N` pagination.
- Verification proof: query test showing `pageToken=page2`; help output showing `--page`.
- Steps:
  1. Add `Page string` to `YoutubeChannelsCmd`.
  2. Pass trimmed non-empty page token to `Channels.List(...).PageToken(...)`.
  3. Extend `TestExecute_YoutubeChannels_JSON` or add a focused test for `--page page2`.
- Validation:
  - `go test ./internal/cmd -run YoutubeChannels`
  - `go test ./...`
  - `go run ./cmd/gog youtube channels --help`
- Do not change:
  - `--mine`, `--id`, or `--max` behavior.
  - Existing JSON keys.
  - Other YouTube list commands that already support `--page`.

### Task 5

- Title: Correct optional output path help
- Why: Shared `--out` help tells operators the default is the gogcli config directory even though several commands use command-specific output paths.
- Files/modules:
  - `internal/cmd/flags_output.go`
  - Any help-output tests if present or added
- Risk: Low
- Expected impact: fewer file-recovery mistakes after exports/downloads.
- Functional impact category: Operator speed
- Workflow improved: Docs/Sheets/Slides exports and Gmail/Chat attachment download workflows.
- Verification proof: help output no longer points all optional `--out` consumers to the config dir.
- Steps:
  1. Change the shared optional `--out` help text to say the default is command-specific.
  2. Add or update a small help test if the repo has an established help-output pattern.
  3. Manually probe representative commands that use optional `--out`.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog sheets export --help`
  - `go run ./cmd/gog gmail attachment download --help`
- Do not change:
  - Any output path behavior.
  - Required `--out` help text.
  - `--out-dir` behavior.

## Final Section

Top 3 Tasks to Execute First:

1. Add offset paging to Analytics reports.
2. Add page-token input to Analytics discovery.
3. Add pagination controls to Business Profile accounts.

Tasks Excluded:

- Task: Move dynamic config/keyring details out of top-level help.
  - Reason: Potentially useful, but it changes broad UX behavior and may remove intentional operator context.
- Task: Convert retry exhaustion into a typed error.
  - Reason: Cross-service error semantics require design review before automation.
- Task: Add raw HTTP request/response verbose logging.
  - Reason: Secret redaction risks are high for OAuth tokens, Gmail bodies, Drive files, and service-account material.
- Task: Bulk dependency updates.
  - Reason: Maintenance-only and higher blast radius than current functional gaps.
