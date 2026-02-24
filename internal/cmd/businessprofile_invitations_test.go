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

func TestExecute_BusinessProfileInvitationsList_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/invitations") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"invitations": []map[string]any{
				{"name": "accounts/123/invitations/1", "role": "MANAGER", "targetType": "ACCOUNTS_ONLY"},
				{"name": "accounts/123/invitations/2", "role": "OWNER", "targetType": "LOCATIONS_ONLY"},
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
				"business-profile", "account-invitations", "list", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	invitations, ok := result["invitations"].([]any)
	if !ok {
		t.Fatalf("expected invitations array")
	}
	if len(invitations) != 2 {
		t.Fatalf("expected 2 invitations, got %d", len(invitations))
	}

	inv1, ok := invitations[0].(map[string]any)
	if !ok {
		t.Fatalf("expected invitation object")
	}
	if inv1["name"] != "accounts/123/invitations/1" {
		t.Fatalf("expected name 'accounts/123/invitations/1', got %v", inv1["name"])
	}
	if inv1["role"] != "MANAGER" {
		t.Fatalf("expected role 'MANAGER', got %v", inv1["role"])
	}
	if inv1["targetType"] != "ACCOUNTS_ONLY" {
		t.Fatalf("expected targetType 'ACCOUNTS_ONLY', got %v", inv1["targetType"])
	}

	inv2, ok := invitations[1].(map[string]any)
	if !ok {
		t.Fatalf("expected invitation object")
	}
	if inv2["name"] != "accounts/123/invitations/2" {
		t.Fatalf("expected name 'accounts/123/invitations/2', got %v", inv2["name"])
	}
	if inv2["role"] != "OWNER" {
		t.Fatalf("expected role 'OWNER', got %v", inv2["role"])
	}
	if inv2["targetType"] != "LOCATIONS_ONLY" {
		t.Fatalf("expected targetType 'LOCATIONS_ONLY', got %v", inv2["targetType"])
	}
}

func TestExecute_BusinessProfileInvitationsList_Text(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"invitations": []map[string]any{
				{"name": "accounts/123/invitations/1", "role": "MANAGER", "targetType": "ACCOUNTS_ONLY"},
				{"name": "accounts/123/invitations/2", "role": "OWNER", "targetType": "LOCATIONS_ONLY"},
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
				"business-profile", "account-invitations", "list", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "accounts/123/invitations/1") {
		t.Fatalf("expected output to contain 'accounts/123/invitations/1', got %q", out)
	}
	if !strings.Contains(out, "accounts/123/invitations/2") {
		t.Fatalf("expected output to contain 'accounts/123/invitations/2', got %q", out)
	}
}

func TestExecute_BusinessProfileInvitationsList_WithFilter(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotFilter string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/invitations") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotFilter = r.URL.Query().Get("filter")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"invitations": []map[string]any{
				{"name": "accounts/123/invitations/1", "role": "MANAGER", "targetType": "ACCOUNTS_ONLY"},
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
				"business-profile", "account-invitations", "list", "123",
				"--filter", "target_type=ACCOUNTS_ONLY",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if gotFilter != "target_type=ACCOUNTS_ONLY" {
		t.Fatalf("expected filter 'target_type=ACCOUNTS_ONLY', got %q", gotFilter)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	invitations, ok := result["invitations"].([]any)
	if !ok {
		t.Fatalf("expected invitations array")
	}
	if len(invitations) != 1 {
		t.Fatalf("expected 1 invitation, got %d", len(invitations))
	}
}

func TestExecute_BusinessProfileInvitationsAccept_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":accept") {
			http.NotFound(w, r)
			return
		}
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

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "account-invitations", "accept",
				"accounts/123/invitations/456",
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
	if !strings.Contains(gotPath, ":accept") {
		t.Fatalf("expected path to contain ':accept', got %q", gotPath)
	}
	if !strings.Contains(stderr, "accepted") {
		t.Fatalf("expected stderr to contain 'accepted', got %q", stderr)
	}
}

func TestExecute_BusinessProfileInvitationsDecline_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	t.Cleanup(func() { newBusinessProfileAccountsService = origAccounts })

	var mu sync.Mutex
	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":decline") {
			http.NotFound(w, r)
			return
		}
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

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "account-invitations", "decline",
				"accounts/123/invitations/456",
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
	if !strings.Contains(gotPath, ":decline") {
		t.Fatalf("expected path to contain ':decline', got %q", gotPath)
	}
	if !strings.Contains(stderr, "declined") {
		t.Fatalf("expected stderr to contain 'declined', got %q", stderr)
	}
}
