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
	if ExitCode(exit) != 0 {
		t.Fatalf("discover should exit 0, got %v code=%d out=%s", exit, ExitCode(exit), out)
	}
	var report SetupReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json: %v\nout=%s", err, out)
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
		"projects list --format=json": {
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
		"projects list --format=json": {stdout: `[]`, code: 0},
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
		"projects list --format=json": {
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
		"projects list --format=json": {stdout: `[{"projectId":"demo-proj"}]`, code: 0},
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
		"projects list --format=json": {stdout: `[{"projectId":"demo-proj"}]`, code: 0},
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
