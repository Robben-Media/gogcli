# gogcli Audit Report

Date: 2026-05-31
Branch: `ai-audit/gogcli-latest`
Audited baseline: `origin/main` at `8ecc57c` (`fix(cli): make plain config and policy output TSV`)
Mode: Audit report only; no source, test, README, or docs changes performed

## Audit Scope

Reviewed current CLI behavior and code across root flag parsing, output formatting, help/version paths, Google API retry behavior, install/build targets, tests, docs, and dependency posture.

Commands run:

- `git status --short --branch`
- `git worktree list --porcelain`
- `gh pr view 25 --json number,title,state,headRefName,baseRefName,url,labels,mergeStateStatus,changedFiles`
- `gh pr diff 25 --name-only`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog --version --json --plain`
- `go run ./cmd/gog --color nope version`
- `GOG_JSON=1 GOG_PLAIN=1 go run ./cmd/gog version`
- `go run ./cmd/gog config list --plain`
- `go run ./cmd/gog policy list --plain`
- `go list -m -u all`
- `go test ./...`

Web references used for comparison:

- [Command Line Interface Guidelines](https://clig.dev/) - primary command output should go to stdout, diagnostics to stderr, and JSON should be available for scripting.
- [gcloud CLI overview](https://docs.cloud.google.com/sdk/gcloud) - successful command output is written to stdout, stderr is not stable for scripting, and output formats support machine-readable use.
- [Scripting gcloud CLI commands](https://docs.cloud.google.com/sdk/docs/scripting-gcloud) - predictable output formats are important for automation.
- [GNU Coding Standards](https://www.gnu.org/prep/standards/standards.html) - standard `--help` and `--version` behavior should be stable and broadly supported.

## 1. Prioritized Improvements

### 1. Usage-mode initialization errors exit silently

- Priority: High
- Why it matters: the CLI already improves Kong parse errors by printing formatted diagnostics to stderr, but later root initialization errors return directly to `main()`. In the compiled binary that means scripts get a non-zero exit with no actionable error text.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:82`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:129`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:143`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:250`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/outfmt/outfmt.go:21`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/ui/ui.go:35`
- What is wrong:
  - `Execute()` prints parse, enabled-command, policy, and command-run errors through `errfmt.Format`.
  - `outfmt.FromFlags(cli.JSON, cli.Plain)` errors are wrapped with `newUsageError(err)` and returned without stderr output.
  - `outputModeFromVersionArgs()` follows the same silent path for `gog --version --json --plain`.
  - `ui.New()` parse errors such as invalid `--color` are also returned before a UI exists and are not printed.
  - Reproduction through `go run` shows only Go's wrapper text (`exit status 1` or `exit status 2`), which indicates `gog` itself did not emit a useful diagnostic.
- Recommended improvement: add a small helper for pre-UI usage/init errors that writes `errfmt.Format(err)` to stderr before returning the wrapped error. Cover `--json` plus `--plain`, `--version --json --plain`, and invalid `--color` with focused tests.
- Expected impact: much better failure ergonomics for scripts and CI without changing successful command output.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Global `--version` still pays parser/help config cost before short-circuiting

- Priority: High
- Why it matters: `--version` should be one of the cheapest and most deterministic CLI paths. Current code creates the full Kong parser with `helpDescription()` before checking for `--version`, so even version-only invocations read local config/keyring backend state.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:76`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:82`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:228`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:239`
- What is wrong:
  - `Execute()` calls `newParser(helpDescription())` before `hasVersionFlag(args)`.
  - `helpDescription()` calls `config.ConfigPath()` and `secrets.ResolveKeyringBackendInfo()`.
  - The current short-circuit avoids command parsing, but not parser construction or help-description state reads.
- Recommended improvement: move the global version-flag check before parser creation, while preserving the existing `--` separator behavior and `--json`/`--plain` mode validation.
- Expected impact: faster, more deterministic `gog --version`, especially in damaged config/keyring environments.
- Estimated risk: Low
- Safe to automate: Yes

### 3. `calendar colors --plain` ignores the advertised TSV contract

- Priority: Medium
- Why it matters: README and root help promise `--plain` as stable TSV. `calendar colors` supports JSON explicitly, but plain mode falls through to human section headings and tabwriter output.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/README.md:257`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/calendar_colors.go:34`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/calendar_colors.go:46`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/calendar_colors.go:68`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/output_helpers.go:13`
- What is wrong:
  - `CalendarColorsCmd.Run` checks only `outfmt.IsJSON(ctx)`.
  - Plain mode emits `EVENT COLORS:`, a blank line, `CALENDAR COLORS:`, and aligned tables.
  - The command does not use `tableWriter(ctx)`, so `--plain` does not become raw TSV.
- Recommended improvement: add a plain-mode branch with one stable schema, for example `TYPE\tID\tBACKGROUND\tFOREGROUND`, where `TYPE` is `event` or `calendar`. Preserve current human output for default mode and JSON output for `--json`.
- Expected impact: makes a small existing command consistent with the documented automation contract.
- Estimated risk: Low
- Safe to automate: Yes

### 4. Analytics Admin mutation commands emit prose under `--plain`

- Priority: Medium
- Why it matters: Analytics Admin has good TSV list/get output, but create/delete/patch paths use prose for non-JSON modes. That makes `--plain` less reliable for automation exactly where follow-up scripts often need resource names.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/analytics_admin_streams.go:88`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/analytics_admin_streams.go:92`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/analytics_admin_streams.go:128`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/analytics_admin_streams.go:331`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/analytics_admin_streams.go:370`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/analytics_admin_streams.go:513`
- What is wrong:
  - Data stream `create`, `delete`, and `patch` return JSON only for `--json`; otherwise they print messages like `Created data stream: ...`.
  - Measurement Protocol secret `create`, `delete`, and `patch` do the same with `Created secret: ...`, `Secret value: ...`, `Deleted secret: ...`, and `Updated secret: ...`.
  - The file's list/get paths already use tables, so the command family has mixed output contracts.
- Recommended improvement: add explicit `outfmt.IsPlain(ctx)` branches for the six mutation paths with narrow TSV shapes, such as `NAME\tMEASUREMENT_ID` for stream create, `DELETED\tNAME` for deletes, and `NAME\tSECRET_VALUE` for secret create.
- Expected impact: easier chaining after GA4 resource creation/deletion while preserving default human output.
- Estimated risk: Low
- Safe to automate: Yes

### 5. README overstates what `--verbose` currently logs

- Priority: Medium
- Why it matters: docs say verbose mode shows API requests and responses, but current verbose implementation only raises the global slog level. The retry transport logs retry/circuit events, and service creation logs a few debug lines, but there is no general request/response dumper.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/README.md:1236`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/README.md:1240`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:121`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/googleapi/transport.go:89`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/googleapi/transport.go:115`
- What is wrong:
  - `--verbose` sets slog level to debug.
  - The transport logs retry attempts, not every API request/response.
  - Full request/response logging would be sensitive because this CLI handles OAuth tokens, message bodies, Drive files, and service-account material.
- Recommended improvement: update the README claim to match current behavior: verbose logging enables debug diagnostics such as retries/service setup. Do not add raw request/response logging as a small automated task.
- Expected impact: fewer false expectations during troubleshooting, with no security risk.
- Estimated risk: Low
- Safe to automate: Yes

### 6. Help text depends on local config/keyring state

- Priority: Low
- Why it matters: help should be safe and deterministic. Current root help includes local config path and keyring backend source, which is useful diagnostics but couples discovery UX to local state.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:77`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:228`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:231`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:239`
- What is wrong:
  - `gog --help` varies by machine and can include `error: ...` in the config block if config/backend resolution fails.
  - That may be intentional for this operator-focused CLI, but it is broader than a low-risk formatting fix.
- Recommended improvement: consider moving dynamic local state to an explicit diagnostics/status command, or gate it behind a help mode. Leave this out of automation unless maintainers approve the UX change.
- Expected impact: more deterministic help output.
- Estimated risk: Medium
- Safe to automate: No

### 7. Retry transport returns raw exhausted responses instead of typed retry outcomes

- Priority: Low
- Why it matters: the repo has retry, circuit-breaker, and error-formatting abstractions. Exhausted 429/5xx retries currently flow upward as raw HTTP responses, which leaves higher layers to infer what happened from API errors.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/googleapi/transport.go:40`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/googleapi/transport.go:83`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/googleapi/transport.go:111`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/googleapi/errors.go`
- What is wrong:
  - `CircuitBreakerError` is emitted directly when the breaker is open.
  - Exhausted 429 and 5xx retries return `resp, nil`.
  - Changing this affects all Google API clients, so it needs design review.
- Recommended improvement: decide whether the transport should stay HTTP-native or promote exhausted retry states into typed errors consistently.
- Expected impact: clearer retry observability if adopted.
- Estimated risk: Medium
- Safe to automate: No

### 8. Direct dependencies have newer releases available

- Priority: Low
- Why it matters: dependency drift is normal, but the repo touches auth, Google APIs, terminal output, and CLI parsing, so periodic bounded updates are useful.
- Exact location:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/go.mod:5`
- What is wrong:
  - `go list -m -u all` reports updates for direct dependencies including `github.com/alecthomas/kong` (`v1.13.0` to `v1.15.0`), `golang.org/x/net` (`v0.49.0` to `v0.55.0`), `golang.org/x/oauth2` (`v0.34.0` to `v0.36.0`), `golang.org/x/term` (`v0.39.0` to `v0.43.0`), and `google.golang.org/api` (`v0.260.0` to `v0.282.0`).
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
- Correct README verbose-mode wording so it matches the current slog-based implementation.

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
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/output_helpers.go:21`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/README.md:260`
  - Why: this matches established CLI scripting practice and is already documented.

- `--json` as the richest machine-readable mode:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/outfmt/outfmt.go:21`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/root.go:35`
  - Why: changing this would break scripts. Add missing plain branches without changing JSON shapes.

- `--plain` as TSV, not pretty tables:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/README.md:258`
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/output_helpers.go:13`
  - Why: this is the right contract for automation; current issues are command-specific gaps.

- Destructive-operation gates:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/internal/cmd/confirm.go:14`
  - Why: `--force`, `--no-input`, non-interactive refusal, and policy checks are central to safe automation.

- Existing non-mutating `fmt-check` target:
  - `/Users/jeremydjohnson/.codex/worktrees/8495/gogcli/Makefile:71`
  - Why: it is already fixed on current `main`; do not re-open or replace it.

- Current `config list --plain` and `policy list --plain` TSV output:
  - Verified outputs: `KEY\tVALUE` for config and `NAME\tACCOUNT\tCLIENT\tALLOW\tDENY` for policy.
  - Why: those previous audit tasks are complete on current `main`; do not duplicate them.

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
  - `internal/cmd/execute_version_exitcodes_test.go`
- Risk: Low
- Expected impact: faster and more reliable version checks in broken local-config environments.
- Steps:
  1. Move the `hasVersionFlag(args)` branch before `newParser(helpDescription())`.
  2. Preserve `--` separator handling and current `outputModeFromVersionArgs()` parsing.
  3. Add a regression test that makes config/keyring resolution observable, or minimally verifies the early version path still works with `--json`, `--plain=false`, and `--`.
- Validation:
  - `go test ./internal/cmd -run Version`
  - `go test ./...`
- Do not change:
  - `gog version` subcommand behavior.
  - Existing version JSON keys: `version`, `commit`, `date`.
  - The rule that `--` stops global version-flag detection.

Task 3:

- Title: Make `calendar colors --plain` TSV
- Why: the command currently emits human section headings under `--plain`, which violates the documented plain-output contract.
- Files/modules:
  - `internal/cmd/calendar_colors.go`
  - `internal/cmd/calendar_colors_test.go`
- Risk: Low
- Expected impact: makes color IDs easy to script and consistent with other list/get commands.
- Steps:
  1. Add an `outfmt.IsPlain(ctx)` branch after JSON handling.
  2. Emit `TYPE\tID\tBACKGROUND\tFOREGROUND`, with rows for both event and calendar colors.
  3. Add a regression test that invokes `--plain` and asserts no `EVENT COLORS:` / `CALENDAR COLORS:` headings are present.
- Validation:
  - `go test ./internal/cmd -run CalendarColors`
  - `go test ./...`
- Do not change:
  - Current `--json` object shape.
  - Current default human output.
  - Sort order within event and calendar color groups.

Task 4:

- Title: Add plain TSV for Analytics Admin mutations
- Why: create/delete/patch commands need parseable resource names when `--plain` is requested.
- Files/modules:
  - `internal/cmd/analytics_admin_streams.go`
  - `internal/cmd/analytics_admin_streams_test.go`
- Risk: Low
- Expected impact: scripts can safely consume created/deleted/updated GA4 stream and MP secret identifiers.
- Steps:
  1. Add `outfmt.IsPlain(ctx)` branches for data-stream create/delete/patch.
  2. Add `outfmt.IsPlain(ctx)` branches for MP secret create/delete/patch.
  3. Extend existing Analytics Admin tests to cover plain output schemas.
- Validation:
  - `go test ./internal/cmd -run 'AA(DataStreams|MpSecrets)'`
  - `go test ./...`
- Do not change:
  - Current JSON payload keys.
  - Confirmation requirements for destructive commands.
  - Default human prose output.

Task 5:

- Title: Correct verbose-mode README wording
- Why: README currently promises full API request/response logging that the code does not implement and should not add casually.
- Files/modules:
  - `README.md`
- Risk: Low
- Expected impact: reduces troubleshooting confusion and avoids encouraging unsafe raw API logging.
- Steps:
  1. Replace `# Shows API requests and responses` with wording that matches debug retry/service diagnostics.
  2. Keep the `gog --verbose ...` example.
  3. Do not add new verbose behavior in code.
- Validation:
  - Markdown-only review.
  - Optional `rg -n "API requests and responses|verbose" README.md`.
- Do not change:
  - CLI flags.
  - Logging implementation.
  - Any API request/response visibility behavior.

## Final Section

Top 3 Tasks to Execute First:

1. Print pre-UI usage errors to stderr.
2. Short-circuit global version before parser construction.
3. Make `calendar colors --plain` TSV.

Tasks Excluded:

- Task: Move dynamic config/keyring state out of root help.
  - Reason: user-facing discovery UX change; needs maintainer approval.
- Task: Redesign retry transport to return typed exhausted-retry errors.
  - Reason: cross-cutting Google API behavior change; not safe as a small automated PR.
- Task: Refresh direct dependencies.
  - Reason: maintenance sweep with compatibility risk; should be a dedicated PR.
- Task: Normalize all remaining prose mutation outputs under `--plain`.
  - Reason: too broad for one low-risk PR; use command-family slices instead.
- Task: Rework full request/response verbose logging.
  - Reason: security-sensitive due OAuth tokens, email bodies, file contents, and service-account material.

## Current Verification

- `go test ./...` passed on current `main` worktree at `8ecc57c`.
- Existing PR #25 is open from `ai-audit/gogcli-latest` to `main`.
- `gh pr diff 25 --name-only` reports only `.codex/audits/gogcli-audit-latest.md`.
