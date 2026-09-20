package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestWriteWatchState_TextAndJSON(t *testing.T) {
	const sentinelToken = "sentinel-hook-token-issue-88-secret"
	state := gmailWatchState{
		Account:              "a@b.com",
		Topic:                "projects/p/topics/t",
		Labels:               []string{"INBOX", "Label_1"},
		HistoryID:            "123",
		ExpirationMs:         1,
		ProviderExpirationMs: 2,
		RenewAfterMs:         3,
		UpdatedAtMs:          4,
		Hook: &gmailWatchHook{
			URL:         "http://example.com/hook",
			IncludeBody: true,
			MaxBytes:    12,
			Token:       sentinelToken,
		},
		LastDeliveryStatus:     "ok",
		LastDeliveryAtMs:       5,
		LastDeliveryStatusNote: "note",
	}

	textOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		if err := writeWatchState(ctx, state); err != nil {
			t.Fatalf("writeWatchState: %v", err)
		}
	})
	if !strings.Contains(textOut, "account\ta@b.com") {
		t.Fatalf("expected account output")
	}
	if !strings.Contains(textOut, "hook_url\thttp://example.com/hook") {
		t.Fatalf("expected hook output")
	}
	if !strings.Contains(textOut, "hook_token_configured\ttrue") {
		t.Fatalf("expected configured flag in text output, got: %q", textOut)
	}
	if strings.Contains(textOut, sentinelToken) || strings.Contains(textOut, "hook_token\t") {
		t.Fatalf("text status leaked hook token: %q", textOut)
	}

	// Absent token reports configured=false without inventing a secret field.
	noTokenState := state
	noTokenState.Hook = &gmailWatchHook{URL: "http://example.com/hook"}
	textAbsent := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		if err := writeWatchState(ctx, noTokenState); err != nil {
			t.Fatalf("writeWatchState absent token: %v", err)
		}
	})
	if !strings.Contains(textAbsent, "hook_token_configured\tfalse") {
		t.Fatalf("expected configured=false for empty token, got: %q", textAbsent)
	}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := writeWatchState(ctx, state); err != nil {
			t.Fatalf("writeWatchState json: %v", err)
		}
	})
	if strings.Contains(jsonOut, sentinelToken) {
		t.Fatalf("json status leaked hook token: %q", jsonOut)
	}
	var parsed struct {
		Watch struct {
			Hook *struct {
				URL         string `json:"url"`
				Token       string `json:"token"`
				IncludeBody bool   `json:"includeBody"`
				MaxBytes    int    `json:"maxBytes"`
			} `json:"hook"`
			HookTokenConfigured bool `json:"hookTokenConfigured"`
		} `json:"watch"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.Watch.Hook == nil || parsed.Watch.Hook.URL == "" {
		t.Fatalf("expected hook in json")
	}
	if parsed.Watch.Hook.Token != "" {
		t.Fatalf("json hook must omit token value, got: %#v", parsed.Watch.Hook)
	}
	if !parsed.Watch.HookTokenConfigured {
		t.Fatalf("expected hookTokenConfigured true")
	}

	jsonAbsent := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := writeWatchState(ctx, noTokenState); err != nil {
			t.Fatalf("writeWatchState json absent: %v", err)
		}
	})
	var parsedAbsent struct {
		Watch struct {
			HookTokenConfigured bool `json:"hookTokenConfigured"`
		} `json:"watch"`
	}
	if err := json.Unmarshal([]byte(jsonAbsent), &parsedAbsent); err != nil {
		t.Fatalf("json absent parse: %v", err)
	}
	if parsedAbsent.Watch.HookTokenConfigured {
		t.Fatalf("expected hookTokenConfigured false when token absent")
	}

	// No hook at all still reports configured=false in every mode.
	noHook := state
	noHook.Hook = nil
	textNoHook := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		if err := writeWatchState(ctx, noHook); err != nil {
			t.Fatalf("writeWatchState no hook: %v", err)
		}
	})
	if !strings.Contains(textNoHook, "hook_token_configured\tfalse") {
		t.Fatalf("expected configured=false when hook missing, got: %q", textNoHook)
	}
}

func TestHookFromFlags(t *testing.T) {
	t.Run("missing url with token", func(t *testing.T) {
		if _, err := hookFromFlags("", "tok", false, 0, false, false); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing url with hook opts", func(t *testing.T) {
		if _, err := hookFromFlags("", "", true, 0, true, false); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("allow no hook", func(t *testing.T) {
		hook, err := hookFromFlags("", "", false, 0, false, true)
		if err == nil || !errors.Is(err, errNoHookConfigured) {
			t.Fatalf("expected no hook error, got: %v", err)
		}
		if hook != nil {
			t.Fatalf("expected nil hook")
		}
	})

	t.Run("defaults max bytes", func(t *testing.T) {
		hook, err := hookFromFlags("http://example.com", "", true, 0, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hook.MaxBytes != defaultHookMaxBytes {
			t.Fatalf("expected default max bytes")
		}
	})

	t.Run("invalid max bytes", func(t *testing.T) {
		if _, err := hookFromFlags("http://example.com", "", false, 0, true, false); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"localhost":   true,
		"127.0.0.1":   true,
		"[::1]":       true,
		"example.com": false,
	}
	for host, want := range cases {
		if got := isLoopbackHost(host); got != want {
			t.Fatalf("isLoopbackHost(%q)=%v want %v", host, got, want)
		}
	}
}

func TestGmailWatchStore_StateHelpers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := newGmailWatchStore("User+X@Example.COM")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	id, startErr := store.StartHistoryID("101")
	if startErr != nil {
		t.Fatalf("start history: %v", startErr)
	}
	if id != 101 {
		t.Fatalf("expected history id 101, got %d", id)
	}
	if store.state.HistoryID != "101" {
		t.Fatalf("expected history set")
	}
	id, startErr = store.StartHistoryID("")
	if startErr != nil {
		t.Fatalf("start history existing: %v", startErr)
	}
	if id != 101 {
		t.Fatalf("expected history id 101, got %d", id)
	}
	id, startErr = store.StartHistoryID("100")
	if startErr != nil {
		t.Fatalf("start history stale: %v", startErr)
	}
	if id != 0 {
		t.Fatalf("expected stale history ignored, got %d", id)
	}
	if store.state.HistoryID != "101" {
		t.Fatalf("expected history unchanged")
	}
	id, startErr = store.StartHistoryID("bad")
	if startErr != nil {
		t.Fatalf("start history invalid push: %v", startErr)
	}
	if id != 101 {
		t.Fatalf("expected history id 101, got %d", id)
	}

	if _, err := parseHistoryID(""); err == nil {
		t.Fatalf("expected parse error")
	}
	if got := formatHistoryID(0); got != "" {
		t.Fatalf("expected empty format")
	}
}

func TestGmailWatchStore_SaveMissingPath(t *testing.T) {
	store := &gmailWatchStore{}
	if err := store.Save(); err == nil {
		t.Fatalf("expected error")
	}
}
