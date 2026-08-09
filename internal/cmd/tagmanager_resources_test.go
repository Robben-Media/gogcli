package cmd

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/tagmanager/v2"
)

func TestExecute_TagManagerMutationsValidateBeforeService(t *testing.T) {
	original := newTagManagerService
	t.Cleanup(func() { newTagManagerService = original })
	called := false
	newTagManagerService = func(context.Context, string) (*tagmanager.Service, error) {
		called = true
		return nil, errors.New("unexpected service construction")
	}

	tests := [][]string{
		{"--account", "admin@example.com", "gtm", "triggers", "get", "accounts/111/containers/c1/workspaces/7/variables/3"},
		{"--account", "admin@example.com", "gtm", "triggers", "update", "accounts/111/containers/c1/workspaces/7/triggers/9"},
		{"--account", "admin@example.com", "gtm", "variables", "update", "accounts/111/containers/c1/workspaces/7/variables/3"},
		{"--account", "admin@example.com", "gtm", "triggers", "create", "--account-id", "111", "--container-id", "c1", "--name", "Bad", "--type", "pageview", "--filter", "null"},
		{"--account", "admin@example.com", "gtm", "variables", "create", "--account-id", "111", "--container-id", "c1", "--name", "Bad", "--type", "c", "--parameter", "null"},
	}
	for _, args := range tests {
		if err := Execute(args); err == nil {
			t.Fatalf("Execute(%v) unexpectedly succeeded", args)
		}
	}
	if called {
		t.Fatal("service constructed before mutation input validation")
	}
}

func TestExecute_TagManagerMutationPlainOutput(t *testing.T) {
	setupTagManagerResourceTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/triggers/9"):
			_, _ = w.Write([]byte(`{"triggerId":"9","name":"Page","type":"pageview"}`))
		case strings.HasSuffix(r.URL.Path, "/variables/3"):
			_, _ = w.Write([]byte(`{"variableId":"3","name":"Constant","type":"c"}`))
		case strings.HasSuffix(r.URL.Path, "/built_in_variables"):
			_, _ = w.Write([]byte(`{"builtInVariable":[{"name":"Page URL","type":"pageUrl"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--plain", "--account", "admin@example.com", "gtm", "triggers", "get", "accounts/111/containers/c1/workspaces/7/triggers/9"}, "TRIGGER_ID\tNAME\tTYPE\n9\tPage\tpageview\n"},
		{[]string{"--plain", "--account", "admin@example.com", "gtm", "variables", "get", "accounts/111/containers/c1/workspaces/7/variables/3"}, "VARIABLE_ID\tNAME\tTYPE\n3\tConstant\tc\n"},
		{[]string{"--plain", "--account", "admin@example.com", "gtm", "built-in-variables", "list", "--account-id", "111", "--container-id", "c1", "--workspace-id", "7"}, "TYPE\tNAME\npageUrl\tPage URL\n"},
	}
	for _, tt := range tests {
		got := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := Execute(tt.args); err != nil {
					t.Fatalf("Execute(%v): %v", tt.args, err)
				}
			})
		})
		if got != tt.want {
			t.Fatalf("Execute(%v) output = %q, want %q", tt.args, got, tt.want)
		}
	}
}
