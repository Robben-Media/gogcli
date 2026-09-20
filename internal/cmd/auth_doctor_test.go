package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

func setupDoctorEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "test-password")
}

func writeDoctorCredentials(t *testing.T) {
	t.Helper()
	if err := config.WriteClientCredentials(config.ClientCredentials{
		ClientID:     "client-id",
		ClientSecret: "super-secret-value",
	}); err != nil {
		t.Fatalf("WriteClientCredentials: %v", err)
	}
}

func withDoctorSeams(t *testing.T, inspector secrets.TokenInspector, check func(context.Context, string, string, []string, time.Duration) error) {
	t.Helper()
	origOpen := openSecretsInspector
	origCheck := checkRefreshToken
	t.Cleanup(func() {
		openSecretsInspector = origOpen
		checkRefreshToken = origCheck
	})
	openSecretsInspector = func() (secrets.TokenInspector, error) {
		if inspector == nil {
			return nil, errors.New("keyring unavailable: simulated")
		}
		return inspector, nil
	}
	if check == nil {
		check = func(context.Context, string, string, []string, time.Duration) error { return nil }
	}
	checkRefreshToken = check
}

func doctorUIContext(t *testing.T, mode outfmt.Mode) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return outfmt.WithMode(ui.WithUI(context.Background(), u), mode)
}

func parseDoctorJSON(t *testing.T, raw string) doctorReport {
	t.Helper()
	var report doctorReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode doctor JSON: %v\nout=%q", err, raw)
	}
	return report
}

func checkByID(t *testing.T, report doctorReport, id string) doctorCheck {
	t.Helper()
	for _, c := range report.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %q not found in %#v", id, report.Checks)
	return doctorCheck{}
}

func TestAuthDoctor_Healthy_JSON(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)

	store := newMemSecretsStore()
	if err := store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{
		Services:     []string{"gmail"},
		Scopes:       []string{"s1"},
		RefreshToken: "rt-good",
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	withDoctorSeams(t, store, func(_ context.Context, client, refresh string, _ []string, _ time.Duration) error {
		if client != config.DefaultClientName || refresh != "rt-good" {
			return errors.New("unexpected token args")
		}
		return nil
	})

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if runErr != nil {
		t.Fatalf("expected success, got %v", runErr)
	}
	if ExitCode(runErr) != 0 {
		t.Fatalf("expected exit 0, got %d", ExitCode(runErr))
	}

	report := parseDoctorJSON(t, out)
	if !report.Healthy || report.Status != doctorStatusOK {
		t.Fatalf("expected healthy ok report, got %#v", report)
	}
	if checkByID(t, report, doctorCheckConfig).Status != doctorStatusOK {
		t.Fatalf("config check: %#v", checkByID(t, report, doctorCheckConfig))
	}
	if checkByID(t, report, doctorCheckKeyring).Status != doctorStatusOK {
		t.Fatalf("keyring check: %#v", checkByID(t, report, doctorCheckKeyring))
	}
	if checkByID(t, report, doctorCheckCredentials).Status != doctorStatusOK {
		t.Fatalf("credentials check: %#v", checkByID(t, report, doctorCheckCredentials))
	}
	if checkByID(t, report, doctorCheckAccounts).Status != doctorStatusOK {
		t.Fatalf("accounts check: %#v", checkByID(t, report, doctorCheckAccounts))
	}
	tok := checkByID(t, report, "token:a@b.com")
	if tok.Status != doctorStatusOK {
		t.Fatalf("token check: %#v", tok)
	}
	if strings.Contains(out, "super-secret-value") || strings.Contains(out, "rt-good") {
		t.Fatalf("secrets leaked in JSON output: %q", out)
	}
}

func TestAuthDoctor_MissingCredentials(t *testing.T) {
	setupDoctorEnv(t)
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt"})
	withDoctorSeams(t, store, nil)

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected non-zero exit, err=%v out=%q", runErr, out)
	}
	report := parseDoctorJSON(t, out)
	if report.Healthy || report.Status != doctorStatusFail {
		t.Fatalf("expected unhealthy fail, got %#v", report)
	}
	cred := checkByID(t, report, doctorCheckCredentials)
	if cred.Status != doctorStatusFail || !strings.Contains(cred.Recovery, "gog auth credentials") {
		t.Fatalf("unexpected credentials check: %#v", cred)
	}
}

func TestAuthDoctor_InaccessibleKeyring(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	withDoctorSeams(t, nil, nil)

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected non-zero exit, err=%v", runErr)
	}
	report := parseDoctorJSON(t, out)
	if report.Healthy {
		t.Fatalf("expected unhealthy: %#v", report)
	}
	if checkByID(t, report, doctorCheckKeyring).Status != doctorStatusFail {
		t.Fatalf("keyring: %#v", checkByID(t, report, doctorCheckKeyring))
	}
	if checkByID(t, report, doctorCheckAccounts).Status != doctorStatusSkip {
		t.Fatalf("accounts should skip: %#v", checkByID(t, report, doctorCheckAccounts))
	}
	if checkByID(t, report, "tokens").Status != doctorStatusSkip {
		t.Fatalf("tokens should skip: %#v", checkByID(t, report, "tokens"))
	}
	// Independent checks still ran.
	if checkByID(t, report, doctorCheckConfig).Status != doctorStatusOK {
		t.Fatalf("config should still run: %#v", checkByID(t, report, doctorCheckConfig))
	}
	if checkByID(t, report, doctorCheckCredentials).Status != doctorStatusOK {
		t.Fatalf("stored credentials should still be inspected: %#v", checkByID(t, report, doctorCheckCredentials))
	}
}

func TestAuthDoctor_NoAccounts(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	withDoctorSeams(t, newMemSecretsStore(), nil)

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected non-zero exit, err=%v out=%q", runErr, out)
	}
	report := parseDoctorJSON(t, out)
	acc := checkByID(t, report, doctorCheckAccounts)
	if acc.Status != doctorStatusFail || !strings.Contains(acc.Recovery, "gog auth add") {
		t.Fatalf("unexpected accounts check: %#v", acc)
	}
}

func TestAuthDoctor_InvalidToken(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt-bad"})
	withDoctorSeams(t, store, func(context.Context, string, string, []string, time.Duration) error {
		return errors.New("refresh access token: invalid_grant")
	})

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected non-zero exit, err=%v", runErr)
	}
	report := parseDoctorJSON(t, out)
	tok := checkByID(t, report, "token:a@b.com")
	if tok.Status != doctorStatusFail || !strings.Contains(tok.Recovery, "gog auth add a@b.com") {
		t.Fatalf("unexpected token check: %#v", tok)
	}
}

func TestAuthDoctor_ServiceAccountOnly(t *testing.T) {
	setupDoctorEnv(t)
	withDoctorSeams(t, newMemSecretsStore(), nil)

	email := "sa-user@example.com"
	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com","private_key":"x"}`), 0o600); err != nil {
		t.Fatalf("write sa key: %v", err)
	}
	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "set", email, "--key", keyPath}); err != nil {
				t.Fatalf("service-account set: %v", err)
			}
		})
	})

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if runErr != nil {
		t.Fatalf("expected success for SA-only, got %v out=%q", runErr, out)
	}
	report := parseDoctorJSON(t, out)
	if !report.Healthy {
		t.Fatalf("expected healthy SA-only report: %#v", report)
	}
	if credentials := checkByID(t, report, doctorCheckCredentials); credentials.Status != doctorStatusSkip {
		t.Fatalf("service-account-only setup should not require OAuth credentials: %#v", credentials)
	}
	tok := checkByID(t, report, "token:"+normalizeEmail(email))
	if tok.Status != doctorStatusSkip || !strings.Contains(tok.Summary, "not applicable") {
		t.Fatalf("expected SA token skip, got %#v", tok)
	}
	if strings.Contains(strings.ToLower(tok.Summary), "usable") {
		t.Fatalf("SA identity must not look OAuth-validated: %#v", tok)
	}
}

func TestAuthDoctor_MultiAccountPartialFailure(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "good@example.com", secrets.Token{RefreshToken: "rt-good"})
	_ = store.SetToken(config.DefaultClientName, "bad@example.com", secrets.Token{RefreshToken: "rt-bad"})
	withDoctorSeams(t, store, func(_ context.Context, _ string, refresh string, _ []string, _ time.Duration) error {
		if refresh == "rt-bad" {
			return errors.New("refresh access token: revoked")
		}
		return nil
	})

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected non-zero exit, err=%v", runErr)
	}
	report := parseDoctorJSON(t, out)
	if checkByID(t, report, "token:good@example.com").Status != doctorStatusOK {
		t.Fatalf("good token missing/failed: %#v", report.Checks)
	}
	if checkByID(t, report, "token:bad@example.com").Status != doctorStatusFail {
		t.Fatalf("bad token missing/failed: %#v", report.Checks)
	}
}

func TestAuthDoctor_PlainAndHumanOutput(t *testing.T) {
	setupDoctorEnv(t)
	// An OAuth token without credentials fails with recovery guidance.
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt"})
	withDoctorSeams(t, store, nil)

	// Plain: parseable stdout; recovery guidance stays on stderr.
	var plainOut, plainErrOut bytes.Buffer
	plainUI, err := ui.New(ui.Options{Stdout: &plainOut, Stderr: &plainErrOut, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New plain: %v", err)
	}
	plainCtx := outfmt.WithMode(ui.WithUI(context.Background(), plainUI), outfmt.Mode{Plain: true})
	plainErr := (&AuthDoctorCmd{}).Run(plainCtx)
	if ExitCode(plainErr) == 0 {
		t.Fatalf("expected non-zero plain exit")
	}
	if !strings.Contains(plainOut.String(), "status\tfail") || !strings.Contains(plainOut.String(), "healthy\tfalse") {
		t.Fatalf("plain missing status rows: %q", plainOut.String())
	}
	if !strings.Contains(plainOut.String(), "check\tcredentials\tfail\t") {
		t.Fatalf("plain missing credentials check: %q", plainOut.String())
	}
	if strings.Contains(plainOut.String(), "gog auth credentials") {
		t.Fatalf("plain stdout must not include recovery guidance: %q", plainOut.String())
	}
	if !strings.Contains(plainErrOut.String(), "Recovery hints:") || !strings.Contains(plainErrOut.String(), "gog auth credentials") {
		t.Fatalf("plain recovery guidance should be on stderr: %q", plainErrOut.String())
	}

	// Human: stdout is readable rather than the stable --plain TSV.
	var humanOut, humanErrOut bytes.Buffer
	humanUI, err := ui.New(ui.Options{Stdout: &humanOut, Stderr: &humanErrOut, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New human: %v", err)
	}
	humanCtx := outfmt.WithMode(ui.WithUI(context.Background(), humanUI), outfmt.Mode{})
	humanErr := (&AuthDoctorCmd{}).Run(humanCtx)
	if ExitCode(humanErr) == 0 {
		t.Fatalf("expected non-zero human exit")
	}
	if !strings.Contains(humanOut.String(), "Auth doctor: FAIL") || !strings.Contains(humanOut.String(), "[FAIL] credentials:") {
		t.Fatalf("human stdout should summarize checks: %q", humanOut.String())
	}
	if strings.Contains(humanOut.String(), "status\tfail") {
		t.Fatalf("human stdout should differ from --plain TSV: %q", humanOut.String())
	}
	if !strings.Contains(humanErrOut.String(), "Recovery hints:") || !strings.Contains(humanErrOut.String(), "gog auth credentials") {
		t.Fatalf("expected recovery hints on stderr: %q", humanErrOut.String())
	}
}

func TestAuthDoctor_RedactsSecretsInTokenErrors(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	store := newMemSecretsStore()
	secretRT := "rt-should-not-leak-12345"
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: secretRT})
	withDoctorSeams(t, store, func(context.Context, string, string, []string, time.Duration) error {
		return errors.New("refresh failed using token " + secretRT + " and secret super-secret-value")
	})

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected failure")
	}
	if strings.Contains(out, secretRT) || strings.Contains(out, "super-secret-value") {
		t.Fatalf("secret material leaked: %q", out)
	}
	report := parseDoctorJSON(t, out)
	detail := checkByID(t, report, "token:a@b.com").Detail
	if !strings.Contains(detail, "[redacted]") {
		t.Fatalf("expected redacted detail, got %q", detail)
	}
}

func TestAuthDoctor_WarningOnlyExitsZero(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	// Force file backend via env (already set) and clear password + non-TTY stdin simulation
	// by relying on checkDoctorKeyring warning path: backend file, no password env, non-terminal stdin.
	t.Setenv("GOG_KEYRING_PASSWORD", "")

	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt"})
	withDoctorSeams(t, store, nil)

	// Ensure stdin is not a terminal by pointing to a pipe-backed file.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	_ = w.Close()
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if runErr != nil {
		t.Fatalf("warning-only should exit successfully, got %v out=%q", runErr, out)
	}
	report := parseDoctorJSON(t, out)
	if !report.Healthy || report.Status != doctorStatusWarn {
		t.Fatalf("expected healthy warn report, got %#v", report)
	}
	if checkByID(t, report, doctorCheckKeyring).Status != doctorStatusWarn {
		t.Fatalf("expected keyring warn: %#v", checkByID(t, report, doctorCheckKeyring))
	}
}

func TestAuthDoctor_PartiallyConfiguredContinues(t *testing.T) {
	setupDoctorEnv(t)
	// Empty accounts fail, while unused OAuth credentials are not required.
	withDoctorSeams(t, newMemSecretsStore(), nil)

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			runErr = (&AuthDoctorCmd{}).Run(ctx)
		})
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected unhealthy")
	}
	report := parseDoctorJSON(t, out)
	for _, id := range []string{doctorCheckConfig, doctorCheckKeyring, doctorCheckCredentials, doctorCheckAccounts} {
		if checkByID(t, report, id).ID == "" {
			t.Fatalf("missing check %s", id)
		}
	}
	if checkByID(t, report, doctorCheckCredentials).Status != doctorStatusSkip {
		t.Fatalf("credentials should skip without OAuth tokens")
	}
	if checkByID(t, report, doctorCheckAccounts).Status != doctorStatusFail {
		t.Fatalf("accounts should fail")
	}
}

func TestAuthDoctor_ExecuteWiring(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt"})
	withDoctorSeams(t, store, nil)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "doctor"}); err != nil {
				t.Fatalf("Execute auth doctor: %v", err)
			}
		})
	})
	report := parseDoctorJSON(t, out)
	if !report.Healthy {
		t.Fatalf("expected healthy via Execute: %#v", report)
	}
}

type doctorTokenInspector struct {
	inspections []secrets.TokenInspection
	err         error
}

func (s doctorTokenInspector) InspectTokens() ([]secrets.TokenInspection, error) {
	return s.inspections, s.err
}

func TestAuthDoctor_CorruptCredentialSetFailsWithoutLeakingValues(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)

	path, err := config.ClientCredentialsPathFor("work")
	if err != nil {
		t.Fatalf("credentials path: %v", err)
	}
	const corruptValue = `{"client_id":"not-for-output","client_secret":"not-for-output"`
	if err := os.WriteFile(path, []byte(corruptValue), 0o600); err != nil {
		t.Fatalf("write corrupt credentials: %v", err)
	}
	withDoctorSeams(t, doctorTokenInspector{inspections: []secrets.TokenInspection{{
		Client: "work",
		Email:  "work@example.com",
		Token:  secrets.Token{RefreshToken: "rt-work"},
	}}}, nil)

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() { runErr = (&AuthDoctorCmd{}).Run(ctx) })
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected corrupt credentials to fail")
	}
	report := parseDoctorJSON(t, out)
	credentials := checkByID(t, report, doctorCheckCredentials)
	if credentials.Status != doctorStatusFail || !strings.Contains(credentials.Detail, "unusable_clients=work") {
		t.Fatalf("expected per-client credential failure, got %#v", credentials)
	}
	if strings.Contains(out, "not-for-output") {
		t.Fatalf("credential material leaked: %q", out)
	}
}

func TestAuthDoctor_ServiceAccountsStillListedWhenKeyringFails(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)

	email := "sa-user@example.com"
	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com","private_key":"x"}`), 0o600); err != nil {
		t.Fatalf("write service account key: %v", err)
	}
	if _, _, err := storeServiceAccountKey(email, keyPath); err != nil {
		t.Fatalf("store service account: %v", err)
	}
	withDoctorSeams(t, nil, nil)

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() { runErr = (&AuthDoctorCmd{}).Run(ctx) })
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected inaccessible keyring to fail")
	}
	report := parseDoctorJSON(t, out)
	accounts := checkByID(t, report, doctorCheckAccounts)
	if accounts.Status != doctorStatusSkip || !strings.Contains(accounts.Summary, "1 service-account") {
		t.Fatalf("service account discovery should survive keyring failure: %#v", accounts)
	}
	if token := checkByID(t, report, "token:"+email); token.Status != doctorStatusSkip || !strings.Contains(token.Summary, "not applicable") {
		t.Fatalf("service account token check: %#v", token)
	}
}

func TestAuthDoctor_CorruptTokenDoesNotSuppressOtherTokensOrLeakDecodeData(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	const sensitiveMarker = "refresh-token-should-not-appear"
	malformedStoredJSON := `{"refresh_token":"` + sensitiveMarker + `"`
	var ignored map[string]any
	decodeErr := json.Unmarshal([]byte(malformedStoredJSON), &ignored)
	if decodeErr == nil {
		t.Fatal("expected malformed stored JSON to fail decoding")
	}
	inspector := doctorTokenInspector{inspections: []secrets.TokenInspection{
		{Client: config.DefaultClientName, Email: "good@example.com", Token: secrets.Token{RefreshToken: "rt-good"}},
		{
			Client: "work",
			Email:  "bad@example.com",
			Token:  secrets.Token{RefreshToken: sensitiveMarker},
			Err:    fmt.Errorf("decode stored token %s: %w", malformedStoredJSON, decodeErr),
		},
	}}
	withDoctorSeams(t, inspector, nil)

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() { runErr = (&AuthDoctorCmd{}).Run(ctx) })
	})
	if ExitCode(runErr) == 0 {
		t.Fatalf("expected corrupt token to fail")
	}
	if strings.Contains(out, sensitiveMarker) || strings.Contains(out, malformedStoredJSON) {
		t.Fatalf("corrupt token data leaked: %q", out)
	}

	report := parseDoctorJSON(t, out)
	if checkByID(t, report, "token:good@example.com").Status != doctorStatusOK {
		t.Fatalf("good token missing from report: %#v", report.Checks)
	}
	bad := checkByID(t, report, "token:bad@example.com:work")
	if bad.Status != doctorStatusFail || !strings.Contains(bad.Summary, "could not be read") {
		t.Fatalf("corrupt token should have its own failure: %#v", bad)
	}
	if !strings.Contains(bad.Detail, "token_data=unreadable") || strings.Contains(bad.Detail, "decode stored token") {
		t.Fatalf("corrupt token should use a safe classified error: %#v", bad)
	}
}

func TestAuthDoctor_ServiceAccountOnlyWithoutKeyringStorageIsHealthy(t *testing.T) {
	setupDoctorEnv(t)

	email := "sa-only@example.com"
	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com","private_key":"x"}`), 0o600); err != nil {
		t.Fatalf("write service account key: %v", err)
	}
	if _, _, err := storeServiceAccountKey(email, keyPath); err != nil {
		t.Fatalf("store service account: %v", err)
	}

	ctx := doctorUIContext(t, outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() { runErr = (&AuthDoctorCmd{}).Run(ctx) })
	})
	if runErr != nil {
		t.Fatalf("service-account-only setup should be healthy: %v\nout=%s", runErr, out)
	}

	report := parseDoctorJSON(t, out)
	if keyring := checkByID(t, report, doctorCheckKeyring); keyring.Status != doctorStatusSkip {
		t.Fatalf("absent OAuth keyring should skip: %#v", keyring)
	}
	if accounts := checkByID(t, report, doctorCheckAccounts); accounts.Status != doctorStatusOK {
		t.Fatalf("service-account identity should be healthy: %#v", accounts)
	}
	if token := checkByID(t, report, "token:"+email); token.Status != doctorStatusSkip {
		t.Fatalf("service-account token check should be not applicable: %#v", token)
	}
}

func TestAuthDoctor_TokenListFailureStillReportsServiceAccounts(t *testing.T) {
	setupDoctorEnv(t)

	email := "sa-user@example.com"
	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com","private_key":"x"}`), 0o600); err != nil {
		t.Fatalf("write service account key: %v", err)
	}
	if _, _, err := storeServiceAccountKey(email, keyPath); err != nil {
		t.Fatalf("store service account: %v", err)
	}
	withDoctorSeams(t, doctorTokenInspector{err: errors.New("list failed")}, nil)

	report := runAuthDoctor(doctorUIContext(t, outfmt.Mode{JSON: true}), time.Second)
	accounts := checkByID(t, report, doctorCheckAccounts)
	if accounts.Status != doctorStatusFail || !strings.Contains(accounts.Summary, "1 service-account") {
		t.Fatalf("account failure should retain service-account count: %#v", accounts)
	}
	if token := checkByID(t, report, "token:"+email); token.Status != doctorStatusSkip {
		t.Fatalf("service-account token check should survive OAuth list failure: %#v", token)
	}
}

func TestAuthDoctor_TokenCheckIDsIncludeNonDefaultClient(t *testing.T) {
	setupDoctorEnv(t)
	writeDoctorCredentials(t)
	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "work-id", ClientSecret: "work-secret"}); err != nil {
		t.Fatalf("write work credentials: %v", err)
	}
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "same@example.com", secrets.Token{RefreshToken: "rt-default"})
	_ = store.SetToken("work", "same@example.com", secrets.Token{RefreshToken: "rt-work"})
	withDoctorSeams(t, store, nil)

	report := runAuthDoctor(doctorUIContext(t, outfmt.Mode{JSON: true}), time.Second)
	if checkByID(t, report, "token:same@example.com").Status != doctorStatusOK {
		t.Fatalf("default-client token check missing: %#v", report.Checks)
	}
	if checkByID(t, report, "token:same@example.com:work").Status != doctorStatusOK {
		t.Fatalf("work-client token check missing: %#v", report.Checks)
	}
}
