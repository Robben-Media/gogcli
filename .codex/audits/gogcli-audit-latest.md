# gogcli Audit Report

Date: 2026-04-23
Mode: Audit report only
Repository state checked with: `git status --short`
Verification run: `go test ./...` (passed)
Behavior sampled with: `go run ./cmd/gog --help`, `go run ./cmd/gog version --json`, `go run ./cmd/gog auth --help`

Best-practice references used:
- [Command Line Interface Guidelines](https://clig.dev/)
- [Cobra shell completion guide](https://cobra.dev/docs/how-to-guides/shell-completion/)
- [kubectl version reference](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_version/)

## 1. Prioritized Improvements

### 1. Eliminate duplicate Drive API calls in `drive url` JSON mode
- Priority: High
- Why it matters: This command currently doubles API traffic in JSON mode, which adds latency and makes automation slower for larger file lists.
- Exact location: `internal/cmd/drive.go:714-748`
- What is wrong: `DriveURLCmd.Run` resolves each file link once in the initial loop and then resolves every file again when building the JSON payload. Text mode does one lookup per file; JSON mode does two.
- Recommended improvement: Collect `{id,url}` results during the first loop and reuse that slice for both text and JSON output paths.
- Expected impact: Lower latency, fewer Drive API requests, lower quota usage, no user-visible behavior change.
- Estimated risk: Low
- Safe to automate: Yes

### 2. Make `make fmt-check` non-mutating
- Priority: High
- Why it matters: The current “check” target rewrites files before diffing them. That is surprising in local automation and a poor fit for CI gates.
- Exact location: `Makefile:71-74`
- What is wrong: `fmt-check` runs `goimports -w .` and `gofumpt -w .`, which mutates the working tree. A formatting check should fail when formatting is needed, not edit files as a side effect.
- Recommended improvement: Switch `fmt-check` to non-writing checks, for example by comparing formatter output against the tree or using list/diff modes.
- Expected impact: Safer CI/local automation, fewer accidental dirty worktrees, clearer separation between `fmt` and `fmt-check`.
- Estimated risk: Low
- Safe to automate: Yes

### 3. Stop reinstalling pinned tools on every `make fmt`, `make lint`, and `make ci`
- Priority: Medium
- Why it matters: Every formatter/lint/test workflow currently pays repeated `go install` cost, which slows local iteration and automation.
- Exact location: `Makefile:61-77`
- What is wrong: `tools` is `.PHONY`, so `make fmt`, `make lint`, and `make ci` reinstall `gofumpt`, `goimports`, and `golangci-lint` on every invocation.
- Recommended improvement: Replace the phony `tools` target with file targets under `.tools/` or a stamp target keyed by the pinned versions.
- Expected impact: Faster local loops and CI runs without changing tool versions or lint behavior.
- Estimated risk: Low
- Safe to automate: Yes

### 4. Remove config/keyring discovery from parser construction on every invocation
- Priority: Medium
- Why it matters: Help rendering and parser setup should be cheap and reliable. Right now parser creation eagerly performs config-path and keyring-backend discovery.
- Exact location: `internal/cmd/root.go:184-237`
- What is wrong: `Execute` calls `newParser(helpDescription())`, and `helpDescription()` immediately calls `config.ConfigPath()` and `secrets.ResolveKeyringBackendInfo()`. That means help, completion, and parse-error paths all perform filesystem/config/keyring work before command execution.
- Recommended improvement: Keep parser construction pure and lazy-load the config/keyring block only when top-level help actually prints, or cache/guard that lookup behind a dedicated helper.
- Expected impact: Cheaper help/completion paths, fewer surprising failures in basic CLI discovery, easier future testing.
- Estimated risk: Low
- Safe to automate: Yes

### 5. Improve top-level help discoverability with examples and a support/docs link
- Priority: Medium
- Why it matters: The current help is clean, but it makes first-use discovery harder than necessary for such a broad multi-service CLI.
- Exact location: `internal/cmd/root.go:214-237`, `internal/cmd/help_printer.go:23-52`
- What is wrong: `gog --help` shows usage, flags, commands, build, and config, but no examples, no docs URL, and no clear support path. CLIG recommends examples first and a web docs/support path in top-level help.
- Recommended improvement: Add 2-3 high-value examples and a docs/support URL to the top-level help text without changing command names or output contracts.
- Expected impact: Better first-run usability and lower setup friction, especially for a large command surface.
- Estimated risk: Low
- Safe to automate: Yes

### 6. Split oversized command files only with human review
- Priority: Low
- Why it matters: Very large command files are harder to reason about, review, and extend safely.
- Exact location: `internal/cmd/docs.go` (1227 lines), `internal/cmd/auth.go` (1103 lines), `internal/cmd/drive.go` (1044 lines)
- What is wrong: Core command families are concentrated in large files with mixed concerns, which increases the cost of future changes and raises merge/conflict risk.
- Recommended improvement: Incrementally split by subdomain/output helper/validation helper, while preserving CLI shape and output formats.
- Expected impact: Better maintainability and easier targeted testing.
- Estimated risk: Medium
- Safe to automate: No

### 7. Add a regression test for `drive url` request count
- Priority: Low
- Why it matters: There is already output coverage for `DriveURLCmd`, but no guard against duplicate per-file lookups in JSON mode.
- Exact location: `internal/cmd/drive_url_cmd_test.go:20-122`
- What is wrong: Existing tests validate returned URLs but do not assert how many HTTP calls are made.
- Recommended improvement: Extend the existing httptest server to count requests and assert one lookup per file in both text and JSON modes.
- Expected impact: Prevents the current performance issue from reappearing after it is fixed.
- Estimated risk: Low
- Safe to automate: Yes

### 8. Consider normalizing `version --json` fallback semantics
- Priority: Low
- Why it matters: Text mode uses `VersionString()` and falls back to `dev`; JSON mode emits `strings.TrimSpace(version)` directly.
- Exact location: `internal/cmd/version.go:18-46`
- What is wrong: If the embedded version were ever empty, text and JSON outputs would disagree (`dev` vs `""`).
- Recommended improvement: Reuse the same normalized version value for both text and JSON payloads.
- Expected impact: More predictable scripting semantics in untagged/dev builds.
- Estimated risk: Low
- Safe to automate: Yes

## 2. Quick Wins vs Larger Refactors

### Quick Wins
- Remove duplicate link resolution in `internal/cmd/drive.go:714-748`.
- Make `fmt-check` non-mutating in `Makefile:71-74`.
- Cache or file-target the `.tools` installs in `Makefile:61-77`.
- Move config/keyring discovery out of eager parser construction in `internal/cmd/root.go:184-237`.
- Add examples and docs/support URL to help generation in `internal/cmd/root.go` and `internal/cmd/help_printer.go`.
- Add request-count coverage to `internal/cmd/drive_url_cmd_test.go:20-122`.
- Normalize version fallback handling in `internal/cmd/version.go:18-46`.

### Larger Refactors
- Split `internal/cmd/docs.go`, `internal/cmd/auth.go`, and `internal/cmd/drive.go` into smaller subdomain-focused files.
- Centralize repeated command helpers for pagination/output/validation across `internal/cmd/`.
- Revisit help generation architecture if the team wants richer docs/examples per subcommand instead of a minimal top-level augmentation.

## 3. Do Not Change List

### Parseable output contracts
- Keep `--json` and `--plain` behavior stable.
- Why: The repository is explicitly built around parseable stdout, and current root flags plus tests in `internal/outfmt/` and many command tests depend on this.

### Stdout vs stderr split
- Keep machine-readable success output on stdout and hints/no-results/errors on stderr.
- Why: This matches CLIG guidance for composable tools and is already followed throughout `internal/cmd/` and `internal/cmd/root.go`.

### Destructive command safety model
- Keep `--force` and `--no-input` semantics stable.
- Why: `internal/cmd/confirm.go` enforces safe non-interactive behavior, and many destructive commands rely on that contract.

### Existing command names and aliases
- Keep top-level commands and aliases stable, including `gmail/mail/email`, `youtube/yt`, `analytics/ga/ga4`, `search-console/gsc/sc`, and `business-profile/gbp/business`.
- Why: This CLI already has a broad surface area and automation value depends on command stability.

### Keyring/config behavior
- Keep the current config file and keyring backend semantics stable.
- Why: Auth storage is security-sensitive and already has targeted coverage in `internal/secrets/`, `internal/config/`, and auth command tests.

### Exit-code behavior
- Keep success as `0`, parse/usage errors as `2`, and command/runtime failures non-zero.
- Why: `cmd/gog/main.go`, `internal/cmd/exit.go`, and tests already encode this contract for scripts.

## 4. Task Plan

### Task 1
- Title: Remove duplicate Drive URL lookups in JSON mode
- Why: Fixes a real per-file performance bug with no intended behavior change.
- Files/modules: `internal/cmd/drive.go`, `internal/cmd/drive_url_cmd_test.go`
- Risk: Low
- Expected impact: Fewer Drive API requests, faster automation, preserved output shape.
- Steps:
  1. Refactor `DriveURLCmd.Run` to collect resolved URLs once.
  2. Reuse the collected results for both text and JSON output branches.
  3. Extend the existing test server to assert one lookup per file in JSON mode.
- Validation: `go test ./internal/cmd -run TestDriveURLCmd_TextAndJSON`
- Do not change: command name, arguments, JSON schema, text output rows, fallback URL behavior.

### Task 2
- Title: Make `fmt-check` a true read-only formatter gate
- Why: The current target dirties the worktree and is unsafe for automation.
- Files/modules: `Makefile`
- Risk: Low
- Expected impact: Cleaner CI/local automation and clearer target semantics.
- Steps:
  1. Replace write-mode formatter invocations in `fmt-check`.
  2. Make the target fail when formatting is needed instead of rewriting files.
  3. Verify `fmt` still performs the actual formatting path.
- Validation: Run `make fmt-check` in a clean tree and on a deliberately misformatted file in a throwaway branch/worktree.
- Do not change: pinned formatter versions, `fmt` behavior, `make ci` structure.

### Task 3
- Title: Cache `.tools` installs instead of reinstalling on every run
- Why: Reduces repeated setup cost across `fmt`, `lint`, and `ci`.
- Files/modules: `Makefile`
- Risk: Low
- Expected impact: Faster local loops and CI without changing tooling versions.
- Steps:
  1. Convert `tools` into file-backed targets or a versioned stamp under `.tools/`.
  2. Make `fmt`, `fmt-check`, and `lint` depend on those artifacts directly.
  3. Preserve the current pinned versions and install locations.
- Validation: Run `make lint` twice and confirm the second invocation skips reinstall work.
- Do not change: tool versions, install directory, formatter/linter command lines.

### Task 4
- Title: Defer help-only config and keyring discovery
- Why: Keeps parser creation cheap and avoids config/keyring work in discovery paths.
- Files/modules: `internal/cmd/root.go`, `internal/cmd/root_more_test.go`
- Risk: Low
- Expected impact: Faster `--help`, completion, and parse-error paths; fewer incidental failures.
- Steps:
  1. Move config/keyring lookup out of eager `helpDescription()` parser setup.
  2. Compute the config block only when top-level help is rendered.
  3. Update tests to cover the new help path.
- Validation: `go test ./internal/cmd -run 'TestHelpDescription|TestMainHelpDoesNotExit'` and verify `go run ./cmd/gog --help`.
- Do not change: displayed config file path text, displayed keyring backend text, normal help layout.

### Task 5
- Title: Add examples and docs/support URL to top-level help
- Why: Current help is serviceable but not especially discoverable for first-run users.
- Files/modules: `internal/cmd/root.go`, `internal/cmd/help_printer.go`, related help tests
- Risk: Low
- Expected impact: Better CLI usability and lower setup friction.
- Steps:
  1. Add 2-3 representative examples to the top-level help text.
  2. Add a docs/support URL to the help footer or description block.
  3. Update help printer tests to lock the new output in place.
- Validation: `go test ./internal/cmd -run Help` and verify `go run ./cmd/gog --help`.
- Do not change: command names, flag names, command ordering, parseable output contracts.

### Task 6
- Title: Normalize version fallback semantics across text and JSON
- Why: Keeps scripting output consistent if embedded version metadata is absent.
- Files/modules: `internal/cmd/version.go`, `internal/cmd/version_test.go`, `internal/cmd/misc_more_test.go`
- Risk: Low
- Expected impact: More predictable version output across build contexts.
- Steps:
  1. Normalize the version string once in `VersionCmd.Run`.
  2. Reuse that normalized value for JSON output.
  3. Add a test covering the empty-version fallback case.
- Validation: `go test ./internal/cmd -run Version`
- Do not change: current JSON keys, text format when version metadata is present.

## 5. Top 3 Tasks to Execute First

1. Remove duplicate Drive URL lookups in JSON mode.
2. Make `fmt-check` a true read-only formatter gate.
3. Cache `.tools` installs instead of reinstalling on every run.

## 6. Tasks Excluded

- Task: Split `internal/cmd/docs.go`, `internal/cmd/auth.go`, and `internal/cmd/drive.go`
  - Reason: Valuable, but not a small independent PR and too likely to create merge/review churn.

- Task: Redesign help output into a larger docs system
  - Reason: The small examples/docs-link improvement is safe; a full help architecture rewrite is not.

- Task: Change command names, aliases, output schemas, auth storage, or destructive-command semantics
  - Reason: These are core workflow contracts and should remain stable.
