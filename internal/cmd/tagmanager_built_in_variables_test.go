package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"google.golang.org/api/tagmanager/v2"
)

func TestExecute_TagManagerBuiltInVariablesLifecycle(t *testing.T) {
	collection := "/tagmanager/v2/accounts/111/containers/c1/workspaces/7/built_in_variables"
	requests := 0
	setupTagManagerResourceTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == collection:
			if got := r.URL.Query()["type"]; len(got) != 2 || got[0] != "pageUrl" || got[1] != "clickId" {
				t.Fatalf("create type query = %#v", got)
			}
			_, _ = w.Write([]byte(`{"builtInVariable":[{"path":"accounts/111/containers/c1/workspaces/7/built_in_variables","name":"Page URL","type":"pageUrl"},{"path":"accounts/111/containers/c1/workspaces/7/built_in_variables","name":"Click ID","type":"clickId"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == collection:
			_, _ = w.Write([]byte(`{"builtInVariable":[{"path":"accounts/111/containers/c1/workspaces/7/built_in_variables","name":"Page URL","type":"pageUrl"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == collection+":revert":
			if got := r.URL.Query().Get("type"); got != "pageUrl" {
				t.Fatalf("revert type = %q", got)
			}
			_, _ = w.Write([]byte(`{"enabled":true}`))
		case r.Method == http.MethodDelete && r.URL.Path == collection:
			if got := r.URL.Query()["type"]; len(got) != 2 || got[0] != "pageUrl" || got[1] != "clickId" {
				t.Fatalf("delete type query = %#v", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))

	common := []string{"--account-id", "111", "--container-id", "c1", "--workspace-id", "7"}
	commands := [][]string{
		append(append([]string{"--json", "--account", "admin@example.com", "gtm", "built-in-variables", "create"}, common...), "--type", "pageUrl", "--type", "clickId"),
		append([]string{"--json", "--account", "admin@example.com", "gtm", "built-in-variables", "list"}, common...),
		append(append([]string{"--json", "--account", "admin@example.com", "gtm", "built-in-variables", "revert"}, common...), "--type", "pageUrl"),
		append(append([]string{"--json", "--force", "--account", "admin@example.com", "gtm", "built-in-variables", "delete"}, common...), "--type", "pageUrl", "--type", "clickId"),
	}
	for index, args := range commands {
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := Execute(args); err != nil {
					t.Fatalf("Execute(%v): %v", args, err)
				}
			})
		})
		switch index {
		case 0:
			var result struct {
				BuiltInVariables []map[string]any `json:"builtInVariables"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.BuiltInVariables) != 2 {
				t.Fatalf("unexpected create output %q: %v", out, err)
			}
		case 1:
			var result []map[string]any
			if err := json.Unmarshal([]byte(out), &result); err != nil || len(result) != 1 || result[0]["type"] != "pageUrl" {
				t.Fatalf("unexpected list output %q: %v", out, err)
			}
		case 2:
			var result struct {
				Type    string `json:"type"`
				Enabled bool   `json:"enabled"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil || result.Type != "pageUrl" || !result.Enabled {
				t.Fatalf("unexpected revert output %q: %v", out, err)
			}
		case 3:
			var result struct {
				Deleted bool     `json:"deleted"`
				Types   []string `json:"types"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil || !result.Deleted || len(result.Types) != 2 {
				t.Fatalf("unexpected delete output %q: %v", out, err)
			}
		}
	}
	if requests != 4 {
		t.Fatalf("requests = %d, want 4", requests)
	}
}

func TestExecute_TagManagerBuiltInVariablesRejectsBlankTypeBeforeService(t *testing.T) {
	original := newTagManagerService
	t.Cleanup(func() { newTagManagerService = original })
	called := false
	newTagManagerService = func(context.Context, string) (*tagmanager.Service, error) {
		called = true
		return nil, errors.New("unexpected service construction")
	}
	err := Execute([]string{
		"--account", "admin@example.com", "gtm", "built-in-variables", "create",
		"--account-id", "111", "--container-id", "c1", "--workspace-id", "7", "--type", " ",
	})
	if err == nil {
		t.Fatal("expected blank type error")
	}
	if called {
		t.Fatal("service constructed for blank type")
	}
	called = false
	err = Execute([]string{
		"--account", "admin@example.com", "gtm", "built-in-variables", "revert",
		"--account-id", "111", "--container-id", "c1", "--workspace-id", "7",
		"--type", "pageUrl", "--type", "clickId",
	})
	if err == nil {
		t.Fatal("expected exactly-one-type error")
	}
	if called {
		t.Fatal("service constructed for multiple revert types")
	}
}

func TestExecute_TagManagerBuiltInVariablesDeleteRequiresConfirmation(t *testing.T) {
	original := newTagManagerService
	t.Cleanup(func() { newTagManagerService = original })
	called := false
	newTagManagerService = func(context.Context, string) (*tagmanager.Service, error) {
		called = true
		return nil, errors.New("unexpected service construction")
	}
	err := Execute([]string{
		"--no-input", "--account", "admin@example.com", "gtm", "built-in-variables", "delete",
		"--account-id", "111", "--container-id", "c1", "--workspace-id", "7", "--type", "pageUrl",
	})
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	if called {
		t.Fatal("service constructed before confirmation")
	}
}
