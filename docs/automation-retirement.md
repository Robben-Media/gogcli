# Retired Mac mini automations

This document records the Codex, Pi, and Hermes automations that previously targeted `Robben-Media/gogcli` from Jeremy's Mac mini. They were retired on 2026-08-08 so a single replacement automation can be built on the local Linux machine.

The exact pre-removal definitions and state are stored outside the repository at:

`/home/jeremy-johnson/.local/share/gogcli/automation-retirement/mac-mini-2026-08-08`

That recovery archive is private to the local user (`0700`) and includes the Codex definitions and memories, Pi LaunchAgents/configuration/state, and Hermes job/configuration files. It may contain operational configuration and must not be committed.

## Retired automations

### Codex: gogcli audit and brainstorm

- Definition: `~/.codex/automations/gogcli-audit-brainstorm/automation.toml`
- Schedule: Sunday at 9:00 AM Central.
- Authority: audit the repository, update `.codex/audits/gogcli-audit-latest.md`, commit and push the audit branch, and open or update an audit pull request.
- Last recorded work: 2026-06-28; it updated PR #35.
- State at retirement: paused since July 2026.
- Retirement: delete the dedicated automation directory and its July 4 backup definition from the Mac mini.

### Codex: gogcli execute ideas

- Definition: `~/.codex/automations/gogcli-execute-ideas/automation.toml`
- Schedule: daily at 9:00 AM Central.
- Authority: read the audit's highest-priority work, implement one item, run focused and full verification, and create or update a pull request.
- Last recorded work: 2026-07-04; it found the then-current Top 3 already merged and made no changes.
- State at retirement: paused since July 2026.
- Retirement: delete the dedicated automation directory and its July 4 backup definition from the Mac mini.

### Pi: project-improvement harness

- Launcher: `com.robben.pi-project-improvement` LaunchAgent.
- Schedule: hourly and at login.
- Configuration: the `gogcli` entry in `~/.codex/project-improvement/projects.json`.
- Authority: brainstorm an eight-item queue, implement one low- or medium-risk item per run on an `ai-improvement/` branch, run `make ci`, and steward the resulting pull request.
- Model routing: GLM 5.2 for brainstorming and primary execution; OpenAI Codex for fallback/review; Grok Code Fast as backup.
- State at retirement: blocked since 2026-08-03 because the sidecar contained the unsupported queue state `pr_open`; launchd showed exit status 78.
- Retirement: remove only the `gogcli` registry and runtime-state entries. The shared LaunchAgent remains for other configured projects.

### Pi: PR automation

- Launcher: `com.robben.pi-pr-automation` LaunchAgent.
- Schedule: hourly.
- Scope: scans open pull requests across configured GitHub owners, including `Robben-Media`.
- Authority: inspect reviews, invoke Pi for fixes, push changes, and merge eligible non-draft pull requests when checks pass. Draft pull requests are skipped.
- Last observed `gogcli` work: scanned PR #36 on 2026-08-03 and skipped it because it was a draft.
- State at retirement: not running; launchd showed exit status 78. Both configured `gh` accounts had invalid tokens.
- Retirement: add `Robben-Media/gogcli` to `PI_PR_REPO_BLOCKLIST`. The shared PR automation and its history remain for other repositories.

### Hermes: GitHub technical-debt scout

- Job ID: `1a8fee495aa4` (`GitHub technical debt — Composer 2.5 scout module`).
- Schedule: daily at 9:15 AM Central.
- Scope: rotates through repositories listed in `~/.hermes/automation-config/github_composer_tech_debt_repos.json`.
- Authority: read-only analysis in a disposable copy; it may write local reports but cannot edit the source checkout, commit, push, open issues, or open pull requests.
- Last `gogcli` run: 2026-08-07, release-operations focus. Cursor authentication failed, so the deterministic fallback reported no high-confidence issue.
- Retirement: remove the `gogcli` repository entry from the shared rotation. Keep historical reports as evidence.

### Hermes: technical-debt action loop

- Job ID: `1f60585ca9cb` (`Tech debt action loop — GitHub + local Mac`).
- Schedule: daily at 9:45 AM Central.
- Scope: consumes the shared technical-debt scout configuration and reports.
- Authority: create or update internal GitHub issues after duplicate checks; it cannot commit, push, deploy, publish, or alter production.
- State at retirement: active and healthy, but current command-line GitHub authentication on the mini was invalid.
- Retirement: removing `gogcli` from the shared scout configuration removes it from this action loop without disabling the job for other repositories.

### Hermes: GitHub Actions failure watch

- Job ID: `f090818cdd01` (`GitHub Actions failure watch — main branch`).
- Schedule: hourly.
- Scope: dynamically monitors recent main-branch GitHub Actions failures across repositories.
- Authority: monitoring and reporting only.
- State at retirement: active and healthy.
- Retirement: add `Robben-Media/gogcli` to the watcher's ignored-repository set. Keep the shared watcher for other repositories.

### Codex: active-repository discovery

- Launcher: user crontab invokes `~/.codex/bin/discover-active-repos.sh` nightly at 2:00 AM Central.
- Scope: inventories recently active repositories for Codex; it does not change repository content.
- State at retirement: ineffective on the mini. `~/projects` is a symlink to the external volume, and the script's `find` invocation does not follow it, so recent runs recorded zero repositories.
- Retirement: retain this host-wide inventory job. It did not currently discover or operate on `gogcli` and is not a replacement for a repository automation.

## Retained history and repo-native automation

Historical Pi sessions/results and Hermes reports are retained because they are evidence, not executable automation. The repository's GitHub Actions CI and release workflows are also retained; they are repo-native validation/release infrastructure rather than Codex, Pi, or Hermes agents.

The Mac mini checkout was on `chore/robben-only-distribution` and had untracked automation artifacts when retired. A replacement automation must use a clean, current `origin/main` baseline or create an isolated worktree from that baseline; it must not inherit the mini's working-tree state.

## Local replacement: remote idea generation and global triage

The replacement runs on the local Linux machine through the user-level `gogcli-idea-generator.timer` every Sunday at 9:00 AM Central.

The first stage clones the remote default branch of `Robben-Media/gogcli` into a disposable temporary directory, deduplicates ideas against remote GitHub issues and merged pull requests, and performs an exhaustive idea dump. There is no numeric issue cap; every defensible idea becomes its own bounded issue. Each issue receives exactly one category label (`bug` or `enhancement`), the `needs-triage` state label, and the `codex-automation` provenance label.

The autonomous triage stage dynamically discovers every non-archived, issue-enabled repository owned by `Robben-Media` or `itsjeremyjohnson`. It processes unlabeled issues, `needs-triage` issues, and `needs-info` issues with new reporter activity. Repositories without the canonical triage labels are reported and skipped rather than reconfigured. Repositories with work are inspected through disposable clones of their remote default branches.

GitHub is the sole durable queue and state store. No persistent audit report, queue JSON, or sidecar state exists. Codex runs with an isolated temporary home, Codex state, GitHub configuration, and XDG configuration; GitHub token variables, DBus keyring access, and SSH-agent access are removed. Command policy forbids the common GitHub, Git, network-transfer, and SSH tools. Only temporary Markdown proposals can have a downstream GitHub effect, after a deterministic credentialed parent validates them. That parent permits only issue creation for `Robben-Media/gogcli`, or issue comments, triage-label changes, and duplicate/already-implemented closures during global triage. It cannot edit source, branch, commit, push, open pull requests, respond to PR reviews, or merge. The active GitHub identity must be `itsjeremyjohnson`, and write permission is verified per repository.

The current Linux host cannot create the user namespace required by Codex's `workspace-write` sandbox. Codex therefore executes with full shell access inside the disposable clone, guarded by the isolated environment and forbidden-command policy above. The deterministic parent enforces the GitHub mutation boundary, but unrelated local filesystem access is not OS-sandboxed. Run the Codex stage under a dedicated OS user or a host with working user namespaces if that stronger local boundary becomes necessary.

The global triage service runs daily at 2:00 PM Central as an independent recovery pass and also starts immediately after a successful weekly `gogcli` idea run. It attaches durable Agent Briefs and promotes verified work to `ready-for-agent`. Implementation remains manual: the maintainer chooses the best ready issue across both owners for the available time and invokes `/implement`.

Local components:

- Runner: `~/.local/bin/github-issue-automation`
- Prompts and authority boundaries: `~/.config/github-issue-automation/{idea-prompt,triage-prompt}.md`
- Scheduler: `~/.config/systemd/user/gogcli-idea-generator.{service,timer}`
- Global triage scheduler: `~/.config/systemd/user/github-issue-triage.{service,timer}`
- Logs: `journalctl --user -u gogcli-idea-generator.service -u github-issue-triage.service`

Run `~/.local/bin/github-issue-automation check` to verify identity, remote discovery, permissions, and label readiness without invoking Codex or changing issues. Run `systemctl --user start gogcli-idea-generator.service` only when an immediate live idea-generation cycle is intended. Run `systemctl --user start github-issue-triage.service` to triage the current cross-repository backlog.
