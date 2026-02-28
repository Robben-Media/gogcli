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

func TestExecute_BusinessProfileInfoAttributesList_JSON(t *testing.T) {
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

		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/attributes") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"attributeMetadata": []map[string]any{
					{
						"parent":      "locations/123",
						"displayName": "Has Wi-Fi",
						"valueType":   "BOOL",
					},
				},
				"nextPageToken": "tok2",
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
				"business-profile", "attributes", "list",
				"--language-code", "en",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if gotMethod != http.MethodGet {
		t.Fatalf("expected GET, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/attributes") {
		t.Fatalf("expected path to contain /attributes, got %q", gotPath)
	}
	mu.Unlock()

	var parsed struct {
		AttributeMetadata []struct {
			Parent      string `json:"parent"`
			DisplayName string `json:"displayName"`
			ValueType   string `json:"valueType"`
		} `json:"attributeMetadata"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.AttributeMetadata) != 1 {
		t.Fatalf("expected 1 attributeMetadata, got %d", len(parsed.AttributeMetadata))
	}
	if parsed.AttributeMetadata[0].Parent != "locations/123" {
		t.Fatalf("unexpected parent: %q", parsed.AttributeMetadata[0].Parent)
	}
	if parsed.AttributeMetadata[0].DisplayName != "Has Wi-Fi" {
		t.Fatalf("unexpected displayName: %q", parsed.AttributeMetadata[0].DisplayName)
	}
	if parsed.AttributeMetadata[0].ValueType != "BOOL" {
		t.Fatalf("unexpected valueType: %q", parsed.AttributeMetadata[0].ValueType)
	}
	if parsed.NextPageToken != "tok2" {
		t.Fatalf("unexpected nextPageToken: %q", parsed.NextPageToken)
	}
}

func TestExecute_BusinessProfileInfoAttributesList_Table(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/attributes") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"attributeMetadata": []map[string]any{
					{
						"parent":      "locations/123",
						"displayName": "Has Wi-Fi",
						"valueType":   "BOOL",
					},
				},
				"nextPageToken": "tok2",
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
				"business-profile", "attributes", "list",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "PARENT") {
		t.Fatalf("expected output to contain PARENT header, got %q", out)
	}
	if !strings.Contains(out, "DISPLAY_NAME") {
		t.Fatalf("expected output to contain DISPLAY_NAME header, got %q", out)
	}
	if !strings.Contains(out, "VALUE_TYPE") {
		t.Fatalf("expected output to contain VALUE_TYPE header, got %q", out)
	}
	if !strings.Contains(out, "locations/123") {
		t.Fatalf("expected output to contain locations/123, got %q", out)
	}
	if !strings.Contains(out, "Has Wi-Fi") {
		t.Fatalf("expected output to contain 'Has Wi-Fi', got %q", out)
	}
	if !strings.Contains(out, "BOOL") {
		t.Fatalf("expected output to contain BOOL, got %q", out)
	}
}

func TestExecute_BusinessProfileInfoLocationAttrsGet_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		mu.Unlock()

		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/attributes") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "locations/123/attributes",
				"attributes": []map[string]any{
					{
						"name":      "has_wifi",
						"valueType": "BOOL",
						"values":    []any{true},
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
				"business-profile", "location-attributes", "get", "locations/123/attributes",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if !strings.Contains(gotPath, "locations/123/attributes") {
		t.Fatalf("expected path to contain 'locations/123/attributes', got %q", gotPath)
	}
	mu.Unlock()

	var parsed struct {
		Name       string `json:"name"`
		Attributes []struct {
			Name      string `json:"name"`
			ValueType string `json:"valueType"`
			Values    []any  `json:"values"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Name != "locations/123/attributes" {
		t.Fatalf("unexpected name: %q", parsed.Name)
	}
	if len(parsed.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(parsed.Attributes))
	}
	if parsed.Attributes[0].Name != "has_wifi" {
		t.Fatalf("unexpected attribute name: %q", parsed.Attributes[0].Name)
	}
	if parsed.Attributes[0].ValueType != "BOOL" {
		t.Fatalf("unexpected valueType: %q", parsed.Attributes[0].ValueType)
	}
}

func TestExecute_BusinessProfileInfoLocationAttrsGetGoogleUpdated_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		mu.Unlock()

		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, ":getGoogleUpdated") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "locations/123/attributes",
				"attributes": []map[string]any{
					{
						"name":      "has_wifi",
						"valueType": "BOOL",
						"values":    []any{true},
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
				"business-profile", "location-attributes", "get-google-updated", "locations/123/attributes",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if !strings.Contains(gotPath, ":getGoogleUpdated") {
		t.Fatalf("expected path to contain ':getGoogleUpdated', got %q", gotPath)
	}
	mu.Unlock()

	var parsed struct {
		Name       string `json:"name"`
		Attributes []struct {
			Name      string `json:"name"`
			ValueType string `json:"valueType"`
			Values    []any  `json:"values"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Name != "locations/123/attributes" {
		t.Fatalf("unexpected name: %q", parsed.Name)
	}
	if len(parsed.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(parsed.Attributes))
	}
	if parsed.Attributes[0].Name != "has_wifi" {
		t.Fatalf("unexpected attribute name: %q", parsed.Attributes[0].Name)
	}
	if parsed.Attributes[0].ValueType != "BOOL" {
		t.Fatalf("unexpected valueType: %q", parsed.Attributes[0].ValueType)
	}
}

func TestExecute_BusinessProfileInfoLocationAttrsUpdate_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() { newBusinessProfileInfoService = origInfo })

	var mu sync.Mutex
	var gotMethod string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		mu.Unlock()

		if (r.Method == http.MethodPut || r.Method == http.MethodPatch) && strings.Contains(r.URL.Path, "/attributes") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "locations/123/attributes",
				"attributes": []map[string]any{
					{
						"name":      "has_wifi",
						"valueType": "BOOL",
						"values":    []any{true},
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
				"business-profile", "location-attributes", "update", "locations/123/attributes",
				"--attributes-json", `[{"name":"has_wifi","valueType":"BOOL"}]`,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if gotMethod != http.MethodPut && gotMethod != http.MethodPatch {
		t.Fatalf("expected PUT or PATCH, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "locations/123/attributes") {
		t.Fatalf("expected path to contain 'locations/123/attributes', got %q", gotPath)
	}
	mu.Unlock()

	var parsed struct {
		Name       string `json:"name"`
		Attributes []struct {
			Name      string `json:"name"`
			ValueType string `json:"valueType"`
			Values    []any  `json:"values"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Name != "locations/123/attributes" {
		t.Fatalf("unexpected name: %q", parsed.Name)
	}
	if len(parsed.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(parsed.Attributes))
	}
	if parsed.Attributes[0].Name != "has_wifi" {
		t.Fatalf("unexpected attribute name: %q", parsed.Attributes[0].Name)
	}
}
