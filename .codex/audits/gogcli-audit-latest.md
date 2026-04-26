# gogcli Audit Report

Date: 2026-04-26
Branch: `ai-audit/gogcli-latest`
Mode: Audit report only; no source changes performed

## Audit Scope

Reviewed repository structure, core CLI entrypoints, output/error helpers, completions, config commands, build/test targets, and representative command behavior.

Commands run:

- `git status --short`
- `go test ./...`
- `go run ./cmd/gog --help`
- `go run ./cmd/gog version --json`
- `go run ./cmd/gog --plain config list`
- `go run ./cmd/gog --plain config keys`
- `go run ./cmd/gog config list --json`
- `go run ./cmd/gog completion zsh`
- `go run ./cmd/gog completion fish`

Web references used for comparison:

- [GitHub CLI formatting docs](https://cli.github.com/manual/gh_help_formatting)
- [Cobra shell completion guide](https://cobra.dev/docs/how-to-guides/shell-completion/)
- [gofmt command docs](https://pkg.go.dev/cmd/gofmt)
- [goimports docs](https://pkg.go.dev/golang.org/x/tools/cmd/goimports)
- [gofumpt README](https://github.com/mvdan/gofumpt)

## 1. Prioritized Improvements

### 1. CI does not validate the TypeScript email-tracking worker

- Priority: High
- Why it matters: the repo ships and tests a non-Go worker under `internal/tracking/worker`, but the advertised full gate does not execute that worker's lint/build/test path. Regressions in the worker can land while `make ci` still passes.
- Exact location:
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/Makefile:79)
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/Makefile:89)
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/Makefile:91)
  - [internal/tracking/worker/package.json](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/tracking/worker/package.json:1)
- What is wrong:
  - `ci` runs `pnpm-gate fmt-check lint test`.
  - `pnpm-gate` only checks for a root `package.json`, `package.json5`, or `package.yaml`.
  - This repo has no root `package.json`, so `pnpm-gate` skips.
  - The actual worker package lives at `internal/tracking/worker/package.json` and only runs via the separate `worker-ci` target, which `ci` does not call.
- Recommended improvement: make `ci` include `worker-ci`, or change `pnpm-gate` to detect and run the worker package explicitly.
- Expected impact: better release confidence and fewer silent regressions in the tracking feature.
- Estimated risk: Low
- Safe to automate: Yes

### 2. `make fmt-check` mutates the worktree instead of acting like a check

- Priority: High
- Why it matters: a target named `fmt-check` should be safe in CI and pre-push flows. Today it rewrites files in place, which is surprising for contributors and awkward for automation.
- Exact location:
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/Makefile:71)
- What is wrong:
  - `fmt-check` runs `goimports -w .` and `gofumpt -w .`, which write changes.
  - It then uses `git diff --exit-code` to fail if formatting changed.
  - This means a "check" target modifies the checkout before failing.
- Recommended improvement: convert `fmt-check` into a non-mutating verification target, for example by using `gofmt -d`/`-l`-style output plus non-write modes for the configured formatters.
- Expected impact: safer CI, cleaner local automation, and less accidental worktree churn.
- Estimated risk: Low
- Safe to automate: Yes

### 3. `config list --plain` violates the documented plain-output contract

- Priority: Medium
- Why it matters: the repository promises that `--plain` is stable TSV on stdout. `config list --plain` currently emits human-oriented labeled lines, which is inconsistent for scripts and agents.
- Exact location:
  - [README.md](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/README.md:255)
  - [internal/cmd/config_cmd.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/config_cmd.go:128)
- What is wrong:
  - `README.md` says `--plain` is stable TSV.
  - `ConfigListCmd.Run` only special-cases JSON; non-JSON mode prints:
    - `Config file: /...`
    - `key: value`
  - Actual command output from `go run ./cmd/gog --plain config list` is human-readable, not TSV.
- Recommended improvement: add an explicit plain-mode branch for `config list`, such as `path\t<path>` and `key\tvalue`, or another documented stable tab-separated schema.
- Expected impact: makes the config surface consistent with the rest of the CLI’s scripting story.
- Estimated risk: Low
- Safe to automate: Yes

### 4. Zsh completion is a Bash compatibility shim, not shell-native completion

- Priority: Medium
- Why it matters: completion quality is part of CLI usability. The current zsh path works by enabling Bash completion emulation, which is less idiomatic and usually less capable than native zsh completion.
- Exact location:
  - [internal/cmd/completion_scripts.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/completion_scripts.go:37)
  - [internal/cmd/completion_test.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/completion_test.go:9)
- What is wrong:
  - `zshCompletionScript()` prepends `bashcompinit` and then concatenates the Bash script verbatim, including the Bash shebang.
  - Tests only assert presence of markers like `bashcompinit`; they do not protect a native zsh contract.
  - Compared with modern CLI completion patterns, native per-shell scripts are the norm.
- Recommended improvement: generate a native zsh completion wrapper or full native zsh script, and tighten tests around shell-specific output instead of marker strings alone.
- Expected impact: better completion reliability for zsh users and less shell-specific glue.
- Estimated risk: Medium
- Safe to automate: Yes

### 5. Contributor/setup guidance is out of sync with the repository layout

- Priority: Medium
- Why it matters: install/setup friction slows contributors and automation. The repo guidance names files and workflows that do not exist in this checkout.
- Exact location:
  - [AGENTS.md](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/AGENTS.md:8)
  - [AGENTS.md](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/AGENTS.md:15)
- What is wrong:
  - `AGENTS.md` references `scripts/gog.mjs`, but that file is absent.
  - `AGENTS.md` says `pnpm gog …` is available, but there is no root `package.json`.
  - That creates a mismatch between contributor instructions and the actual repo.
- Recommended improvement: update contributor-facing guidance so it only describes runnable paths that exist in the repository.
- Expected impact: lower onboarding friction and fewer dead-end setup attempts.
- Estimated risk: Low
- Safe to automate: Yes

### 6. The global machine-output contract is only partially enforced command by command

- Priority: Low
- Why it matters: `gog` positions itself as script-friendly, but the implementation still relies on each command to opt into plain/JSON behavior individually. That scales poorly as the command surface grows.
- Exact location:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/root.go:119)
  - [internal/outfmt/outfmt.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/outfmt/outfmt.go:12)
  - [internal/cmd/config_cmd.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/config_cmd.go:128)
- What is wrong:
  - Root mode selection is centralized, but rendering policy is distributed across many commands.
  - The `config list --plain` inconsistency is a symptom of that design.
- Recommended improvement: build a small output-contract checklist or shared helpers for common shapes so new commands cannot silently drift from JSON/plain expectations.
- Expected impact: fewer format regressions as the CLI grows.
- Estimated risk: Medium
- Safe to automate: No

## 2. Quick Wins Vs Larger Refactors

### Quick Wins

- Fix `make ci` so it runs `worker-ci` or otherwise validates `internal/tracking/worker`.
- Make `fmt-check` non-mutating.
- Add a real plain-mode formatter for `config list`.
- Replace the zsh Bash shim with a thinner native zsh completion path and strengthen completion tests.
- Correct stale contributor/setup guidance in `AGENTS.md`.

### Larger Refactors

- Build a shared output-contract layer so command authors declare rows/objects once and get consistent default/plain/JSON rendering automatically.
- Audit the entire command tree for plain-mode parity against the documented "stable TSV" promise.
- Introduce shell-script validation in CI for generated completion output across Bash, zsh, Fish, and PowerShell.

## 3. Do Not Change List

These areas look intentional, stable, and worth preserving unless there is a strong compatibility reason to revisit them.

- Root `--json` / `--plain` / stderr split:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/root.go:119)
  - [internal/ui/ui.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/ui/ui.go:21)
  - Why: the repo consistently treats stdout as the data channel and stderr as the human-hint/error channel. That is the right baseline for automation.
- Exit code handling for parse/usage failures:
  - [internal/cmd/root.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/root.go:159)
  - [internal/cmd/execute_version_exitcodes_test.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/execute_version_exitcodes_test.go:60)
  - Why: the current `ExitError` wrapping and exit code `2` coverage give scripts a stable failure mode.
- Account auto-selection and alias resolution:
  - [internal/cmd/account.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/account.go:11)
  - Why: the fallback order is practical for both humans and agents and already integrates config aliases, env vars, defaults, and stored tokens.
- Version/build metadata pattern:
  - [internal/cmd/version.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/version.go:12)
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/Makefile:13)
  - Why: injecting version/commit/date via `ldflags` is conventional and already test-covered.
- Policy and top-level command restrictions:
  - [internal/cmd/enabled_commands.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/enabled_commands.go:9)
  - [internal/cmd/policy_enforcement.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/policy_enforcement.go:28)
  - Why: this is a differentiating safety feature for agent/sandbox use. Improvements should preserve current semantics.

## 4. Execution Plan

Only items marked `Safe to automate: Yes` are turned into tasks below.

### Task 1

- Title: Include the tracking worker in the default CI gate
- Why: `make ci` currently reports success without validating `internal/tracking/worker`.
- Files/modules:
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/Makefile:79)
  - [internal/tracking/worker/package.json](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/tracking/worker/package.json:1)
- Risk: Low
- Expected impact: closes a real test/lint/build gap for the shipped worker feature.
- Steps:
  1. Decide whether `ci` should call `worker-ci` directly or whether `pnpm-gate` should discover the worker package.
  2. Update the Makefile so the default full gate executes the worker checks.
  3. Add or adjust tests/docs only as needed to reflect the new gate behavior.
- Validation:
  - `make ci`
  - Confirm worker lint/build/test execute instead of being skipped by the root-package check.
- Do not change:
  - Existing Go test behavior
  - Worker scripts in `internal/tracking/worker/package.json`

### Task 2

- Title: Make `fmt-check` a true read-only verification target
- Why: contributors and automation should be able to run a check target without modifying the checkout.
- Files/modules:
  - [Makefile](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/Makefile:71)
- Risk: Low
- Expected impact: safer local and CI workflows.
- Steps:
  1. Replace write-mode formatter invocations in `fmt-check` with non-mutating checks.
  2. Keep `fmt` as the mutating formatter target.
  3. Verify failure output still makes formatting drift obvious.
- Validation:
  - Run `make fmt-check` on a clean tree and confirm no files change.
  - Intentionally create formatting drift and confirm the target fails without rewriting files.
- Do not change:
  - The `fmt` target
  - Formatter tool versions or import-local prefix behavior

### Task 3

- Title: Make `config list --plain` emit stable TSV
- Why: the current output breaks the repo’s plain-mode contract.
- Files/modules:
  - [internal/cmd/config_cmd.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/config_cmd.go:128)
  - [README.md](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/README.md:255)
- Risk: Low
- Expected impact: better scripting ergonomics and fewer command-specific exceptions.
- Steps:
  1. Add a plain-mode branch to `ConfigListCmd.Run`.
  2. Emit a documented, stable tab-separated schema.
  3. Add command tests that lock down both default and plain output.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog --plain config list`
  - Confirm stdout is stable TSV and stderr stays clean.
- Do not change:
  - Existing JSON payload shape for `config list`
  - Existing default human-readable output unless needed for shared code paths

### Task 4

- Title: Replace zsh Bash-emulation completion with a native zsh path
- Why: the current zsh completion is functional but not idiomatic and only lightly tested.
- Files/modules:
  - [internal/cmd/completion_scripts.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/completion_scripts.go:37)
  - [internal/cmd/completion_test.go](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/internal/cmd/completion_test.go:9)
- Risk: Medium
- Expected impact: better zsh UX and more trustworthy completion generation.
- Steps:
  1. Replace the `bashcompinit` shim with a native zsh completion implementation or a tighter zsh wrapper.
  2. Expand tests so they verify shell-specific structure rather than marker presence alone.
  3. Confirm Bash, Fish, and PowerShell outputs remain unchanged.
- Validation:
  - `go test ./internal/cmd`
  - `go run ./cmd/gog completion zsh`
  - Manual spot-check in zsh if available.
- Do not change:
  - `__complete` protocol
  - Existing Bash/Fish/PowerShell completion behavior

### Task 5

- Title: Remove stale contributor/setup guidance
- Why: current repo guidance references non-existent files/workflows.
- Files/modules:
  - [AGENTS.md](/Users/jeremydjohnson/.codex/worktrees/2bd6/gogcli/AGENTS.md:8)
- Risk: Low
- Expected impact: fewer dead-end setup attempts for contributors and agents.
- Steps:
  1. Remove or correct references to `scripts/gog.mjs`.
  2. Remove or correct references to `pnpm gog …` if that path is no longer supported.
  3. Re-verify that documented commands match the current tree.
- Validation:
  - `rg -n "gog.mjs|pnpm gog" AGENTS.md README.md docs specs`
  - Manual existence check for any mentioned scripts.
- Do not change:
  - Actual build/test commands that currently work
  - User-facing CLI behavior

## Final Section

### Top 3 Tasks to Execute First

1. Include the tracking worker in the default CI gate.
2. Make `fmt-check` a true read-only verification target.
3. Make `config list --plain` emit stable TSV.

### Tasks Excluded

- Task: Build a shared output-contract layer across the full command tree.
  - Reason: useful, but this crosses too many commands to be a safe small automation PR.
- Task: Full plain-mode parity audit for every command.
  - Reason: broad surface area and likely to uncover behavior choices that need human review.
- Task: Redesign help/build/config presentation globally.
  - Reason: current help behavior is mostly sound; only narrow inconsistencies were worth prioritizing.
