# gogcli Audit Report

Date: 2026-05-03
Branch: `ai-audit/gogcli-latest`
Mode: Audit report only; no source changes performed

## Audit Scope

Reviewed the CLI entrypoint, root flag handling, completion generation, config/policy output paths, retry/error helpers, build/test targets, and current dependency posture.

Commands run:

- `git status --short --branch`
- `git worktree list --porcelain`
- `go test ./...`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog --version --json`
- `go run ./cmd/gog version --json`
- `go run ./cmd/gog completion zsh`
- `go run ./cmd/gog --plain config list`
- `go run ./cmd/gog --plain config keys`
- `go run ./cmd/gog --plain policy list`
- `go list -m -u all`

Web references used for comparison:

- [Cobra shell completion guide](https://cobra.dev/docs/how-to-guides/shell-completion/)
- [gcloud CLI overview: stdout/stderr and prompting](https://docs.cloud.google.com/sdk/gcloud)
- [GitHub CLI manual](https://cli.github.com/manual/)

## 1. Prioritized Improvements

### 1. `make fmt-check` rewrites files in a target that should be read-only

- Priority: High
- Why it matters: CI and local verification commands should be safe to run in dirty worktrees. A check target that mutates files is easy to misuse in automation and surprising for contributors.
- Exact location:
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/Makefile:71)
- What is wrong:
  - `fmt-check` runs `goimports -w .` and `gofumpt -w .`, then fails on `git diff --exit-code`.
  - That means the “check” target edits the checkout before reporting failure.
- Recommended improvement: make `fmt-check` non-mutating and keep `fmt` as the write-mode target.
- Expected impact: safer CI, safer agent automation, and less accidental worktree churn.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Global `--version` bypasses structured output mode

- Priority: High
- Why it matters: the CLI advertises `--json` for scripting, but the global version flag skips the normal execution path. That creates a real inconsistency for scripts and wrappers.
- Exact location:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:43)
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:119)
  - [internal/cmd/version.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/version.go:37)
  - [internal/cmd/execute_version_exitcodes_test.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/execute_version_exitcodes_test.go:10)
- What is wrong:
  - `gog version --json` returns JSON.
  - `gog --version --json` returns a bare string because `kong.VersionFlag` exits before `outfmt.FromFlags` and `VersionCmd.Run` are used.
- Recommended improvement: route global version handling through the same formatter as the `version` subcommand, or remove the special-case divergence.
- Expected impact: better scripting ergonomics and fewer command-specific exceptions.
- Estimated risk: Low
- Safe to automate: Yes

### 3. Zsh completion output is malformed and the test suite does not catch it

- Priority: High
- Why it matters: completion is part of the CLI surface. Current zsh output includes a Bash shebang in the middle of the generated file, which is a real quality bug and a sign that shell-specific structure is under-tested.
- Exact location:
  - [internal/cmd/completion_scripts.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/completion_scripts.go:20)
  - [internal/cmd/completion_scripts.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/completion_scripts.go:37)
  - [internal/cmd/completion_test.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/completion_test.go:9)
- What is wrong:
  - `zshCompletionScript()` prepends a zsh header and then concatenates `bashCompletionScript()`.
  - `bashCompletionScript()` starts with `#!/usr/bin/env bash`, so `gog completion zsh` emits:
    - `#compdef gog`
    - `autoload -Uz bashcompinit`
    - `bashcompinit`
    - `#!/usr/bin/env bash`
  - The current test only checks for a marker like `bashcompinit`, so this malformed output still passes.
- Recommended improvement: emit a valid zsh wrapper without an embedded Bash shebang and tighten tests around shell-specific structure.
- Expected impact: more reliable zsh completion and stronger regression coverage.
- Estimated risk: Low
- Safe to automate: Yes

### 4. The documented `--plain` contract is not honored consistently by config and policy commands

- Priority: Medium
- Why it matters: root help says `--plain` is “stable, parseable text to stdout (TSV; no colors)”. Some commands still emit human-oriented labels or sentinel text instead of a stable schema.
- Exact location:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:34)
  - [internal/cmd/config_cmd.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/config_cmd.go:128)
  - [internal/cmd/policy.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/policy.go:100)
- What is wrong:
  - `gog --plain config list` prints `Config file: ...` and `key: value` lines.
  - `gog --plain policy list` prints `No policies` on an empty state.
  - Both are readable, but neither is a stable TSV shape.
- Recommended improvement: add explicit plain-mode branches for these commands with a simple tab-separated schema, including a stable empty-state representation.
- Expected impact: better automation ergonomics and fewer ad hoc parsers.
- Estimated risk: Low
- Safe to automate: Yes

### 5. Help generation depends on reading local config state

- Priority: Medium
- Why it matters: help output is normally the safest command in a CLI. Here it pulls in local config and keyring backend resolution before any command runs, so help text varies with local state and can surface config-read errors.
- Exact location:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:76)
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:218)
  - [internal/secrets/store.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/secrets/store.go:69)
- What is wrong:
  - `Execute()` always builds the parser with `helpDescription()`.
  - `helpDescription()` calls `config.ConfigPath()` and `secrets.ResolveKeyringBackendInfo()`.
  - `ResolveKeyringBackendInfo()` reads config to determine the keyring backend source.
- Recommended improvement: decouple static help text from config reads, or move environment/state details behind an explicit diagnostics command.
- Expected impact: more deterministic help output and less coupling between discovery UX and local config health.
- Estimated risk: Medium
- Safe to automate: No

### 6. Retry/error abstractions are only partially wired through the transport layer

- Priority: Low
- Why it matters: richer typed errors exist, but the retry transport mostly returns raw HTTP responses after exhausting retries. That limits higher-level error reporting and makes the abstractions harder to rely on.
- Exact location:
  - [internal/googleapi/transport.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/googleapi/transport.go:39)
  - [internal/googleapi/errors.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/googleapi/errors.go:28)
- What is wrong:
  - `RetryTransport.RoundTrip()` returns the raw `*http.Response` after exhausting `429` and `5xx` retries.
  - `RateLimitError`, `QuotaExceededError`, and `PermissionDeniedError` exist, but only `CircuitBreakerError` is emitted directly from transport.
- Recommended improvement: decide whether transport should stay HTTP-native or promote exhausted retry states into typed errors consistently, then wire formatting around that decision.
- Expected impact: cleaner error semantics and better observability for retries/quota cases.
- Estimated risk: Medium
- Safe to automate: No

### 7. Core dependencies are behind current upstream releases

- Priority: Low
- Why it matters: there is no immediate breakage, but a dependency refresh would likely pick up parser, Google API client, and standard-library-adjacent fixes.
- Exact location:
  - [go.mod](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/go.mod:5)
- What is wrong:
  - `go list -m -u all` reports newer versions for key packages, including `github.com/alecthomas/kong` (`v1.13.0` -> `v1.15.0`) and `google.golang.org/api` (`v0.260.0` -> `v0.277.0`).
- Recommended improvement: do a bounded dependency-update pass, starting with direct dependencies and full regression coverage.
- Expected impact: incremental maintenance headroom.
- Estimated risk: Medium
- Safe to automate: No

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Make `fmt-check` read-only.
- Unify global `--version` with `version --json` behavior.
- Fix zsh completion generation and add a regression test that asserts the zsh script shape.
- Add explicit TSV plain-mode branches for `config` and `policy` list/get surfaces.

### Larger Refactors

- Decouple help generation from local config/keyring inspection.
- Decide whether the Google API layer should expose richer typed retry/quota errors or stay fully HTTP-response-driven.
- Run a direct-dependency refresh and compatibility pass for `kong`, `google.golang.org/api`, and related transitive updates.

## 3. Do Not Change List

- Stdout for data, stderr for hints/errors:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:95)
  - [internal/cmd/output_helpers.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/output_helpers.go:18)
  - Why: this matches the gcloud-style scripting model where successful machine output stays on stdout and unstable guidance stays on stderr.

- Exit-code handling for usage failures:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:159)
  - [internal/cmd/usage.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/usage.go:8)
  - Why: current parse/usage failures already resolve cleanly to exit code `2`, which is important for scripts.

- Safety gates around destructive commands:
  - [internal/cmd/confirm.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/confirm.go:14)
  - [internal/cmd/policy.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/policy.go:12)
  - Why: `--force`, `--no-input`, and persisted policies are part of the repo’s agent-safe posture and should stay intact.

- Version metadata injection via ldflags:
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/Makefile:13)
  - [internal/cmd/version.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/version.go:12)
  - Why: the build metadata pattern is conventional, lightweight, and already covered by tests.

- The internal `__complete` protocol:
  - [internal/cmd/completion.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/completion.go:22)
  - [internal/cmd/completion_internal.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/completion_internal.go:22)
  - Why: shell-specific script fixes should preserve the internal completion contract instead of replacing it wholesale.

## 4. Execution Plan

Only items marked `Safe to automate: Yes` are turned into tasks below.

### Task 1

- Title: Make `fmt-check` a true verification target
- Why: contributors and automation should be able to run the formatting gate without altering the worktree.
- Files/modules:
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/Makefile:71)
- Risk: Low
- Expected impact: safer CI and local pre-push flows.
- Steps:
  1. Replace write-mode formatter invocations in `fmt-check` with non-mutating checks.
  2. Keep `fmt` as the mutating target.
  3. Preserve the same formatter versions and import-local behavior.
- Validation:
  - `make fmt-check`
  - Confirm a clean tree stays clean after the command runs.
- Do not change:
  - The `fmt` target
  - Formatter versions

### Task 2

- Title: Unify global `--version` with structured output modes
- Why: `gog --version --json` should not behave differently from `gog version --json`.
- Files/modules:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/root.go:43)
  - [internal/cmd/version.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/version.go:37)
  - [internal/cmd/execute_version_exitcodes_test.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/execute_version_exitcodes_test.go:10)
- Risk: Low
- Expected impact: better scripting consistency and fewer top-level edge cases.
- Steps:
  1. Remove or redirect the special-case global version handling.
  2. Ensure `--json` and `--plain` are resolved before version output is emitted.
  3. Add regression coverage for `gog --version --json`.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog --version --json`
  - `go run ./cmd/gog version --json`
- Do not change:
  - The JSON shape of the `version` subcommand
  - Exit code behavior for `--version`

### Task 3

- Title: Fix malformed zsh completion output
- Why: current zsh output contains an embedded Bash shebang and the tests do not protect against it.
- Files/modules:
  - [internal/cmd/completion_scripts.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/completion_scripts.go:37)
  - [internal/cmd/completion_test.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/completion_test.go:9)
- Risk: Low
- Expected impact: cleaner zsh UX and better completion regression coverage.
- Steps:
  1. Remove the embedded Bash shebang from the zsh output path.
  2. Keep the existing `__complete` transport intact.
  3. Tighten tests to assert zsh-specific structure, not just marker presence.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog completion zsh`
- Do not change:
  - Bash completion output
  - Fish and PowerShell completion output

### Task 4

- Title: Make `config` plain output match the advertised TSV contract
- Why: `config list` currently requires human parsing even under `--plain`.
- Files/modules:
  - [internal/cmd/config_cmd.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/config_cmd.go:25)
  - [internal/cmd/config_cmd.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/config_cmd.go:128)
  - [internal/cmd/config_cmd_test.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/config_cmd_test.go:11)
- Risk: Low
- Expected impact: more predictable scripting for `config get`, `config list`, and `config path`.
- Steps:
  1. Add explicit plain-mode rendering for config commands that currently fall back to human text.
  2. Keep default human-readable output intact.
  3. Add tests that lock down the plain-mode schema.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog --plain config list`
- Do not change:
  - Existing JSON payloads
  - Default non-plain output

### Task 5

- Title: Make `policy` plain output stable in both empty and populated states
- Why: `policy list` currently returns `No policies` on stdout, which breaks the plain-mode contract and forces special-case parsers.
- Files/modules:
  - [internal/cmd/policy.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/policy.go:65)
  - [internal/cmd/policy.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/policy.go:100)
  - [internal/cmd/policy_test.go](/Users/jeremydjohnson/.codex/worktrees/960b/gogcli/internal/cmd/policy_test.go:12)
- Risk: Low
- Expected impact: consistent machine-readable policy inspection.
- Steps:
  1. Define a stable TSV schema for `policy get` and `policy list`.
  2. Represent empty-state results without ad hoc prose.
  3. Add regression tests for empty and non-empty plain output.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog --plain policy list`
- Do not change:
  - Existing JSON payloads
  - Policy enforcement semantics

## Final Section

### Top 3 Tasks to Execute First

1. Make `fmt-check` a true verification target.
2. Unify global `--version` with structured output modes.
3. Fix malformed zsh completion output.

### Tasks Excluded

- Task: Decouple help generation from config/keyring reads.
  - Reason: behavior is user-visible and intertwined with parser construction; it needs explicit product judgment.

- Task: Rework transport retry failures into typed API errors.
  - Reason: this crosses transport, formatting, and call-site behavior; not a safe single small PR.

- Task: Refresh direct dependencies.
  - Reason: the repo is healthy today and dependency bumps need a broader regression pass than this automation should assume.
