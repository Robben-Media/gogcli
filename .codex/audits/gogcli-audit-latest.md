# gogcli Audit Report

Date: 2026-06-07
Branch: `ai-audit/gogcli-latest`
Audited baseline: `origin/main` at `8ecc57c` (`fix(cli): make plain config and policy output TSV`)
Mode: Audit report only; no source, test, README, or docs changes performed

## Audit Scope

Reviewed the current CLI across performance, developer experience, CLI usability, code structure, error handling, logging and observability, test coverage, documentation, install/setup friction, and dependency posture.

Commands run:

- `git status --short --branch`
- `git branch --show-current`
- `git diff --name-status origin/main...HEAD`
- `gh pr view 25 --json number,title,state,headRefName,baseRefName,url,labels,mergeStateStatus,changedFiles`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog --version --json --plain`
- `go run ./cmd/gog --color nope version`
- `GOG_JSON=1 GOG_PLAIN=1 go run ./cmd/gog version`
- `go list -m -u all`
- `go test ./...`

Web references used for comparison:

- Command Line Interface Guidelines: <https://clig.dev/>
- Cobra flag guidance: <https://cobra.dev/docs/how-to-guides/working-with-flags/>
- The CLI Spec: <https://clispec.dev/>
- gcloud output formatting: <https://docs.cloud.google.com/sdk/gcloud/reference/topic/formats>
- AWS CLI output formats: <https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-output-format.html>

## 1. Prioritized Improvements

### 1. Pre-UI usage and initialization errors exit without a gog diagnostic

- Priority: High
- Why it matters: CLI tools should put diagnostics on stderr while keeping stdout reserved for command output. `gog` already does this for Kong parse errors and command-run errors, but several root initialization paths return before any formatter writes a message. Scripts then receive only a non-zero exit and no actionable `gog` error.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:82`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:85`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:129`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:131`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:143`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:149`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/outfmt/outfmt.go:21`
- What is wrong:
  - `Execute()` prints formatted stderr for parser, enabled-command, policy, and command-run failures.
  - `outfmt.FromFlags(cli.JSON, cli.Plain)` errors are wrapped with `newUsageError(err)` and returned without stderr output.
  - `outputModeFromVersionArgs()` follows the same silent path for `gog --version --json --plain`.
  - `ui.New()` errors such as invalid `--color` return before a UI exists and are not printed.
  - Reproduction through `go run` showed empty stdout and only Go wrapper text (`exit status 1` or `exit status 2`) on stderr, which means `gog` itself emitted no useful diagnostic.
- Recommended improvement: add a small helper for pre-UI usage/init errors that writes `errfmt.Format(err)` to stderr before returning the wrapped error. Use it for conflicting output modes, version output-mode parsing, and invalid UI color setup.
- Expected impact: clearer CI and scripting failures without changing successful stdout output.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Global `--version` still builds parser/help metadata before short-circuiting

- Priority: High
- Why it matters: `--version` should be cheap and deterministic. Current code creates the Kong parser and builds dynamic help text before checking for `--version`, so a version-only call can still read local config and keyring backend state.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:76`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:77`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:82`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:228`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:231`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:239`
- What is wrong:
  - `Execute()` calls `newParser(helpDescription())` before `hasVersionFlag(args)`.
  - `helpDescription()` calls `config.ConfigPath()` and `secrets.ResolveKeyringBackendInfo()`.
  - The short-circuit avoids command parsing, but not parser construction or local state reads.
- Recommended improvement: move global version-flag handling before parser creation, while preserving current `--` separator behavior and `--json`/`--plain` validation.
- Expected impact: faster and more reliable `gog --version`, especially when local config or keyring setup is damaged.
- Estimated risk: Low
- Safe to automate: Yes

### 3. `calendar colors --plain` ignores the documented TSV contract

- Priority: Medium
- Why it matters: README documents `--plain` as stable TSV for automation. `calendar colors` supports JSON explicitly, but plain mode falls through to human section headings and pretty tabwriter output.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/README.md:257`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/README.md:258`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/calendar_colors.go:34`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/calendar_colors.go:46`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/calendar_colors.go:68`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/output_helpers.go:13`
- What is wrong:
  - `CalendarColorsCmd.Run` checks only `outfmt.IsJSON(ctx)`.
  - Plain mode emits `EVENT COLORS:`, a blank line, `CALENDAR COLORS:`, and aligned tables.
  - The command does not use `tableWriter(ctx)` and has no single stable schema for both color families.
- Recommended improvement: add an `outfmt.IsPlain(ctx)` branch with one stable schema, for example `TYPE\tID\tBACKGROUND\tFOREGROUND`, where `TYPE` is `event` or `calendar`. Preserve current default human output and current JSON output.
- Expected impact: makes an existing read-only command consistent with the automation contract.
- Estimated risk: Low
- Safe to automate: Yes

### 4. Analytics Admin mutation commands emit prose under `--plain`

- Priority: Medium
- Why it matters: Analytics Admin list/get paths already provide TSV-friendly output, but create/delete/patch paths use human prose in all non-JSON modes. That makes automation less reliable exactly where follow-up scripts need created or deleted resource names.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:88`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:93`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:128`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:133`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:280`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:285`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:331`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:336`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:370`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:375`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:513`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/analytics_admin_streams.go:518`
- What is wrong:
  - Data stream `create`, `delete`, and `patch` return JSON only for `--json`; otherwise they print messages like `Created data stream: ...`.
  - Measurement Protocol secret `create`, `delete`, and `patch` follow the same pattern with `Created secret: ...`, `Secret value: ...`, `Deleted secret: ...`, and `Updated secret: ...`.
  - The same file's list/get paths already use table output, so this command family has mixed machine-output behavior.
- Recommended improvement: add explicit `outfmt.IsPlain(ctx)` branches for these six mutation paths with narrow TSV schemas, such as `NAME\tMEASUREMENT_ID` for stream create, `DELETED\tNAME` for deletes, and `NAME\tSECRET_VALUE` for secret create.
- Expected impact: easier chaining after GA4 resource mutations while preserving default human output and existing JSON shapes.
- Estimated risk: Low
- Safe to automate: Yes

### 5. Shared `--out` help text is misleading for several export/download commands

- Priority: Medium
- Why it matters: Clear flag help is part of CLI usability. The shared optional output flag says the default is the gogcli config directory, but not every consumer writes there. For example, Gmail attachments default to the Gmail attachments directory, while Google Workspace exports often derive a file name and path through `exportViaDrive`.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/flags_output.go:3`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/flags_output.go:4`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/sheets.go:50`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/sheets.go:52`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/gmail_attachment.go:19`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/gmail_attachment.go:22`
- What is wrong:
  - `OutputPathFlag` is reused across optional export/download commands.
  - Its help text hard-codes `default: gogcli config dir`, which is too specific for all current call sites.
  - The required output flag and output-dir flag are clearer because their help text reflects their actual semantics.
- Recommended improvement: make the shared optional `--out` help text generic, for example `Output file path (default: command-specific)`, or split the optional output flag into command-specific variants only where help needs to name the default.
- Expected impact: more accurate `--help` output with no runtime behavior change.
- Estimated risk: Low
- Safe to automate: Yes

### 6. README overstates what `--verbose` currently logs

- Priority: Medium
- Why it matters: The docs say verbose mode shows API requests and responses, but the implementation only raises the global slog level. The retry transport logs retry/circuit events; there is no general request/response dumper. Full request/response logging would also be sensitive because this CLI handles OAuth tokens, Gmail bodies, Drive files, and service-account material.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/README.md:1236`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/README.md:1240`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:121`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:125`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:89`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:115`
- What is wrong:
  - `--verbose` sets slog level to debug.
  - The transport logs retry attempts, not every API request and response.
  - The README promise is stronger than the current behavior and stronger than what should be added without a redaction design.
- Recommended improvement: update README wording to say verbose logging enables debug diagnostics such as retry/service setup logs. Do not add raw request/response logging as an automated small task.
- Expected impact: fewer false expectations during troubleshooting, with no security risk.
- Estimated risk: Low
- Safe to automate: Yes

### 7. Help text depends on local config and keyring state

- Priority: Low
- Why it matters: Help should usually be stable and safe to render anywhere. Current root help includes local config and keyring backend details, which is useful diagnostics but couples discovery UX to machine state.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:77`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:228`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:231`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:239`
- What is wrong:
  - `gog --help` varies by machine and can include config/keyring resolution errors in the description block.
  - The behavior may be intentional for this operator-focused CLI, so changing it is not a low-risk automation task.
- Recommended improvement: consider moving dynamic local state to an explicit diagnostics/status command, or make dynamic help diagnostics opt-in.
- Expected impact: more deterministic help output.
- Estimated risk: Medium
- Safe to automate: No

### 8. Retry transport returns raw exhausted responses instead of typed retry outcomes

- Priority: Low
- Why it matters: The repo has retry, circuit-breaker, and error-formatting abstractions. Exhausted 429/5xx retries currently return raw HTTP responses, which leaves higher layers to infer whether a request failed after retry exhaustion from API-specific errors.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:40`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:41`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:83`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:85`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:106`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/googleapi/transport.go:112`
- What is wrong:
  - `CircuitBreakerError` is returned directly when the breaker is open.
  - Exhausted 429 and 5xx retries return `resp, nil`.
  - Changing this affects all Google API clients, so it needs a design decision rather than an automated small PR.
- Recommended improvement: decide whether the transport should stay HTTP-native or promote exhausted retry states into typed errors consistently.
- Expected impact: clearer retry observability if adopted.
- Estimated risk: Medium
- Safe to automate: No

### 9. Direct dependencies have newer releases available

- Priority: Low
- Why it matters: Dependency drift is normal, but this repository touches auth, Google APIs, terminal output, and CLI parsing, so periodic bounded updates are useful.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/go.mod:5`
- What is wrong:
  - `go list -m -u all` reports newer direct or security-adjacent dependencies, including `github.com/alecthomas/kong` `v1.13.0 -> v1.15.0`, `golang.org/x/crypto` `v0.47.0 -> v0.52.0`, `golang.org/x/net` `v0.49.0 -> v0.55.0`, `golang.org/x/oauth2` `v0.34.0 -> v0.36.0`, `golang.org/x/term` `v0.39.0 -> v0.43.0`, and `google.golang.org/api` `v0.260.0 -> v0.283.0`.
- Recommended improvement: handle dependency updates as a dedicated maintenance PR with full `make ci`, not as part of the CLI UX queue.
- Expected impact: keeps parser/API/security-adjacent packages current.
- Estimated risk: Medium
- Safe to automate: No

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Print pre-UI usage/init errors to stderr for conflicting output modes and invalid color mode.
- Short-circuit global `--version` before parser/help config reads.
- Add a stable TSV `--plain` branch for `calendar colors`.
- Add stable TSV `--plain` output for Analytics Admin stream and MP secret mutations.
- Correct generic optional `--out` help copy.
- Correct README verbose-mode wording so it matches current slog-based implementation.

### Larger Refactors

- Move dynamic config/keyring state out of root help or redesign help diagnostics.
- Redesign retry transport semantics around typed exhausted-retry errors.
- Refresh direct dependencies in a maintenance PR with full regression coverage.
- Normalize every remaining prose mutation command under `--plain` in one sweep. The safer path is one command family per PR.

## 3. Do Not Change List

- Stable command names and aliases:
  - `gmail` aliases `mail,email`; `youtube` alias `yt`; `bigquery` alias `bq`; `analytics` aliases `ga,ga4`; `search-console` aliases `gsc,sc`; `tag-manager` alias `gtm`; `business-profile` aliases `gbp,business`.
  - Why: these are user-facing entrypoints and likely embedded in scripts.

- Successful machine output on stdout, hints/errors on stderr:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/output_helpers.go:21`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/README.md:260`
  - Why: this matches established CLI scripting practice and is already documented.

- `--json` as the richest machine-readable mode:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/outfmt/outfmt.go:21`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/root.go:35`
  - Why: changing this would break scripts. Add missing plain branches without changing JSON shapes.

- `--plain` as TSV, not pretty tables:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/README.md:258`
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/output_helpers.go:13`
  - Why: this is the right contract for automation; current issues are command-specific gaps.

- Destructive-operation gates:
  - `/Users/jeremydjohnson/.codex/worktrees/2b04/gogcli/internal/cmd/confirm.go:14`
  - Why: `--force`, `--no-input`, non-interactive refusal, and policy checks are central to safe automation.

- Existing `config list --plain` and `policy list --plain` TSV output:
  - Why: those previous audit tasks are complete on current `main`; do not duplicate them.

- Root help diagnostic block without maintainer approval:
  - Why: local config/keyring details appear intentional for operator support, even though they make help dynamic.

## 4. Task Plan

Task 1:

- Title: Print pre-UI usage errors to stderr
- Why: conflicting output modes and invalid color modes should not fail silently.
- Files/modules:
  - `internal/cmd/root.go`
  - `internal/cmd/execute_version_exitcodes_test.go`
  - `internal/cmd/root_test.go` or a focused root execution test file
- Risk: Low
- Expected impact: clearer CI/script failures for bad global flag combinations.
- Steps:
  1. Add a helper in `internal/cmd/root.go` that writes `errfmt.Format(err)` to `os.Stderr` and returns the original or wrapped error.
  2. Use it for `outputModeFromVersionArgs()` errors, `outfmt.FromFlags()` errors, and `ui.New()` errors before UI initialization.
  3. Add tests for `--version --json --plain`, normal command `--json --plain`, env `GOG_JSON=1 GOG_PLAIN=1`, and `--color nope`.
- Validation:
  - `go test ./internal/cmd`
  - `go test ./...`
  - Manual compiled-binary spot check if desired: `gog --json --plain version` should exit non-zero with a clear stderr message.
- Do not change:
  - Exit code `2` for usage errors.
  - JSON/plain successful output shapes.
  - Existing parse-error formatting.

Task 2:

- Title: Short-circuit global version before parser construction
- Why: `gog --version` should not read config/keyring state or build help metadata.
- Files/modules:
  - `internal/cmd/root.go`
  - `internal/cmd/version_test.go`
  - `internal/cmd/execute_version_exitcodes_test.go`
- Risk: Low
- Expected impact: faster, more deterministic version output in damaged local environments.
- Steps:
  1. Move `hasVersionFlag(args)` and `outputModeFromVersionArgs(args)` handling to the top of `Execute()`, before `newParser(helpDescription())`.
  2. Preserve the existing `--` behavior where `gog -- --version` is not treated as a global version request.
  3. Add or adjust tests that prove `--version`, `--version --json`, `--version --plain`, and conflicting output-mode errors still behave correctly.
- Validation:
  - `go test ./internal/cmd`
  - `go test ./...`
- Do not change:
  - `version` subcommand behavior.
  - `VersionString()` formatting.
  - JSON keys for version output.

Task 3:

- Title: Make `calendar colors --plain` stable TSV
- Why: the command is read-only and currently violates the documented `--plain` TSV contract.
- Files/modules:
  - `internal/cmd/calendar_colors.go`
  - `internal/cmd/calendar_colors_test.go`
- Risk: Low
- Expected impact: better automation ergonomics for color lookup scripts.
- Steps:
  1. Add an `outfmt.IsPlain(ctx)` branch before the human-output branch.
  2. Emit one schema for both color families, for example `TYPE\tID\tBACKGROUND\tFOREGROUND`.
  3. Add a test fixture that verifies ordering and stdout for event and calendar colors in plain mode.
- Validation:
  - `go test ./internal/cmd -run CalendarColors`
  - `go test ./...`
- Do not change:
  - Current JSON payload shape.
  - Current default human headings and pretty table output.
  - Authentication/service construction behavior.

Task 4:

- Title: Add plain TSV for Analytics Admin mutations
- Why: Analytics Admin list/get commands are already script-friendly; create/delete/patch should expose narrow plain output too.
- Files/modules:
  - `internal/cmd/analytics_admin_streams.go`
  - `internal/cmd/analytics_admin_streams_test.go`
- Risk: Low
- Expected impact: resource creation/deletion scripts can parse names and measurement IDs without prose handling.
- Steps:
  1. Add `outfmt.IsPlain(ctx)` branches for data stream create/delete/patch.
  2. Add `outfmt.IsPlain(ctx)` branches for Measurement Protocol secret create/delete/patch.
  3. Add focused tests for the TSV schemas using existing Analytics Admin command test patterns.
- Validation:
  - `go test ./internal/cmd -run AnalyticsAdmin`
  - `go test ./...`
- Do not change:
  - Current human prose output.
  - Current JSON payload shapes.
  - Destructive confirmation behavior.

Task 5:

- Title: Correct optional `--out` help copy
- Why: shared help text currently claims a gogcli config-dir default for commands whose defaults are command-specific.
- Files/modules:
  - `internal/cmd/flags_output.go`
  - `internal/cmd/flags_output_test.go`
- Risk: Low
- Expected impact: more accurate `--help` output with no behavior change.
- Steps:
  1. Replace `OutputPathFlag` help text with generic wording such as `Output file path (default: command-specific)`.
  2. Keep `OutputPathRequiredFlag` and `OutputDirFlag` wording unchanged unless tests show a direct mismatch.
  3. Update help/flag tests to assert the alias still works and the copy is accurate.
- Validation:
  - `go test ./internal/cmd -run OutputPathFlag`
  - `go test ./...`
- Do not change:
  - `--out` and `--output` aliases.
  - Runtime output path defaults.

Task 6:

- Title: Correct README verbose logging claim
- Why: docs should not promise raw API requests/responses when the implementation only enables debug logging.
- Files/modules:
  - `README.md`
- Risk: Low
- Expected impact: safer and more accurate troubleshooting expectations.
- Steps:
  1. Replace the `# Shows API requests and responses` comment with wording about debug diagnostics such as retry/service setup logs.
  2. Avoid promising raw HTTP logging unless a redaction design exists.
  3. Keep the example command unchanged.
- Validation:
  - `rg -n "requests and responses|verbose" README.md`
  - `go test ./...`
- Do not change:
  - CLI verbose implementation.
  - Logging of sensitive request/response payloads.

## 5. Final Section

Top 3 Tasks to Execute First:

1. Print pre-UI usage errors to stderr
2. Short-circuit global version before parser construction
3. Make `calendar colors --plain` stable TSV

Tasks Excluded:

- Task: Move dynamic config/keyring state out of root help
  - Reason: likely product/UX decision; current behavior may be intentional diagnostics.

- Task: Redesign retry transport exhausted-retry behavior
  - Reason: changes cross-service error semantics and needs maintainer design review.

- Task: Update direct dependencies
  - Reason: maintenance PR with broader regression surface; not a narrow CLI usability task.

- Task: Repo-wide plain-output normalization
  - Reason: too broad for a single safe automation PR; use one command family per PR.

- Task: Add full API request/response verbose logging
  - Reason: high sensitivity and requires redaction design for OAuth tokens, message bodies, Drive files, and service-account material.
