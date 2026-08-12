package config

import (
	"path/filepath"
	"testing"
)

func TestParseGoogleOAuthClientJSON_ProjectID(t *testing.T) {
	got, err := ParseGoogleOAuthClientJSON([]byte(`{"installed":{"client_id":"id","client_secret":"sec","project_id":"my-proj"}}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got.ProjectID != "my-proj" {
		t.Fatalf("project_id=%q", got.ProjectID)
	}
}

func TestClientCredentials_ProjectIDRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	in := ClientCredentials{ClientID: "id", ClientSecret: "secret", ProjectID: "proj-1"}
	if err := WriteClientCredentials(in); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, err := ReadClientCredentials()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if out.ProjectID != "proj-1" {
		t.Fatalf("project_id=%q", out.ProjectID)
	}
}

func TestSameClientCredentials(t *testing.T) {
	a := ClientCredentials{ClientID: "a", ClientSecret: "b", ProjectID: "p"}
	b := a

	if !SameClientCredentials(a, b) {
		t.Fatalf("expected same")
	}

	b.ProjectID = "other"

	if SameClientCredentials(a, b) {
		t.Fatalf("expected different")
	}
}
