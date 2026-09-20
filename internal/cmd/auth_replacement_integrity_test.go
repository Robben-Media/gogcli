package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type replacementFailingStore struct{ secrets.Store }

func (replacementFailingStore) DeleteToken(string, string) error {
	return errors.New("injected delete failure")
}

func TestInstallCredentials_WriteFailurePreservesTokens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "old", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	store := newMemSecretsStore()
	if err := store.SetToken("work", "person@example.com", secrets.Token{RefreshToken: "token"}); err != nil {
		t.Fatal(err)
	}
	origWrite := writeClientCredentials
	writeClientCredentials = func(string, config.ClientCredentials) error { return errors.New("injected write failure") }
	t.Cleanup(func() { writeClientCredentials = origWrite })
	called := false
	_, err := InstallClientCredentials(InstallCredentialsOptions{
		Client: "work", Raw: []byte(`{"installed":{"client_id":"new","client_secret":"secret"}}`),
		AfterReplacement: func(string) (int, error) { called = true; return 0, nil },
	})
	if err == nil {
		t.Fatal("replacement unexpectedly succeeded")
	}
	if called {
		t.Fatal("tokens invalidated before credential write")
	}
	if _, tokenErr := store.GetToken("work", "person@example.com"); tokenErr != nil {
		t.Fatalf("token was not preserved: %v", tokenErr)
	}
	cfg, err := config.ReadConfig()
	if err != nil || !config.GetClientSetup(cfg, "work").ReauthorizationRequired {
		t.Fatalf("reauthorization guard missing after failed replacement: %#v err=%v", cfg, err)
	}
}

func TestAuthSetup_ReplacementInvalidationFailureBlocksStaleToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "old", ClientSecret: "secret", ProjectID: "demo", ClientType: config.OAuthClientTypeInstalled}); err != nil {
		t.Fatal(err)
	}
	store := newMemSecretsStore()
	services := parseSetupServices(t, "gmail")
	if err := store.SetToken("work", "person@example.com", secrets.Token{Services: authServiceNames(services), Scopes: accountScopes(services), RefreshToken: "token"}); err != nil {
		t.Fatal(err)
	}
	origOpen, origOpenNoInput := authSetupOpen, authSetupOpenNoInput
	authSetupOpen = func() (secrets.Store, error) { return store, nil }
	authSetupOpenNoInput = authSetupOpen
	t.Cleanup(func() { authSetupOpen, authSetupOpenNoInput = origOpen, origOpenNoInput })
	path := filepath.Join(t.TempDir(), "new.json")
	if err := os.WriteFile(path, []byte(`{"installed":{"client_id":"new","client_secret":"secret","project_id":"demo"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	failingStore := &replacementFailingStore{Store: store}
	origOpen, origOpenNoInput = authSetupOpen, authSetupOpenNoInput
	authSetupOpen = func() (secrets.Store, error) { return failingStore, nil }
	authSetupOpenNoInput = authSetupOpen
	t.Cleanup(func() { authSetupOpen, authSetupOpenNoInput = origOpen, origOpenNoInput })
	rt := &setupRuntime{cmd: &AuthSetupCmd{CredentialsPath: path, AccountEmail: "person@example.com"}, flags: &RootFlags{Force: true, NoInput: true}, u: mustSetupUI(t), client: "work", force: true, services: services, report: SetupReport{ProjectID: "demo"}}
	if stop, err := rt.installCredentials(context.Background(), "demo"); !stop || err != nil {
		t.Fatalf("replacement failure stop=%t err=%v", stop, err)
	}
	if _, err := store.GetToken("work", "person@example.com"); err != nil {
		t.Fatalf("stale token should remain when invalidation fails: %v", err)
	}
	cfg, err := config.ReadConfig()
	if err != nil || !config.GetClientSetup(cfg, "work").ReauthorizationRequired {
		t.Fatalf("reauthorization guard missing: %#v err=%v", cfg, err)
	}
	rt.setupRec = config.GetClientSetup(cfg, "work")
	if stop, err := rt.runAccount(context.Background()); stop || err != nil {
		t.Fatalf("guarded account stage stop=%t err=%v", stop, err)
	}
	if got := rt.report.Stages[len(rt.report.Stages)-1].Status; got != stageStatusManual {
		t.Fatalf("stale token satisfied setup, stage=%s", got)
	}
}

func TestStandaloneReplacementRequiresSetupReauthorization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "old", ClientSecret: "secret", ProjectID: "demo", ClientType: config.OAuthClientTypeInstalled}); err != nil {
		t.Fatal(err)
	}
	services := parseSetupServices(t, "gmail")
	store := newMemSecretsStore()
	if err := store.SetToken("work", "person@example.com", secrets.Token{Services: authServiceNames(services), Scopes: accountScopes(services), RefreshToken: "token"}); err != nil {
		t.Fatal(err)
	}
	origOpen, origOpenNoInput := authSetupOpen, authSetupOpenNoInput
	authSetupOpen = func() (secrets.Store, error) { return store, nil }
	authSetupOpenNoInput = authSetupOpen
	t.Cleanup(func() { authSetupOpen, authSetupOpenNoInput = origOpen, origOpenNoInput })
	if _, err := InstallClientCredentials(InstallCredentialsOptions{Client: "work", Raw: []byte(`{"installed":{"client_id":"new","client_secret":"secret","project_id":"demo"}}`)}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ReadConfig()
	if err != nil || !config.GetClientSetup(cfg, "work").ReauthorizationRequired {
		t.Fatalf("standalone replacement did not require reauthorization: %#v err=%v", cfg, err)
	}
	rt := &setupRuntime{cmd: &AuthSetupCmd{AccountEmail: "person@example.com"}, flags: &RootFlags{NoInput: true}, u: mustSetupUI(t), client: "work", services: services, setupRec: config.GetClientSetup(cfg, "work"), report: SetupReport{ProjectID: "demo"}}
	if stop, err := rt.runAccount(context.Background()); stop || err != nil {
		t.Fatalf("guarded account stage stop=%t err=%v", stop, err)
	}
	if got := rt.report.Stages[0].Status; got != stageStatusManual {
		t.Fatalf("stale standalone token satisfied setup, stage=%s", got)
	}
	if _, err := store.GetToken("work", "person@example.com"); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatal(err)
	}
}
