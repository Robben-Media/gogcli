# gog-mcp spec

`gog-mcp` is a native Google MCP server written in Go. It exposes Google account data and selected mutations to MCP clients over stdio or Streamable HTTP, backed by native Google API clients rather than a command-line wrapper. This document describes the current behavior of the server; `cmd/gog-mcp` and `internal/mcpserver` are authoritative where details differ.

## Goals

- One Go binary (`gog-mcp`) implementing MCP with stdio by default and optional authenticated stateless Streamable HTTP.
- Explicit, least-privilege access: operation allow-lists, account/client/action grants, and config policies; no implicit default account.
- Bounded, agent-safe behavior: capped API attempts, deadlines, concurrency, payload sizes, and honest truncation.
- Writes as a separately enabled, non-replayable path. Reads are the default surface.

## Non-goals

- Full Google API parity. The extended catalog follows a pinned discovery snapshot and excludes media upload/download transports; curated reads cover a deliberately small surface.
- Service account keys or domain-wide delegation. Accounts authenticate through the app-owned OAuth client only.
- Native MCP Skills. Guidance ships as versioned workflow resources that clients must read explicitly.
- A bundled web UI. The account connect page is a functional loopback onboarding surface, not a dashboard.

## Language and framework

- Go (see `go.mod`); module `github.com/steipete/gogcli` (retained for path/state compatibility; the executable is `gog-mcp`).
- MCP SDK `github.com/modelcontextprotocol/go-sdk/mcp`; schemas from `github.com/google/jsonschema-go`.
- Flags parsed with `github.com/alecthomas/kong`.

## Transports

### stdio (default)

MCP frames on stdout; structured logs on stderr. Grants and policies load at startup and reload on SIGHUP. The optional account connect page can run alongside stdio on a loopback address (`--connect-addr`).

### Streamable HTTP (optional, stateless)

Enabled by `--http-addr` together with `--http-config`, a trusted caller file. The endpoint is `/mcp` (`internal/mcpserver/http.go`). Properties:

- Each caller has an ID, the SHA-256 hex digest of its bearer token, a principal ID, and its own `allow_operations` and grants. Up to 32 callers; IDs and digests are unique.
- Distinct harnesses may share a principal to access the same owner's accounts; they then share that owner's concurrency limits. Distinct owners require distinct principals.
- `Host` must exactly match the external hostname; supplied `Origin` values must exactly match configured `allowed_origins` (exact http(s) origins). Forwarded identity headers are never trusted.
- The caller file is a startup snapshot; HTTP mode logs and ignores SIGHUP. Rotating access means replacing the file and restarting the service. A restart terminates in-flight requests; reconcile unknown write outcomes before retrying.

## Discovery modes

### Expanded (default)

`--discovery=expanded` registers one MCP tool per granted operation from the curated catalog, plus `accounts_list` when it is allowed and the principal has at least one grant. Clients see stable tool names.

### Compact (opt-in)

`--discovery=compact` registers three gateway tools. It also registers `accounts_list` when that operation is allowed and the principal has at least one grant; with no grants, only the gateways appear:

- `capabilities_search` — rank granted operations by task words; bounded summaries, never schemas.
- `capabilities_describe` — full description of one granted operation: required inputs, input/output schema (optionally a dotted `schema_path` projection), service guidance, and optional prepared workflow recipes when `intent_id` is supplied.
- `capabilities_execute` — execute one granted operation by name with a JSON arguments object, including `account_id`.

Gateway tools reject their own names as execute targets and expose only operations visible to the caller's grants.

### Extended API catalog (opt-in)

`--discovery=compact --api-catalog` additionally loads:

- Pinned Google discovery methods from `internal/googlecatalog/manifest.json` (1,108 methods across 29 services), executed as JSON REST with user OAuth scopes and an 8 MiB response bound. Media upload and download transports are excluded.
- Bounded Business Profile reads: `businessprofile_list_accounts` (one page, at most 20 accounts) and `businessprofile_list_locations` (one page, at most 100 locations, narrow read mask).
- Media operations (below) and authoring workflows (below), plus their temporary media artifact store.

The catalog widens discovery, not authorization: `--api-catalog` grants nothing by itself, and every method still passes the same grant checks.

## Curated read catalog

All curated operations are `SafeRead` (retryable) and bounded. Grants may reference them by name or by action string:

| Operation | Action | Scope |
| --- | --- | --- |
| `gmail_search` | `gmail:messages.search` | `gmail.readonly` |
| `gmail_get_message` | `gmail:get` | `gmail.readonly` |
| `gmail_get_thread` | `gmail:thread.get` | `gmail.readonly` |
| `drive_search` | `drive:search` | `drive.readonly` |
| `drive_get_file` | `drive:get` | `drive.readonly` |
| `docs_get_text` | `docs:cat` | `documents.readonly` |
| `calendar_list` | `calendar:calendars.list` | `calendar.readonly` |
| `calendar_list_events` | `calendar:events` | `calendar.readonly` |
| `calendar_freebusy` | `calendar:freebusy` | `calendar.readonly` |
| `analytics_list_properties` | `analytics:properties` | `analytics.readonly` |
| `analytics_metadata` | `analytics:dimensions` / `analytics:metrics` | `analytics.readonly` |
| `analytics_report` | `analytics:report` | `analytics.readonly` |
| `searchconsole_list_sites` | `searchconsole:sites.list` | `webmasters.readonly` |
| `searchconsole_query` | `searchconsole:query` | `webmasters.readonly` |
| `sheets_get_metadata` | `sheets:metadata` | `spreadsheets.readonly` |
| `sheets_read_range` | `sheets:get` | `spreadsheets.readonly` |

`accounts_list` is caller-local: it lists connected accounts with email, client name, auth mode, and usable capability groups (for example `gmail.read`), filtered by grants and write enablement.

## Media operations (API catalog opt-in)

- `gmail_get_attachment`, `drive_download_file`, `drive_export_file` are read-only. Default delivery is a temporary account-bound artifact reference; explicit `delivery=inline` returns base64 bytes capped at 1 MiB by default and 2 MiB hard maximum.
- `drive_create_file` and `drive_update_file` upload bounded caller-supplied bytes; both are writes. `drive_update_file` supports `expected_version` for an atomic If-Match content replacement.
- Artifacts are served through the `gog://media/{id}` resource template with fresh per-read authorization. References expire after 15 minutes, are never enumerated, belong to the owner/account/operation rather than a bearer, and never expose local paths.

## Authoring and mail workflows (API catalog opt-in)

- `mail_compose_prepare` (read) renders a formatted mail preview from a verified sending identity with optional signature and reply context.
- `mail_compose_draft` and `mail_compose_send` (writes) create or send formatted mail with derived reply headers; both have explicit call caps and non-replayable semantics.
- `docs_create_document`, `slides_create_presentation`, and `sheets_create_spreadsheet` (writes) create structured documents in one to two API calls, never share them, and return the created ID even on partial failure.

## Workflow resources

Five read-only Markdown resources at `gog://workflows/v1/{mail,documents,calendar,reporting,sheets}` describe bounded read patterns per service. Each carries a `sha256` digest. They are documentation, not executable state: resources are not native MCP Skills and never run by themselves.

## Accounts and onboarding

- The registry (`mcp-accounts.json` in the gogcli config dir by default; `--registry-file` overrides) stores connections and supports exactly one process owner at a time.
- The account connect page (`--connect-addr`, loopback only) serves `/accounts`, `/connect`, `/reconnect`, `/disconnect`, `/status`, and `/oauth/callback`. It uses the app-owned OAuth client bucket (`--client-name`, default `native-mcp`) and performs browser OAuth consent. Default consent is Gmail read; `--connect-scopes` offers additional scopes explicitly, chosen by the operator, never inferred from available methods.
- `--redirect-url` must equal the callback URL registered on the OAuth client; otherwise it is derived from the connect address. The connect server binds loopback hosts only.
- Connect, reconnect, and disconnect invalidate live tool state so token changes take effect without lying about cached clients.

## Authorization

- A request passes: operation allow-list (or grants), per-account/client/action grants, and optional persisted policies from `config.json`.
- Grants bind principal, account IDs, client names, and operations. An empty grants list grants nothing. Validation fails closed: unknown operations, principal mismatches, and invalid snapshots are startup errors.
- Writes require `--enable-writes` at the server and an explicit write grant for the caller; otherwise write operations are invisible and their execution returns `forbidden_operation`.
- Every Google operation names its account via `account_id` from `accounts_list`. There is no ambient account.

## Limits and error contract

Per tool request: an upstream call budget of 1–256 attempts including retries (`--max-upstream-calls`, default 32), a per-tool deadline (`--request-timeout`, default 30s), server-wide concurrency (`--max-concurrency`, default 32), and an 8 MiB request/response body bound. Search outputs shrink honestly: responses are trimmed with an explicit `truncated` flag rather than silently clipped.

Tool errors are typed by `internal/mcpcontract` with categories: `invalid_input`, `forbidden_operation`, `authentication_required`, `insufficient_scope`, `not_found`, `conflict`, `precondition_failed`, `budget_exhausted`, `quota_exhausted`, `deadline_exceeded`, `upstream_failure`, and `outcome_unknown`. `outcome_unknown` marks writes whose result cannot be proven; callers must reconcile with reads before repeating them. Retries are only safe for `SafeRead` operations.

## Security properties

- Tokens live in the OS keyring or an encrypted keyring file; the server opens the store non-interactively. OAuth client credentials, tokens, and registries are provisioned outside Git.
- The server never accepts service account keys.
- The account connect page binds loopback only; the deployed service keeps no account-page listener. Only one writer may hold the registry/state directory.
- HTTP callers authenticate with bearer tokens stored as SHA-256 digests; logs record caller IDs, never tokens.

## Build and test

- `make` / `make build` — build `bin/gog-mcp`.
- `make tools` — pinned `gofumpt`, `goimports` (local prefix `github.com/steipete/gogcli`), `golangci-lint` into `.tools/`.
- `make fmt` / `make fmt-check`, `make lint`, `make test`, `make ci`.
- Tests: stdlib `testing` and `httptest` next to the code; contract fixtures in `internal/mcpcontract` and `internal/mcpserver` pin tool schemas and transport behavior.

Deployment (container, secrets layout, smoke testing, rollout) lives in [deploy/google-mcp/README.md](../deploy/google-mcp/README.md).
