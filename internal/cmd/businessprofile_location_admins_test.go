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

func TestExecute_BusinessProfileLocationAdminsList_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admins") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"admins": []map[string]any{
				{"name": "locations/123/admins/1", "admin": "admin@test.com", "role": "MANAGER"},
				{"name": "locations/123/admins/2", "admin": "owner@test.com", "role": "SITE_MANAGER"},
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
				"business-profile", "location-admins", "list", "123",
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
	first := admins[0].(map[string]any)
	if first["name"] != "locations/123/admins/1" {
		t.Fatalf("expected 'locations/123/admins/1', got %v", first["name"])
	}
	if first["admin"] != "admin@test.com" {
		t.Fatalf("expected 'admin@test.com', got %v", first["admin"])
	}
}

func TestExecute_BusinessProfileLocationAdminsList_Text(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"admins": []map[string]any{
				{"name": "locations/123/admins/1", "admin": "admin@test.com", "role": "MANAGER"},
				{"name": "locations/123/admins/2", "admin": "owner@test.com", "role": "SITE_MANAGER"},
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
				"business-profile", "location-admins", "list", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "locations/123/admins/1") {
		t.Fatalf("expected output to contain 'locations/123/admins/1', got %q", out)
	}
	if !strings.Contains(out, "admin@test.com") {
		t.Fatalf("expected output to contain 'admin@test.com', got %q", out)
	}
	if !strings.Contains(out, "owner@test.com") {
		t.Fatalf("expected output to contain 'owner@test.com', got %q", out)
	}
}

func TestExecute_BusinessProfileLocationAdminsCreate_JSON(t *testing.T) {
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
			"name":  "locations/123/admins/3",
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
				"business-profile", "location-admins", "create",
				"--admin", "newadmin@test.com", "--role", "MANAGER", "123",
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
	if admin["name"] != "locations/123/admins/3" {
		t.Fatalf("expected 'locations/123/admins/3', got %v", admin["name"])
	}
}

func TestExecute_BusinessProfileLocationAdminsDelete_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
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
				"--json", "--account", "a@b.com",
				"business-profile", "location-admins", "delete",
				"--force", "locations/123/admins/1",
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
	if !strings.Contains(gotPath, "/admins/") {
		t.Fatalf("expected path to contain '/admins/', got %q", gotPath)
	}
}

func TestExecute_BusinessProfileLocationAdminsPatch_JSON(t *testing.T) {
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
			"name":  "locations/123/admins/1",
			"admin": "admin@test.com",
			"role":  "SITE_MANAGER",
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
				"business-profile", "location-admins", "patch",
				"locations/123/admins/1", "--role", "SITE_MANAGER",
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
	if admin["role"] != "SITE_MANAGER" {
		t.Fatalf("expected 'SITE_MANAGER', got %v", admin["role"])
	}
}

func TestExecute_BusinessProfileLocationAdminsPatch_NoFields(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		t.Fatalf("unexpected service call — no fields provided should fail before API call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "a@b.com", "business-profile", "location-admins", "patch", "locations/123/admins/1"})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_BusinessProfileLocationTransfer_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotBody map[string]any
	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
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
				"--json", "--account", "a@b.com",
				"business-profile", "locations-transfer",
				"--force", "--destination-account", "accounts/456",
				"locations/123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST method, got %q", gotMethod)
	}
	if !strings.Contains(gotPath, ":transfer") {
		t.Fatalf("expected path to contain ':transfer', got %q", gotPath)
	}
	if gotBody["destinationAccount"] != "accounts/456" {
		t.Fatalf("expected destinationAccount 'accounts/456', got %v", gotBody["destinationAccount"])
	}
}

func TestExecute_BusinessProfileLocationTransfer_AutoPrefix(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotBody map[string]any
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
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
				"--json", "--account", "a@b.com",
				"business-profile", "locations-transfer",
				"--force", "--destination-account", "456",
				"123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(gotPath, "/locations/123") {
		t.Fatalf("expected path to contain '/locations/123', got %q", gotPath)
	}
	if gotBody["destinationAccount"] != "accounts/456" {
		t.Fatalf("expected destinationAccount 'accounts/456', got %v", gotBody["destinationAccount"])
	}
}
