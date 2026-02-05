package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tagmanager/v2"
)

func newTestTagManagerServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// accounts list
		case r.URL.Path == "/tagmanager/v2/accounts" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"account": []map[string]any{
					{"accountId": "111", "name": "My GTM Account"},
					{"accountId": "222", "name": "Other Account"},
				},
			})

		// containers list
		case strings.HasSuffix(r.URL.Path, "/containers") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"container": []map[string]any{
					{"containerId": "c1", "name": "Web Container", "publicId": "GTM-XXXX"},
				},
			})

		// tags list
		case strings.Contains(r.URL.Path, "/workspaces/") && strings.HasSuffix(r.URL.Path, "/tags") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag": []map[string]any{
					{"tagId": "t1", "name": "GA4 Config", "type": "gaawc"},
					{"tagId": "t2", "name": "Custom HTML", "type": "html"},
				},
			})

		// single tag get (path contains /tags/ followed by an ID)
		case strings.Contains(r.URL.Path, "/tags/") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tagId":           "t1",
				"name":            "GA4 Config",
				"type":            "gaawc",
				"firingTriggerId": []string{"tr1"},
				"parameter": []map[string]any{
					{"key": "trackingId", "value": "G-XXXXX"},
				},
			})

		// triggers list
		case strings.Contains(r.URL.Path, "/workspaces/") && strings.HasSuffix(r.URL.Path, "/triggers") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"trigger": []map[string]any{
					{"triggerId": "tr1", "name": "All Pages", "type": "pageview"},
				},
			})

		// variables list
		case strings.Contains(r.URL.Path, "/workspaces/") && strings.HasSuffix(r.URL.Path, "/variables") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"variable": []map[string]any{
					{"variableId": "v1", "name": "Page URL", "type": "u"},
				},
			})

		// version headers list
		case strings.Contains(r.URL.Path, "/version_headers") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"containerVersionHeader": []map[string]any{
					{
						"containerVersionId": "5",
						"name":               "v5 release",
						"numTags":            "10",
						"numTriggers":        "3",
						"numVariables":       "7",
					},
				},
			})

		default:
			http.NotFound(w, r)
		}
	}))
}

func setupTagManagerTest(t *testing.T) {
	t.Helper()
	origNew := newTagManagerService
	t.Cleanup(func() { newTagManagerService = origNew })

	srv := newTestTagManagerServer(t)
	t.Cleanup(srv.Close)

	svc, err := tagmanager.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTagManagerService = func(context.Context, string) (*tagmanager.Service, error) { return svc, nil }
}

func TestExecute_TagManagerAccounts_JSON(t *testing.T) {
	setupTagManagerTest(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "tag-manager", "accounts"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Accounts []struct {
			AccountID string `json:"accountId"`
			Name      string `json:"name"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(parsed.Accounts))
	}
	if parsed.Accounts[0].AccountID != "111" {
		t.Fatalf("unexpected first account ID: %q", parsed.Accounts[0].AccountID)
	}
	if parsed.Accounts[0].Name != "My GTM Account" {
		t.Fatalf("unexpected first account name: %q", parsed.Accounts[0].Name)
	}
}

func TestExecute_TagManagerContainers_JSON(t *testing.T) {
	setupTagManagerTest(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "tag-manager", "containers", "--account-id", "111"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Containers []struct {
			ContainerID string `json:"containerId"`
			Name        string `json:"name"`
			PublicID    string `json:"publicId"`
		} `json:"containers"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(parsed.Containers))
	}
	if parsed.Containers[0].ContainerID != "c1" {
		t.Fatalf("unexpected container ID: %q", parsed.Containers[0].ContainerID)
	}
	if parsed.Containers[0].PublicID != "GTM-XXXX" {
		t.Fatalf("unexpected public ID: %q", parsed.Containers[0].PublicID)
	}
}

func TestExecute_TagManagerTags_JSON(t *testing.T) {
	setupTagManagerTest(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "tag-manager", "tags", "--account-id", "111", "--container-id", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Tags []struct {
			TagID string `json:"tagId"`
			Name  string `json:"name"`
			Type  string `json:"type"`
		} `json:"tags"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(parsed.Tags))
	}
	if parsed.Tags[0].TagID != "t1" {
		t.Fatalf("unexpected first tag ID: %q", parsed.Tags[0].TagID)
	}
	if parsed.Tags[0].Type != "gaawc" {
		t.Fatalf("unexpected first tag type: %q", parsed.Tags[0].Type)
	}
}

func TestExecute_TagManagerAccounts_NoAccount(t *testing.T) {
	err := Execute([]string{"--json", "tag-manager", "accounts"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestExecute_TagManagerTriggers_JSON(t *testing.T) {
	setupTagManagerTest(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "tag-manager", "triggers", "--account-id", "111", "--container-id", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Triggers []struct {
			TriggerID string `json:"triggerId"`
			Name      string `json:"name"`
			Type      string `json:"type"`
		} `json:"triggers"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(parsed.Triggers))
	}
	if parsed.Triggers[0].TriggerID != "tr1" {
		t.Fatalf("unexpected trigger ID: %q", parsed.Triggers[0].TriggerID)
	}
}

func TestExecute_TagManagerVariables_JSON(t *testing.T) {
	setupTagManagerTest(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "tag-manager", "variables", "--account-id", "111", "--container-id", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Variables []struct {
			VariableID string `json:"variableId"`
			Name       string `json:"name"`
			Type       string `json:"type"`
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Variables) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(parsed.Variables))
	}
	if parsed.Variables[0].VariableID != "v1" {
		t.Fatalf("unexpected variable ID: %q", parsed.Variables[0].VariableID)
	}
}

func TestExecute_TagManagerVersions_JSON(t *testing.T) {
	setupTagManagerTest(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "tag-manager", "versions", "--account-id", "111", "--container-id", "c1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		VersionHeaders []struct {
			ContainerVersionID string `json:"containerVersionId"`
			Name               string `json:"name"`
			NumTags            string `json:"numTags"`
			NumTriggers        string `json:"numTriggers"`
			NumVariables       string `json:"numVariables"`
		} `json:"versionHeaders"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.VersionHeaders) != 1 {
		t.Fatalf("expected 1 version header, got %d", len(parsed.VersionHeaders))
	}
	if parsed.VersionHeaders[0].ContainerVersionID != "5" {
		t.Fatalf("unexpected version ID: %q", parsed.VersionHeaders[0].ContainerVersionID)
	}
	if parsed.VersionHeaders[0].NumTags != "10" {
		t.Fatalf("unexpected numTags: %q", parsed.VersionHeaders[0].NumTags)
	}
}

func TestExecute_TagManagerTag_JSON(t *testing.T) {
	setupTagManagerTest(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "tag-manager", "tag", "accounts/111/containers/c1/workspaces/0/tags/t1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Tag struct {
			TagID string `json:"tagId"`
			Name  string `json:"name"`
			Type  string `json:"type"`
		} `json:"tag"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Tag.TagID != "t1" {
		t.Fatalf("unexpected tag ID: %q", parsed.Tag.TagID)
	}
	if parsed.Tag.Name != "GA4 Config" {
		t.Fatalf("unexpected tag name: %q", parsed.Tag.Name)
	}
}
