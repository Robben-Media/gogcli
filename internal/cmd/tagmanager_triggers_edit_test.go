package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tagmanager/v2"
)

func setupTagManagerResourceTest(t *testing.T, handler http.Handler) {
	t.Helper()

	original := newTagManagerService
	t.Cleanup(func() { newTagManagerService = original })

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	service, err := tagmanager.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	newTagManagerService = func(context.Context, string) (*tagmanager.Service, error) { return service, nil }
}

func TestExecute_TagManagerTriggersCreate(t *testing.T) {
	setupTagManagerResourceTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tagmanager/v2/accounts/111/containers/c1/workspaces/7/triggers" {
			http.NotFound(w, r)

			return
		}

		var body tagmanager.Trigger
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Name != "Checkout" || body.Type != "customEvent" || len(body.Filter) != 1 || len(body.CustomEventFilter) != 1 {
			t.Fatalf("unexpected trigger: %#v", body)
		}
		if body.Filter[0].Type != "equals" || body.Filter[0].Parameter[0].Key != "arg0" || body.Filter[0].Parameter[0].Type != "template" {
			t.Fatalf("unexpected filter: %#v", body.Filter[0])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"path":"accounts/111/containers/c1/workspaces/7/triggers/9","triggerId":"9","name":"Checkout","type":"customEvent"}`))
	}))

	filter := `{"type":"equals","parameter":[{"type":"template","key":"arg0","value":"{{Page URL}}"},{"type":"template","key":"arg1","value":"https://example.com"}]}`
	customEventFilter := `{"type":"matchRegex","parameter":[{"type":"template","key":"arg0","value":"{{Event}}"},{"type":"template","key":"arg1","value":"^checkout$"}]}`
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "gtm", "triggers", "create",
				"--account-id", "111", "--container-id", "c1", "--workspace-id", "7",
				"--name", "Checkout", "--type", "customEvent",
				"--filter", filter, "--custom-event-filter", customEventFilter,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result struct {
		Trigger *tagmanager.Trigger `json:"trigger"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.Trigger == nil || result.Trigger.TriggerId != "9" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecute_TagManagerTriggerConfigurations(t *testing.T) {
	setupTagManagerResourceTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body tagmanager.Trigger
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		switch body.Type {
		case "click":
			if len(body.AutoEventFilter) != 1 || body.AutoEventFilter[0].Parameter[0].Type != "template" {
				t.Fatalf("click filter not preserved: %#v", body)
			}
		case "timer":
			if body.EventName == nil || body.Interval == nil || body.Limit == nil || body.Interval.Type != "template" {
				t.Fatalf("timer parameters not preserved: %#v", body)
			}
		default:
			t.Fatalf("unexpected trigger type: %q", body.Type)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))

	autoEventFilter := `{"type":"equals","parameter":[{"type":"template","key":"arg0","value":"{{Click ID}}"},{"type":"template","key":"arg1","value":"buy"}]}`
	parameter := `{"type":"template","value":"value"}`
	for _, args := range [][]string{
		{"--json", "--account", "admin@example.com", "gtm", "triggers", "create", "--account-id", "111", "--container-id", "c1", "--name", "Click", "--type", "click", "--auto-event-filter", autoEventFilter},
		{"--json", "--account", "admin@example.com", "gtm", "triggers", "create", "--account-id", "111", "--container-id", "c1", "--name", "Timer", "--type", "timer", "--event-name", parameter, "--interval", parameter, "--limit", parameter},
	} {
		captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := Execute(args); err != nil {
					t.Fatalf("Execute(%v): %v", args, err)
				}
			})
		})
	}
}

func TestExecute_TagManagerTriggersLifecycle(t *testing.T) {
	path := "/tagmanager/v2/accounts/111/containers/c1/workspaces/7/triggers/9"
	requests := 0
	setupTagManagerResourceTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == path:
			_, _ = w.Write([]byte(`{"path":"accounts/111/containers/c1/workspaces/7/triggers/9","triggerId":"9","name":"Old","type":"pageview"}`))
		case r.Method == http.MethodPut && r.URL.Path == path:
			var body tagmanager.Trigger
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update: %v", err)
			}
			if body.Name != "New" || body.Type != "pageview" {
				t.Fatalf("partial update lost fields: %#v", body)
			}
			_, _ = w.Write([]byte(`{"path":"accounts/111/containers/c1/workspaces/7/triggers/9","triggerId":"9","name":"New","type":"pageview"}`))
		case r.Method == http.MethodPost && r.URL.Path == path+":revert":
			_, _ = w.Write([]byte(`{"trigger":{"path":"accounts/111/containers/c1/workspaces/7/triggers/9","triggerId":"9","name":"Published","type":"pageview"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == path:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))

	resource := "accounts/111/containers/c1/workspaces/7/triggers/9"
	for _, args := range [][]string{
		{"--json", "--account", "admin@example.com", "gtm", "triggers", "get", resource},
		{"--json", "--account", "admin@example.com", "gtm", "triggers", "update", resource, "--name", "New"},
		{"--json", "--account", "admin@example.com", "gtm", "triggers", "revert", resource},
		{"--json", "--force", "--account", "admin@example.com", "gtm", "triggers", "delete", resource},
	} {
		captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := Execute(args); err != nil {
					t.Fatalf("Execute(%v): %v", args, err)
				}
			})
		})
	}
	if requests != 5 { // update reads the current trigger before replacing it.
		t.Fatalf("requests = %d, want 5", requests)
	}
}

func TestExecute_TagManagerTriggersDeleteRequiresConfirmation(t *testing.T) {
	called := false
	setupTagManagerResourceTest(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	err := Execute([]string{
		"--json", "--no-input", "--account", "admin@example.com", "gtm", "triggers", "delete",
		"accounts/111/containers/c1/workspaces/7/triggers/9",
	})
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	if called {
		t.Fatal("API called before confirmation")
	}
}
