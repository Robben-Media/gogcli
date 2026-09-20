package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/gcloud"
	"github.com/steipete/gogcli/internal/secrets"
)

func setupReportStage(t *testing.T, report SetupReport, id string) SetupStage {
	t.Helper()
	for _, stage := range report.Stages {
		if stage.ID == id {
			return stage
		}
	}
	t.Fatalf("missing stage %q in %#v", id, report.Stages)
	return SetupStage{}
}

func TestAuthSetupClosure_DiscoveryUnauthenticatedIsIncomplete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{
		"version --format=json": {stdout: `{}`}, "auth list --filter=status:ACTIVE --format=json": {stdout: `[]`},
	}}
	withSetupGCloud(t, r)
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() { runErr = Execute([]string{"--json", "--no-input", "auth", "setup", "--discover"}) })
	})
	if ExitCode(runErr) != 1 {
		t.Fatalf("exit=%d, want 1", ExitCode(runErr))
	}
	var report SetupReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.DiscoveryComplete {
		t.Fatalf("unauthenticated discovery complete: %#v", report)
	}
}

func TestAuthSetupClosure_ContinuationQuotesAndPreservesInputs(t *testing.T) {
	cmd := &AuthSetupCmd{CreateProject: true, ProjectName: "O'Reilly demo", ProjectParent: "folders/42", ServicesCSV: "gmail,drive", EnableAPIs: true, CredentialsPath: "/tmp/a b.json", AccountEmail: "a b@example.com", ManualOAuth: true, AckBranding: true, AckAudience: true, AckDataAccess: true, ProjectLimit: 7}
	got := continueSetupCmd("client name", cmd, "project name", true)
	for _, want := range []string{"'gog'", "'client name'", "'project name'", "'O'\\\"'\\\"'Reilly demo'", "'/tmp/a b.json'", "'a b@example.com'", "'--project-limit' '7'"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
}

func TestAuthSetupClosure_ProjectFilteringAndExplicitValidation(t *testing.T) {
	if got := filterActiveProjects([]gcloud.Project{{ProjectID: "active", LifecycleState: "ACTIVE"}, {ProjectID: "deleted", LifecycleState: "DELETE_REQUESTED"}}); len(got) != 1 || got[0].ProjectID != "active" {
		t.Fatalf("active=%#v", got)
	}
	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{"projects describe dead --format=json": {stdout: `{"projectId":"dead","lifecycleState":"DELETE_REQUESTED"}`}}}
	rt := &setupRuntime{cmd: &AuthSetupCmd{Project: "dead"}, gc: gcloud.New(r), u: mustSetupUI(t), report: SetupReport{ProjectID: "dead"}}
	if stop, err := rt.runProject(context.Background()); !stop || err != nil || setupReportStage(t, rt.report, stageProject).Status != stageStatusBlocked {
		t.Fatalf("stop=%t err=%v report=%#v", stop, err, rt.report)
	}
}

func TestAuthSetupClosure_ExplicitProjectSkipsForbiddenList(t *testing.T) {
	r := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{"projects describe demo --format=json": {stdout: `{"projectId":"demo","lifecycleState":"ACTIVE"}`}}}
	rt := &setupRuntime{cmd: &AuthSetupCmd{Project: "demo"}, gc: gcloud.New(r), u: mustSetupUI(t), report: SetupReport{ProjectID: "demo"}, cfg: config.File{}}
	if stop, err := rt.runProject(context.Background()); stop || err != nil {
		t.Fatalf("stop=%t err=%v", stop, err)
	}
	for _, call := range r.calls {
		if strings.Contains(strings.Join(call, " "), "projects list") {
			t.Fatal("explicit project listed globally")
		}
	}
}

func TestAuthSetupClosure_APIInspectionAndReinspection(t *testing.T) {
	permission := &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{"services list --enabled --project demo --format=json": {stderr: "PERMISSION_DENIED", code: 1}}}
	rt := &setupRuntime{cmd: &AuthSetupCmd{}, gc: gcloud.New(permission), u: mustSetupUI(t), report: SetupReport{ProjectID: "demo"}, usageIDs: []string{"gmail.googleapis.com"}}
	if stop, _ := rt.runAPIs(context.Background()); !stop || setupReportStage(t, rt.report, stageAPIs).Status != stageStatusBlocked {
		t.Fatalf("report=%#v", rt.report)
	}

	calls := 0
	r := &seqRunner{inner: &setupFakeRunner{byArgs: map[string]struct {
		stdout, stderr string
		code           int
		err            error
	}{"services enable --project demo": {stderr: "quota", code: 1}}}, listHook: func() (string, int) {
		calls++
		if calls == 1 {
			return `[]`, 0
		}
		return `[{"name":"gmail.googleapis.com","state":"ENABLED"}]`, 0
	}}
	rt = &setupRuntime{cmd: &AuthSetupCmd{EnableAPIs: true}, gc: gcloud.New(r), u: mustSetupUI(t), force: true, report: SetupReport{ProjectID: "demo"}, usageIDs: []string{"gmail.googleapis.com"}}
	if stop, _ := rt.runAPIs(context.Background()); stop || setupReportStage(t, rt.report, stageAPIs).Status != stageStatusOK || len(rt.report.MissingAPIs) != 0 {
		t.Fatalf("report=%#v", rt.report)
	}
}

func TestAuthSetupClosure_DiscoveryDoesNotOpenSecretsAndAccountErrorsSurface(t *testing.T) {
	orig, origNoInput := authSetupOpen, authSetupOpenNoInput
	t.Cleanup(func() { authSetupOpen, authSetupOpenNoInput = orig, origNoInput })
	called := false
	authSetupOpen = func() (secrets.Store, error) { called = true; return nil, errors.New("open") }
	// AuthSetupCmd.Run excludes runAccount entirely for discovery; invoking the
	// discovery runner set must not require this store seam.
	if !(&AuthSetupCmd{Discover: true}).Discover || called {
		t.Fatal("discovery opened secrets")
	}
	authSetupOpenNoInput = func() (secrets.Store, error) { return nil, errors.New("keyring unavailable") }
	rt := &setupRuntime{cmd: &AuthSetupCmd{}, flags: &RootFlags{NoInput: true}, u: mustSetupUI(t), client: "default", services: parseSetupServices(t, "gmail"), report: SetupReport{ProjectID: "demo"}}
	if stop, _ := rt.runAccount(context.Background()); !stop || setupReportStage(t, rt.report, stageAccount).Status != stageStatusFailed {
		t.Fatalf("report=%#v", rt.report)
	}
}

func TestAuthSetupClosure_ProjectlessCredentialsRequireDurableAssociation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	if err := config.WriteClientCredentialsFor("default", config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	rt := &setupRuntime{cmd: &AuthSetupCmd{}, flags: &RootFlags{NoInput: true}, u: mustSetupUI(t), client: "default", priorProjectID: "demo", report: SetupReport{ProjectID: "demo"}}
	if rt.appendCredentialsExisting(context.Background(), "demo") {
		t.Fatal("prior project bypassed association confirmation")
	}
}
