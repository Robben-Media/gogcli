package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1alpha"
	"google.golang.org/api/option"
)

func setupAnalyticsAdminAlphaTest(t *testing.T, handler http.Handler) {
	t.Helper()
	original := newAnalyticsAdminAlphaService
	t.Cleanup(func() { newAnalyticsAdminAlphaService = original })

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	service, err := analyticsadmin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAnalyticsAdminAlphaService = func(context.Context, string) (*analyticsadmin.Service, error) {
		return service, nil
	}
}

func TestExecute_AAAccessBindingsCreate_NormalizesRoles(t *testing.T) {
	setupAnalyticsAdminAlphaTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1alpha/properties/123/accessBindings" {
			http.NotFound(w, r)
			return
		}
		var body analyticsadmin.GoogleAnalyticsAdminV1alphaAccessBinding
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.User != "person@example.com" {
			t.Fatalf("user = %q", body.User)
		}
		wantRoles := []string{"predefinedRoles/viewer", "predefinedRoles/no-cost-data"}
		if strings.Join(body.Roles, ",") != strings.Join(wantRoles, ",") {
			t.Fatalf("roles = %#v, want %#v", body.Roles, wantRoles)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "properties/123/accessBindings/456",
			"user":  body.User,
			"roles": body.Roles,
		})
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com",
				"analytics", "admin", "access-bindings", "create",
				"properties/123", "--email", "person@example.com",
				"--roles", "viewer,no-cost-data",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var parsed struct {
		AccessBinding struct {
			Name string `json:"name"`
		} `json:"accessBinding"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if parsed.AccessBinding.Name != "properties/123/accessBindings/456" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecute_AAAccessBindingsList_PropertyPagination(t *testing.T) {
	setupAnalyticsAdminAlphaTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1alpha/properties/123/accessBindings" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("pageSize") != "25" || r.URL.Query().Get("pageToken") != "next" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessBindings":[{"name":"properties/123/accessBindings/456","user":"person@example.com","roles":["predefinedRoles/viewer"]}],"nextPageToken":"later"}`))
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com",
				"analytics", "admin", "access-bindings", "list", "properties/123",
				"--max", "25", "--page", "next",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var parsed struct {
		AccessBindings []json.RawMessage `json:"accessBindings"`
		NextPageToken  string            `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(parsed.AccessBindings) != 1 || parsed.NextPageToken != "later" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecute_AAAccessBindingsPatch_Account(t *testing.T) {
	setupAnalyticsAdminAlphaTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1alpha/accounts/123/accessBindings/456" {
			http.NotFound(w, r)
			return
		}
		var body analyticsadmin.GoogleAnalyticsAdminV1alphaAccessBinding
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if strings.Join(body.Roles, ",") != "predefinedRoles/admin,predefinedRoles/no-revenue-data" {
			t.Fatalf("unexpected roles: %#v", body.Roles)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"accounts/123/accessBindings/456","user":"person@example.com","roles":["predefinedRoles/admin","predefinedRoles/no-revenue-data"]}`))
	}))

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "admin@example.com",
				"analytics", "admin", "access-bindings", "patch",
				"accounts/123/accessBindings/456", "--roles", "predefinedRoles/admin,no-revenue-data",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_AAAccessBindingsDelete_ConfirmationAndForce(t *testing.T) {
	var deleteCalls int
	setupAnalyticsAdminAlphaTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1alpha/properties/123/accessBindings/456" {
			http.NotFound(w, r)
			return
		}
		deleteCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))

	err := Execute([]string{
		"--account", "admin@example.com", "--no-input",
		"analytics", "admin", "access-bindings", "delete", "properties/123/accessBindings/456",
	})
	if err == nil || !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if deleteCalls != 0 {
		t.Fatalf("delete called before confirmation")
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "--force",
				"analytics", "admin", "access-bindings", "delete", "properties/123/accessBindings/456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if deleteCalls != 1 {
		t.Fatalf("delete calls = %d", deleteCalls)
	}
}

func TestExecute_AAAccessBindingsRejectsUnknownRole(t *testing.T) {
	err := Execute([]string{
		"--account", "admin@example.com",
		"analytics", "admin", "access-bindings", "create", "accounts/123",
		"--email", "person@example.com", "--roles", "owner",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported role") {
		t.Fatalf("expected role validation error, got %v", err)
	}
}
