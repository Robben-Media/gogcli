# gogcli Audit Report

Date: 2026-05-24
Branch: `ai-audit/gogcli-latest`
Mode: Audit report only; no source, test, README, or documentation changes performed

## Audit Scope

Audited current `origin/main` at `8ecc57c` (`fix(cli): make plain config and policy output TSV`). The report branch is intentionally limited to `.codex/audits/gogcli-audit-latest.md`; command evidence was gathered from a sibling detached worktree at `origin/main` because the shared audit branch still carries only the report-only PR diff.

The previous audit's `--plain config list` and empty `--plain policy list` tasks are now landed on `main` and are excluded from the open task queue.

Audit areas covered:

1. Performance
2. Developer Experience (DX)
3. CLI Usability
4. Code Structure & Modularity
5. Error Handling
6. Logging & Observability
7. Test Coverage
8. Documentation
9. Install / Setup Friction
10. Security / Dependencies

Commands and evidence used:

- `git status --short --branch`
- `git worktree list --porcelain`
- `git log --graph --oneline --decorate --all -12`
- `git diff --name-status origin/main...HEAD`
- `git show origin/main:internal/cmd/root.go`
- `git show origin/main:internal/cmd/config_cmd.go`
- `git show origin/main:internal/cmd/policy.go`
- `git show origin/main:internal/cmd/execute_version_exitcodes_test.go`
- `git show origin/main:internal/cmd/root_test.go`
- `git show origin/main:internal/cmd/root_more_test.go`
- `git show origin/main:internal/googleapi/transport.go`
- `git show origin/main:internal/googleapi/errors.go`
- `git show origin/main:Makefile`
- `git show origin/main:README.md`
- `git show origin/main:go.mod`
- `go run ./cmd/gog --version --json`
- `go run ./cmd/gog --plain config list`
- `go run ./cmd/gog --plain policy list`
- `go run ./cmd/gog version --json --plain`
- `go run ./cmd/gog --version --json --plain`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog config keys`
- `go list -m -u all`
- `go test ./...`

Validation result:

- `go test ./...` passed on `origin/main` worktree `8ecc57c`.

Web references used for comparison:

- Command Line Interface Guidelines: https://clig.dev/
- Rain's Rust CLI recommendations, machine-readable output: https://rust-cli-recommendations.sunshowers.io/machine-readable-output.html
- Google Cloud SDK scripting guide: https://docs.cloud.google.com/sdk/docs/scripting-gcloud
- Microsoft System.CommandLine design guidance: https://learn.microsoft.com/en-us/dotnet/standard/commandline/design-guidance

Relevant best-practice anchors:

- CLI Guidelines recommends non-zero exit codes for failures, primary output on stdout, and diagnostics/errors on stderr.
- Machine-readable output guidance recommends stable stdout output and disabling colors for machine-readable modes.
- Google Cloud's scripting guidance emphasizes predictable formatted output and exit status for automation.

## 1. Prioritized Improvements

### 1. Print invalid output-mode errors to stderr

- Priority: High
- Why it matters: invalid CLI usage should produce a visible `gog` diagnostic on stderr. Scripts can use the exit code, but humans and agents still need the reason for the failure.
- Exact location:
  - `internal/cmd/root.go:82-86`
  - `internal/cmd/root.go:129-132`
  - `internal/outfmt/outfmt.go:21-24`
  - `cmd/gog/main.go:9-12`
  - `internal/cmd/root_test.go:64-104`
  - `internal/cmd/execute_version_exitcodes_test.go:9-148`
- What is wrong:
  - `outfmt.FromFlags(true, true)` returns `invalid output mode (cannot combine --json and --plain)`.
  - The normal command path wraps that error at `root.go:129-132` and returns without printing it.
  - The global version fast path wraps the same class of error at `root.go:82-86` and returns without printing it.
  - `main()` only maps the error to an exit code, so the compiled binary path would exit with code 2 without the underlying usage message.
  - Observed on `origin/main`:
    - `go run ./cmd/gog version --json --plain` prints only Go's wrapper text `exit status 2`, exits from `go run` with code 1, and emits no `gog` error body.
    - `go run ./cmd/gog --version --json --plain` has the same visible behavior.
- Recommended improvement: format and print `newUsageError(err)` to stderr before returning in both output-mode parse paths. Add focused regression coverage that captures stderr for `version --json --plain` and `--version --json --plain`.
- Expected impact: clearer CLI failures, better scripted diagnostics, and consistency with the existing stdout/stderr contract.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Short-circuit global `--version` before building help/config state

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
  - As a result, `gog --version` performs local config/keyring discovery before printing static build metadata.
  - Observed on `origin/main`: `go run ./cmd/gog --version --json` succeeds, but the code path still constructs help/config state first.
- Recommended improvement: move the `hasVersionFlag(args)` fast path before `newParser(helpDescription())`. Preserve the existing `--` separator behavior covered by `TestExecute_VersionFlag_StopsAtSeparator` and `TestExecute_VersionFlag_ModeStopsAtSeparator`.
- Expected impact: lower setup friction, fewer local-state dependencies for first-run discovery, and a smaller failure surface in restricted environments.
- Estimated risk: Low
- Safe to automate: Yes

### 3. Top-level help generation reads config and keyring state

- Priority: Medium
- Why it matters: `--help` is discovery output. It should be deterministic and cheap enough to run in any environment, including CI, docs generation, and restricted agent sandboxes.
- Exact location:
  - `internal/cmd/root.go:76-80`
  - `internal/cmd/root.go:228-247`
  - `internal/secrets/store.go` through `secrets.ResolveKeyringBackendInfo()`
  - `internal/cmd/root_test.go:20-37`
  - `internal/cmd/root_more_test.go:58-71`
- What is wrong:
  - `Execute()` always constructs the parser with `helpDescription()`.
  - `helpDescription()` embeds machine-specific config and keyring details in top-level help.
  - `TestExecute_Help` and `TestHelpDescription` assert that this local-state block is present, locking in the coupling.
  - Observed on `origin/main`: `go run ./cmd/gog --help` prints `/Users/jeremydjohnson/Library/Application Support/gogcli/config.json` and `keyring backend: file (source: config)`.
- Recommended improvement: move local-state details to an explicit diagnostic command or a narrower config/auth help surface. Keep top-level help static and deterministic.
- Expected impact: cleaner help output, faster help generation, and fewer environment-specific snapshots in tests and docs.
- Estimated risk: Medium
- Safe to automate: No

### 4. README scripting examples understate `--plain` after TSV fixes

- Priority: Medium
- Why it matters: `--plain` is now a stronger automation contract after the current `config` and `policy` TSV fixes, but the high-level docs still make JSON feel like the only scripting path.
- Exact location:
  - `README.md:6`
  - `README.md:30`
  - `README.md:255-260`
  - `README.md:1118-1127`
- What is wrong:
  - The Output section documents `--plain`.
  - The high-level feature bullet still says `JSON mode for scripting and automation`.
  - The "Useful pattern" section only shows `gog --json ... | jq .`; there is no short `--plain` pipeline example for shell tools.
  - Observed on `origin/main`:
    - `go run ./cmd/gog --plain config list` prints `KEY\tVALUE` rows.
    - `go run ./cmd/gog --plain policy list` prints `NAME\tACCOUNT\tCLIENT\tALLOW\tDENY` even when empty.
- Recommended improvement: update the feature bullet to mention JSON and TSV/plain output, then add one concise `--plain` example near the existing JSON pipeline.
- Expected impact: better discoverability for shell users without any runtime behavior change.
- Estimated risk: Low
- Safe to automate: Yes

### 5. `fmt-check` still bootstraps tools before checking formatting

- Priority: Medium
- Why it matters: `fmt-check` is now read-only, but it still invokes the phony `tools` target. Repeated verification can trigger unnecessary `go install` work and network/setup friction.
- Exact location:
  - `Makefile:61-65`
  - `Makefile:67-83`
  - `Makefile:98`
- What is wrong:
  - `tools` is phony and is a prerequisite of `fmt-check`.
  - `fmt-check` therefore mixes verification with tool installation.
  - The command is no longer source-mutating, which is good, but it is still not a pure local check.
- Recommended improvement: split bootstrap from verification with version-stamped tool targets, or make `fmt-check` fail with a clear "run make tools" message when pinned tools are missing.
- Expected impact: faster local and automation checks after initial setup, fewer network-dependent verification failures.
- Estimated risk: Medium
- Safe to automate: No

### 6. Retry transport has typed retry/quota errors that are not surfaced after exhaustion

- Priority: Low
- Why it matters: the codebase defines richer error types, but exhausted retry states return raw HTTP responses. That makes rate-limit and quota observability depend on every caller handling status codes consistently.
- Exact location:
  - `internal/googleapi/errors.go:28-59`
  - `internal/googleapi/errors.go:96-112`
  - `internal/googleapi/transport.go:82-112`
  - `internal/googleapi/transport_test.go:101-135`
  - `internal/googleapi/transport_more_test.go:127-155`
- What is wrong:
  - `RateLimitError` and `QuotaExceededError` exist.
  - `RetryTransport.RoundTrip()` returns the final `429` or `5xx` response with `nil` error after max retries.
  - Tests currently assert the raw response behavior, so this is an explicit contract decision rather than a one-line bug.
- Recommended improvement: decide whether the transport should stay HTTP-native or promote exhausted retry states into typed errors. If changing, update callers and tests around that explicit contract.
- Expected impact: clearer retry semantics and better observability for quota/rate-limit failures.
- Estimated risk: Medium
- Safe to automate: No

### 7. Direct and security-sensitive dependencies have newer upstream releases

- Priority: Low
- Why it matters: current tests pass, but core parser, Google API, OAuth, crypto, and network dependencies have newer upstream releases available.
- Exact location:
  - `go.mod:5-14`
  - `go.mod:16-48`
- What is wrong:
  - `go list -m -u all` reports updates including:
    - `github.com/alecthomas/kong v1.13.0 -> v1.15.0`
    - `golang.org/x/crypto v0.47.0 -> v0.52.0`
    - `golang.org/x/net v0.49.0 -> v0.55.0`
    - `golang.org/x/oauth2 v0.34.0 -> v0.36.0`
    - `golang.org/x/term v0.39.0 -> v0.43.0`
    - `google.golang.org/api v0.260.0 -> v0.280.0`
    - `google.golang.org/grpc v1.78.0 -> v1.81.1`
- Recommended improvement: run a bounded dependency refresh in a dedicated PR with full tests and focused smoke checks for auth, help, parsing, and Google client construction.
- Expected impact: maintenance headroom and current upstream fixes.
- Estimated risk: Medium
- Safe to automate: No

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Print invalid output-mode errors to stderr and add stderr regression tests.
- Move the global `--version` fast path before parser/help construction.
- Add one short README `--plain` scripting example now that config/policy plain-mode behavior is tested.

### Larger Refactors

- Decouple top-level help generation from config/keyring inspection.
- Separate tool bootstrapping from verification targets in the Makefile.
- Decide and document the retry transport contract before changing typed retry/quota behavior.
- Refresh direct and security-sensitive dependencies with full regression and smoke coverage.

## 3. Do Not Change List

- Stdout/stderr split:
  - Keep successful data on stdout and diagnostics/errors/progress on stderr.
  - Why: this is already reflected in `internal/cmd/root.go:105-166`, `internal/cmd/output_helpers.go:13-20`, README output docs, and modern CLI scripting guidance.

- Existing JSON payload shapes:
  - Do not change `version`, `config`, `policy`, auth, or service JSON fields while fixing stderr or version-fast-path behavior.
  - Why: JSON mode is the safest existing automation contract.

- `--plain config list` and `--plain policy list` TSV contracts:
  - Keep the landed `KEY\tVALUE` and `NAME\tACCOUNT\tCLIENT\tALLOW\tDENY` shapes.
  - Why: `origin/main` has tests at `internal/cmd/config_cmd_test.go:108-138` and `internal/cmd/policy_test.go:78-126`.

- Default human text:
  - Preserve default prose/table output unless a separate product decision changes it.
  - Why: the open runtime issues are about machine/discovery paths, not interactive text.

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
- Why: `--json --plain` currently returns a usage exit without a visible `gog` error on stderr.
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
  3. Add or adjust a regression test proving global version output still supports `--json` without requiring parser/help construction first.
- Validation:
  - `go test ./internal/cmd -run Version`
  - `go run ./cmd/gog --version`
  - `go run ./cmd/gog --version --json`
  - `go run ./cmd/gog -- --version`
- Do not change:
  - `version` subcommand behavior
  - `--version` precedence before normal command execution
  - Config/keyring storage semantics

### Task 3

- Title: Document `--plain` as a first-class scripting path
- Why: `--plain` now provides stable TSV for important config/policy commands, but README examples still steer scripting users almost exclusively to JSON.
- Files/modules:
  - `README.md`
- Risk: Low
- Expected impact: better discoverability for shell pipelines and less unnecessary JSON parsing in simple scripts.
- Steps:
  1. Update the feature bullet from JSON-only wording to JSON plus plain/TSV parseable output.
  2. Add one short `--plain` example near the existing `gog --json ... | jq .` useful pattern.
  3. Keep the existing Output section wording intact unless a tiny cross-reference is needed.
- Validation:
  - `go test ./...`
  - Manual README review for accurate command names and no behavior claims beyond current CLI output.
- Do not change:
  - Runtime behavior
  - Existing JSON examples
  - README install/auth guidance

## Final Section

### Top 3 Tasks to Execute First

1. Print invalid output-mode errors to stderr.
2. Make global `--version` independent of help/config state.
3. Document `--plain` as a first-class scripting path.

### Tasks Excluded

- Task: Decouple top-level help from config/keyring state.
  - Reason: Useful, but it changes an asserted help contract and should get human review.

- Task: Split `fmt-check` from tool bootstrapping.
  - Reason: Build-tooling behavior can affect contributor setup and CI assumptions; not a safe blind automation task.

- Task: Change exhausted retry states to typed errors.
  - Reason: Existing tests assert raw response behavior, so this needs a transport contract decision first.

- Task: Refresh direct/security-sensitive dependencies.
  - Reason: Dependency updates are valuable but require a dedicated compatibility pass and possibly OAuth/API smoke checks.
