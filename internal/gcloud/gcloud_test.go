package gcloud

import (
	"context"
	"errors"
	"strings"
	"testing"
)

var errFakeMissing = errors.New(`exec: "gcloud": executable file not found in $PATH`)

type fakeRunner struct {
	calls [][]string
	// map key is joined args after binary
	responses map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}
	defaultResp struct {
		stdout, stderr string
		code           int
		err            error
	}
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	_ = ctx

	f.calls = append(f.calls, append([]string{name}, args...))
	key := strings.Join(args, " ")

	if resp, ok := f.responses[key]; ok {
		return resp.stdout, resp.stderr, resp.code, resp.err
	}

	// prefix match for services enable with variable ids
	for k, resp := range f.responses {
		if strings.HasPrefix(key, k) {
			return resp.stdout, resp.stderr, resp.code, resp.err
		}
	}

	return f.defaultResp.stdout, f.defaultResp.stderr, f.defaultResp.code, f.defaultResp.err
}

func TestInstalled_MissingBinary(t *testing.T) {
	r := &fakeRunner{}
	r.defaultResp.err = errFakeMissing
	r.defaultResp.code = -1
	c := New(r)

	ok, res := c.Installed(context.Background())
	if ok {
		t.Fatalf("expected not installed")
	}

	if res.Kind != BlockerNotInstalled {
		t.Fatalf("kind=%q", res.Kind)
	}
}

func TestActiveAccount_NotLoggedIn(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"auth list --filter=status:ACTIVE --format=json": {stdout: "[]", code: 0},
	}}
	c := New(r)

	acct, res, err := c.ActiveAccount(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if acct.Account != "" {
		t.Fatalf("expected empty account")
	}

	if res.Kind != BlockerNotLoggedIn {
		t.Fatalf("kind=%q", res.Kind)
	}
}

func TestActiveAccount_OK(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"auth list --filter=status:ACTIVE --format=json": {
			stdout: `[{"account":"user@example.com","status":"ACTIVE"}]`,
			code:   0,
		},
	}}
	c := New(r)

	acct, _, err := c.ActiveAccount(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if acct.Account != "user@example.com" {
		t.Fatalf("got %q", acct.Account)
	}
}

func TestListProjects_Parses(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"projects list --format=json --limit 100": {
			stdout: `[{"projectId":"p1","name":"One"},{"projectId":"p2","name":"Two"}]`,
			code:   0,
		},
	}}
	c := New(r)

	projects, _, err := c.ListProjects(context.Background(), 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(projects) != 2 || projects[0].ProjectID != "p1" {
		t.Fatalf("got %#v", projects)
	}
}

func TestCreateProject_ParentFlagsAndNoDefault(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"projects create my-proj --format=json --name Demo --folder 123": {
			stdout: `{"projectId":"my-proj","name":"Demo"}`,
			code:   0,
		},
	}}
	c := New(r)

	p, _, err := c.CreateProject(context.Background(), "my-proj", "Demo", "folders/123")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if p.ProjectID != "my-proj" {
		t.Fatalf("got %#v", p)
	}

	if len(r.calls) != 1 {
		t.Fatalf("calls=%v", r.calls)
	}

	joined := strings.Join(r.calls[0], " ")
	if strings.Contains(joined, "--set-as-default") {
		t.Fatalf("must not set default: %s", joined)
	}

	if !strings.Contains(joined, "--folder 123") {
		t.Fatalf("expected folder flag: %s", joined)
	}
}

func TestCreateProject_OrganizationParent(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"projects create my-proj --format=json --organization 9": {
			stdout: `{"projectId":"my-proj"}`,
			code:   0,
		},
	}}
	c := New(r)

	_, _, err := c.CreateProject(context.Background(), "my-proj", "", "organizations/9")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	joined := strings.Join(r.calls[0], " ")
	if !strings.Contains(joined, "--organization 9") {
		t.Fatalf("expected org flag: %s", joined)
	}
}

func TestListEnabledServices_ProjectScoped(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"services list --enabled --project demo --format=json": {
			stdout: `[{"name":"projects/demo/services/gmail.googleapis.com","state":"ENABLED"},{"config":{"name":"drive.googleapis.com"},"state":"ENABLED"}]`,
			code:   0,
		},
	}}
	c := New(r)

	states, _, err := c.ListEnabledServices(context.Background(), "demo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(states) != 2 {
		t.Fatalf("got %#v", states)
	}

	joined := strings.Join(r.calls[0], " ")
	if !strings.Contains(joined, "--project demo") {
		t.Fatalf("project not scoped: %s", joined)
	}
}

func TestEnableServices_PartialMissingAfterVerify(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"services enable --project demo gmail.googleapis.com drive.googleapis.com": {code: 0},
		"services list --enabled --project demo --format=json": {
			stdout: `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`,
			code:   0,
		},
	}}
	c := New(r)

	enabled, missing, _, err := c.EnableServices(context.Background(), "demo", []string{"gmail.googleapis.com", "drive.googleapis.com"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(enabled) != 1 || enabled[0] != "gmail.googleapis.com" {
		t.Fatalf("enabled=%v", enabled)
	}

	if len(missing) != 1 || missing[0] != "drive.googleapis.com" {
		t.Fatalf("missing=%v", missing)
	}
}

func TestMissingServices(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"services list --enabled --project demo --format=json": {
			stdout: `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`,
			code:   0,
		},
	}}
	c := New(r)

	missing, _, _, err := c.MissingServices(context.Background(), "demo", []string{"gmail.googleapis.com", "calendar-json.googleapis.com"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(missing) != 1 || missing[0] != "calendar-json.googleapis.com" {
		t.Fatalf("missing=%v", missing)
	}
}

func TestClassify_Permission(t *testing.T) {
	if got := classify("PERMISSION_DENIED: caller does not have permission", 1); got != BlockerPermission {
		t.Fatalf("got %q", got)
	}
}

func TestClassify_AlreadyInUse(t *testing.T) {
	if got := classify("project ID demo is already in use", 1); got != BlockerAlreadyExists {
		t.Fatalf("got %q", got)
	}
}

func TestSanitize_RedactsSecrets(t *testing.T) {
	if got := sanitize("client_secret=abc access denied"); got != "[redacted gcloud error]" {
		t.Fatalf("got %q", got)
	}
}

type interactiveFakeRunner struct {
	fakeRunner
	interactiveCalls [][]string
}

func (f *interactiveFakeRunner) RunInteractive(ctx context.Context, name string, args ...string) (int, error) {
	_ = ctx

	f.interactiveCalls = append(f.interactiveCalls, append([]string{name}, args...))

	return 0, nil
}

func TestLoginUsesInteractiveRunner(t *testing.T) {
	r := &interactiveFakeRunner{}
	if got := New(r).Login(context.Background()); got.ExitCode != 0 {
		t.Fatalf("login=%#v", got)
	}

	if len(r.interactiveCalls) != 1 || len(r.calls) != 0 {
		t.Fatalf("interactive=%v captured=%v", r.interactiveCalls, r.calls)
	}
}

func TestListProjectsUsesBoundedLimit(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"projects list --format=json --limit 2": {stdout: `[]`},
	}}
	if _, _, err := New(r).ListProjects(context.Background(), 2); err != nil {
		t.Fatal(err)
	}

	if len(r.calls) != 1 || !strings.Contains(strings.Join(r.calls[0], " "), "--limit 2") {
		t.Fatalf("calls=%v", r.calls)
	}
}

func TestNeverUsesConfigSet(t *testing.T) {
	r := &fakeRunner{responses: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"projects list --format=json --limit 100":           {stdout: "[]", code: 0},
		"auth list --filter=status:ACTIVE --format=json":    {stdout: "[]", code: 0},
		"config get-value project --format=json":            {stdout: "null", code: 0},
		"services list --enabled --project p --format=json": {stdout: "[]", code: 0},
		"version --format=json":                             {stdout: `{"Google Cloud SDK":"1"}`, code: 0},
		"projects create p --format=json":                   {stdout: `{"projectId":"p"}`, code: 0},
		"services enable --project p gmail.googleapis.com":  {code: 0},
		"auth login --brief":                                {code: 0},
	}}
	c := New(r)
	ctx := context.Background()
	_, _ = c.Installed(ctx)
	_, _, _ = c.ActiveAccount(ctx)
	_, _, _ = c.ListProjects(ctx, 100)
	_, _, _ = c.ActiveProjectID(ctx)
	_, _, _ = c.CreateProject(ctx, "p", "", "")
	_, _, _ = c.ListEnabledServices(ctx, "p")
	_, _, _, _ = c.EnableServices(ctx, "p", []string{"gmail.googleapis.com"})
	_ = c.Login(ctx)

	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "config set") || strings.Contains(joined, "--set-as-default") || strings.Contains(joined, "application-default") {
			t.Fatalf("forbidden call: %s", joined)
		}
	}
}
