# Repository Guidelines

## Project Structure

- `cmd/gog-mcp/`: MCP server entrypoint (flags, grants loading, connect page wiring).
- `internal/mcpserver/`: MCP runtime — tools, resources, transports, limits.
- `internal/mcpcontract/`: public tool catalog, action/scope bindings, typed error contract.
- `internal/googleapi/`, `internal/googlecatalog/`: native Google API clients, retry/budget guards, pinned discovery catalog.
- `internal/googleops/`: operation implementations (curated reads, catalog executors, media, authoring, workflows).
- `internal/accountconnect/`: browser OAuth onboarding, account registry, token stores.
- `internal/access/`, `internal/secrets/`, `internal/config/`: grants/policies enforcement and credential storage.
- `bin/`: build outputs. `docs/`: specs and agent guidance. `deploy/google-mcp/`: deployment package.
- Tests: `*_test.go` next to the code they cover.

## Build, Test, and Development Commands

- `make` / `make build`: build `bin/gog-mcp`.
- `make tools`: install pinned dev tools into `.tools/`.
- `make fmt` / `make lint` / `make test` / `make ci`: format, lint, test, full local gate.

## Coding Style & Naming Conventions

- Formatting: `make fmt` (`goimports` local prefix `github.com/steipete/gogcli` + `gofumpt`).
- Keep MCP frames on stdout; send logs and human hints to stderr.
- Use domain vocabulary from `CONTEXT.md` (capabilities, grants, write gates, artifacts) consistently in code, issues, and docs.

## Testing Guidelines

- Unit tests: stdlib `testing` (and `httptest` for transport/OAuth fixtures).
- Contract tests in `internal/mcpcontract` and `internal/mcpserver` pin tool schemas and error categories; update them together with behavior changes.
- Container smoke checks live in `deploy/google-mcp/smoke.py`; they do not prove live Google API outcomes.
- Never run against user-owned live state without an explicit request.

## Commit & Pull Request Guidelines

- Create commits with `committer "<msg>" <file...>`; avoid manual staging.
- Follow Conventional Commits + action-oriented subjects (e.g. `feat(mcp): add --verbose logging`).
- Group related changes; avoid bundling unrelated refactors.
- PRs should summarize scope, note testing performed, and mention any user-facing changes or new flags.
- PR review flow: when given a PR link, review via `gh pr view` / `gh pr diff` and do not change branches.

### PR Workflow (Review vs Land)

- **Review mode (PR link only):** read `gh pr view/diff`; do not switch branches; do not change code.
- **Landing mode:** temp branch from `main`; bring in PR (squash default; rebase/merge when needed); fix; update `CHANGELOG.md` (PR #/issue + thanks); run `make ci`; final commit; merge to `main`; delete temp; end on `main`.
- If we squash, add `Co-authored-by:` for the PR author when appropriate; leave a PR comment with what landed + SHAs.
- New contributor: thank in `CHANGELOG.md` (and update README contributors list if present).

## Security & Configuration Tips

- Never commit OAuth client credential JSON files, bearer tokens, keyring files, or account registries.
- Server state (registry, tokens) is user-owned live data; do not modify or delete it while a process owns it.
- Prefer OS keychain backends; use `GOG_KEYRING_BACKEND=file` + `GOG_KEYRING_PASSWORD` only for headless environments.
- Writes stay disabled unless explicitly enabled and granted; never grant write access casually in examples or fixtures.

## Agent skills

### Issue tracker

Issues and specs live in this repository's GitHub Issues. See `docs/agents/issue-tracker.md`.

### Triage labels

Triage uses the canonical Matt Pocock state labels without aliases. See `docs/agents/triage-labels.md`.

### Domain docs

This is a single-context repository. See `docs/agents/domain.md` for how skills consume domain documentation and ADRs.
