# Codex code-review disposition — feat/skill-pack-update

**Fixed point:** `origin/main`  
**Spec:** `docs/plans/gog-skill-pack-update-2026-08-07.html`  
**Reviewer:** Codex `codex review --base origin/main` (2026-08-07)

## Blocking (P1) — addressed

| Finding | Fix |
|---------|-----|
| Skills refresh after binary update used old embed FS | Re-exec new binary with `gog update --skills-after-binary` after successful binary replace |
| Current installs without state later classified dirty | On `skipped_current` with empty state, record baseline pack hash |
| Stale skill commands (drive) | Corrected drive skill flags/commands; added “prefer live `--help`” note to pack skills |

## P2 — addressed

| Finding | Fix |
|---------|-----|
| Version inequality could downgrade | Semver-ish `versionLess` — update only when remote is greater |
| Update check during `go test` | `testing.Testing()` short-circuit in `MaybeNotify` |
| Install skipped when only project copy exists | Install mode always ensures canonical `~/.agents/skills` root |
| Symlink privilege failures aborted install | Symlink creation is best-effort (errors ignored) |

## Residual nits (non-blocking)

- Full editorial pass of all 10 skills against every subcommand is larger than this PR; help-first note + drive fix covers the worst agent footgun. Follow-up: regenerate skill tables from `gog <cmd> --help`.
- Windows binary replace still best-effort (rename dance).
- Skill pack embeds Robben-local account conventions (existing practice on machine).

## Merge recommendation

**Approve after disposition commit** — P1/P2 from Codex addressed; residual skill editorial is follow-up.
