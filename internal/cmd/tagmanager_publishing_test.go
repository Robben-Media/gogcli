package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tagmanager/v2"
)

func setupTagManagerPublishingTest(t *testing.T, handler http.Handler) {
	t.Helper()

	original := newTagManagerService
	t.Cleanup(func() { newTagManagerService = original })

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	service, err := tagmanager.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	newTagManagerService = func(context.Context, string) (*tagmanager.Service, error) { return service, nil }
}

func TestExecute_TagManagerWorkspacesCreateVersion(t *testing.T) {
	setupTagManagerPublishingTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tagmanager/v2/accounts/111/containers/c1/workspaces/7:create_version" {
			http.NotFound(w, r)

			return
		}

		var body tagmanager.CreateContainerVersionRequestVersionOptions
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Name != "Release 42" || body.Notes != "Publish checkout tracking" {
			t.Fatalf("unexpected request body: %#v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"containerVersion": {
				"accountId": "111",
				"containerId": "c1",
				"containerVersionId": "42",
				"name": "Release 42",
				"path": "accounts/111/containers/c1/versions/42",
				"fingerprint": "fp-42"
			},
			"newWorkspacePath": "accounts/111/containers/c1/workspaces/8"
		}`))
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "gtm", "workspaces", "create-version",
				"accounts/111/containers/c1/workspaces/7",
				"--name", "Release 42", "--notes", "Publish checkout tracking",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result struct {
		Path             string                       `json:"path"`
		ContainerVersion *tagmanager.ContainerVersion `json:"containerVersion"`
		NewWorkspacePath string                       `json:"newWorkspacePath"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode output: %v\nout=%q", err, out)
	}
	if result.Path != "accounts/111/containers/c1/versions/42" {
		t.Fatalf("path = %q", result.Path)
	}
	if result.ContainerVersion == nil || result.ContainerVersion.ContainerVersionId != "42" {
		t.Fatalf("unexpected container version: %#v", result.ContainerVersion)
	}
	if result.NewWorkspacePath != "accounts/111/containers/c1/workspaces/8" {
		t.Fatalf("newWorkspacePath = %q", result.NewWorkspacePath)
	}
}

func TestExecute_TagManagerVersionsPublish(t *testing.T) {
	setupTagManagerPublishingTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tagmanager/v2/accounts/111/containers/c1/versions/42:publish" {
			http.NotFound(w, r)

			return
		}
		if got := r.URL.Query().Get("fingerprint"); got != "fp-42" {
			t.Fatalf("fingerprint = %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"containerVersion": {
				"accountId": "111",
				"containerId": "c1",
				"containerVersionId": "42",
				"name": "Release 42",
				"path": "accounts/111/containers/c1/versions/42",
				"fingerprint": "fp-published"
			}
		}`))
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "gtm", "versions", "publish",
				"accounts/111/containers/c1/versions/42", "--fingerprint", "fp-42",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result struct {
		Path             string                       `json:"path"`
		ContainerVersion *tagmanager.ContainerVersion `json:"containerVersion"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode output: %v\nout=%q", err, out)
	}
	if result.Path != "accounts/111/containers/c1/versions/42" {
		t.Fatalf("path = %q", result.Path)
	}
	if result.ContainerVersion == nil || result.ContainerVersion.Fingerprint != "fp-published" {
		t.Fatalf("unexpected container version: %#v", result.ContainerVersion)
	}
}

func TestExecute_TagManagerVersionsPublishPlain(t *testing.T) {
	setupTagManagerPublishingTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tagmanager/v2/accounts/111/containers/c1/versions/42:publish" {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"containerVersion": {
				"containerVersionId": "42",
				"name": "Release 42",
				"path": "accounts/111/containers/c1/versions/42",
				"fingerprint": "fp-published"
			}
		}`))
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "admin@example.com", "gtm", "versions", "publish",
				"accounts/111/containers/c1/versions/42",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	want := "PATH\tVERSION_ID\tNAME\tFINGERPRINT\tCOMPILER_ERROR\n" +
		"accounts/111/containers/c1/versions/42\t42\tRelease 42\tfp-published\tfalse\n"
	if out != want {
		t.Fatalf("plain output:\n%s\nwant:\n%s", out, want)
	}
}

func TestExecute_TagManagerWorkspacesCreateVersionPlain(t *testing.T) {
	setupTagManagerPublishingTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tagmanager/v2/accounts/111/containers/c1/workspaces/7:create_version" {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"containerVersion": {
				"containerVersionId": "42",
				"name": "Release 42",
				"path": "accounts/111/containers/c1/versions/42",
				"fingerprint": "fp-42"
			},
			"newWorkspacePath": "accounts/111/containers/c1/workspaces/8"
		}`))
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "admin@example.com", "gtm", "workspaces", "create-version",
				"accounts/111/containers/c1/workspaces/7",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	want := "PATH\tVERSION_ID\tNAME\tFINGERPRINT\tNEW_WORKSPACE_PATH\tCOMPILER_ERROR\n" +
		"accounts/111/containers/c1/versions/42\t42\tRelease 42\tfp-42\taccounts/111/containers/c1/workspaces/8\tfalse\n"
	if out != want {
		t.Fatalf("plain output:\n%s\nwant:\n%s", out, want)
	}
}

func TestExecute_TagManagerPublishingValidatesPathsBeforeAPIRequest(t *testing.T) {
	original := newTagManagerService
	t.Cleanup(func() { newTagManagerService = original })

	apiCalled := false
	unexpectedCallErr := errors.New("unexpected API service creation")
	newTagManagerService = func(context.Context, string) (*tagmanager.Service, error) {
		apiCalled = true

		return nil, unexpectedCallErr
	}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "workspace path uses workspace resource",
			args: []string{
				"--account", "admin@example.com", "gtm", "workspaces", "create-version",
				"accounts/111/containers/c1/versions/42",
			},
			wantErr: "path must be accounts/ACCOUNT_ID/containers/CONTAINER_ID/workspaces/WORKSPACE_ID",
		},
		{
			name: "version path uses version resource",
			args: []string{
				"--account", "admin@example.com", "gtm", "versions", "publish",
				"accounts/111/containers/c1/workspaces/7",
			},
			wantErr: "path must be accounts/ACCOUNT_ID/containers/CONTAINER_ID/versions/VERSION_ID",
		},
		{
			name:    "missing version path",
			args:    []string{"--account", "admin@example.com", "gtm", "versions", "publish"},
			wantErr: "path must be accounts/ACCOUNT_ID/containers/CONTAINER_ID/versions/VERSION_ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Execute(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q, got %v", tt.wantErr, err)
			}
		})
	}

	if apiCalled {
		t.Fatal("API service was created for an invalid path")
	}
}
