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

func setupTagManagerUserPermissionsTest(t *testing.T, handler http.Handler) {
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

func TestExecute_TagManagerUserPermissionsCreate(t *testing.T) {
	setupTagManagerUserPermissionsTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tagmanager/v2/accounts/111/user_permissions" {
			http.NotFound(w, r)
			return
		}
		var body tagmanager.UserPermission
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.EmailAddress != "person@example.com" || body.AccountAccess == nil || body.AccountAccess.Permission != "user" {
			t.Fatalf("unexpected permission: %#v", body)
		}
		if len(body.ContainerAccess) != 2 || body.ContainerAccess[0].ContainerId != "c1" || body.ContainerAccess[0].Permission != "publish" {
			t.Fatalf("unexpected container access: %#v", body.ContainerAccess)
		}
		body.Path = "accounts/111/user_permissions/222"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&body)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "gtm", "user-permissions", "create",
				"--account-id", "111", "--email", "person@example.com", "--account-access-type", "user",
				"--container-access", `{"containerId":"c1","permission":"publish"}`,
				"--container-access", `{"containerId":"c2","permission":"read"}`,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "accounts/111/user_permissions/222") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecute_TagManagerUserPermissionsDeleteRequiresForce(t *testing.T) {
	err := Execute([]string{
		"--json", "--account", "admin@example.com", "--no-input",
		"gtm", "user-permissions", "delete", "accounts/111/user_permissions/222",
	})
	if err == nil || !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestExecute_TagManagerUserPermissionsListGetUpdateDelete(t *testing.T) {
	var updateBody tagmanager.UserPermission
	var deleteCalls int
	setupTagManagerUserPermissionsTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/tagmanager/v2/accounts/111/user_permissions":
			if r.URL.Query().Get("pageToken") != "next" {
				t.Fatalf("pageToken = %q", r.URL.Query().Get("pageToken"))
			}
			_, _ = w.Write([]byte(`{"userPermission":[{"path":"accounts/111/user_permissions/222","emailAddress":"one@example.com","accountAccess":{"permission":"user"}},{"path":"accounts/111/user_permissions/333","emailAddress":"two@example.com","accountAccess":{"permission":"admin"}}],"nextPageToken":"later"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/tagmanager/v2/accounts/111/user_permissions/222":
			_, _ = w.Write([]byte(`{"path":"accounts/111/user_permissions/222","emailAddress":"one@example.com","accountAccess":{"permission":"user"},"containerAccess":[{"containerId":"c1","permission":"approve"}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/tagmanager/v2/accounts/111/user_permissions/222":
			if err := json.NewDecoder(r.Body).Decode(&updateBody); err != nil {
				t.Fatalf("decode update: %v", err)
			}
			_, _ = w.Write([]byte(`{"path":"accounts/111/user_permissions/222","emailAddress":"one@example.com","accountAccess":{"permission":"admin"},"containerAccess":[{"containerId":"c1","permission":"approve"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/tagmanager/v2/accounts/111/user_permissions/222":
			deleteCalls++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "gtm", "user-permissions", "list",
				"--account-id", "111", "--page", "next",
			}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})
	var listResult struct {
		UserPermissions []json.RawMessage `json:"userPermissions"`
		NextPageToken   string            `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(listOut), &listResult); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResult.UserPermissions) != 2 || listResult.NextPageToken != "later" {
		t.Fatalf("unexpected list output: %q", listOut)
	}

	getOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "admin@example.com", "gtm", "user-permissions", "get",
				"accounts/111/user_permissions/222",
			}); err != nil {
				t.Fatalf("get: %v", err)
			}
		})
	})
	if getOut != "PATH\tEMAIL\tACCOUNT_ACCESS\naccounts/111/user_permissions/222\tone@example.com\tuser\n" {
		t.Fatalf("unexpected get output: %q", getOut)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "gtm", "user-permissions", "update",
				"accounts/111/user_permissions/222", "--account-access-type", "admin",
			}); err != nil {
				t.Fatalf("update: %v", err)
			}
		})
	})
	if updateBody.AccountAccess == nil || updateBody.AccountAccess.Permission != "admin" || len(updateBody.ContainerAccess) != 1 || updateBody.ContainerAccess[0].Permission != "approve" {
		t.Fatalf("unexpected update body: %#v", updateBody)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "--force", "gtm", "user-permissions", "delete",
				"accounts/111/user_permissions/222",
			}); err != nil {
				t.Fatalf("delete: %v", err)
			}
		})
	})
	if deleteCalls != 1 {
		t.Fatalf("delete calls = %d", deleteCalls)
	}
}

func TestExecute_TagManagerUserPermissionsValidatesAccountAccess(t *testing.T) {
	err := Execute([]string{
		"--account", "admin@example.com", "gtm", "user-permissions", "create",
		"--account-id", "111", "--email", "person@example.com", "--account-access-type", "owner",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid --account-access-type") {
		t.Fatalf("expected account access validation error, got %v", err)
	}
}
