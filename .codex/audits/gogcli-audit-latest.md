# gogcli Audit Report

Date: 2026-06-14
Branch: `ai-audit/gogcli-latest`
Audited baseline: `origin/main` at `9722c31` (`fix(calendar): stabilize colors plain output (#30)`)
Mode: Audit report only; no source, test, README, or docs changes performed

## Audit Scope

Reviewed the current CLI across performance, developer experience, CLI usability, code structure, error handling, logging and observability, test coverage, documentation, install/setup friction, and dependency posture.

Commands run:

- `git status --short --branch`
- `git log --oneline --decorate --max-count=5`
- `git diff --name-status origin/main...HEAD`
- `gh pr list --head ai-audit/gogcli-latest --base main --json number,title,state,url,headRefName,baseRefName,labels`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog --version --json --plain`
- `go run ./cmd/gog --color nope version`
- `GOG_JSON=1 GOG_PLAIN=1 go run ./cmd/gog version`
- `go run ./cmd/gog config list --plain`
- `go run ./cmd/gog policy list --plain`
- `go run ./cmd/gog version --json`
- `go run ./cmd/gog analytics admin data-streams list --help`
- `go run ./cmd/gog bigquery datasets --help`
- `go run ./cmd/gog bigquery tables --help`
- `go list -m -u all`
- `go test ./...`

Web references used for comparison:

- Command Line Interface Guidelines: <https://clig.dev/>
- GNU command-line standards: <https://www.gnu.org/prep/standards/standards.html>
- gcloud output formatting: <https://docs.cloud.google.com/sdk/gcloud/reference/topic/formats>
- AWS CLI output formats: <https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-output-format.html>

Current-state notes:

- Prior Top 3 tasks from the 2026-06-07 audit have landed in `origin/main`: pre-UI diagnostics (#28), global `--version` short-circuiting (#29), and `calendar colors --plain` TSV output (#30).
- `go test ./...` passes on the audited baseline.
- `config list --plain`, empty `policy list --plain`, and `version --json` now produce parseable output.

## 1. Prioritized Improvements

### 1. BigQuery paginated list commands expose next tokens without accepting them back

- Priority: High
- Why it matters: A CLI should make suggested follow-up commands executable. `gog` prints a next-page hint for BigQuery list commands, and JSON output includes `nextPageToken`, but the command help exposes no page-token input. Users can discover that another page exists but cannot fetch it through the CLI.
- Exact location:
  - `internal/cmd/bigquery.go:111` (`BigqueryDatasetsCmd`)
  - `internal/cmd/bigquery.go:159` (`printNextPageHint(u, resp.NextPageToken)` for datasets)
  - `internal/cmd/bigquery.go:165` (`BigqueryTablesCmd`)
  - `internal/cmd/bigquery.go:218` (`printNextPageHint(u, resp.NextPageToken)` for tables)
  - `internal/cmd/bigquery.go:281` (`BigqueryJobsCmd`)
  - `internal/cmd/bigquery.go:348` (`printNextPageHint(u, resp.NextPageToken)` for jobs)
  - `internal/cmd/output_helpers.go:21` (`printNextPageHint`)
- What is wrong:
  - `BigqueryDatasetsCmd`, `BigqueryTablesCmd`, and `BigqueryJobsCmd` have `Max` but no `Page`/`PageToken` field.
  - Their run paths return `nextPageToken` in JSON and print `# Next page: --page <token>` in human/plain modes.
  - `go run ./cmd/gog bigquery datasets --help` and `go run ./cmd/gog bigquery tables --help` show no `--page` or `--page-token` flag.
- Recommended improvement: add `Page string 'name:"page" help:"Page token"'` to the three BigQuery list commands and pass it to `Datasets.List(...).PageToken(c.Page)`, `Tables.List(...).PageToken(c.Page)`, and `Jobs.List(...).PageToken(c.Page)`. Add tests that assert the outbound request includes `pageToken=...`.
- Expected impact: fixes a broken pagination workflow without changing first-page output.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Analytics Admin mutation commands still emit prose under `--plain`

- Priority: High
- Why it matters: The root flag promises stable, parseable text for `--plain`. gcloud and AWS both treat output format as a scripting contract, and `gog` already follows that pattern across many read commands. Analytics Admin mutations are exactly the kind of commands automation chains need to parse after creation, deletion, or patching.
- Exact location:
  - `internal/cmd/analytics_admin_streams.go:88` through `internal/cmd/analytics_admin_streams.go:97` (`AADataStreamsCreateCmd.Run`)
  - `internal/cmd/analytics_admin_streams.go:128` through `internal/cmd/analytics_admin_streams.go:134` (`AADataStreamsDeleteCmd.Run`)
  - `internal/cmd/analytics_admin_streams.go:280` through `internal/cmd/analytics_admin_streams.go:286` (`AADataStreamsPatchCmd.Run`)
  - `internal/cmd/analytics_admin_streams.go:331` through `internal/cmd/analytics_admin_streams.go:338` (`AAMpSecretsCreateCmd.Run`)
  - `internal/cmd/analytics_admin_streams.go:370` through `internal/cmd/analytics_admin_streams.go:376` (`AAMpSecretsDeleteCmd.Run`)
  - `internal/cmd/analytics_admin_streams.go:513` through `internal/cmd/analytics_admin_streams.go:519` (`AAMpSecretsPatchCmd.Run`)
  - `internal/cmd/analytics_admin_streams_test.go` (JSON tests exist; no `--plain` coverage)
- What is wrong:
  - Data stream create/delete/patch return JSON for `--json`; every non-JSON mode prints prose such as `Created data stream: ...`.
  - Measurement Protocol secret create/delete/patch do the same with `Created secret: ...`, `Secret value: ...`, and deletion/update messages.
  - List/get commands in the same file already use TSV-style table output, so the command family is inconsistent.
- Recommended improvement: add explicit `outfmt.IsPlain(ctx)` branches for those six mutation paths. Use narrow TSV schemas, for example `NAME\tMEASUREMENT_ID` for stream create, `DELETED\tNAME` for deletes, `NAME\tSECRET_VALUE` for secret create, and `NAME` for patch results. Preserve default human prose and existing JSON shapes.
- Expected impact: makes GA4 Admin automation easier and aligns mutation output with the root `--plain` contract.
- Estimated risk: Low
- Safe to automate: Yes

### 3. Analytics Admin pagination hints name the wrong flag

- Priority: Medium
- Why it matters: Helpful next-step hints should match the command surface. The shared helper prints `--page`, but Analytics Admin data-stream and Measurement Protocol secret list commands expose `--page-token`.
- Exact location:
  - `internal/cmd/output_helpers.go:21` through `internal/cmd/output_helpers.go:25`
  - `internal/cmd/analytics_admin_streams.go:186` (`PageToken string 'name:"page-token"'`)
  - `internal/cmd/analytics_admin_streams.go:238` (`printNextPageHint(u, resp.NextPageToken)`)
  - `internal/cmd/analytics_admin_streams.go:422` (`PageToken string 'name:"page-token"'`)
  - `internal/cmd/analytics_admin_streams.go:470` (`printNextPageHint(u, resp.NextPageToken)`)
- What is wrong:
  - `go run ./cmd/gog analytics admin data-streams list --help` shows `--page-token=STRING`.
  - The shared hint would still print `# Next page: --page <token>` for any non-empty Analytics Admin response.
  - The existing helper tests intentionally lock in `--page`, which is correct for most commands but not for these two.
- Recommended improvement: either add `aliases:"page"` to both Analytics Admin `PageToken` fields, or make the helper accept the displayed flag name and update only these two call sites to print `--page-token`. The alias approach is the smallest compatibility-preserving fix; the helper-parameter approach is clearer but touches more tests.
- Expected impact: users can copy the next-page hint without translating the flag name.
- Estimated risk: Low
- Safe to automate: Yes

### 4. Shared optional `--out` help text is inaccurate for several commands

- Priority: Medium
- Why it matters: Flag help is part of the CLI contract. The current shared optional output flag says the default is the gogcli config directory even when the command derives a file name, uses an attachment default, or delegates to an export helper.
- Exact location:
  - `internal/cmd/flags_output.go:3` through `internal/cmd/flags_output.go:5`
  - `internal/cmd/sheets.go:52`
  - `internal/cmd/docs.go:47`
  - `internal/cmd/slides.go:24`
  - `internal/cmd/gmail_attachment.go:22`
  - `internal/cmd/chat_media.go:134`
- What is wrong:
  - `OutputPathFlag` hard-codes `Output file path (default: gogcli config dir)`.
  - The same embedded flag is used by sheet/doc/slide exports, Gmail attachment download, and Chat media download.
  - Required output and output-directory flags have more accurate wording.
- Recommended improvement: change the shared optional help to a generic phrase such as `Output file path (default: command-specific)` or split specialized flag structs only where a precise default is important.
- Expected impact: clearer `--help` output with no runtime behavior change.
- Estimated risk: Low
- Safe to automate: Yes

### 5. README overstates what `--verbose` logs

- Priority: Medium
- Why it matters: The README says verbose mode shows API requests and responses, but the implementation only raises the global slog level. Full request/response logging would need a redaction design because this CLI handles OAuth tokens, Gmail bodies, Drive files, and service-account material.
- Exact location:
  - `README.md:1231` through `README.md:1240`
  - `internal/cmd/root.go:121` through `internal/cmd/root.go:126`
  - `internal/googleapi/transport.go:89` through `internal/googleapi/transport.go:115`
- What is wrong:
  - `--verbose` sets slog to debug.
  - The retry transport logs retry and circuit-breaker events, not all API requests/responses.
  - The docs promise more than the code safely provides.
- Recommended improvement: update README wording to say verbose logging enables debug diagnostics such as retry/service setup logs. Do not add raw HTTP logging as an automated small task.
- Expected impact: fewer false troubleshooting expectations and no additional secret exposure.
- Estimated risk: Low
- Safe to automate: Yes

### 6. Help rendering still depends on local config and keyring state

- Priority: Low
- Why it matters: Help output should normally be stable and safe to render anywhere. `gog` currently includes local config and keyring backend details in the top-level description, which is useful but makes `--help` machine-dependent.
- Exact location:
  - `internal/cmd/root.go:76` through `internal/cmd/root.go:78`
  - `internal/cmd/root.go:228` through `internal/cmd/root.go:247`
- What is wrong:
  - `helpDescription()` calls `config.ConfigPath()` and `secrets.ResolveKeyringBackendInfo()`.
  - `gog --help` varies by machine and can include local resolution errors in the description block.
  - This may be intentional for an operator-focused Google CLI, so it is not a safe automatic change.
- Recommended improvement: consider moving dynamic environment diagnostics to an explicit `gog config doctor` or status command, or make dynamic help diagnostics opt-in.
- Expected impact: more deterministic help output.
- Estimated risk: Medium
- Safe to automate: No

### 7. Retry exhaustion remains HTTP-native rather than a typed outcome

- Priority: Low
- Why it matters: Retry and circuit-breaker behavior is centralized, but exhausted 429/5xx retries return the final HTTP response with no typed retry-exhausted signal. Higher layers then rely on Google API error handling to infer what happened.
- Exact location:
  - `internal/googleapi/transport.go:35` through `internal/googleapi/transport.go:113`
- What is wrong:
  - An open circuit returns `CircuitBreakerError`.
  - Exhausted 429/5xx retries return `resp, nil`.
  - Changing this affects every Google API client and needs a design decision about compatibility with Google client internals.
- Recommended improvement: decide whether retry exhaustion should remain HTTP-native or be promoted into typed errors with consistent user-facing formatting.
- Expected impact: clearer observability if adopted.
- Estimated risk: Medium
- Safe to automate: No

### 8. Direct and security-adjacent dependencies have newer releases available

- Priority: Low
- Why it matters: Dependency drift is normal, but this CLI depends on Google APIs, OAuth, terminal handling, Kong parsing, and security-adjacent Go packages.
- Exact location:
  - `go.mod`
- What is wrong:
  - `go list -m -u all` reports newer direct or security-adjacent versions, including `github.com/alecthomas/kong v1.13.0 -> v1.15.0`, `golang.org/x/crypto v0.47.0 -> v0.53.0`, `golang.org/x/net v0.49.0 -> v0.56.0`, `golang.org/x/oauth2 v0.34.0 -> v0.36.0`, `golang.org/x/term v0.39.0 -> v0.44.0`, and `google.golang.org/api v0.260.0 -> v0.284.0`.
- Recommended improvement: handle dependency updates as a dedicated maintenance PR with `make ci` and smoke checks for auth/help/version output.
- Expected impact: keeps parser/API/security-adjacent packages current.
- Estimated risk: Medium
- Safe to automate: No

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Add `--page` support to BigQuery datasets/tables/jobs list commands.
- Add explicit `--plain` TSV branches for Analytics Admin data-stream and Measurement Protocol secret mutation commands.
- Make Analytics Admin next-page hints executable by supporting or printing the right page-token flag.
- Correct shared optional `--out` help text.
- Correct README `--verbose` wording.

### Larger Refactors

- Moving dynamic config/keyring state out of help and into a diagnostic command.
- Changing retry exhaustion from raw HTTP responses to typed errors.
- Batching dependency updates across Google API, OAuth, and `x/*` modules.
- Designing safe full HTTP request/response logging with redaction.

## 3. Do Not Change List

- Stable top-level commands and aliases: `gmail/mail/email`, `analytics/ga/ga4`, `search-console/gsc/sc`, `tag-manager/gtm`, `business-profile/gbp/business`, `bigquery/bq`, and existing Google service command names. These are user-facing workflows and likely scripted.
- Output mode contract: keep `--json` and `--plain` as separate global booleans and preserve stdout for primary output. Scripts depend on these modes.
- Existing JSON shapes: do not rename keys such as `nextPageToken`, `dataStream`, `measurementProtocolSecret`, `deleted`, or existing resource wrappers in small automation PRs.
- Default human output: keep readable prose/tables for non-JSON, non-plain modes unless the specific task is to fix misleading or broken output.
- Config and keyring behavior: do not alter config path resolution, keyring backend selection, OAuth client selection, or token storage semantics as part of UX cleanup.
- Destructive-command safeguards: keep `--force`, `--no-input`, and confirmation behavior intact for delete/update flows.
- README examples and existing documented command names: docs wording can be corrected, but do not redesign the command vocabulary in an audit-driven PR.

## 4. Execution Plan

Convert only `Safe to automate: Yes` items into independent PR tasks.

### Task 1

- Title: Add BigQuery page-token input flags
- Why: BigQuery datasets/tables/jobs expose `nextPageToken` and print next-page hints but cannot accept the token back.
- Files/modules:
  - `internal/cmd/bigquery.go`
  - `internal/cmd/bigquery_test.go`
- Risk: Low
- Expected impact: fixes a broken pagination loop without changing first-page behavior.
- Steps:
  1. Add `Page string 'name:"page" help:"Page token"'` to `BigqueryDatasetsCmd`, `BigqueryTablesCmd`, and `BigqueryJobsCmd`.
  2. Pass the field to each corresponding Google API list call with `.PageToken(c.Page)` when non-empty or directly in the call chain.
  3. Add tests that run each command with `--page tok` and assert the test server receives `pageToken=tok`.
- Validation:
  - `go test ./internal/cmd -run Bigquery`
  - `go test ./...`
  - `go run ./cmd/gog bigquery datasets --help`
- Do not change:
  - JSON output shapes.
  - Human/TSV columns.
  - BigQuery query/schema command behavior.

### Task 2

- Title: Make Analytics Admin mutations plain-parseable
- Why: Six Analytics Admin mutation paths print prose under `--plain`, violating the root scripting contract.
- Files/modules:
  - `internal/cmd/analytics_admin_streams.go`
  - `internal/cmd/analytics_admin_streams_test.go`
- Risk: Low
- Expected impact: enables reliable parsing of created, deleted, and updated GA4 Admin resources.
- Steps:
  1. Add `outfmt.IsPlain(ctx)` branches after existing JSON branches in data-stream create/delete/patch.
  2. Add matching `outfmt.IsPlain(ctx)` branches in Measurement Protocol secret create/delete/patch.
  3. Add focused tests for `--plain` output schemas using the existing `analyticsAdminStreamsTestServer`.
- Validation:
  - `go test ./internal/cmd -run 'AA(DataStreams|MpSecrets)'`
  - `go test ./...`
- Do not change:
  - Existing JSON response wrappers.
  - Default human prose.
  - Confirmation requirements for delete commands.

### Task 3

- Title: Fix Analytics Admin next-page hints
- Why: The commands expose `--page-token`, but the shared hint tells users to run `--page`.
- Files/modules:
  - `internal/cmd/analytics_admin_streams.go`
  - `internal/cmd/analytics_admin_streams_test.go`
  - optionally `internal/cmd/output_helpers.go` if choosing a helper-parameter approach
- Risk: Low
- Expected impact: makes copied pagination hints executable.
- Steps:
  1. Choose the smaller compatibility path: add `aliases:"page"` to both Analytics Admin `PageToken` fields, or update the helper to accept a displayed flag name.
  2. Add tests with non-empty `nextPageToken` responses that verify the suggested or accepted flag works.
  3. Confirm `analytics admin data-streams list --help` still clearly documents page-token usage.
- Validation:
  - `go test ./internal/cmd -run AnalyticsAdmin`
  - `go run ./cmd/gog analytics admin data-streams list --help`
  - `go test ./...`
- Do not change:
  - Existing `--page-token` flag support.
  - JSON `nextPageToken` keys.
  - Pagination behavior for commands that already use `--page`.

### Task 4

- Title: Correct optional output-path help
- Why: The shared `--out` help names the gogcli config directory even when consumers use command-specific defaults.
- Files/modules:
  - `internal/cmd/flags_output.go`
  - `internal/cmd/flags_output_test.go`
- Risk: Low
- Expected impact: clearer help text with no runtime behavior change.
- Steps:
  1. Replace the shared optional help string with generic command-specific wording.
  2. Keep `--out` and `--output` parsing unchanged.
  3. Update or add help/parser tests only if they assert the help string.
- Validation:
  - `go test ./internal/cmd -run OutputPathFlag`
  - `go run ./cmd/gog sheets export --help`
  - `go test ./...`
- Do not change:
  - Output path resolution.
  - Required `--out` help.
  - `--out-dir` behavior.

### Task 5

- Title: Narrow README verbose wording
- Why: Documentation promises request/response logging that the implementation does not provide and should not add without redaction.
- Files/modules:
  - `README.md`
- Risk: Low
- Expected impact: removes a misleading troubleshooting claim.
- Steps:
  1. Replace the verbose-mode comment with wording that matches debug retry/service diagnostics.
  2. Avoid promising raw request/response logging.
  3. Keep the example command unchanged.
- Validation:
  - `rg -n "Verbose Mode|request|response|--verbose" README.md`
- Do not change:
  - CLI behavior.
  - Logging implementation.
  - Auth or token handling.

## Final Section

Top 3 Tasks to Execute First:

1. Add BigQuery page-token input flags.
2. Make Analytics Admin mutations plain-parseable.
3. Fix Analytics Admin next-page hints.

Tasks Excluded:

- Task: Move dynamic config/keyring details out of help.
  - Reason: Potentially intentional operator diagnostics; needs product decision.
- Task: Convert retry exhaustion into typed errors.
  - Reason: Cross-cutting transport semantics; could affect every Google client.
- Task: Bulk dependency updates.
  - Reason: Maintenance batch with broader CI surface, not a low-risk CLI UX PR.
- Task: Add full API request/response verbose logging.
  - Reason: Requires secret/body redaction design before implementation.
- Task: Re-run prior Top 3 from 2026-06-07.
  - Reason: Those items are already merged in #28, #29, and #30.
