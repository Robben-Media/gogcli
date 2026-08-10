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

// Public CLI seam: Gmail settings update commands under --plain emit a fixed
// multi-row TSV receipt (SETTING/FIELD/VALUE) with no prose on stdout.

func TestGmailImapUpdateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/imap") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":         false,
				"autoExpunge":     false,
				"expungeBehavior": "archive",
				"maxFolderSize":   1000,
			})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":         true,
				"autoExpunge":     true,
				"expungeBehavior": "trash",
				"maxFolderSize":   2000,
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailImapUpdateCmd{}, []string{
			"--enable",
			"--auto-expunge",
			"--expunge-behavior", "trash",
			"--max-folder-size", "2000",
		}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"imap\tenabled\ttrue\n" +
		"imap\tauto_expunge\ttrue\n" +
		"imap\texpunge_behavior\ttrash\n" +
		"imap\tmax_folder_size\t2000\n"
	if out != want {
		t.Fatalf("plain imap update output = %q, want %q", out, want)
	}
	if strings.Contains(strings.ToLower(out), "updated successfully") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
}

func TestGmailImapUpdateCmd_PlainReceiptOmitsEmptyOptionalFields(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/imap") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":     true,
				"autoExpunge": true,
			})
		case http.MethodPut:
			// Optional fields absent/zero in server response.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":     false,
				"autoExpunge": false,
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailImapUpdateCmd{}, []string{"--disable", "--no-auto-expunge"}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"imap\tenabled\tfalse\n" +
		"imap\tauto_expunge\tfalse\n"
	if out != want {
		t.Fatalf("plain imap update optional fields = %q, want %q", out, want)
	}
}

func TestGmailPopUpdateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/pop") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessWindow": "disabled",
				"disposition":  "leaveInInbox",
			})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessWindow": "allMail",
				"disposition":  "archive",
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailPopUpdateCmd{}, []string{
			"--access-window", "allMail",
			"--disposition", "archive",
		}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"pop\taccess_window\tallMail\n" +
		"pop\tdisposition\tarchive\n"
	if out != want {
		t.Fatalf("plain pop update output = %q, want %q", out, want)
	}
	if strings.Contains(strings.ToLower(out), "updated successfully") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
}

func TestGmailLanguageUpdateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/language") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"displayLanguage": "ja",
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailLanguageUpdateCmd{}, []string{"--display-language", "ja"}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"language\tdisplay_language\tja\n"
	if out != want {
		t.Fatalf("plain language update output = %q, want %q", out, want)
	}
	if strings.Contains(strings.ToLower(out), "updated successfully") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
}

func TestGmailAutoForwardUpdateCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/autoForwarding") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":      false,
				"emailAddress": "old@example.com",
				"disposition":  "leaveInInbox",
			})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":      true,
				"emailAddress": "new@example.com",
				"disposition":  "archive",
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailAutoForwardUpdateCmd{}, []string{
			"--enable",
			"--email", "new@example.com",
			"--disposition", "archive",
		}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"auto_forwarding\tenabled\ttrue\n" +
		"auto_forwarding\temail_address\tnew@example.com\n" +
		"auto_forwarding\tdisposition\tarchive\n"
	if out != want {
		t.Fatalf("plain auto-forward update output = %q, want %q", out, want)
	}
	if strings.Contains(strings.ToLower(out), "updated successfully") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
}

func TestGmailAutoForwardUpdateCmd_PlainReceiptOmitsEmptyOptionalFields(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/autoForwarding") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled": true,
			})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled": false,
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailAutoForwardUpdateCmd{}, []string{"--disable"}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"auto_forwarding\tenabled\tfalse\n"
	if out != want {
		t.Fatalf("plain auto-forward optional fields = %q, want %q", out, want)
	}
}

func TestGmailVacationUpdateCmd_PlainReceiptSanitizesFreeForm(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/vacation") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enableAutoReply": false,
			})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enableAutoReply":       true,
				"responseSubject":       "Out\tof\toffice\nnow",
				"responseBodyHtml":      "<p>Back\r\nsoon</p>",
				"responseBodyPlainText": "Back\rsoon",
				"startTime":             "1704067200000",
				"endTime":               "1704153600000",
				"restrictToContacts":    true,
				"restrictToDomain":      false,
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailVacationUpdateCmd{}, []string{
			"--enable",
			"--subject", "ignored",
			"--body", "<b>ignored</b>",
			"--start", "2024-01-01T00:00:00Z",
			"--end", "2024-01-02T00:00:00Z",
			"--contacts-only",
		}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"vacation\tenable_auto_reply\ttrue\n" +
		"vacation\tresponse_subject\tOut of office now\n" +
		"vacation\tresponse_body_html\t<p>Back soon</p>\n" +
		"vacation\tresponse_body_plain_text\tBack soon\n" +
		"vacation\tstart_time\t1704067200000\n" +
		"vacation\tend_time\t1704153600000\n" +
		"vacation\trestrict_to_contacts\ttrue\n" +
		"vacation\trestrict_to_domain\tfalse\n"
	if out != want {
		t.Fatalf("plain vacation update output = %q, want %q", out, want)
	}
	if strings.Contains(strings.ToLower(out), "updated successfully") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
	if strings.Contains(out, "\tOut\tof") || strings.Contains(out, "Back\nsoon") {
		t.Fatalf("free-form fields must be sanitized for TSV, got %q", out)
	}
}

func TestGmailVacationUpdateCmd_PlainReceiptOmitsZeroTimes(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/vacation") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enableAutoReply": true,
			})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enableAutoReply":       false,
				"responseSubject":       "",
				"responseBodyHtml":      "",
				"responseBodyPlainText": "",
				"restrictToContacts":    false,
				"restrictToDomain":      false,
			})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailVacationUpdateCmd{}, []string{"--disable"}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	// Empty free-form fields are still emitted (server response fields);
	// optional start/end times are omitted when zero.
	want := "" +
		"SETTING\tFIELD\tVALUE\n" +
		"vacation\tenable_auto_reply\tfalse\n" +
		"vacation\tresponse_subject\t\n" +
		"vacation\tresponse_body_html\t\n" +
		"vacation\tresponse_body_plain_text\t\n" +
		"vacation\trestrict_to_contacts\tfalse\n" +
		"vacation\trestrict_to_domain\tfalse\n"
	if out != want {
		t.Fatalf("plain vacation optional times = %q, want %q", out, want)
	}
}

func TestGmailImapUpdateCmd_JSONUnchanged(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/settings/imap") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": false, "autoExpunge": true})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true, "autoExpunge": true, "expungeBehavior": "archive", "maxFolderSize": 1000})
		default:
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if runErr := runKong(t, &GmailImapUpdateCmd{}, []string{"--enable"}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	var parsed struct {
		Imap struct {
			Enabled         bool   `json:"enabled"`
			AutoExpunge     bool   `json:"autoExpunge"`
			ExpungeBehavior string `json:"expungeBehavior"`
			MaxFolderSize   int64  `json:"maxFolderSize"`
		} `json:"imap"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Imap.Enabled || !parsed.Imap.AutoExpunge || parsed.Imap.ExpungeBehavior != "archive" || parsed.Imap.MaxFolderSize != 1000 {
		t.Fatalf("unexpected json payload: %#v", parsed.Imap)
	}
	if strings.Contains(out, "SETTING") {
		t.Fatalf("json mode must not emit TSV receipt, got %q", out)
	}
}
