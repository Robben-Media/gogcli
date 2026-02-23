# CLI Design System — gogcli

> Single source of truth for command conventions. Every spec and every agent MUST conform to these rules. Divergence is a bug.

## Command Naming

### Hierarchy

```
gog <service> [resource] [action]
```

| Level | Example | Rule |
|-------|---------|------|
| Service | `gog gmail`, `gog drive` | Top-level, matches Google API name |
| Resource | `gog gmail messages`, `gog drive files` | Plural noun for resource collections |
| Action | `gog gmail messages search`, `gog drive files list` | Verb for the operation |

### Naming Rules

- **Lowercase** everything: `gog bigquery datasets list` not `gog BigQuery Datasets List`
- **Hyphenated** multi-word names: `--include-body`, `--send-updates`, `--page-token`
- **No underscores** in command or flag names (even if Google API uses them)
- **Aliases** allowed on service level: `gmail`=`mail`=`email`, `youtube`=`yt`, `bigquery`=`bq`
- **Short flags** sparingly: only for very common flags (e.g., `-z` for `--timezone`)

### Action Verbs

| Action | CLI Verb | Google API Method |
|--------|----------|-------------------|
| List resources | `list` | `.List()` |
| Get single resource | `get` | `.Get()` |
| Create resource | `create` | `.Create()` / `.Insert()` |
| Update (partial) | `patch` or `update` | `.Patch()` |
| Update (full replace) | `update` | `.Update()` |
| Delete resource | `delete` | `.Delete()` |
| Search | `search` | `.List()` with query or `.Search()` |

When Google uses `Insert` (e.g., `events.insert`), prefer `create` as the CLI verb unless `insert` has distinct semantics (e.g., `tabledata.insertAll` for streaming inserts).

## Global Flags (RootFlags)

Every command inherits these. Do NOT re-declare them on individual commands.

| Flag | Type | Default | Env Var | Purpose |
|------|------|---------|---------|---------|
| `--account` | string | — | `GOG_ACCOUNT` | Account email for API auth |
| `--client` | string | `"default"` | `GOG_CLIENT` | OAuth client name |
| `--json` | bool | false | `GOG_JSON` | JSON output to stdout |
| `--plain` | bool | false | `GOG_PLAIN` | TSV output, no colors |
| `--color` | string | `"auto"` | `GOG_COLOR` | Color mode: auto/always/never |
| `--force` | bool | false | — | Skip destructive confirmations |
| `--no-input` | bool | false | — | Never prompt, fail instead |
| `--verbose` | bool | false | — | Enable debug logging |
| `--enable-commands` | string | — | `GOG_ENABLE_COMMANDS` | Restrict available commands |

## Output Contract

### Three Modes

| Mode | Flag | Behavior | Target Audience |
|------|------|----------|-----------------|
| Default | (none) | Colored table, human hints to stderr | Humans in terminal |
| JSON | `--json` | Pretty JSON to stdout, nothing to stderr | Scripts, piping |
| Plain | `--plain` | TSV to stdout, no colors | grep/awk/cut pipelines |

### JSON Structure

**List operations** — always wrap items + pagination:
```json
{
  "<resource_plural>": [...],
  "nextPageToken": ""
}
```
Key name matches the resource (e.g., `"files"`, `"messages"`, `"events"`, `"accounts"`).

**Single resource** — wrap under singular key:
```json
{
  "<resource_singular>": {...}
}
```

**Delete operations** — confirmation output:
```json
{
  "deleted": true,
  "<resourceIdField>": "<id>"
}
```

**Batch operations** — count + identifiers:
```json
{
  "deleted": ["id1", "id2"],
  "count": 2
}
```

### Text Output

- Tables: tab-separated via `tableWriter(ctx)`, header row in ALL CAPS
- Key-value: `u.Out().Printf("key\t%s", value)` format
- Pagination hint: `printNextPageHint(u, resp.NextPageToken)` to stderr
- Empty state: `u.Err().Println("No <resources>")` to stderr

## Pagination

### Standard Flags

Every list command MUST include:

| Flag | Type | Default | Purpose |
|------|------|---------|---------|
| `--max` | int64 | 10-100 | Max results per page |
| `--page` | string | — | Opaque page token |

Aliases: `--limit` for `--max`.

### Response Contract

- JSON: Always include `"nextPageToken"` key (empty string if last page)
- Text: Call `printNextPageHint(u, resp.NextPageToken)` to stderr
- Empty: Print `"No <resources>"` to stderr, return nil (not an error)

### No `--all` Flag

Clients loop manually using `--page`. This is intentional — prevents accidental full-table scans on large datasets.

## Error Taxonomy

### Exit Codes

| Code | Meaning | When |
|------|---------|------|
| 0 | Success | Command completed normally |
| 1 | Runtime error | API error, auth error, network error, user cancelled |
| 2 | Usage error | Missing args, invalid flags, parse errors |

### Error Types (Priority Order)

| Type | Exit Code | Example |
|------|-----------|---------|
| Usage error | 2 | `usage("empty calendarId")` |
| Auth required | 1 | Missing credentials for service |
| Google API error | 1 | 404 Not Found, 403 Forbidden, 429 Rate Limited |
| Cancellation | 1 | User declined `confirmDestructive()` |
| Runtime error | 1 | Network failure, file not found |

### Error Creation

```go
// Usage errors (exit code 2) — for bad input
return usage("empty resourceId")
return usagef("invalid scope: %q", scope)

// Runtime errors (exit code 1) — for failures
return fmt.Errorf("create resource: %w", err)

// Google API errors — pass through (errfmt.Format handles display)
return err
```

## Destructive Operations

Any command that deletes or permanently modifies data MUST:

1. Call `confirmDestructive(ctx, flags, description)` before the API call
2. Support `--force` flag (inherited from RootFlags) to skip confirmation
3. Respect `--no-input` flag (fail rather than prompt)

```go
if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete %s %s", resource, id)); err != nil {
    return err
}
```

## Auth Pattern

Every command that calls a Google API:

```go
account, err := requireAccount(flags)
if err != nil {
    return err
}
svc, err := newXxxService(ctx, account)
if err != nil {
    return err
}
```

`requireAccount` resolution order:
1. `--account` flag
2. `GOG_ACCOUNT` env var
3. Default account in keyring
4. Auto-select if only one token stored
5. Error with usage message

## Input Validation

```go
// Always trim before checking
id := strings.TrimSpace(c.ResourceID)
if id == "" {
    return usage("empty resourceId")
}

// Validate enums
switch strings.ToLower(c.Visibility) {
case "public", "private", "default":
    // ok
default:
    return usagef("invalid visibility: %q", c.Visibility)
}
```

## Testing

Every command gets a unit test following this pattern:

```go
func TestXxxCmd_JSON(t *testing.T) {
    // 1. Save/restore service factory
    origNew := newXxxService
    t.Cleanup(func() { newXxxService = origNew })

    // 2. Create httptest server
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Match method + path, respond with JSON
    }))
    defer srv.Close()

    // 3. Create service pointing to test server
    svc, _ := xxx.NewService(ctx,
        option.WithoutAuthentication(),
        option.WithHTTPClient(srv.Client()),
        option.WithEndpoint(srv.URL+"/"),
    )
    newXxxService = func(context.Context, string) (*xxx.Service, error) { return svc, nil }

    // 4. Set up context with JSON mode
    u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
    ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
    flags := &RootFlags{Account: "a@b.com", Force: true} // Force=true for delete tests

    // 5. Run command, capture stdout, assert JSON
    out := captureStdout(t, func() {
        cmd := &XxxCmd{...}
        if err := cmd.Run(ctx, flags); err != nil {
            t.Fatalf("Run: %v", err)
        }
    })

    var payload map[string]any
    json.Unmarshal([]byte(out), &payload)
    // Assert fields...
}
```

## 10 Golden Commands (Templates)

These commands are the reference implementations. When in doubt, match their patterns.

| Pattern | Golden Command | File |
|---------|---------------|------|
| List + pagination | `drive ls` | `internal/cmd/drive.go:46` |
| Get single resource | `drive get` | `internal/cmd/drive.go:204` |
| Create/upload | `drive upload` | `internal/cmd/drive.go:322` |
| Delete + confirm | `drive delete` | `internal/cmd/drive.go:448` |
| Move/update | `drive move` | `internal/cmd/drive.go:486` |
| Batch operation | `gmail batch delete` | `internal/cmd/gmail_batch.go:19` |
| Search + concurrency | `gmail messages search` | `internal/cmd/gmail_messages.go:17` |
| Watch/state | `gmail watch start` | `internal/cmd/gmail_watch_cmds.go:35` |
| Read with options | `sheets get` | `internal/cmd/sheets.go:71` |
| Create with validation | `drive share` | `internal/cmd/drive.go:585` |

## Rate Limiting & Retry

Already implemented in `internal/googleapi/client.go`:
- Exponential backoff with jitter
- Respects `Retry-After` header
- Retries 429 (rate limit) and 5xx (server errors)
- HTTP timeout: 30 seconds
- TLS 1.2+ enforced

Commands do NOT need to implement retry logic — it's handled at the transport layer.
