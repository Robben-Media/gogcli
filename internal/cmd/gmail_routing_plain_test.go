package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Plain mutation receipts for Gmail routing settings (issue #75):
// RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS (+ one data row).

func TestGmailDelegatesAddCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/settings/delegates") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"delegateEmail":      "del\teg@example.com",
			"verificationStatus": "pending",
		})
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailDelegatesAddCmd{}, []string{"del\teg@example.com"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\ndelegate\tcreate\tdel eg@example.com\tpending\n"
	if out != want {
		t.Fatalf("plain delegates add = %q, want %q", out, want)
	}
	if strings.Contains(out, "invitation") || strings.Contains(out, "successfully") {
		t.Fatalf("plain stdout leaked advice/prose: %q", out)
	}
}

func TestGmailDelegatesRemoveCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailDelegatesRemoveCmd{}, []string{"d@b.com"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\ndelegate\tdelete\td@b.com\tsuccess\n"
	if out != want {
		t.Fatalf("plain delegates remove = %q, want %q", out, want)
	}
}

func TestGmailForwardingCreateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/forwardingAddresses") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"forwardingEmail":    "fwd@example.com",
			"verificationStatus": "pending",
		})
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailForwardingCreateCmd{}, []string{"fwd@example.com"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nforwarding_address\tcreate\tfwd@example.com\tpending\n"
	if out != want {
		t.Fatalf("plain forwarding create = %q, want %q", out, want)
	}
	if strings.Contains(out, "verification email") || strings.Contains(out, "successfully") {
		t.Fatalf("plain stdout leaked advice/prose: %q", out)
	}
}

func TestGmailForwardingDeleteCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailForwardingDeleteCmd{}, []string{"fwd@example.com"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nforwarding_address\tdelete\tfwd@example.com\tsuccess\n"
	if out != want {
		t.Fatalf("plain forwarding delete = %q, want %q", out, want)
	}
}

func TestGmailSendAsCreateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/settings/sendAs") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sendAsEmail":        "alias@example.com",
			"verificationStatus": "pending",
		})
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailSendAsCreateCmd{}, []string{"alias@example.com", "--display-name", "Alias"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nsend_as\tcreate\talias@example.com\tpending\n"
	if out != want {
		t.Fatalf("plain send-as create = %q, want %q", out, want)
	}
	// Advice may stay on stderr; must not appear on stdout.
	if strings.Contains(out, "Verification email") {
		t.Fatalf("plain stdout leaked advice: %q", out)
	}
}

func TestGmailSendAsVerifyCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/verify") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailSendAsVerifyCmd{}, []string{"alias@example.com"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nsend_as\tverify\talias@example.com\tsuccess\n"
	if out != want {
		t.Fatalf("plain send-as verify = %q, want %q", out, want)
	}
}

func TestGmailSendAsUpdateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/settings/sendAs/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"displayName":        "Old",
				"verificationStatus": "accepted",
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/settings/sendAs/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"displayName":        "New",
				"verificationStatus": "accepted",
			})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailSendAsUpdateCmd{}, []string{"alias@example.com", "--display-name", "New"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nsend_as\tupdate\talias@example.com\taccepted\n"
	if out != want {
		t.Fatalf("plain send-as update = %q, want %q", out, want)
	}
}

func TestGmailSendAsDeleteCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailSendAsDeleteCmd{}, []string{"alias@example.com"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nsend_as\tdelete\talias@example.com\tsuccess\n"
	if out != want {
		t.Fatalf("plain send-as delete = %q, want %q", out, want)
	}
}

func TestGmailFiltersCreateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/settings/filters") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "filter-123",
			"criteria": map[string]any{
				"from": "a@example.com",
			},
			"action": map[string]any{
				"addLabelIds": []string{"STARRED"},
			},
		})
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailFiltersCreateCmd{}, []string{"--from", "a@example.com", "--star"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nfilter\tcreate\tfilter-123\tsuccess\n"
	if out != want {
		t.Fatalf("plain filter create = %q, want %q", out, want)
	}
	if strings.Contains(out, "successfully") {
		t.Fatalf("plain stdout leaked prose: %q", out)
	}
}

func TestGmailFiltersDeleteCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailFiltersDeleteCmd{}, []string{"filter-123"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})

	want := "RESOURCE_TYPE\tACTION\tRESOURCE\tSTATUS\nfilter\tdelete\tfilter-123\tsuccess\n"
	if out != want {
		t.Fatalf("plain filter delete = %q, want %q", out, want)
	}
}

func TestGmailDelegatesAddCmd_JSONUnchanged(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"delegateEmail":      "d@b.com",
			"verificationStatus": "pending",
		})
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		ctx := ui.WithUI(context.Background(), mustUI(t))
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if runErr := runKong(t, &GmailDelegatesAddCmd{}, []string{"d@b.com"}, ctx, flags); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})
	if !strings.Contains(out, `"delegate"`) || !strings.Contains(out, `"d@b.com"`) {
		t.Fatalf("json shape changed: %q", out)
	}
	if strings.Contains(out, "RESOURCE_TYPE") {
		t.Fatalf("json path emitted plain receipt: %q", out)
	}
}

func mustUI(t *testing.T) *ui.UI {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return u
}
