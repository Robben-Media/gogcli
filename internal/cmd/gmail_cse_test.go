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

// Test CSE Identities List

func TestGmailCseIdentitiesListCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/cse/identities") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := &gmail.ListCseIdentitiesResponse{
			CseIdentities: []*gmail.CseIdentity{
				{EmailAddress: "user@example.com", PrimaryKeyPairId: "kp123"},
				{EmailAddress: "alias@example.com", PrimaryKeyPairId: "kp456"},
			},
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesListCmd{}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseIdentitiesListCmd_Empty(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &gmail.ListCseIdentitiesResponse{CseIdentities: []*gmail.CseIdentity{}}
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
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesListCmd{}
	// Should not error on empty list
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

// Test CSE Identities Get

func TestGmailCseIdentitiesGetCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/cse/identities/user@example.com") {
			t.Errorf("unexpected path: %s", r.URL.Path)
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesGetCmd{Email: "user@example.com"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseIdentitiesGetCmd_EmptyEmail(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesGetCmd{Email: ""}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

// Test CSE Identities Create

func TestGmailCseIdentitiesCreateCmd_JSON(t *testing.T) {
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesCreateCmd{
		Email:            "user@example.com",
		PrimaryKeyPairID: "kp123",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseIdentitiesCreateCmd_SignAndEncrypt_MissingBoth(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesCreateCmd{
		Email:            "user@example.com",
		SigningKeyPairID: "sign123", // Missing EncryptionKeyPairID
	}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing encryption key pair ID")
	}
}

func TestGmailCseIdentitiesCreateCmd_EmptyEmail(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseIdentitiesCreateCmd{Email: ""}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

// Test CSE Identities Delete

func TestGmailCseIdentitiesDeleteCmd_Force(t *testing.T) {
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseIdentitiesDeleteCmd{Email: "user@example.com"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseIdentitiesDeleteCmd_EmptyEmail(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseIdentitiesDeleteCmd{Email: ""}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

// Test CSE Key Pairs List

func TestGmailCseKeypairsListCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/cse/keypairs") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := &gmail.ListCseKeyPairsResponse{
			CseKeyPairs: []*gmail.CseKeyPair{
				{KeyPairId: "kp123", EnablementState: "enabled", SubjectEmailAddresses: []string{"user@example.com"}},
				{KeyPairId: "kp456", EnablementState: "disabled", SubjectEmailAddresses: []string{"alias@example.com"}},
			},
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsListCmd{}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

// Test CSE Key Pairs Get

func TestGmailCseKeypairsGetCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/cse/keypairs/kp123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := &gmail.CseKeyPair{
			KeyPairId:             "kp123",
			EnablementState:       "enabled",
			SubjectEmailAddresses: []string{"user@example.com"},
			Pem:                   "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsGetCmd{KeyPairID: "kp123"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseKeypairsGetCmd_EmptyKeyPairID(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsGetCmd{KeyPairID: ""}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty keyPairId")
	}
}

// Test CSE Key Pairs Create

func TestGmailCseKeypairsCreateCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		resp := &gmail.CseKeyPair{
			KeyPairId:             "kp123",
			EnablementState:       "enabled",
			SubjectEmailAddresses: []string{"user@example.com"},
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsCreateCmd{
		Pkcs7: "-----BEGIN PKCS7-----\ntest\n-----END PKCS7-----",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseKeypairsCreateCmd_EmptyPkcs7(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsCreateCmd{}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty pkcs7")
	}
}

// Test CSE Key Pairs Enable

func TestGmailCseKeypairsEnableCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsEnableCmd{KeyPairID: "kp123"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseKeypairsEnableCmd_EmptyKeyPairID(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com"}

	cmd := &GmailCseKeypairsEnableCmd{KeyPairID: ""}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty keyPairId")
	}
}

// Test CSE Key Pairs Disable

func TestGmailCseKeypairsDisableCmd_Force(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseKeypairsDisableCmd{KeyPairID: "kp123"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseKeypairsDisableCmd_EmptyKeyPairID(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseKeypairsDisableCmd{KeyPairID: ""}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty keyPairId")
	}
}

// Test CSE Key Pairs Obliterate

func TestGmailCseKeypairsObliterateCmd_Force(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseKeypairsObliterateCmd{KeyPairID: "kp123"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestGmailCseKeypairsObliterateCmd_EmptyKeyPairID(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "test@gmail.com", Force: true}

	cmd := &GmailCseKeypairsObliterateCmd{KeyPairID: ""}
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty keyPairId")
	}
}

// Test CLI Registration

func TestGmailCseCmd_Exists(t *testing.T) {
	_ = GmailCseCmd{}
	_ = GmailCseIdentitiesCmd{}
	_ = GmailCseKeypairsCmd{}
}

func TestGmailCseIdentitiesCmd_Exists(t *testing.T) {
	_ = GmailCseIdentitiesListCmd{}
	_ = GmailCseIdentitiesGetCmd{}
	_ = GmailCseIdentitiesCreateCmd{}
	_ = GmailCseIdentitiesDeleteCmd{}
	_ = GmailCseIdentitiesPatchCmd{}
}

func TestGmailCseKeypairsCmd_Exists(t *testing.T) {
	_ = GmailCseKeypairsListCmd{}
	_ = GmailCseKeypairsGetCmd{}
	_ = GmailCseKeypairsCreateCmd{}
	_ = GmailCseKeypairsEnableCmd{}
	_ = GmailCseKeypairsDisableCmd{}
	_ = GmailCseKeypairsObliterateCmd{}
}
