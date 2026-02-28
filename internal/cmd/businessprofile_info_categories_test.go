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

func TestExecute_BusinessProfileInfoCategoriesList_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotPath string
	var gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		gotMethod = r.Method
		mu.Unlock()

		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/categories") && !strings.Contains(r.URL.Path, ":batchGet") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"categories": []map[string]any{
					{
						"name":        "gcid:restaurant",
						"displayName": "Restaurant",
					},
					{
						"name":        "gcid:hotel",
						"displayName": "Hotel",
					},
				},
				"nextPageToken": "page2",
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
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "categories", "list",
				"--region-code", "US",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if gotMethod != http.MethodGet {
		t.Fatalf("expected GET, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/categories") {
		t.Fatalf("expected path to contain /categories, got %q", gotPath)
	}
	if strings.Contains(gotPath, ":batchGet") {
		t.Fatalf("expected path to NOT contain :batchGet, got %q", gotPath)
	}
	mu.Unlock()

	var parsed struct {
		Categories []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"categories"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(parsed.Categories))
	}
	if parsed.Categories[0].Name != "gcid:restaurant" {
		t.Fatalf("unexpected first category name: %q", parsed.Categories[0].Name)
	}
	if parsed.Categories[0].DisplayName != "Restaurant" {
		t.Fatalf("unexpected first category displayName: %q", parsed.Categories[0].DisplayName)
	}
	if parsed.Categories[1].Name != "gcid:hotel" {
		t.Fatalf("unexpected second category name: %q", parsed.Categories[1].Name)
	}
	if parsed.Categories[1].DisplayName != "Hotel" {
		t.Fatalf("unexpected second category displayName: %q", parsed.Categories[1].DisplayName)
	}
	if parsed.NextPageToken != "page2" {
		t.Fatalf("unexpected nextPageToken: %q", parsed.NextPageToken)
	}
}

func TestExecute_BusinessProfileInfoCategoriesList_Text(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/categories") && !strings.Contains(r.URL.Path, ":batchGet") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"categories": []map[string]any{
					{
						"name":        "gcid:restaurant",
						"displayName": "Restaurant",
					},
					{
						"name":        "gcid:hotel",
						"displayName": "Hotel",
					},
				},
				"nextPageToken": "page2",
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
			if err := Execute([]string{
				"--account", "a@b.com",
				"business-profile", "categories", "list",
				"--region-code", "US",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "gcid:restaurant") {
		t.Fatalf("expected output to contain gcid:restaurant, got %q", out)
	}
	if !strings.Contains(out, "Restaurant") {
		t.Fatalf("expected output to contain Restaurant, got %q", out)
	}
	if !strings.Contains(out, "gcid:hotel") {
		t.Fatalf("expected output to contain gcid:hotel, got %q", out)
	}
	if !strings.Contains(out, "Hotel") {
		t.Fatalf("expected output to contain Hotel, got %q", out)
	}
}

func TestExecute_BusinessProfileInfoCategoriesBatchGet_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotPath string
	var gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		gotMethod = r.Method
		mu.Unlock()

		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, ":batchGet") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"categories": []map[string]any{
					{
						"name":        "gcid:restaurant",
						"displayName": "Restaurant",
					},
				},
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
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"business-profile", "categories", "batch-get",
				"--names", "gcid:restaurant",
				"--names", "gcid:hotel",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if gotMethod != http.MethodGet {
		t.Fatalf("expected GET, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, ":batchGet") {
		t.Fatalf("expected path to contain :batchGet, got %q", gotPath)
	}
	mu.Unlock()

	var parsed struct {
		Categories []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"categories"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(parsed.Categories))
	}
	if parsed.Categories[0].Name != "gcid:restaurant" {
		t.Fatalf("unexpected category name: %q", parsed.Categories[0].Name)
	}
	if parsed.Categories[0].DisplayName != "Restaurant" {
		t.Fatalf("unexpected category displayName: %q", parsed.Categories[0].DisplayName)
	}
}
