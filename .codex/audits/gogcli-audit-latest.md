# gogcli Audit Report

Date: 2026-05-17
Branch: `ai-audit/gogcli-latest`
Mode: Audit report only; no source changes performed

## Audit Scope

Audited current `origin/main` at `8ecc57c` (`fix(cli): make plain config and policy output TSV`) while keeping the report-only PR branch `ai-audit/gogcli-latest`. The previous audit's `--plain config list` and `--plain policy list` findings are now landed on `main` and were not carried forward as open work.

Commands run:

- `git worktree list`
- `git status --short --branch`
- `git log --oneline --decorate --max-count=20 --all -- .codex/audits/gogcli-audit-latest.md Makefile internal/cmd/root.go internal/cmd/config_cmd.go internal/cmd/policy.go internal/cmd/completion_scripts.go README.md go.mod`
- `git log --oneline --decorate --max-count=12 origin/main`
- `git diff --name-status origin/main...HEAD`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog --version --json`
- `go run ./cmd/gog version --json --plain`
- `go run ./cmd/gog --version --json --plain`
- `go run ./cmd/gog --plain config list`
- `go run ./cmd/gog --plain policy list`
- `go run ./cmd/gog config keys`
- `go run ./cmd/gog completion zsh | sed -n '1,30p'`
- `go list -m -u all`
- `go test ./...`

Web references used for comparison:

- Command Line Interface Guidelines: https://clig.dev/
- Google Cloud SDK scripting guide: https://cloud.google.com/sdk/docs/scripting-gcloud
- Cobra shell completion guide: https://cobra.dev/docs/how-to-guides/shell-completion/
- Heroku CLI style guide: https://devcenter.heroku.com/articles/cli-style-guide

## 1. Prioritized Improvements

### 1. Print invalid output-mode errors to stderr

- Priority: High
- Why it matters: invalid CLI usage should produce a visible `gog` error on stderr. Scripts should be able to trust the non-zero exit code, and humans should not have to infer the cause from an empty stderr stream.
- Exact location:
  - `internal/cmd/root.go:82-86`
  - `internal/cmd/root.go:129-132`
  - `internal/outfmt/outfmt.go:21-24`
  - `cmd/gog/main.go:9-12`
  - `internal/cmd/root_test.go:64-104`
  - `internal/cmd/execute_version_exitcodes_test.go:9-130`
- What is wrong:
  - `outfmt.FromFlags(true, true)` returns `invalid output mode (cannot combine --json and --plain)`.
  - The normal command path wraps the error at `root.go:129-132` and returns without printing it.
  - The global version fast path wraps the error at `root.go:82-86` and also returns without printing it.
  - `main()` only exits with `cmd.ExitCode(err)`, so the direct binary path is silent.
  - Observed commands on `origin/main`:
    - `go run ./cmd/gog version --json --plain` exits 2 and only shows the Go wrapper text `exit status 2`.
    - `go run ./cmd/gog --version --json --plain` has the same behavior.
- Recommended improvement: format and print `newUsageError(err)` to stderr before returning in both output-mode parse paths. Add focused regression coverage that captures stderr for `version --json --plain` and `--version --json --plain`.
- Expected impact: clearer CLI failures, better agent/debug workflows, and consistency with CLI guidance that diagnostics belong on stderr while successful data stays on stdout.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Short-circuit `--version` before building help/config state

- Priority: Medium
- Why it matters: `gog --version` should be one of the cheapest and safest discovery commands. It should not depend on config-path or keyring-backend resolution.
- Exact location:
  - `internal/cmd/root.go:76-88`
  - `internal/cmd/root.go:228-247`
  - `internal/cmd/version.go:35-47`
  - `internal/cmd/execute_version_exitcodes_test.go:9-130`
- What is wrong:
  - `Execute()` calls `newParser(helpDescription())` before checking `hasVersionFlag(args)`.
  - `helpDescription()` reads `config.ConfigPath()` and `secrets.ResolveKeyringBackendInfo()`.
  - As a result, the global `--version` path performs local config/keyring discovery before printing static build metadata.
- Recommended improvement: move the `hasVersionFlag(args)` fast path before `newParser(helpDescription())`. Keep the existing `--` separator behavior tested in `execute_version_exitcodes_test.go`.
- Expected impact: lower setup friction, fewer surprising local-state dependencies for `--version`, and a smaller surface for failures in first-run or restricted environments.
- Estimated risk: Low
- Safe to automate: Yes

### 3. Help generation reads config and keyring state before command execution

- Priority: Medium
- Why it matters: `--help` should be deterministic discovery output. Current help includes machine-specific config and keyring details, so help output varies by environment and depends on local storage resolution.
- Exact location:
  - `internal/cmd/root.go:76-80`
  - `internal/cmd/root.go:228-247`
  - `internal/secrets/store.go` via `secrets.ResolveKeyringBackendInfo()`
  - `internal/cmd/root_test.go:20-37`
- What is wrong:
  - `Execute()` always constructs the parser with `helpDescription()`.
  - `helpDescription()` embeds `Config:\n  file: ...\n  keyring backend: ...` in top-level help.
  - `TestExecute_Help` asserts that config and keyring details are present, locking in the coupling.
  - Observed command: `go run ./cmd/gog --help` prints `/Users/jeremydjohnson/Library/Application Support/gogcli/config.json` and `keyring backend: file (source: config)`.
- Recommended improvement: move local-state details to an explicit diagnostic command or a narrower config/auth help surface, then keep top-level help static and deterministic.
- Expected impact: cleaner help output, lower help latency, fewer environment-specific snapshots in tests and docs.
- Estimated risk: Medium
- Safe to automate: No

### 4. `fmt-check` still installs tools before checking formatting

- Priority: Medium
- Why it matters: `fmt-check` is read-only now, but it still bootstraps formatter binaries on every invocation because `tools` is phony. Repeated verification can trigger unnecessary `go install` work and network/setup friction.
- Exact location:
  - `Makefile:61-65`
  - `Makefile:67-83`
- What is wrong:
  - `tools` is listed as phony and is a prerequisite of `fmt-check`.
  - `fmt-check` therefore mixes verification with tool installation.
  - The command is no longer source-mutating, which is good, but it is still not a pure local check.
- Recommended improvement: split bootstrap from verification with version-stamped tool targets or a missing-tool error that tells contributors to run `make tools`.
- Expected impact: faster local and automation checks after the first setup, fewer network-dependent verification failures.
- Estimated risk: Medium
- Safe to automate: No

### 5. Retry transport has typed retry/quota errors that are not surfaced on retry exhaustion

- Priority: Low
- Why it matters: the codebase defines richer error types, but exhausted retry states return raw HTTP responses. That makes rate-limit and quota observability depend on every caller handling status codes consistently.
- Exact location:
  - `internal/googleapi/transport.go:82-112`
  - `internal/googleapi/errors.go:28-59`
- What is wrong:
  - `RateLimitError` and `QuotaExceededError` exist.
  - `RetryTransport.RoundTrip()` returns the final `429` or `5xx` response with `nil` error after max retries.
  - Only the circuit-breaker state is surfaced as a typed transport error.
- Recommended improvement: decide whether the transport contract should stay HTTP-native or promote exhausted retry states into typed errors, then update callers/tests around that explicit contract.
- Expected impact: clearer retry semantics and better observability for quota/rate-limit failures.
- Estimated risk: Medium
- Safe to automate: No

### 6. Direct and security-sensitive dependencies have newer upstream releases

- Priority: Low
- Why it matters: tests pass, but core parser, Google API, OAuth, crypto, and network dependencies have newer releases available.
- Exact location:
  - `go.mod:5-14`
  - `go.mod:38-48`
- What is wrong:
  - `go list -m -u all` reports updates including `github.com/alecthomas/kong v1.13.0 -> v1.15.0`, `google.golang.org/api v0.260.0 -> v0.279.0`, `golang.org/x/oauth2 v0.34.0 -> v0.36.0`, `golang.org/x/crypto v0.47.0 -> v0.51.0`, `golang.org/x/net v0.49.0 -> v0.54.0`, and `google.golang.org/grpc v1.78.0 -> v1.81.1`.
- Recommended improvement: run a bounded dependency refresh in a dedicated PR with full tests and focused smoke checks for auth, help, parsing, and Google client construction.
- Expected impact: maintenance headroom and current upstream fixes.
- Estimated risk: Medium
- Safe to automate: No

### 7. README's high-level scripting examples still emphasize JSON over `--plain`

- Priority: Low
- Why it matters: `--plain` is now a clearer contract after the latest config/policy fixes, but the top feature summary and primary scripting pattern still point users almost exclusively toward JSON.
- Exact location:
  - `README.md:6`
  - `README.md:30`
  - `README.md:255-260`
  - `README.md:1118-1127`
  - `README.md:1249-1250`
- What is wrong:
  - The detailed Output section documents `--plain`.
  - The high-level feature bullet still says only `JSON mode for scripting and automation`.
  - The "Useful pattern" section only shows `gog --json ... | jq .`, with no `--plain` example for `cut`, `awk`, or shell pipelines.
- Recommended improvement: update the feature bullet and add one short `--plain` pipeline example near the existing JSON scripting example.
- Expected impact: better discoverability for shell users without changing runtime behavior.
- Estimated risk: Low
- Safe to automate: Yes

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Print invalid output-mode errors to stderr and add stderr regression tests.
- Move the global `--version` fast path before parser/help construction.
- Add a small README `--plain` scripting example now that config/policy plain-mode behavior is covered by tests.

### Larger Refactors

- Decouple top-level help generation from config/keyring inspection.
- Separate tool bootstrapping from verification targets in the Makefile.
- Decide and document the retry transport contract before changing typed retry/quota behavior.
- Refresh direct dependencies with full regression and smoke coverage.

## 3. Do Not Change List

- Stdout/stderr split:
  - Keep successful data on stdout and diagnostics/errors/progress on stderr.
  - Why: this is already reflected in `internal/cmd/root.go:105-166`, `internal/cmd/output_helpers.go:13-20`, README output docs, and modern CLI scripting guidance.

- Existing JSON payload shapes:
  - Do not change `version`, `config`, `policy`, auth, or service JSON fields while fixing stderr or version-fast-path behavior.
  - Why: JSON mode is the safest existing automation contract.

- `--plain config list` and `--plain policy list` TSV contracts:
  - Keep the newly landed `KEY\tVALUE` and `NAME\tACCOUNT\tCLIENT\tALLOW\tDENY` shapes.
  - Why: `origin/main` now has tests at `internal/cmd/config_cmd_test.go:108-138` and `internal/cmd/policy_test.go:78-126`.

- Default human text:
  - Preserve default prose/table output unless a separate product decision changes it.
  - Why: the open issues are about machine and discovery paths, not interactive text.

- Mutating formatter command:
  - Keep `make fmt` as the write-mode formatter.
  - Why: `fmt-check` should be verification-only, but the explicit formatter remains useful.

- Completion command and internal `__complete` protocol:
  - Keep `completion <shell>` and `__complete` behavior stable.
  - Why: zsh completion was recently fixed, and shell completion changes should be isolated from this audit's top tasks.

- Config path, credential, token, and keyring storage semantics:
  - Do not change where config, OAuth clients, refresh tokens, or keyring backend settings live during help/version cleanup.
  - Why: auth/config storage is security-sensitive and should not be bundled with discovery-output cleanup.

- Safety policies, `--force`, and `--no-input`:
  - Preserve destructive-command confirmation and persisted policy semantics.
  - Why: these are core agent-safety controls.

## 4. Execution Plan

Only items marked `Safe to automate: Yes` are turned into tasks below.

### Task 1

- Title: Print invalid output-mode errors to stderr
- Why: `--json --plain` currently returns exit code 2 without a visible `gog` error on stderr.
- Files/modules:
  - `internal/cmd/root.go`
  - `internal/cmd/root_test.go`
  - `internal/cmd/execute_version_exitcodes_test.go`
- Risk: Low
- Expected impact: clearer CLI failures and better automation diagnostics.
- Steps:
  1. Print `errfmt.Format(newUsageError(err))` to stderr before returning from the normal `outfmt.FromFlags` error path.
  2. Do the same in the global `--version` fast path when `outputModeFromVersionArgs` returns an error.
  3. Add regression tests for both `gog version --json --plain` and `gog --version --json --plain`.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog version --json --plain`
  - `go run ./cmd/gog --version --json --plain`
- Do not change:
  - Exit code 2 for usage errors
  - Valid `--json` or `--plain` output
  - JSON payload shapes

### Task 2

- Title: Make global `--version` independent of help/config state
- Why: `gog --version` currently builds parser help text, which reads config and keyring state before printing static build metadata.
- Files/modules:
  - `internal/cmd/root.go`
  - `internal/cmd/execute_version_exitcodes_test.go`
- Risk: Low
- Expected impact: lower first-run/setup friction and a safer discovery command for restricted environments.
- Steps:
  1. Move the `hasVersionFlag(args)` fast path before `newParser(helpDescription())`.
  2. Preserve existing `--` separator behavior from `TestExecute_VersionFlag_StopsAtSeparator` and `TestExecute_VersionFlag_ModeStopsAtSeparator`.
  3. Add or adjust a regression test proving global version output still supports `--json` and does not require parser construction.
- Validation:
  - `go test ./internal/cmd -run Version`
  - `go run ./cmd/gog --version`
  - `go run ./cmd/gog --version --json`
  - `go run ./cmd/gog -- --version`
- Do not change:
  - `version` subcommand behavior
  - `--version` precedence before normal command execution
  - Config/keyring storage behavior

### Task 3

- Title: Add a README `--plain` scripting example
- Why: `--plain` is now documented and tested, but the top-level scripting copy still emphasizes JSON almost exclusively.
- Files/modules:
  - `README.md`
- Risk: Low
- Expected impact: better discoverability for shell users and agents using TSV pipelines.
- Steps:
  1. Update the high-level parseable-output feature bullet to mention JSON and stable TSV.
  2. Add one short `--plain` pipeline example near the existing JSON `jq` pattern.
  3. Keep the existing JSON examples and output contract language intact.
- Validation:
  - `rg -n -- '--plain|--json|Parseable output' README.md`
  - `go test ./...`
- Do not change:
  - Runtime behavior
  - Installation instructions
  - OAuth/auth setup steps

## Final Section

Top 3 Tasks to Execute First:

1. Print invalid output-mode errors to stderr.
2. Make global `--version` independent of help/config state.
3. Add a README `--plain` scripting example.

Tasks Excluded:

- Task: Decouple top-level help from config/keyring inspection.
  - Reason: useful, but it changes established help content and the existing test contract; it needs human review.

- Task: Make `fmt-check` independent from tool installation.
  - Reason: Makefile/tool bootstrapping changes are small but affect contributor and CI setup behavior.

- Task: Change retry exhaustion to typed errors.
  - Reason: the transport/caller contract must be decided first.

- Task: Refresh direct and transitive dependencies.
  - Reason: dependency updates need a dedicated PR with broader smoke coverage.
