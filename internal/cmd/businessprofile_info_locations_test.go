package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"
	"google.golang.org/api/option"
)

func TestExecute_BusinessProfileInfoLocationsCreate_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotBody map[string]any
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/locations") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "locations/new1",
			"title": "My Shop",
		})
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
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "info-locations", "create", "123",
				"--title", "My Shop",
				"--category-id", "gcid:restaurant",
				"--phone", "+1234567890",
				"--address-lines", "123 Main St",
				"--locality", "Springfield",
				"--country", "US",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()

	if !strings.Contains(gotPath, "/locations") {
		t.Fatalf("expected path to contain '/locations', got %q", gotPath)
	}

	// Verify request body contains expected fields
	title, _ := gotBody["title"].(string)
	if title != "My Shop" {
		t.Fatalf("expected title 'My Shop' in body, got %q", title)
	}

	categories, ok := gotBody["categories"].(map[string]any)
	if !ok {
		t.Fatalf("expected categories in body")
	}
	primary, ok := categories["primaryCategory"].(map[string]any)
	if !ok {
		t.Fatalf("expected primaryCategory in categories")
	}
	if primary["name"] != "gcid:restaurant" {
		t.Fatalf("expected category name 'gcid:restaurant', got %v", primary["name"])
	}

	phoneNumbers, ok := gotBody["phoneNumbers"].(map[string]any)
	if !ok {
		t.Fatalf("expected phoneNumbers in body")
	}
	if phoneNumbers["primaryPhone"] != "+1234567890" {
		t.Fatalf("expected primaryPhone '+1234567890', got %v", phoneNumbers["primaryPhone"])
	}

	address, ok := gotBody["storefrontAddress"].(map[string]any)
	if !ok {
		t.Fatalf("expected storefrontAddress in body")
	}
	lines, ok := address["addressLines"].([]any)
	if !ok || len(lines) == 0 {
		t.Fatalf("expected addressLines in address")
	}
	if lines[0] != "123 Main St" {
		t.Fatalf("expected address line '123 Main St', got %v", lines[0])
	}
	if address["locality"] != "Springfield" {
		t.Fatalf("expected locality 'Springfield', got %v", address["locality"])
	}
	if address["regionCode"] != "US" {
		t.Fatalf("expected regionCode 'US', got %v", address["regionCode"])
	}

	// Verify JSON output
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v\nout=%q", err, out)
	}
	loc, ok := result["location"].(map[string]any)
	if !ok {
		t.Fatalf("expected location object in output")
	}
	if loc["name"] != "locations/new1" {
		t.Fatalf("expected name 'locations/new1', got %v", loc["name"])
	}
	if loc["title"] != "My Shop" {
		t.Fatalf("expected title 'My Shop', got %v", loc["title"])
	}
}

func TestExecute_BusinessProfileInfoLocationsDelete_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/locations/") {
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

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--force", "--account", "a@b.com",
				"business-profile", "info-locations", "delete", "123",
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
	if !strings.Contains(gotPath, "locations/123") {
		t.Fatalf("expected path to contain 'locations/123', got %q", gotPath)
	}
}

func TestExecute_BusinessProfileInfoLocationsGetGoogleUpdated_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, ":getGoogleUpdated") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"location": map[string]any{
				"name":  "locations/123",
				"title": "Updated Shop",
			},
			"diffMask":    "title",
			"pendingMask": "",
		})
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
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "info-locations", "get-google-updated", "123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v\nout=%q", err, out)
	}

	loc, ok := result["location"].(map[string]any)
	if !ok {
		t.Fatalf("expected location object in output")
	}
	if loc["name"] != "locations/123" {
		t.Fatalf("expected name 'locations/123', got %v", loc["name"])
	}
	if loc["title"] != "Updated Shop" {
		t.Fatalf("expected title 'Updated Shop', got %v", loc["title"])
	}
	if result["diffMask"] != "title" {
		t.Fatalf("expected diffMask 'title', got %v", result["diffMask"])
	}
	if result["pendingMask"] != "" {
		t.Fatalf("expected empty pendingMask, got %v", result["pendingMask"])
	}
}

func TestExecute_BusinessProfileInfoLocationsPatch_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/locations/") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotUpdateMask = r.URL.Query().Get("updateMask")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "locations/123",
			"title": "New Name",
		})
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
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "info-locations", "patch", "123",
				"--title", "New Name",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if !strings.Contains(gotUpdateMask, "title") {
		t.Fatalf("expected updateMask to contain 'title', got %q", gotUpdateMask)
	}
	mu.Unlock()

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v\nout=%q", err, out)
	}
	loc, ok := result["location"].(map[string]any)
	if !ok {
		t.Fatalf("expected location object in output")
	}
	if loc["name"] != "locations/123" {
		t.Fatalf("expected name 'locations/123', got %v", loc["name"])
	}
	if loc["title"] != "New Name" {
		t.Fatalf("expected title 'New Name', got %v", loc["title"])
	}
}

func TestExecute_BusinessProfileInfoLocationsPatch_NoFields(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })
	newBusinessProfileInfoService = func(context.Context, string) (*mybusinessbusinessinformation.Service, error) {
		t.Fatalf("unexpected service call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{
		"--account", "a@b.com",
		"business-profile", "info-locations", "patch", "123",
	})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}
