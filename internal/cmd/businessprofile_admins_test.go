package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	mybusinessaccountmanagement "google.golang.org/api/mybusinessaccountmanagement/v1"
	"google.golang.org/api/option"
)

func TestExecute_BusinessProfileAccountAdminsList_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admins") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accountAdmins": []map[string]any{
				{"name": "accounts/123/admins/1", "admin": "admin@test.com", "role": "MANAGER"},
				{"name": "accounts/123/admins/2", "admin": "owner@test.com", "role": "OWNER"},
			},
		})
	}))
	defer srv.Close()

	svc, err := mybusinessaccountmanagement.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		return svc, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "account-admins", "list", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	admins, ok := result["admins"].([]any)
	if !ok {
		t.Fatalf("expected admins array")
	}
	if len(admins) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(admins))
	}
	first, ok := admins[0].(map[string]any)
	if !ok {
		t.Fatalf("expected admin object")
	}
	if first["name"] != "accounts/123/admins/1" {
		t.Fatalf("expected name 'accounts/123/admins/1', got %v", first["name"])
	}
	if first["admin"] != "admin@test.com" {
		t.Fatalf("expected admin 'admin@test.com', got %v", first["admin"])
	}
	if first["role"] != "MANAGER" {
		t.Fatalf("expected role 'MANAGER', got %v", first["role"])
	}
	second, ok := admins[1].(map[string]any)
	if !ok {
		t.Fatalf("expected admin object")
	}
	if second["role"] != "OWNER" {
		t.Fatalf("expected role 'OWNER', got %v", second["role"])
	}
}

func TestExecute_BusinessProfileAccountAdminsList_Text(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accountAdmins": []map[string]any{
				{"name": "accounts/123/admins/1", "admin": "admin@test.com", "role": "MANAGER"},
				{"name": "accounts/123/admins/2", "admin": "owner@test.com", "role": "OWNER"},
			},
		})
	}))
	defer srv.Close()

	svc, err := mybusinessaccountmanagement.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		return svc, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"business-profile", "account-admins", "list", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "accounts/123/admins/1") {
		t.Fatalf("expected output to contain 'accounts/123/admins/1', got %q", out)
	}
	if !strings.Contains(out, "admin@test.com") {
		t.Fatalf("expected output to contain 'admin@test.com', got %q", out)
	}
	if !strings.Contains(out, "MANAGER") {
		t.Fatalf("expected output to contain 'MANAGER', got %q", out)
	}
	if !strings.Contains(out, "OWNER") {
		t.Fatalf("expected output to contain 'OWNER', got %q", out)
	}
}

func TestExecute_BusinessProfileAccountAdminsCreate_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/admins") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "accounts/123/admins/new",
			"admin": "newadmin@test.com",
			"role":  "MANAGER",
		})
	}))
	defer srv.Close()

	svc, err := mybusinessaccountmanagement.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		return svc, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "account-admins", "create", "123",
				"--admin", "newadmin@test.com",
				"--role", "MANAGER",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if gotBody["admin"] != "newadmin@test.com" {
		t.Fatalf("expected admin 'newadmin@test.com', got %v", gotBody["admin"])
	}
	if gotBody["role"] != "MANAGER" {
		t.Fatalf("expected role 'MANAGER', got %v", gotBody["role"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	admin, ok := result["admin"].(map[string]any)
	if !ok {
		t.Fatalf("expected admin object")
	}
	if admin["name"] != "accounts/123/admins/new" {
		t.Fatalf("expected 'accounts/123/admins/new', got %v", admin["name"])
	}
	if admin["admin"] != "newadmin@test.com" {
		t.Fatalf("expected 'newadmin@test.com', got %v", admin["admin"])
	}
}

func TestExecute_BusinessProfileAccountAdminsCreate_Text(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "accounts/123/admins/new",
			"admin": "newadmin@test.com",
			"role":  "MANAGER",
		})
	}))
	defer srv.Close()

	svc, err := mybusinessaccountmanagement.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		return svc, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"business-profile", "account-admins", "create", "123",
				"--admin", "newadmin@test.com",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "accounts/123/admins/new") {
		t.Fatalf("expected output to contain 'accounts/123/admins/new', got %q", out)
	}
	if !strings.Contains(out, "newadmin@test.com") {
		t.Fatalf("expected output to contain 'newadmin@test.com', got %q", out)
	}
	if !strings.Contains(out, "MANAGER") {
		t.Fatalf("expected output to contain 'MANAGER', got %q", out)
	}
}

func TestExecute_BusinessProfileAccountAdminsDelete_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/admins/") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotMethod = r.Method
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	svc, err := mybusinessaccountmanagement.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		return svc, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com", "--force",
				"business-profile", "account-admins", "delete", "accounts/123/admins/456",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE method, got %q", gotMethod)
	}
}

func TestExecute_BusinessProfileAccountAdminsPatch_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/admins/") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotUpdateMask = r.URL.Query().Get("updateMask")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "accounts/123/admins/1",
			"admin": "admin@test.com",
			"role":  "OWNER",
		})
	}))
	defer srv.Close()

	svc, err := mybusinessaccountmanagement.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		return svc, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "account-admins", "patch", "accounts/123/admins/1",
				"--role", "OWNER",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(gotUpdateMask, "role") {
		t.Fatalf("expected updateMask to contain 'role', got %q", gotUpdateMask)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	admin, ok := result["admin"].(map[string]any)
	if !ok {
		t.Fatalf("expected admin object")
	}
	if admin["role"] != "OWNER" {
		t.Fatalf("expected role 'OWNER', got %v", admin["role"])
	}
	if admin["name"] != "accounts/123/admins/1" {
		t.Fatalf("expected name 'accounts/123/admins/1', got %v", admin["name"])
	}
}

func TestExecute_BusinessProfileAccountAdminsPatch_NoFields(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		t.Fatalf("unexpected service call — no fields provided should fail before API call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "a@b.com", "business-profile", "account-admins", "patch", "accounts/123/admins/1"})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}
