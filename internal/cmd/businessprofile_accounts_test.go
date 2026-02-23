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

func TestExecute_BusinessProfileAccountsCreate_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/accounts") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "accounts/new123",
			"accountName": "My New Business",
			"type":        "LOCATION_GROUP",
			"role":        "PRIMARY_OWNER",
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
				"business-profile", "accounts", "create",
				"--account-name", "My New Business",
				"--type", "LOCATION_GROUP",
				"--primary-owner", "accounts/owner1",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if gotBody["accountName"] != "My New Business" {
		t.Fatalf("expected accountName 'My New Business', got %v", gotBody["accountName"])
	}
	if gotBody["type"] != "LOCATION_GROUP" {
		t.Fatalf("expected type 'LOCATION_GROUP', got %v", gotBody["type"])
	}
	if gotBody["primaryOwner"] != "accounts/owner1" {
		t.Fatalf("expected primaryOwner 'accounts/owner1', got %v", gotBody["primaryOwner"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	acct, ok := result["account"].(map[string]any)
	if !ok {
		t.Fatalf("expected account object")
	}
	if acct["name"] != "accounts/new123" {
		t.Fatalf("expected accounts/new123, got %v", acct["name"])
	}
}

func TestExecute_BusinessProfileAccountsCreate_Text(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "accounts/new123",
			"accountName": "My New Business",
			"type":        "LOCATION_GROUP",
			"role":        "PRIMARY_OWNER",
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
				"business-profile", "accounts", "create",
				"--account-name", "My New Business",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "accounts/new123") {
		t.Fatalf("expected output to contain 'accounts/new123', got %q", out)
	}
	if !strings.Contains(out, "My New Business") {
		t.Fatalf("expected output to contain 'My New Business', got %q", out)
	}
}

func TestExecute_BusinessProfileAccountsGet_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/accounts/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":            "accounts/123",
			"accountName":     "Test Business",
			"type":            "PERSONAL",
			"role":            "PRIMARY_OWNER",
			"permissionLevel": "OWNER_LEVEL",
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
				"business-profile", "accounts", "get", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	acct, ok := result["account"].(map[string]any)
	if !ok {
		t.Fatalf("expected account object")
	}
	if acct["accountName"] != "Test Business" {
		t.Fatalf("expected 'Test Business', got %v", acct["accountName"])
	}
	if acct["permissionLevel"] != "OWNER_LEVEL" {
		t.Fatalf("expected 'OWNER_LEVEL', got %v", acct["permissionLevel"])
	}
}

func TestExecute_BusinessProfileAccountsGet_Text(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":            "accounts/123",
			"accountName":     "Test Business",
			"type":            "PERSONAL",
			"role":            "PRIMARY_OWNER",
			"permissionLevel": "OWNER_LEVEL",
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
				"business-profile", "accounts", "get", "accounts/123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "accounts/123") {
		t.Fatalf("expected output to contain 'accounts/123', got %q", out)
	}
	if !strings.Contains(out, "Test Business") {
		t.Fatalf("expected output to contain 'Test Business', got %q", out)
	}
	if !strings.Contains(out, "OWNER_LEVEL") {
		t.Fatalf("expected output to contain 'OWNER_LEVEL', got %q", out)
	}
}

func TestExecute_BusinessProfileAccountsPatch_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/accounts/") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotUpdateMask = r.URL.Query().Get("updateMask")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "accounts/123",
			"accountName": "Updated Business",
			"type":        "PERSONAL",
			"role":        "PRIMARY_OWNER",
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
				"business-profile", "accounts", "patch", "123",
				"--account-name", "Updated Business",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(gotUpdateMask, "accountName") {
		t.Fatalf("expected updateMask to contain 'accountName', got %q", gotUpdateMask)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	acct, ok := result["account"].(map[string]any)
	if !ok {
		t.Fatalf("expected account object")
	}
	if acct["accountName"] != "Updated Business" {
		t.Fatalf("expected 'Updated Business', got %v", acct["accountName"])
	}
}

func TestExecute_BusinessProfileAccountsPatch_NoFields(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })
	newBusinessProfileAccountsService = func(context.Context, string) (*mybusinessaccountmanagement.Service, error) {
		t.Fatalf("unexpected service call — no fields provided should fail before API call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "a@b.com", "business-profile", "accounts", "patch", "123"})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_BusinessProfileAccountsGet_AutoPrefix(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "accounts/123",
			"accountName": "Test",
			"type":        "PERSONAL",
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
			// Pass bare ID without accounts/ prefix
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "accounts", "get", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	// Verify the path includes the accounts/ prefix
	if !strings.Contains(gotPath, "/accounts/123") {
		t.Fatalf("expected path to contain '/accounts/123', got %q", gotPath)
	}

	if !strings.Contains(out, "accounts/123") {
		t.Fatalf("expected output to contain 'accounts/123', got %q", out)
	}
}
