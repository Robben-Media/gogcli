# gogcli Audit Report

Date: 2026-06-21
Branch: `ai-audit/gogcli-latest`
Audited baseline: `origin/main` at `37e1042` (`fix(analytics): use page-token in next-page hints (#34)`)
Mode: Audit report only; no source, test, README, or docs changes performed

## Audit Scope

Reviewed the current CLI for missing Workspace capabilities, pagination/data completeness, parseable output contracts, exit/error behavior, auth/config safety, mutation safety, verification surfaces, and agent/script operability.

Commands and read-only probes run:

- `git status --short --branch`
- `git log origin/main --oneline --decorate --max-count=20`
- `git show origin/main:internal/cmd/analytics.go`
- `git show origin/main:internal/cmd/analytics_audience.go`
- `git show origin/main:internal/cmd/businessprofile_accounts.go`
- `git show origin/main:internal/cmd/bigquery.go`
- `git show origin/main:internal/cmd/analytics_admin_streams.go`
- `git grep -n "printNextPageHint" origin/main -- internal/cmd internal/googleapi`
- `git grep -n "PageToken\\|NextPageToken\\|nextPageToken\\|MaxResults\\|PageSize" origin/main -- internal/cmd internal/googleapi`
- `git grep -n "confirmDestructive\\|NoInput\\|Force\\|dry-run\\|DryRun" origin/main -- internal/cmd`

Web references used for comparison:

- Command Line Interface Guidelines: <https://clig.dev/>
- Google Cloud CLI scripting and formatting guidance: <https://cloud.google.com/cli>
- AWS CLI output format guidance: <https://docs.aws.amazon.com/cli/v1/userguide/cli-usage-output-format.html>
- AWS CLI output and pagination overview: <https://docs.aws.amazon.com/cli/v1/userguide/cli-usage-output.html>

Current-state notes:

- The prior Top 3 from the 2026-06-14 audit have landed on `origin/main`: BigQuery page-token input (#32), Analytics Admin plain mutation output (#33), and Analytics Admin `--page-token` hints (#34).
- The current audit therefore excludes those stale tasks from the new Top 3.
- The strongest remaining gaps are still functional: incomplete access to paged GA and Business Profile data, plus one misleading command help surface that can waste operator recovery time.

## 1. Prioritized Improvements

### 1. Analytics report supports `--limit` but not `--offset`

- Priority: High
- Why it matters: GA4 Data API reports can return more rows than one request. `gog analytics report` exposes `--limit` and returns `rowCount`, but there is no way to request the next row window. Agents generating SEO or performance reports can see that data exists beyond the first response but cannot fetch the next slice through the CLI.
- Exact location:
  - `internal/cmd/analytics.go:50` through `internal/cmd/analytics.go:57` (`AnalyticsReportCmd`)
  - `internal/cmd/analytics.go:94` through `internal/cmd/analytics.go:103` (`RunReportRequest` sets `Limit` only)
  - `internal/cmd/analytics.go:115` onward (JSON/text output includes returned rows and `rowCount`, but no offset context)
- What is wrong: `RunReportRequest` supports offset-based paging, and `AnalyticsAudienceExportsQueryCmd` already exposes `--offset` plus `--limit` for a similar row-window workflow. The main GA report command has no `Offset` field and never sets `req.Offset`.
- Recommended improvement: add `Offset int64 'name:"offset" help:"Starting row offset" default:"0"'` to `AnalyticsReportCmd`, set `req.Offset` when positive, and add a focused test that sends `--limit 100 --offset 100` and verifies the request body includes both values.
- Expected impact: complete GA report extraction becomes scriptable without changing default first-page behavior.
- Functional impact category: Data completeness
- Workflow improved: `gog --json analytics report --property ... --metrics ... --dimensions ... --limit N` loops that need all rows when `rowCount > len(rows)`.
- Verification proof: `go test ./internal/cmd -run AnalyticsReport` plus a command-level help check showing `analytics report --help` documents `--offset`.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Analytics account/property listing emits page tokens but cannot consume them

- Priority: High
- Why it matters: Account and property discovery is the first step before GA reporting. Both commands emit `nextPageToken` and print a next-page hint, but the command structs only expose `--page-size`; a copied `# Next page: --page ...` hint is currently not executable.
- Exact location:
  - `internal/cmd/analytics.go:255` through `internal/cmd/analytics.go:257` (`AnalyticsPropertiesCmd` lacks `Page`)
  - `internal/cmd/analytics.go:270` through `internal/cmd/analytics.go:283` (`AccountSummaries.List` returns `nextPageToken`)
  - `internal/cmd/analytics.go:301` (`printNextPageHint`)
  - `internal/cmd/analytics.go:307` through `internal/cmd/analytics.go:309` (`AnalyticsAccountsCmd` lacks `Page`)
  - `internal/cmd/analytics.go:322` through `internal/cmd/analytics.go:335` (`Accounts.List` returns `nextPageToken`)
  - `internal/cmd/analytics.go:351` (`printNextPageHint`)
- What is wrong: The commands advertise pagination through output and hints without a matching `--page` input path.
- Recommended improvement: add `Page string 'name:"page" help:"Page token"'` to both commands and call `.PageToken(strings.TrimSpace(c.Page))` when non-empty. Add tests that assert `pageToken=tok` reaches `/v1beta/accountSummaries` and `/v1beta/accounts`.
- Expected impact: agents can enumerate all GA accounts/properties instead of silently stopping at the first API page.
- Functional impact category: Data completeness
- Workflow improved: account/property discovery before GA4 reporting or audit generation.
- Verification proof: `go test ./internal/cmd -run 'Analytics(Accounts|Properties)'` and `gog analytics accounts --help` / `gog analytics properties --help` showing `--page`.
- Estimated risk: Low
- Safe to automate: Yes

### 3. Business Profile account list exposes `nextPageToken` without page controls

- Priority: High
- Why it matters: Business Profile operations often start by listing accessible accounts before locations/admins can be managed. The command returns `nextPageToken` and prints a next-page hint, but its struct is empty and the API call does not set page size or page token.
- Exact location:
  - `internal/cmd/businessprofile_accounts.go:202` (`BusinessProfileAccountsListCmd struct{}`)
  - `internal/cmd/businessprofile_accounts.go:216` (`svc.Accounts.List().Do()`)
  - `internal/cmd/businessprofile_accounts.go:221` through `internal/cmd/businessprofile_accounts.go:225` (JSON includes `nextPageToken`)
  - `internal/cmd/businessprofile_accounts.go:239` (`printNextPageHint`)
- What is wrong: Account discovery can become incomplete for users with multiple agencies/location groups. The command says another page exists but cannot request it.
- Recommended improvement: add `Max int64 'name:"max" aliases:"limit" help:"Max results" default:"50"'` and `Page string 'name:"page" help:"Page token"'`, then pass them to `Accounts.List().PageSize(c.Max).PageToken(c.Page)` where supported by the Google client. Add tests for first-page omission and explicit `pageToken`.
- Expected impact: complete Business Profile account enumeration becomes reliable for scripts and agency workflows.
- Functional impact category: Data completeness
- Workflow improved: `gog business-profile accounts list` before location, admin, invitation, and account-management operations.
- Verification proof: `go test ./internal/cmd -run BusinessProfileAccounts` with request-query assertions; `gog business-profile accounts list --help` documents `--page` and `--max`.
- Estimated risk: Low
- Safe to automate: Yes

### 4. Shared optional `--out` help text is inaccurate for command-specific outputs

- Priority: Medium
- Why it matters: Output paths are used in document, sheet, slide, Gmail attachment, and Chat media download workflows. The shared help says the default is the gogcli config directory even though several commands derive filenames, use the current directory, or choose command-specific defaults. This can send agents looking in the wrong place after a download/export.
- Exact location:
  - `internal/cmd/flags_output.go:3` through `internal/cmd/flags_output.go:5`
  - Consumers include `internal/cmd/sheets.go`, `internal/cmd/docs.go`, `internal/cmd/slides.go`, `internal/cmd/gmail_attachment.go`, and `internal/cmd/chat_media.go`.
- What is wrong: The help text over-specifies one default for a shared flag that has multiple runtime behaviors.
- Recommended improvement: change the shared optional help to `Output file path (default: command-specific)` or split specialized flag structs where a precise default is important.
- Expected impact: fewer file-recovery mistakes after exports/downloads, with no behavior change.
- Functional impact category: Operator speed
- Workflow improved: Drive/Docs/Sheets/Slides exports and Gmail/Chat attachment downloads.
- Verification proof: `go test ./internal/cmd -run OutputPathFlag` if help tests exist, plus `gog sheets export --help` and `gog gmail attachment download --help`.
- Estimated risk: Low
- Safe to automate: Yes

### 5. README verbose-mode wording promises raw request/response logs

- Priority: Medium
- Why it matters: The README says `--verbose` shows API requests and responses, but the implementation raises slog verbosity and logs retry/service diagnostics, not full HTTP traffic. Adding raw HTTP logs would need a redaction design because this CLI handles OAuth tokens, Gmail bodies, Drive files, and service-account material.
- Exact location:
  - `README.md:1234` through `README.md:1240`
  - `internal/cmd/root.go` global verbose handling
  - `internal/googleapi/transport.go` retry/circuit-breaker logging
- What is wrong: The operator troubleshooting surface overpromises what the flag provides and implies a risky logging mode that does not exist.
- Recommended improvement: update wording to say verbose logging enables debug diagnostics such as retry/service setup logs; do not add raw request/response logging as an automated task.
- Expected impact: reduces false recovery expectations without increasing secret-exposure risk.
- Functional impact category: Operator speed
- Workflow improved: debugging failed Google API calls with `gog --verbose ...`.
- Verification proof: `rg -n "Verbose Mode|request|response|--verbose" README.md`; no code behavior expected.
- Estimated risk: Low
- Safe to automate: No, because it is docs-only and lower functional value than current data-completeness gaps.

### 6. Help rendering still depends on local config and keyring state

- Priority: Low
- Why it matters: Help output is usually expected to be stable anywhere, while `gog --help` includes dynamic config/keyring diagnostics. This can be useful for operators but makes help snapshots machine-dependent.
- Exact location:
  - `internal/cmd/root.go` (`helpDescription`, config path, keyring backend display)
- What is wrong: Dynamic environment data is mixed into general help rather than an explicit diagnostic/doctor command.
- Recommended improvement: consider moving dynamic diagnostics to a dedicated `config doctor` or status command, or make dynamic help diagnostics opt-in.
- Expected impact: more deterministic help and docs/test fixtures if adopted.
- Functional impact category: Reliability
- Workflow improved: scripted help inspection and reproducible CLI documentation generation.
- Verification proof: compare `gog --help` output under different `GOG_KEYRING_BACKEND` values.
- Estimated risk: Medium
- Safe to automate: No, because the current behavior may be an intentional operator convenience.

### 7. Retry exhaustion remains HTTP-native rather than a typed outcome

- Priority: Low
- Why it matters: Retry and circuit-breaker behavior is centralized, but exhausted retryable HTTP responses return as ordinary Google API errors. Agents get less explicit information about whether a command failed after retry exhaustion versus a normal non-retryable API error.
- Exact location:
  - `internal/googleapi/transport.go`
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
- Correct shared optional `--out` help text.

### Larger Refactors

- Move dynamic config/keyring details out of top-level help into a diagnostic command.
- Convert retry exhaustion into a typed error across the shared transport.
- Design safe full HTTP request/response logging with redaction.
- Bulk dependency updates across Google APIs, OAuth, Kong, and `x/*` packages.

## 3. Do Not Change List

- Stable command names and aliases: keep `gmail/mail/email`, `analytics/ga/ga4`, `business-profile/gbp/business`, `bigquery/bq`, and existing service command names because user scripts likely call them.
- Existing `--json` shapes: do not rename `nextPageToken`, `rowCount`, `accounts`, `accountSummaries`, `files`, or resource wrapper keys in small automation PRs.
- Plain/text stdout contract: preserve primary data on stdout and progress/hints on stderr. This follows modern CLI scripting guidance and is already a repo pattern.
- Default first-page behavior: adding page/offset flags should not change current default list/report output.
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
- Do not change:
  - `--page-size` behavior.
  - Output columns.
  - JSON wrapper keys.

### Task 3

- Title: Add pagination controls to Business Profile accounts
- Why: `business-profile accounts list` exposes `nextPageToken` but has no `--page` or size control.
- Files/modules:
  - `internal/cmd/businessprofile_accounts.go`
  - `internal/cmd/businessprofile_accounts_test.go`
- Risk: Low
- Expected impact: complete account discovery for agency/location-group workflows.
- Functional impact category: Data completeness
- Workflow improved: Business Profile account discovery before location/admin operations.
- Verification proof: request-query tests for `pageSize` and `pageToken`.
- Steps:
  1. Add `Max` and `Page` fields to `BusinessProfileAccountsListCmd`.
  2. Build the list call with `PageSize(c.Max)` and optional `PageToken(c.Page)`.
  3. Add tests covering default list behavior and `--page tok --max 25`.
- Validation:
  - `go test ./internal/cmd -run BusinessProfileAccounts`
  - `go test ./...`
  - `go run ./cmd/gog business-profile accounts list --help`
- Do not change:
  - Account create/get/patch/delete behavior.
  - Output table columns.
  - Existing JSON keys.

### Task 4

- Title: Correct optional output-path help
- Why: The shared optional `--out` help points users to the gogcli config dir even when command-specific output defaults apply.
- Files/modules:
  - `internal/cmd/flags_output.go`
  - help/parser tests if present
- Risk: Low
- Expected impact: fewer recovery mistakes after exports/downloads.
- Functional impact category: Operator speed
- Workflow improved: document/sheet/slide exports and attachment downloads.
- Verification proof: command help checks for representative consumers.
- Steps:
  1. Replace the shared optional help with command-specific wording.
  2. Keep `--out` and `--output` parsing unchanged.
  3. Update only tests that assert the help string.
- Validation:
  - `go test ./internal/cmd -run OutputPathFlag`
  - `go run ./cmd/gog sheets export --help`
  - `go run ./cmd/gog gmail attachment download --help`
- Do not change:
  - Output path resolution.
  - Required `--out` behavior.
  - `--out-dir` behavior.

## Final Section

Top 3 Tasks to Execute First:

1. Add offset paging to Analytics reports.
2. Add page-token input to Analytics discovery.
3. Add pagination controls to Business Profile accounts.

Tasks Excluded:

- Task: Re-run the 2026-06-14 Top 3.
  - Reason: Already merged in #32, #33, and #34.
- Task: Narrow README verbose wording.
  - Reason: True but docs-only; lower functional value than current data-completeness tasks.
- Task: Move dynamic config/keyring details out of help.
  - Reason: Could remove intentional operator diagnostics; needs product decision.
- Task: Convert retry exhaustion into typed errors.
  - Reason: Cross-cutting transport semantics across all Google clients.
- Task: Add raw HTTP request/response verbose logging.
  - Reason: Requires redaction design before implementation.
- Task: Bulk dependency updates.
  - Reason: Maintenance batch with broad CI/regression surface, not a focused workflow fix.
