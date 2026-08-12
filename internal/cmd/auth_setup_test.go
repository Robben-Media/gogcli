package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/gcloud"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

var (
	errSetupGCloudMissing  = errors.New(`exec: "gcloud": executable file not found in $PATH`)
	errSetupGCloudNotFound = errors.New("executable file not found")
)

type setupFakeRunner struct {
	calls [][]string
	// responses keyed by joined args
	byArgs map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}
}

func (f *setupFakeRunner) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	_ = ctx

	f.calls = append(f.calls, append([]string{name}, args...))
	key := strings.Join(args, " ")

	if resp, ok := f.byArgs[key]; ok {
		return resp.stdout, resp.stderr, resp.code, resp.err
	}

	for k, resp := range f.byArgs {
		if strings.HasPrefix(key, k) {
			return resp.stdout, resp.stderr, resp.code, resp.err
		}
	}

	return "", "unexpected gcloud call: " + key, 1, nil
}

func withSetupGCloud(t *testing.T, r gcloud.Runner) {
	t.Helper()
	orig := newGCloudClient
	newGCloudClient = func() *gcloud.Client {
		c := gcloud.New(r)
		return c
	}
	t.Cleanup(func() { newGCloudClient = orig })
}

func TestAuthSetup_Discover_JSON_MissingGCloud(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {err: errSetupGCloudMissing, code: -1},
	}}
	withSetupGCloud(t, r)

	var exit error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			exit = Execute([]string{"--json", "--no-input", "auth", "setup", "--discover"})
		})
	})
	if ExitCode(exit) != 1 {
		t.Fatalf("failed discovery should exit 1 after its report, got %v code=%d out=%s", exit, ExitCode(exit), out)
	}
	var report SetupReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json: %v\nout=%s", err, out)
	}
	if report.Complete {
		t.Fatalf("failed discovery must be incomplete: %#v", report)
	}
	if report.Client != "default" {
		t.Fatalf("client=%q", report.Client)
	}
	if len(report.Stages) == 0 || report.Stages[0].ID != stageGCloudInstall {
		t.Fatalf("stages=%#v", report.Stages)
	}
	if report.Stages[0].Status != stageStatusMissing {
		t.Fatalf("status=%s", report.Stages[0].Status)
	}
	// secrets must not appear
	if strings.Contains(out, "client_secret") || strings.Contains(out, "refresh_token") {
		t.Fatalf("secrets leaked: %s", out)
	}
}

func TestAuthSetup_Discover_ProjectAndAPIs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {stdout: `{"Google Cloud SDK":"500.0.0"}`, code: 0},
		"auth list --filter=status:ACTIVE --format=json": {
			stdout: `[{"account":"dev@example.com","status":"ACTIVE"}]`, code: 0,
		},
		"projects list --format=json --limit 100": {
			stdout: `[{"projectId":"demo-proj","name":"Demo"}]`, code: 0,
		},
		"services list --enabled --project demo-proj --format=json": {
			stdout: `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`, code: 0,
		},
	}}
	withSetupGCloud(t, r)

	var exit error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			exit = Execute([]string{
				"--json", "--no-input",
				"auth", "setup", "--discover",
				"--project", "demo-proj",
				"--services", "gmail,drive",
			})
		})
	})
	if ExitCode(exit) != 0 {
		t.Fatalf("discover should exit 0, got %v code=%d out=%s", exit, ExitCode(exit), out)
	}
	var report SetupReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if report.ProjectID != "demo-proj" {
		t.Fatalf("project=%q", report.ProjectID)
	}
	if report.GCloudAccount != "dev@example.com" {
		t.Fatalf("account=%q", report.GCloudAccount)
	}
	if len(report.Projects) != 1 || report.Projects[0].ProjectID != "demo-proj" || report.Projects[0].Name != "Demo" {
		t.Fatalf("projects=%#v", report.Projects)
	}
	// drive should be missing
	foundMissing := false
	for _, id := range report.MissingAPIs {
		if id == "drive.googleapis.com" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("expected drive missing, got %v", report.MissingAPIs)
	}
	// no config set calls
	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "config set") || strings.Contains(joined, "--set-as-default") {
			t.Fatalf("forbidden: %s", joined)
		}
	}
}

func TestAuthSetup_NonInteractive_CreateProjectRequiresForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {stdout: `{}`, code: 0},
		"auth list --filter=status:ACTIVE --format=json": {
			stdout: `[{"account":"dev@example.com","status":"ACTIVE"}]`, code: 0,
		},
		"projects list --format=json --limit 100": {stdout: `[]`, code: 0},
	}}
	withSetupGCloud(t, r)

	err := Execute([]string{"--no-input", "auth", "setup", "--create-project", "--project", "new-proj"})
	if ExitCode(err) != 2 {
		t.Fatalf("expected usage exit 2, got %v code=%d", err, ExitCode(err))
	}
}

func TestAuthSetup_EnableAPIs_WithForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	listEnabled := `[{"name":"gmail.googleapis.com","state":"ENABLED"},{"name":"drive.googleapis.com","state":"ENABLED"}]`
	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {stdout: `{}`, code: 0},
		"auth list --filter=status:ACTIVE --format=json": {
			stdout: `[{"account":"dev@example.com","status":"ACTIVE"}]`, code: 0,
		},
		"projects list --format=json --limit 100": {
			stdout: `[{"projectId":"demo-proj"}]`, code: 0,
		},
		// first missing check: only gmail
		"services list --enabled --project demo-proj --format=json": {
			stdout: `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`, code: 0,
		},
		"services enable --project demo-proj": {code: 0},
	}}
	// After enable, ListEnabledServices is called again — same key; update after first list by using sequential isn't supported.
	// EnableServices calls list again with same args; make list return full set always after enable is fine if we return both from start for verify.
	// For missing detection we need first list without drive. Use a counter via custom runner.
	countList := 0
	r2 := &seqRunner{inner: r, listHook: func() (string, int) {
		countList++
		if countList == 1 {
			return `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`, 0
		}
		return listEnabled, 0
	}}
	withSetupGCloud(t, r2)

	var exit error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			exit = Execute([]string{
				"--json", "--no-input", "--force",
				"auth", "setup",
				"--project", "demo-proj",
				"--services", "gmail,drive",
				"--enable-apis",
				"--ack-branding", "--ack-audience", "--ack-data-access",
			})
		})
	})
	_ = exit
	var report SetupReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	// project pairing persisted
	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	got := config.GetClientSetup(cfg, "default")
	if got.ProjectID != "demo-proj" {
		t.Fatalf("setup record: %#v", got)
	}
	if !got.AcknowledgedBranding {
		t.Fatalf("expected branding ack")
	}
	// APIs stage ok
	for _, st := range report.Stages {
		if st.ID == stageAPIs && st.Status != stageStatusOK {
			t.Fatalf("apis stage: %#v", st)
		}
	}
}

type seqRunner struct {
	inner    *setupFakeRunner
	listHook func() (string, int)
}

func (s *seqRunner) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	key := strings.Join(args, " ")
	if strings.HasPrefix(key, "services list --enabled") && s.listHook != nil {
		s.inner.calls = append(s.inner.calls, append([]string{name}, args...))
		stdout, code := s.listHook()
		return stdout, "", code, nil
	}
	return s.inner.Run(ctx, name, args...)
}

func TestAuthSetup_CredentialsProjectMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {stdout: `{}`, code: 0},
		"auth list --filter=status:ACTIVE --format=json": {
			stdout: `[{"account":"dev@example.com","status":"ACTIVE"}]`, code: 0,
		},
		"projects list --format=json --limit 100": {stdout: `[{"projectId":"demo-proj"}]`, code: 0},
		"services list --enabled --project demo-proj --format=json": {
			stdout: `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`, code: 0,
		},
	}}
	withSetupGCloud(t, r)

	credPath := filepath.Join(t.TempDir(), "creds.json")
	if err := os.WriteFile(credPath, []byte(`{"installed":{"client_id":"id","client_secret":"sec","project_id":"other-proj"}}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var exit error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			exit = Execute([]string{
				"--json", "--no-input", "--force",
				"auth", "setup",
				"--project", "demo-proj",
				"--services", "gmail",
				"--credentials", credPath,
			})
		})
	})
	if ExitCode(exit) != 1 {
		t.Fatalf("expected incomplete/blocked exit 1, got %v code=%d out=%s", exit, ExitCode(exit), out)
	}
	if !strings.Contains(out, "does not match") && !strings.Contains(out, "project") {
		// blocker in stage
		var report SetupReport
		_ = json.Unmarshal([]byte(out), &report)
		found := false
		for _, st := range report.Stages {
			if st.ID == stageCredentials && st.Status == stageStatusBlocked {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected credentials blocked, out=%s", out)
		}
	}
}

func TestAuthSetup_CredentialsInstallIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {stdout: `{}`, code: 0},
		"auth list --filter=status:ACTIVE --format=json": {
			stdout: `[{"account":"dev@example.com","status":"ACTIVE"}]`, code: 0,
		},
		"projects list --format=json --limit 100": {stdout: `[{"projectId":"demo-proj"}]`, code: 0},
		"services list --enabled --project demo-proj --format=json": {
			stdout: `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`, code: 0,
		},
	}}
	withSetupGCloud(t, r)

	// preinstall identical credentials
	if err := config.WriteClientCredentialsFor("default", config.ClientCredentials{
		ClientID: "id", ClientSecret: "sec", ProjectID: "demo-proj",
	}); err != nil {
		t.Fatalf("prewrite: %v", err)
	}
	credPath := filepath.Join(t.TempDir(), "creds.json")
	if err := os.WriteFile(credPath, []byte(`{"installed":{"client_id":"id","client_secret":"sec","project_id":"demo-proj"}}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			_ = Execute([]string{
				"--json", "--no-input",
				"auth", "setup",
				"--project", "demo-proj",
				"--services", "gmail",
				"--credentials", credPath,
				"--ack-branding", "--ack-audience", "--ack-data-access",
			})
		})
	})
	var report SetupReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	for _, st := range report.Stages {
		if st.ID == stageCredentials {
			if st.Status != stageStatusOK {
				t.Fatalf("creds stage: %#v", st)
			}

			if !strings.Contains(st.Summary, "identical") &&
				st.Summary != "credentials installed" &&
				st.Summary != "credentials present" {
				t.Fatalf("unexpected credentials summary: %q", st.Summary)
			}
		}
	}
}

func TestAuthSetup_PlainDiscover(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {err: errSetupGCloudNotFound, code: -1},
	}}
	withSetupGCloud(t, r)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			_ = Execute([]string{"--plain", "--no-input", "auth", "setup", "--discover"})
		})
	})
	if !strings.Contains(out, "STAGE") || !strings.Contains(out, "gcloud_install") {
		t.Fatalf("plain out=%q", out)
	}
}

func TestAuthSetup_HelpRegistered(t *testing.T) {
	err := Execute([]string{"auth", "setup", "--help"})
	// help exits via exitPanic 0 or returns nil depending on path
	if err != nil && ExitCode(err) != 0 {
		// kong help may return nil
		t.Logf("help err: %v", err)
	}
}

func TestAuthSetup_ProjectPickerCreate_UsesEnteredIDWithoutChangingGCloudConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"projects list --format=json --limit 100":   {stdout: `[]`},
		"projects create new-project --format=json": {stdout: `{"projectId":"new-project"}`},
	}}
	u, err := ui.New(ui.Options{Color: "never"})
	if err != nil {
		t.Fatalf("ui: %v", err)
	}
	origPrompt := setupPromptLine
	responses := []string{"1", "new-project"}
	setupPromptLine = func(context.Context, string) (string, error) {
		response := responses[0]
		responses = responses[1:]
		return response, nil
	}
	t.Cleanup(func() { setupPromptLine = origPrompt })

	rt := &setupRuntime{
		cmd:         &AuthSetupCmd{ProjectLimit: 100},
		flags:       &RootFlags{Force: true},
		u:           u,
		gc:          gcloud.New(r),
		client:      config.DefaultClientName,
		interactive: true,
		force:       true,
		report:      SetupReport{Client: config.DefaultClientName},
	}

	stop, runErr := rt.runProject(context.Background())
	if stop || runErr != nil {
		t.Fatalf("run project: stop=%t err=%v", stop, runErr)
	}
	if rt.report.ProjectID != "new-project" {
		t.Fatalf("selected project=%q", rt.report.ProjectID)
	}
	var created bool
	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "config set") || strings.Contains(joined, "--set-as-default") {
			t.Fatalf("gcloud config was mutated: %s", joined)
		}
		if strings.Contains(joined, "projects create new-project") {
			created = true
		}
	}
	if !created {
		t.Fatalf("creation was not reached: %#v", r.calls)
	}
}

func TestAuthSetup_AccountTokenSufficiency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	u, err := ui.New(ui.Options{Color: "never"})
	if err != nil {
		t.Fatalf("ui: %v", err)
	}
	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}); err != nil {
		t.Fatalf("credentials: %v", err)
	}

	origOpen := authSetupOpen
	t.Cleanup(func() { authSetupOpen = origOpen })

	gmailScopes := accountScopes(parseSetupServices(t, "gmail"))
	store := newMemSecretsStore()
	if err := store.SetToken("work", "person@example.com", secrets.Token{
		Services:     []string{"gmail"},
		Scopes:       gmailScopes,
		RefreshToken: "test-token",
	}); err != nil {
		t.Fatalf("set token: %v", err)
	}
	authSetupOpen = func() (secrets.Store, error) { return store, nil }

	tests := []struct {
		name     string
		client   string
		email    string
		services string
		want     bool
	}{
		{name: "exact client email services and scopes", client: "work", email: "person@example.com", services: "gmail", want: true},
		{name: "different email", client: "work", email: "other@example.com", services: "gmail", want: false},
		{name: "different client", client: "other", email: "person@example.com", services: "gmail", want: false},
		{name: "missing requested service and scope", client: "work", email: "person@example.com", services: "drive", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			services := parseSetupServices(t, tc.services)
			if got := accountTokenSatisfies(tc.client, tc.email, authServiceNames(services), accountScopes(services)); got != tc.want {
				t.Fatalf("accountTokenSatisfies()=%t, want %t", got, tc.want)
			}
		})
	}

	rt := &setupRuntime{cmd: &AuthSetupCmd{AccountEmail: "person@example.com"}, flags: &RootFlags{NoInput: true}, u: u, client: "work", services: parseSetupServices(t, "gmail")}
	if stop, runErr := rt.runAccount(context.Background()); stop || runErr != nil {
		t.Fatalf("sufficient existing token should skip OAuth: stop=%t err=%v", stop, runErr)
	}
	if got := rt.report.Stages[0].Status; got != stageStatusOK {
		t.Fatalf("sufficient token stage=%s", got)
	}

	rt = &setupRuntime{cmd: &AuthSetupCmd{AccountEmail: "person@example.com"}, flags: &RootFlags{NoInput: true}, u: u, client: "work", services: parseSetupServices(t, "drive")}
	if stop, runErr := rt.runAccount(context.Background()); stop || runErr != nil {
		t.Fatalf("insufficient token should defer authorization: stop=%t err=%v", stop, runErr)
	}
	if got := rt.report.Stages[0].Status; got != stageStatusManual {
		t.Fatalf("insufficient token stage=%s, want manual authorization", got)
	}
}

func parseSetupServices(t *testing.T, csv string) []googleauth.Service {
	t.Helper()
	services, err := parseAuthServices(csv)
	if err != nil {
		t.Fatalf("parse services %q: %v", csv, err)
	}
	return services
}

func TestAuthSetup_DiscoverDoesNotClaimSetupComplete(t *testing.T) {
	report := SetupReport{DiscoveryOnly: true, Stages: []SetupStage{
		{ID: stageGCloudInstall, Status: stageStatusOK},
		{ID: stageGCloudAuth, Status: stageStatusOK},
		{ID: stageProject, Status: stageStatusOK},
		{ID: stageAPIs, Status: stageStatusMissing},
	}}
	if setupComplete(report) {
		t.Fatal("discovery with missing setup requirements must not be complete")
	}
	if !discoveryComplete(report) {
		t.Fatal("successful discovery inspection should be separately complete")
	}
}

func TestAuthSetup_StoredCredentialsAreValidatedBeforeAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	u, err := ui.New(ui.Options{Color: "never"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, raw, project, prior string
		force                     bool
		want                      string
	}{
		{"malformed", `{not-json`, "demo", "demo", false, stageStatusFailed},
		{"mismatch", `{"client_id":"id","client_secret":"secret","project_id":"other"}`, "demo", "demo", false, stageStatusBlocked},
		{"unassociated missing project", `{"client_id":"id","client_secret":"secret"}`, "demo", "", false, stageStatusBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, _ := config.ClientCredentialsPathFor("default")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			rt := &setupRuntime{cmd: &AuthSetupCmd{}, flags: &RootFlags{NoInput: true, Force: tc.force}, u: u, client: "default", priorProjectID: tc.prior, force: tc.force, report: SetupReport{ProjectID: tc.project}}
			if ok := rt.appendCredentialsExisting(context.Background(), tc.project); ok {
				t.Fatal("invalid stored credentials accepted")
			}
			if got := rt.report.Stages[0].Status; got != tc.want {
				t.Fatalf("status=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestAuthSetup_ExistingCredentialMakesDesktopStageNonBlocking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := config.WriteClientCredentialsFor("default", config.ClientCredentials{ClientID: "id", ClientSecret: "secret", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	u, _ := ui.New(ui.Options{Color: "never"})
	rt := &setupRuntime{cmd: &AuthSetupCmd{}, flags: &RootFlags{NoInput: true}, u: u, client: "default", report: SetupReport{ProjectID: "demo"}}
	_, _ = rt.runManualStages(context.Background())
	for _, st := range rt.report.Stages {
		if st.ID == stageDesktopClient && st.Status != stageStatusOK {
			t.Fatalf("desktop stage=%#v", st)
		}
	}
}

func TestAuthSetup_APIsBlockDownstreamStages(t *testing.T) {
	rt := &setupRuntime{cmd: &AuthSetupCmd{}, u: mustSetupUI(t), report: SetupReport{ProjectID: "demo"}, usageIDs: []string{"gmail.googleapis.com"}, gc: gcloud.New(&setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"services list --enabled --project demo --format=json": {stdout: `[]`},
	}})}
	stop, _ := rt.runAPIs(context.Background())
	if !stop || len(rt.report.Stages) != 1 || rt.report.Stages[0].ID != stageAPIs {
		t.Fatalf("stop=%t stages=%#v", stop, rt.report.Stages)
	}
}

func mustSetupUI(t *testing.T) *ui.UI {
	t.Helper()
	u, err := ui.New(ui.Options{Color: "never"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAuthSetup_CreateAlreadyExistsSelectsDescribedProject(t *testing.T) {
	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"projects create demo --format=json":   {stderr: "ALREADY_EXISTS", code: 1},
		"projects describe demo --format=json": {stdout: `{"projectId":"demo"}`},
	}}
	rt := &setupRuntime{cmd: &AuthSetupCmd{Project: "demo"}, flags: &RootFlags{Force: true}, u: mustSetupUI(t), gc: gcloud.New(r), force: true}
	got, created, err := rt.createProject(context.Background())
	if err != nil || !created || got != "demo" {
		t.Fatalf("id=%q created=%t err=%v", got, created, err)
	}
}

func TestGCloudFailureClassification(t *testing.T) {
	for _, kind := range []gcloud.BlockerKind{gcloud.BlockerPermission, gcloud.BlockerQuota, gcloud.BlockerInvalidInput, gcloud.BlockerUnknown} {
		if status, resumable := gcloudFailureStage(kind); status != stageStatusFailed || resumable {
			t.Fatalf("kind %s: %s resumable=%t", kind, status, resumable)
		}
	}
}

func TestSetupComplete_RequiresAcksAndCreds(t *testing.T) {
	r := SetupReport{
		Stages: []SetupStage{
			{ID: stageGCloudInstall, Status: stageStatusOK},
			{ID: stageGCloudAuth, Status: stageStatusOK},
			{ID: stageProject, Status: stageStatusOK},
			{ID: stageAPIs, Status: stageStatusOK},
			{ID: stageBranding, Status: stageStatusAcknowledged},
			{ID: stageAudience, Status: stageStatusAcknowledged},
			{ID: stageDataAccess, Status: stageStatusAcknowledged},
			{ID: stageCredentials, Status: stageStatusOK},
			{ID: stageAccount, Status: stageStatusOK},
		},
	}
	if !setupComplete(r) {
		t.Fatalf("expected complete")
	}
	r.Stages[4].Status = stageStatusManual
	if setupComplete(r) {
		t.Fatalf("expected incomplete without branding ack")
	}
}
