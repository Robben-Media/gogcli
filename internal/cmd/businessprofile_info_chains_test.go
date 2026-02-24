package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"
	"google.golang.org/api/option"
)

func TestExecute_BusinessProfileInfoChainsGet_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() {
		newBusinessProfileInfoService = origInfo
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/chains/123") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":          "chains/123",
				"chainNames":    []map[string]any{{"displayName": "Starbucks"}},
				"locationCount": 35000,
				"websites":      []map[string]any{{"uri": "https://starbucks.com"}},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "business-profile", "chains", "get", "chains/123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Chain struct {
			Name       string `json:"name"`
			ChainNames []struct {
				DisplayName string `json:"displayName"`
			} `json:"chainNames"`
			LocationCount int `json:"locationCount"`
			Websites      []struct {
				URI string `json:"uri"`
			} `json:"websites"`
		} `json:"chain"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Chain.Name != "chains/123" {
		t.Fatalf("unexpected chain name: %q", parsed.Chain.Name)
	}
	if len(parsed.Chain.ChainNames) != 1 || parsed.Chain.ChainNames[0].DisplayName != "Starbucks" {
		t.Fatalf("unexpected chainNames: %+v", parsed.Chain.ChainNames)
	}
	if parsed.Chain.LocationCount != 35000 {
		t.Fatalf("unexpected locationCount: %d", parsed.Chain.LocationCount)
	}
	if len(parsed.Chain.Websites) != 1 || parsed.Chain.Websites[0].URI != "https://starbucks.com" {
		t.Fatalf("unexpected websites: %+v", parsed.Chain.Websites)
	}
}

func TestExecute_BusinessProfileInfoChainsGet_AutoPrefix(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() {
		newBusinessProfileInfoService = origInfo
	})

	var mu sync.Mutex
	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/chains/") && r.Method == http.MethodGet {
			mu.Lock()
			capturedPath = r.URL.Path
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":          "chains/123",
				"chainNames":    []map[string]any{{"displayName": "Test Chain"}},
				"locationCount": 100,
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

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "business-profile", "chains", "get", "123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	path := capturedPath
	mu.Unlock()

	if !strings.Contains(path, "/chains/123") {
		t.Fatalf("expected path to contain /chains/123, got %q", path)
	}
}

func TestExecute_BusinessProfileInfoChainsSearch_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() {
		newBusinessProfileInfoService = origInfo
	})

	var mu sync.Mutex
	var capturedChainName string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/chains") && r.Method == http.MethodGet {
			mu.Lock()
			capturedChainName = r.URL.Query().Get("chainName")
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chains": []map[string]any{
					{
						"name":          "chains/123",
						"chainNames":    []map[string]any{{"displayName": "Starbucks"}},
						"locationCount": 35000,
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "business-profile", "chains", "search", "--chain-name", "Starbucks"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	chainName := capturedChainName
	mu.Unlock()

	if chainName != "Starbucks" {
		t.Fatalf("expected chainName query param to be 'Starbucks', got %q", chainName)
	}

	var parsed struct {
		Chains []struct {
			Name       string `json:"name"`
			ChainNames []struct {
				DisplayName string `json:"displayName"`
			} `json:"chainNames"`
			LocationCount int `json:"locationCount"`
		} `json:"chains"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(parsed.Chains))
	}
	if parsed.Chains[0].Name != "chains/123" {
		t.Fatalf("unexpected chain name: %q", parsed.Chains[0].Name)
	}
	if len(parsed.Chains[0].ChainNames) != 1 || parsed.Chains[0].ChainNames[0].DisplayName != "Starbucks" {
		t.Fatalf("unexpected chainNames: %+v", parsed.Chains[0].ChainNames)
	}
	if parsed.Chains[0].LocationCount != 35000 {
		t.Fatalf("unexpected locationCount: %d", parsed.Chains[0].LocationCount)
	}
}

func TestExecute_BusinessProfileInfoGoogleLocationsSearch_JSON(t *testing.T) {
	origInfo := newBusinessProfileInfoService
	t.Cleanup(func() {
		newBusinessProfileInfoService = origInfo
	})

	var mu sync.Mutex
	var capturedBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "googleLocations:search") && r.Method == http.MethodPost {
			mu.Lock()
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"googleLocations": []map[string]any{
					{
						"name": "googleLocations/abc",
						"location": map[string]any{
							"title": "Starbucks",
							"storefrontAddress": map[string]any{
								"addressLines": []string{"123 Pike St"},
								"locality":     "Seattle",
								"regionCode":   "US",
							},
						},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "business-profile", "google-locations", "--query", "Starbucks Seattle"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	body := capturedBody
	mu.Unlock()

	var reqBody map[string]any
	if err := json.Unmarshal([]byte(body), &reqBody); err != nil {
		t.Fatalf("failed to parse request body: %v\nbody=%q", err, body)
	}
	if reqBody["query"] != "Starbucks Seattle" {
		t.Fatalf("expected query 'Starbucks Seattle' in request body, got %v", reqBody["query"])
	}

	var parsed struct {
		GoogleLocations []struct {
			Name     string `json:"name"`
			Location struct {
				Title             string `json:"title"`
				StorefrontAddress struct {
					AddressLines []string `json:"addressLines"`
					Locality     string   `json:"locality"`
					RegionCode   string   `json:"regionCode"`
				} `json:"storefrontAddress"`
			} `json:"location"`
		} `json:"googleLocations"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.GoogleLocations) != 1 {
		t.Fatalf("expected 1 google location, got %d", len(parsed.GoogleLocations))
	}
	gl := parsed.GoogleLocations[0]
	if gl.Name != "googleLocations/abc" {
		t.Fatalf("unexpected google location name: %q", gl.Name)
	}
	if gl.Location.Title != "Starbucks" {
		t.Fatalf("unexpected location title: %q", gl.Location.Title)
	}
	if gl.Location.StorefrontAddress.Locality != "Seattle" {
		t.Fatalf("unexpected locality: %q", gl.Location.StorefrontAddress.Locality)
	}
	if gl.Location.StorefrontAddress.RegionCode != "US" {
		t.Fatalf("unexpected regionCode: %q", gl.Location.StorefrontAddress.RegionCode)
	}
	if len(gl.Location.StorefrontAddress.AddressLines) != 1 || gl.Location.StorefrontAddress.AddressLines[0] != "123 Pike St" {
		t.Fatalf("unexpected addressLines: %v", gl.Location.StorefrontAddress.AddressLines)
	}
}
