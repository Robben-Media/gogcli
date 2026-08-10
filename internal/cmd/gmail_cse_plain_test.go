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

// Plain mutation receipts for CSE identity / key-pair lifecycle (issue #71).

func TestGmailCseIdentitiesCreateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		resp := &gmail.CseIdentity{
			EmailAddress:     "user@example.com",
			PrimaryKeyPairId: "kp123",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesCreateCmd{
		Email:            "user@example.com",
		PrimaryKeyPairID: "kp123",
	}
	out := captureStdout(t, func() {
		if runErr := cmd.Run(ctx, flags); runErr != nil {
			t.Fatalf("Run failed: %v", runErr)
		}
	})

	want := "ACTION\tEMAIL\tPRIMARY_KEY_PAIR_ID\tSTATUS\ncreate\tuser@example.com\tkp123\tok\n"
	if out != want {
		t.Fatalf("plain identity create output = %q, want %q", out, want)
	}
}

func TestGmailCseIdentitiesPatchCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET":
			resp := &gmail.CseIdentity{
				EmailAddress:     "user@example.com",
				PrimaryKeyPairId: "kp-old",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == "PATCH" || r.Method == "POST" || r.Method == "PUT":
			resp := &gmail.CseIdentity{
				EmailAddress:     "user@example.com",
				PrimaryKeyPairId: "kp-new",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Errorf("unexpected method %s path %s", r.Method, r.URL.Path)
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesPatchCmd{
		Email:            "user@example.com",
		PrimaryKeyPairID: "kp-new",
	}
	out := captureStdout(t, func() {
		if runErr := runKong(t, cmd, []string{"user@example.com", "--primary-keypair-id", "kp-new"}, ctx, flags); runErr != nil {
			t.Fatalf("Run failed: %v", runErr)
		}
	})

	want := "ACTION\tEMAIL\tPRIMARY_KEY_PAIR_ID\tSTATUS\npatch\tuser@example.com\tkp-new\tok\n"
	if out != want {
		t.Fatalf("plain identity patch output = %q, want %q", out, want)
	}
}

func TestGmailCseIdentitiesDeleteCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseIdentitiesDeleteCmd{Email: "user@example.com"}
	out := captureStdout(t, func() {
		if runErr := cmd.Run(ctx, flags); runErr != nil {
			t.Fatalf("Run failed: %v", runErr)
		}
	})

	// Delete has no server primary key pair id after success.
	want := "ACTION\tEMAIL\tPRIMARY_KEY_PAIR_ID\tSTATUS\ndelete\tuser@example.com\t\tok\n"
	if out != want {
		t.Fatalf("plain identity delete output = %q, want %q", out, want)
	}
}

func TestGmailCseKeypairsCreateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		resp := &gmail.CseKeyPair{
			KeyPairId:       "kp123",
			EnablementState: "enabled",
			DisableTime:     "",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsCreateCmd{
		Pkcs7: "-----BEGIN PKCS7-----\ntest\n-----END PKCS7-----",
	}
	out := captureStdout(t, func() {
		if runErr := cmd.Run(ctx, flags); runErr != nil {
			t.Fatalf("Run failed: %v", runErr)
		}
	})

	want := "ACTION\tKEY_PAIR_ID\tENABLEMENT_STATE\tDISABLE_TIME\tSTATUS\ncreate\tkp123\tenabled\t\tok\n"
	if out != want {
		t.Fatalf("plain keypair create output = %q, want %q", out, want)
	}
}

func TestGmailCseKeypairsEnableCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/cse/keypairs/kp123:enable") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := &gmail.CseKeyPair{
			KeyPairId:       "kp123",
			EnablementState: "enabled",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsEnableCmd{KeyPairID: "kp123"}
	out := captureStdout(t, func() {
		if runErr := cmd.Run(ctx, flags); runErr != nil {
			t.Fatalf("Run failed: %v", runErr)
		}
	})

	want := "ACTION\tKEY_PAIR_ID\tENABLEMENT_STATE\tDISABLE_TIME\tSTATUS\nenable\tkp123\tenabled\t\tok\n"
	if out != want {
		t.Fatalf("plain keypair enable output = %q, want %q", out, want)
	}
}

func TestGmailCseKeypairsDisableCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/cse/keypairs/kp123:disable") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := &gmail.CseKeyPair{
			KeyPairId:       "kp123",
			EnablementState: "disabled",
			DisableTime:     "2024-01-01T00:00:00Z",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseKeypairsDisableCmd{KeyPairID: "kp123"}
	out := captureStdout(t, func() {
		if runErr := cmd.Run(ctx, flags); runErr != nil {
			t.Fatalf("Run failed: %v", runErr)
		}
	})

	want := "ACTION\tKEY_PAIR_ID\tENABLEMENT_STATE\tDISABLE_TIME\tSTATUS\ndisable\tkp123\tdisabled\t2024-01-01T00:00:00Z\tok\n"
	if out != want {
		t.Fatalf("plain keypair disable output = %q, want %q", out, want)
	}
}

func TestGmailCseKeypairsObliterateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/cse/keypairs/kp123:obliterate") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseKeypairsObliterateCmd{KeyPairID: "kp123"}
	out := captureStdout(t, func() {
		if runErr := cmd.Run(ctx, flags); runErr != nil {
			t.Fatalf("Run failed: %v", runErr)
		}
	})

	// Obliterate has no server enablement state / disable time after success.
	want := "ACTION\tKEY_PAIR_ID\tENABLEMENT_STATE\tDISABLE_TIME\tSTATUS\nobliterate\tkp123\t\t\tok\n"
	if out != want {
		t.Fatalf("plain keypair obliterate output = %q, want %q", out, want)
	}
}

func TestGmailCseIdentitiesCreateCmd_HumanUnchanged(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &gmail.CseIdentity{
			EmailAddress:     "user@example.com",
			PrimaryKeyPairId: "kp123",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	var stdout strings.Builder
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesCreateCmd{
		Email:            "user@example.com",
		PrimaryKeyPairID: "kp123",
	}
	if runErr := cmd.Run(ctx, flags); runErr != nil {
		t.Fatalf("Run failed: %v", runErr)
	}

	out := stdout.String()
	if !strings.Contains(out, "CSE identity created successfully") {
		t.Fatalf("human create output missing success prose: %q", out)
	}
	if strings.Contains(out, "ACTION\tEMAIL") {
		t.Fatalf("human create output unexpectedly used plain receipt: %q", out)
	}
}
