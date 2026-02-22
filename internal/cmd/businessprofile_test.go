package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mybusinessaccountmanagement "google.golang.org/api/mybusinessaccountmanagement/v1"
	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"
	"google.golang.org/api/option"
)

func TestExecute_BusinessProfileAccounts_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() {
		newBusinessProfileAccountsService = origAccounts
		newBusinessProfileInfoService = origInfo
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/accounts") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": []map[string]any{
					{
						"name":        "accounts/123456789",
						"accountName": "Test Business",
						"type":        "PERSONAL",
						"role":        "PRIMARY_OWNER",
					},
					{
						"name":        "accounts/987654321",
						"accountName": "Another Business",
						"type":        "LOCATION_GROUP",
						"role":        "OWNER",
					},
				},
				"nextPageToken": "page2",
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "business-profile", "accounts"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Accounts []struct {
			Name        string `json:"name"`
			AccountName string `json:"accountName"`
			Type        string `json:"type"`
			Role        string `json:"role"`
		} `json:"accounts"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(parsed.Accounts))
	}
	if parsed.Accounts[0].Name != "accounts/123456789" {
		t.Fatalf("unexpected first account name: %q", parsed.Accounts[0].Name)
	}
	if parsed.Accounts[0].AccountName != "Test Business" {
		t.Fatalf("unexpected first account accountName: %q", parsed.Accounts[0].AccountName)
	}
	if parsed.Accounts[1].Role != "OWNER" {
		t.Fatalf("unexpected second account role: %q", parsed.Accounts[1].Role)
	}
	if parsed.NextPageToken != "page2" {
		t.Fatalf("unexpected nextPageToken: %q", parsed.NextPageToken)
	}
}

func TestExecute_BusinessProfileLocations_JSON(t *testing.T) {
	origAccounts := newBusinessProfileAccountsService
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() {
		newBusinessProfileAccountsService = origAccounts
		newBusinessProfileInfoService = origInfo
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/locations") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"locations": []map[string]any{
					{
						"name":  "locations/111",
						"title": "Main Street Store",
						"storefrontAddress": map[string]any{
							"addressLines":       []string{"123 Main St"},
							"locality":           "Springfield",
							"administrativeArea": "IL",
							"postalCode":         "62701",
							"regionCode":         "US",
						},
					},
					{
						"name":  "locations/222",
						"title": "Downtown Office",
						"storefrontAddress": map[string]any{
							"addressLines": []string{"456 Oak Ave"},
							"locality":     "Chicago",
							"regionCode":   "US",
						},
					},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := mybusinessbusinessinformation.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBusinessProfileInfoService = func(context.Context, string) (*mybusinessbusinessinformation.Service, error) {
		return svc, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "business-profile", "locations", "123456789"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Locations []struct {
			Name  string `json:"name"`
			Title string `json:"title"`
		} `json:"locations"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Locations) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(parsed.Locations))
	}
	if parsed.Locations[0].Name != "locations/111" {
		t.Fatalf("unexpected first location name: %q", parsed.Locations[0].Name)
	}
	if parsed.Locations[0].Title != "Main Street Store" {
		t.Fatalf("unexpected first location title: %q", parsed.Locations[0].Title)
	}
}

func TestExecute_BusinessProfileAccounts_NoAccount(t *testing.T) {
	err := Execute([]string{"--json", "business-profile", "accounts"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}
