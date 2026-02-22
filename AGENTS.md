# AGENTS.md — gogcli API Gap Coverage

## Project Overview

Go CLI (`gog`) wrapping 19 Google Workspace/Cloud APIs using the Kong framework. Adding 588 missing API methods to achieve full parity with Google Discovery API surface. Each method becomes a Kong command with struct-based flags, httptest-mocked unit tests, and JSON/TSV output.

## Build Commands

```bash
make build          # Build bin/gog
make fmt            # Format (goimports + gofumpt)
make lint           # Lint (golangci-lint)
make test           # Run unit tests
make ci             # Full local gate (fmt + lint + test + build)
```

## Feedback Loops (Required Before Commit)

Run ALL loops. Do NOT commit if any fail. Fix issues first.

1. `make fmt` — Must produce no diffs
2. `make test` — All tests must pass
3. `make lint` — Must pass with zero warnings
4. `make build` — Must compile successfully
5. `make ci` — Full gate (runs all above)

## Runtime

- **Go 1.24** — use standard library where possible
- **CLI framework**: `alecthomas/kong` (struct tags, not cobra)
- **Google API clients**: `google.golang.org/api` official packages
- **Never use `~`** in file paths — use full absolute paths

## Codebase Structure

```
gogcli/
├── cmd/gog/main.go                    # Entry point
├── internal/
│   ├── cmd/                           # ALL command implementations (one file per resource group)
│   │   ├── root.go                    # CLI struct with all top-level commands
│   │   ├── gmail.go                   # Gmail commands + GmailCmd struct
│   │   ├── gmail_get_cmd.go           # Individual command files
│   │   ├── gmail_get_cmd_test.go      # Tests alongside commands
│   │   ├── calendar.go                # Calendar commands
│   │   ├── calendar_edit.go           # Calendar create/update/delete
│   │   ├── calendar_delete_test.go    # Test file
│   │   ├── account.go                 # requireAccount() helper
│   │   ├── confirm.go                 # confirmDestructive() helper
│   │   ├── output_helpers.go          # tableWriter(), printNextPageHint()
│   │   └── usage.go                   # usage(), usagef() error helpers
│   ├── googleapi/                     # Service client factories (one per API)
│   │   ├── client.go                  # Auth flow, retry transport, HTTP client
│   │   ├── gmail.go                   # NewGmail(ctx, email)
│   │   ├── calendar.go                # NewCalendar(ctx, email)
│   │   └── ...                        # One file per Google API service
│   ├── googleauth/                    # OAuth + service registry
│   │   └── service.go                 # Service enum, scope definitions
│   ├── config/                        # Credential/client management
│   ├── secrets/                       # Token storage (OS keyring)
│   ├── outfmt/                        # JSON/plain/text output modes
│   ├── ui/                            # Terminal output formatting
│   ├── errfmt/                        # Error formatting
│   └── input/                         # User input helpers
├── docs/
│   ├── spec.md                        # Architecture spec
│   └── reports/                       # Gap analysis reports
└── specs/                             # Ralph Wiggum specs (this build)
```

## Key Reference Files

- **`specs/`** — Feature specs for all 19 API gap implementations
- **`docs/reports/cli-vs-api-gap-report-2026-02-22.json`** — Authoritative gap data
- **`internal/cmd/root.go`** — CLI struct (register new commands here)
- **`internal/googleauth/service.go`** — Service registry (scopes, APIs)
- **`internal/googleapi/client.go`** — Auth flow and HTTP client setup

## Command Implementation Patterns

### Pattern 1: LIST command

```go
type XxxListCmd struct {
    // Positional args
    ParentID string `arg:"" name:"parentId" help:"Parent resource ID"`
    // Pagination
    Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"10"`
    Page string `name:"page" help:"Page token"`
    // Optional filters
    Query string `name:"query" help:"Filter query"`
}

func (c *XxxListCmd) Run(ctx context.Context, flags *RootFlags) error {
    account, err := requireAccount(flags)
    if err != nil {
        return err
    }
    parentID := strings.TrimSpace(c.ParentID)
    if parentID == "" {
        return usage("empty parentId")
    }

    svc, err := newXxxService(ctx, account)
    if err != nil {
        return err
    }

    resp, err := svc.Resources.List(parentID).
        MaxResults(c.Max).
        PageToken(c.Page).
        Context(ctx).
        Do()
    if err != nil {
        return err
    }

    if outfmt.IsJSON(ctx) {
        return outfmt.WriteJSON(os.Stdout, map[string]any{
            "items":         resp.Items,
            "nextPageToken": resp.NextPageToken,
        })
    }
    u := ui.FromContext(ctx)
    if len(resp.Items) == 0 {
        u.Err().Println("No items")
        return nil
    }
    w, flush := tableWriter(ctx)
    defer flush()
    fmt.Fprintln(w, "ID\tNAME\tSTATUS")
    for _, item := range resp.Items {
        fmt.Fprintf(w, "%s\t%s\t%s\n", item.Id, item.Name, item.Status)
    }
    printNextPageHint(u, resp.NextPageToken)
    return nil
}
```

### Pattern 2: GET command

```go
type XxxGetCmd struct {
    ResourceID string `arg:"" name:"resourceId" help:"Resource ID"`
}

func (c *XxxGetCmd) Run(ctx context.Context, flags *RootFlags) error {
    account, err := requireAccount(flags)
    if err != nil {
        return err
    }
    resourceID := strings.TrimSpace(c.ResourceID)
    if resourceID == "" {
        return usage("empty resourceId")
    }

    svc, err := newXxxService(ctx, account)
    if err != nil {
        return err
    }

    item, err := svc.Resources.Get(resourceID).Context(ctx).Do()
    if err != nil {
        return err
    }

    if outfmt.IsJSON(ctx) {
        return outfmt.WriteJSON(os.Stdout, map[string]any{"item": item})
    }
    // Print human-readable output
    u := ui.FromContext(ctx)
    u.Out().Printf("ID\t%s", item.Id)
    u.Out().Printf("Name\t%s", item.Name)
    return nil
}
```

### Pattern 3: CREATE command

```go
type XxxCreateCmd struct {
    ParentID string `arg:"" name:"parentId" help:"Parent resource ID"`
    Name     string `name:"name" required:"" help:"Resource name"`
    // ... additional fields
}

func (c *XxxCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
    account, err := requireAccount(flags)
    if err != nil {
        return err
    }
    // Validate inputs...

    svc, err := newXxxService(ctx, account)
    if err != nil {
        return err
    }

    resource := &api.Resource{
        Name: strings.TrimSpace(c.Name),
    }
    created, err := svc.Resources.Create(parentID, resource).Context(ctx).Do()
    if err != nil {
        return err
    }

    if outfmt.IsJSON(ctx) {
        return outfmt.WriteJSON(os.Stdout, map[string]any{"item": created})
    }
    u := ui.FromContext(ctx)
    u.Out().Printf("ID\t%s", created.Id)
    return nil
}
```

### Pattern 4: DELETE command

```go
type XxxDeleteCmd struct {
    ResourceID string `arg:"" name:"resourceId" help:"Resource ID"`
}

func (c *XxxDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
    account, err := requireAccount(flags)
    if err != nil {
        return err
    }
    resourceID := strings.TrimSpace(c.ResourceID)
    if resourceID == "" {
        return usage("empty resourceId")
    }

    if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete resource %s", resourceID)); err != nil {
        return err
    }

    svc, err := newXxxService(ctx, account)
    if err != nil {
        return err
    }

    if err := svc.Resources.Delete(resourceID).Context(ctx).Do(); err != nil {
        return err
    }

    if outfmt.IsJSON(ctx) {
        return outfmt.WriteJSON(os.Stdout, map[string]any{
            "deleted":    true,
            "resourceId": resourceID,
        })
    }
    u := ui.FromContext(ctx)
    u.Out().Printf("deleted\ttrue")
    u.Out().Printf("resourceId\t%s", resourceID)
    return nil
}
```

### Pattern 5: PATCH/UPDATE command

```go
type XxxPatchCmd struct {
    ResourceID string `arg:"" name:"resourceId" help:"Resource ID"`
    Name       string `name:"name" help:"New name"`
}

func (c *XxxPatchCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
    account, err := requireAccount(flags)
    if err != nil {
        return err
    }
    // Validate...

    patch := &api.Resource{}
    changed := false
    if flagProvided(kctx, "name") {
        patch.Name = strings.TrimSpace(c.Name)
        changed = true
    }
    if !changed {
        return usage("no updates provided")
    }

    svc, err := newXxxService(ctx, account)
    if err != nil {
        return err
    }

    updated, err := svc.Resources.Patch(resourceID, patch).Context(ctx).Do()
    if err != nil {
        return err
    }

    if outfmt.IsJSON(ctx) {
        return outfmt.WriteJSON(os.Stdout, map[string]any{"item": updated})
    }
    // Print updated resource...
    return nil
}
```

### Pattern 6: Test with httptest mock

```go
func TestXxxGetCmd_JSON(t *testing.T) {
    origNew := newXxxService
    t.Cleanup(func() { newXxxService = origNew })

    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/resources/"):
            w.Header().Set("Content-Type", "application/json")
            _ = json.NewEncoder(w).Encode(map[string]any{
                "id":   "r1",
                "name": "Test Resource",
            })
        default:
            http.NotFound(w, r)
        }
    }))
    defer srv.Close()

    svc, err := api.NewService(context.Background(),
        option.WithoutAuthentication(),
        option.WithHTTPClient(srv.Client()),
        option.WithEndpoint(srv.URL+"/"),
    )
    if err != nil {
        t.Fatalf("NewService: %v", err)
    }
    newXxxService = func(context.Context, string) (*api.Service, error) { return svc, nil }

    u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
    if err != nil {
        t.Fatalf("ui.New: %v", err)
    }
    ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
    flags := &RootFlags{Account: "a@b.com"}

    out := captureStdout(t, func() {
        cmd := &XxxGetCmd{ResourceID: "r1"}
        if err := cmd.Run(ctx, flags); err != nil {
            t.Fatalf("Run: %v", err)
        }
    })

    var payload map[string]any
    if err := json.Unmarshal([]byte(out), &payload); err != nil {
        t.Fatalf("decode: %v", err)
    }
    // Assert fields...
}
```

## Service Factory Pattern

Each API has a factory in `internal/googleapi/`:

```go
// internal/googleapi/xxx.go
var newXxxService = googleapi.NewXxx

func NewXxx(ctx context.Context, email string) (*xxx.Service, error) {
    opts, err := optionsForAccount(ctx, googleauth.ServiceXxx, email)
    if err != nil {
        return nil, fmt.Errorf("xxx options: %w", err)
    }
    svc, err := xxx.NewService(ctx, opts...)
    if err != nil {
        return nil, fmt.Errorf("create xxx service: %w", err)
    }
    return svc, nil
}
```

## Command Registration

New commands are registered by adding fields to parent structs in `root.go` or resource-level files:

```go
// In root.go CLI struct:
Xxx XxxCmd `cmd:"" help:"Description"`

// In the resource group file:
type XxxCmd struct {
    List   XxxListCmd   `cmd:"" help:"List resources"`
    Get    XxxGetCmd    `cmd:"" help:"Get a resource"`
    Create XxxCreateCmd `cmd:"" help:"Create a resource"`
    Delete XxxDeleteCmd `cmd:"" help:"Delete a resource"`
    Update XxxPatchCmd  `cmd:"" help:"Update a resource"`
}
```

## Do NOT

- Leave `// TODO` comments in code
- Skip or disable tests
- Leave debug logging
- Use `npm` or `node` — this is Go
- Create commands without corresponding tests
- Use hardcoded API endpoints — use the service client
- Skip input validation (always trim + check empty)
- Skip `confirmDestructive()` for delete/destructive operations
- Forget to register commands in parent struct
- Use `cobra` patterns — this is `kong`
