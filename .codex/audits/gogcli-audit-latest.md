# gogcli Audit Report

Date: 2026-05-10
Branch: `ai-audit/gogcli-latest`
Mode: Audit report only; no source changes performed

## Audit Scope

Reviewed current `origin/main` (`fbc9eb2`) after re-anchoring the stale local audit branch so the eventual PR can remain report-only. The previous Top 3 audit tasks are already landed on `main`:

- `d66bd36` / PR #20: `fmt-check` is now read-only.
- `98b7013` / PR #21: global `--version --json` now emits JSON.
- `6cf9a9e` / PR #22: zsh completion no longer embeds the Bash shebang.

Commands run:

- `git status --short --branch`
- `git fetch origin main --prune`
- `git diff --name-status origin/main...HEAD`
- `git log --oneline --decorate --max-count=20 --all -- Makefile internal/cmd/root.go internal/cmd/version.go internal/cmd/completion_scripts.go internal/cmd/completion_test.go .codex/audits/gogcli-audit-latest.md`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog --version --json`
- `go run ./cmd/gog version --json --plain`
- `go run ./cmd/gog --plain config list`
- `go run ./cmd/gog --plain policy list`
- `go run ./cmd/gog completion zsh | sed -n '1,24p'`
- `go list -m -u all`
- `go test ./...`

Web references used for comparison:

- Cobra shell completion guide: https://cobra.dev/docs/how-to-guides/shell-completion/
- Google Cloud SDK scripting guide: https://cloud.google.com/sdk/docs/scripting-gcloud
- Command Line Interface Guidelines: https://clig.dev/

## 1. Prioritized Improvements

### 1. Print output-mode usage errors to stderr

- Priority: High
- Why it matters: invalid CLI usage should be visible without relying on the caller to print returned errors. Scripts should be able to trust the non-zero exit code, and humans should see a clear reason on stderr.
- Exact location:
  - `internal/cmd/root.go:82-85`
  - `internal/cmd/root.go:129-131`
  - `internal/outfmt/outfmt.go:21-24`
  - `cmd/gog/main.go:9-12`
- What is wrong:
  - `outfmt.FromFlags(true, true)` returns `invalid output mode (cannot combine --json and --plain)`.
  - `Execute()` wraps that as exit code 2 but returns before the UI/stderr formatter is available.
  - `main()` only exits with `cmd.ExitCode(err)`, so the direct binary path can fail silently.
  - Observed command: `go run ./cmd/gog version --json --plain` exits 2; the only visible text is the Go tool wrapper's `exit status 2`, not a `gog` error message.
- Recommended improvement: format and print `newUsageError(err)` to stderr before returning in both the global version fast path and normal output-mode parse path. Add coverage that captures stderr for `--json --plain`.
- Expected impact: better CLI usability, easier agent/debug workflows, and a consistent usage-error surface.
- Estimated risk: Low
- Safe to automate: Yes

### 2. `--plain config list` violates the advertised TSV contract

- Priority: Medium
- Why it matters: root help advertises `--plain` as stable, parseable TSV. `config list` is a common setup/diagnostic command, and its current plain output still uses prose labels and colon separators.
- Exact location:
  - `internal/cmd/root.go:36`
  - `internal/cmd/config_cmd.go:128-149`
  - `internal/cmd/config_cmd_test.go:10-88`
- What is wrong:
  - Observed command: `go run ./cmd/gog --plain config list`
  - Current output includes `Config file: ...` and `timezone: ...` lines.
  - JSON parity is tested, but there is no plain-mode schema test for `config list`.
- Recommended improvement: add an explicit plain-mode branch that emits a stable TSV shape, such as `KEY\tVALUE` rows plus a separate `path` row or a documented `SOURCE\tKEY\tVALUE` schema. Preserve default human text.
- Expected impact: simpler scripting around config inspection without breaking default text output.
- Estimated risk: Low
- Safe to automate: Yes

### 3. `--plain policy list` has a prose empty state

- Priority: Medium
- Why it matters: policy state is part of the agent-safety surface. Automation should not need a special parser for the empty state.
- Exact location:
  - `internal/cmd/policy.go:98-120`
  - `internal/cmd/policy_test.go:10-64`
- What is wrong:
  - Observed command: `go run ./cmd/gog --plain policy list`
  - Empty state prints `No policies`.
  - Non-empty text uses `tableWriter(ctx)`, so plain mode is TSV-like once rows exist; only the empty path breaks the contract.
- Recommended improvement: when `outfmt.IsPlain(ctx)` and no policies exist, emit the same header with zero data rows or emit no rows, and test that behavior. Preserve `No policies` for default human text.
- Expected impact: less brittle safety-policy automation.
- Estimated risk: Low
- Safe to automate: Yes

### 4. Help generation reads config and keyring state before command execution

- Priority: Medium
- Why it matters: `--help` should be the safest discovery command. Today it reads local config/keyring backend state before parsing any command, which makes help output machine-specific and couples discovery UX to config health.
- Exact location:
  - `internal/cmd/root.go:76-80`
  - `internal/cmd/root.go:231-247`
  - `internal/secrets/store.go` via `secrets.ResolveKeyringBackendInfo()`
  - `internal/cmd/root_test.go:19-31`
- What is wrong:
  - `Execute()` always calls `newParser(helpDescription())`.
  - `helpDescription()` calls `config.ConfigPath()` and `secrets.ResolveKeyringBackendInfo()`.
  - The test currently asserts config/keyring details are present in help, locking in the coupling.
- Recommended improvement: move local state details to an explicit diagnostics command or lazy help section, then keep baseline help static and deterministic.
- Expected impact: lower help latency, less surprising help output, and fewer failure modes for first-time users.
- Estimated risk: Medium
- Safe to automate: No

### 5. Tool installation is coupled to every formatter check

- Priority: Medium
- Why it matters: `fmt-check` is now read-only, but it still depends on the phony `tools` target. That means a verification command can trigger `go install` for formatter/linter binaries before it checks formatting.
- Exact location:
  - `Makefile:61-65`
  - `Makefile:71-83`
- What is wrong:
  - `tools` is phony and always runs when invoked as a prerequisite.
  - `fmt-check` therefore mixes tool bootstrapping with verification.
- Recommended improvement: split tool bootstrapping from checks with file targets or version-stamped binaries, or add a lightweight missing-tool check that tells contributors to run `make tools`.
- Expected impact: faster local verification and less network/setup friction in repeated automation runs.
- Estimated risk: Medium
- Safe to automate: No

### 6. Retry transport returns raw exhausted retry responses despite typed retry errors

- Priority: Low
- Why it matters: richer error types exist, but the transport mostly returns raw HTTP responses after retry exhaustion. That makes retry/quota observability depend on each caller decoding status codes consistently.
- Exact location:
  - `internal/googleapi/transport.go:82-112`
  - `internal/googleapi/errors.go:28-59`
- What is wrong:
  - `RateLimitError` and `QuotaExceededError` exist.
  - `RetryTransport.RoundTrip()` returns the final `429` or `5xx` response with `nil` error after max retries.
  - Only the circuit-breaker state is surfaced as a typed transport error.
- Recommended improvement: decide whether the transport contract should remain HTTP-native or promote exhausted retry states into typed errors, then wire callers/tests around that explicit contract.
- Expected impact: clearer retry semantics and better observability for quota/rate-limit failures.
- Estimated risk: Medium
- Safe to automate: No

### 7. Direct and security-sensitive dependencies have newer upstream releases

- Priority: Low
- Why it matters: no breakage was observed, but the CLI depends on parser, Google API, OAuth, and crypto/network packages that have newer versions available.
- Exact location:
  - `go.mod:5-17`
- What is wrong:
  - `go list -m -u all` reports updates including `github.com/alecthomas/kong v1.13.0 -> v1.15.0`, `google.golang.org/api v0.260.0 -> v0.278.0`, `golang.org/x/oauth2 v0.34.0 -> v0.36.0`, `golang.org/x/crypto v0.47.0 -> v0.51.0`, and `golang.org/x/net v0.49.0 -> v0.54.0`.
- Recommended improvement: run a bounded dependency refresh in a dedicated PR with full tests and focused smoke checks for auth/help/parsing.
- Expected impact: maintenance headroom and current upstream fixes.
- Estimated risk: Medium
- Safe to automate: No

### 8. README documents parseable output as JSON-only

- Priority: Low
- Why it matters: the CLI help advertises both JSON and plain TSV modes, but the README feature list only calls out JSON mode. Users may miss the lower-friction TSV mode for shell pipelines.
- Exact location:
  - `README.md:6`
  - `README.md:30`
  - `internal/cmd/root.go:35-36`
- What is wrong:
  - README says `JSON-first output` and `Parseable output - JSON mode for scripting and automation`.
  - It does not mention `--plain`, despite the root help promising stable parseable TSV.
- Recommended improvement: add a small README example for `--plain` after the plain-mode behavior is made consistent for config/policy surfaces.
- Expected impact: clearer documentation for shell users without changing runtime behavior.
- Estimated risk: Low
- Safe to automate: Yes

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Print invalid output-mode errors to stderr and add stderr regression coverage.
- Add `--plain config list` TSV output while preserving default text and JSON.
- Make `--plain policy list` empty output machine-parseable while preserving default text.
- Add README mention/examples for `--plain` after the behavior gaps are fixed.

### Larger Refactors

- Decouple top-level help generation from config/keyring inspection.
- Separate tool bootstrapping from verification targets in the Makefile.
- Decide and document the retry transport contract before changing typed retry/quota errors.
- Refresh direct dependencies with full regression and smoke coverage.

## 3. Do Not Change List

- Stdout/stderr split:
  - Keep successful data on stdout and errors/hints/progress on stderr.
  - Why: this matches the scripting guidance from gcloud and clig.dev and is already reflected in `internal/cmd/root.go:105-166` plus `internal/cmd/output_helpers.go:13-20`.

- Existing JSON payload shapes:
  - Do not change `version`, `config`, or `policy` JSON fields while fixing plain-mode behavior.
  - Why: JSON mode is the safest existing automation contract.

- Default human text for config and policy commands:
  - Keep prose output for default mode unless a separate product decision changes it.
  - Why: the proposed fixes should target `--plain`, not surprise interactive users.

- Mutating `fmt` target:
  - Keep `make fmt` as the write-mode formatter.
  - Why: PR #20 already made `fmt-check` read-only; the explicit formatter command remains useful.

- Completion command and internal `__complete` protocol:
  - Keep `completion <shell>` and `__complete` behavior stable.
  - Why: zsh completion was fixed in PR #22, and shell-specific changes should not replace the internal completion contract.

- Config path/keyring behavior:
  - Do not change where config, credentials, tokens, or keyring backend settings live during help cleanup.
  - Why: auth/config storage is security-sensitive and should not be bundled with help text refactoring.

- Safety policies, `--force`, and `--no-input`:
  - Preserve destructive-command confirmation and persisted policy semantics.
  - Why: these are core agent-safety controls.

## 4. Execution Plan

Only items marked `Safe to automate: Yes` are turned into tasks below.

### Task 1

- Title: Print invalid output-mode errors to stderr
- Why: `--json --plain` currently returns exit code 2 without a `gog` stderr explanation on the direct binary path.
- Files/modules:
  - `internal/cmd/root.go`
  - `internal/cmd/root_test.go` or a focused execute test file
  - `cmd/gog/main.go` only if the cleaner fix belongs at the process boundary
- Risk: Low
- Expected impact: clearer CLI failures and better automation diagnostics.
- Steps:
  1. Route `outfmt.FromFlags` errors through the same stderr formatter used for parse and command errors.
  2. Cover both `gog version --json --plain` and `gog --version --json --plain`, because the global version fast path has separate parsing.
  3. Assert exit code 2, empty stdout, and non-empty stderr containing the invalid mode message.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog version --json --plain`
  - `go run ./cmd/gog --version --json --plain`
- Do not change:
  - Exit code 2 for usage errors
  - Valid `--json` or `--plain` output

### Task 2

- Title: Add TSV output for `--plain config list`
- Why: `config list` currently emits prose labels under `--plain`, despite the root help promising stable TSV.
- Files/modules:
  - `internal/cmd/config_cmd.go`
  - `internal/cmd/config_cmd_test.go`
- Risk: Low
- Expected impact: scriptable config inspection without special colon parsing.
- Steps:
  1. Add an `outfmt.IsPlain(ctx)` branch before default text output.
  2. Emit a simple stable schema and document it in the test name/assertions.
  3. Add tests proving default text and JSON stay unchanged.
- Validation:
  - `go test ./internal/cmd -run Config`
  - `go run ./cmd/gog --plain config list`
  - `go run ./cmd/gog --json config list`
- Do not change:
  - `config get`, `config keys`, `config path`
  - JSON field names
  - Default human text mode

### Task 3

- Title: Make empty `--plain policy list` parseable
- Why: `policy list` emits TSV when rows exist but prints `No policies` for the empty plain-mode state.
- Files/modules:
  - `internal/cmd/policy.go`
  - `internal/cmd/policy_test.go`
- Risk: Low
- Expected impact: predictable safety-policy automation in empty and non-empty states.
- Steps:
  1. Branch on `outfmt.IsPlain(ctx)` before printing the default empty-state prose.
  2. Emit the same TSV header with zero rows, or another deliberately documented stable empty representation.
  3. Add tests for empty plain mode and one-policy plain mode.
- Validation:
  - `go test ./internal/cmd -run Policy`
  - `go run ./cmd/gog --plain policy list`
  - `go run ./cmd/gog policy list`
- Do not change:
  - JSON list shape
  - Default empty-state text
  - Policy creation/deletion semantics

### Task 4

- Title: Document `--plain` after plain-mode gaps are fixed
- Why: the README currently advertises parseable output as JSON-only even though the CLI has a TSV-oriented `--plain` mode.
- Files/modules:
  - `README.md`
- Risk: Low
- Expected impact: better discoverability for shell users.
- Steps:
  1. Wait until Tasks 2 and 3 are landed.
  2. Add one concise README mention/example for `--plain`.
  3. Keep the example on a command whose plain output is stable.
- Validation:
  - Markdown review
  - Run the documented example locally if it does not require auth
- Do not change:
  - Install instructions
  - Auth workflow instructions
  - Claims about unsupported commands

## Final Section

Top 3 Tasks to Execute First:

1. Print invalid output-mode errors to stderr.
2. Add TSV output for `--plain config list`.
3. Make empty `--plain policy list` parseable.

Tasks Excluded:

- Task: Decouple help generation from config/keyring inspection.
  - Reason: Medium-risk UX and test-contract change; needs product decision on whether config diagnostics belong in help.
- Task: Separate tool bootstrapping from verification targets.
  - Reason: Makefile/DX behavior change could disrupt contributors; should be designed with maintainer preference.
- Task: Promote exhausted retry responses to typed errors.
  - Reason: Cross-cutting transport contract decision; not safe as an automated cleanup.
- Task: Refresh dependencies.
  - Reason: Requires compatibility review across parser, auth, Google API, and transitive packages.
- Task: Replace `--json`/`--plain` with a single `--output` flag.
  - Reason: Unnecessary breaking CLI change; current flags are stable and only need targeted consistency fixes.
