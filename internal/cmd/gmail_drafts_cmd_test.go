package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailDraftsListCmd_TextAndJSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"drafts": []map[string]any{
					{"id": "d1", "message": map[string]any{"id": "m1", "threadId": "t1"}},
					{"id": "d2"},
				},
				"nextPageToken": "next",
			})
			return
		}
		http.NotFound(w, r)
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

	textOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		cmd := &GmailDraftsListCmd{}
		if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(textOut, "ID") || !strings.Contains(textOut, "d1") {
		t.Fatalf("unexpected text: %q", textOut)
	}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailDraftsListCmd{}
		if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Drafts []struct {
			ID        string `json:"id"`
			MessageID string `json:"messageId"`
			ThreadID  string `json:"threadId"`
		} `json:"drafts"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Drafts) != 2 || parsed.Drafts[0].ID != "d1" || parsed.NextPageToken != "next" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsGetCmd_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	payloadText := base64.RawURLEncoding.EncodeToString([]byte("Hello"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
					"payload": map[string]any{
						"mimeType": "multipart/mixed",
						"headers": []map[string]any{
							{"name": "To", "value": "a@example.com"},
							{"name": "Cc", "value": "b@example.com"},
							{"name": "Subject", "value": "Draft"},
						},
						"parts": []map[string]any{
							{"mimeType": "text/plain", "body": map[string]any{"data": payloadText}},
							{
								"filename": "file.txt",
								"mimeType": "text/plain",
								"body":     map[string]any{"attachmentId": "att1", "size": 10},
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
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		cmd := &GmailDraftsGetCmd{}
		if err := runKong(t, cmd, []string{"d1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "Draft-ID:") || !strings.Contains(out, "Subject:") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "Attachments:") || !strings.Contains(out, "file.txt") {
		t.Fatalf("expected attachment output: %q", out)
	}
	if !strings.Contains(out, "attachment\tfile.txt\t10 B\ttext/plain\tatt1") {
		t.Fatalf("expected attachment line output: %q", out)
	}
}

func TestExecute_GmailDraftsGet_Plain(t *testing.T) {
	payloadText := base64.RawURLEncoding.EncodeToString([]byte("Hello\nworld\tline"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
					"payload": map[string]any{
						"mimeType": "multipart/mixed",
						"headers": []map[string]any{
							{"name": "To", "value": "a@example.com"},
							{"name": "Cc", "value": "b@example.com"},
							{"name": "Bcc", "value": "c@example.com"},
							{"name": "Subject", "value": "Draft\tSubject\nNext"},
						},
						"parts": []map[string]any{
							{"mimeType": "text/plain", "body": map[string]any{"data": payloadText}},
							{
								"filename": "file\tname.txt",
								"mimeType": "text/plain",
								"body":     map[string]any{"attachmentId": "att1", "size": 10},
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
	stubGmailService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "drafts", "get", "d1",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	wantHeader := "RECORD_TYPE\tDRAFT_ID\tMESSAGE_ID\tNAME\tVALUE\tPATH\tBYTES\tCACHED\n"
	if !strings.HasPrefix(out, wantHeader) {
		t.Fatalf("missing plain header:\nwant prefix %q\ngot %q", wantHeader, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected records after header, got %q", out)
	}
	for i, line := range lines {
		cols := strings.Split(line, "\t")
		if len(cols) != 8 {
			t.Fatalf("line %d has %d columns (want 8): %q", i, len(cols), line)
		}
		if strings.ContainsAny(cols[3], "\r\n") || strings.ContainsAny(cols[4], "\r\n") {
			t.Fatalf("free-form field not sanitized on line %d: %q", i, line)
		}
		if i == 0 {
			continue
		}
		if cols[1] != "d1" || cols[2] != "m1" {
			t.Fatalf("line %d missing repeated IDs: %q", i, line)
		}
	}

	joined := out
	for _, want := range []string{
		"metadata\td1\tm1\tTo\ta@example.com\t\t\t",
		"metadata\td1\tm1\tCc\tb@example.com\t\t\t",
		"metadata\td1\tm1\tBcc\tc@example.com\t\t\t",
		"metadata\td1\tm1\tSubject\tDraft Subject Next\t\t\t",
		"body\td1\tm1\t\tHello world line\t\t\t",
		"attachment\td1\tm1\tfile name.txt\ttext/plain\t\t10\t",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing record %q in output:\n%s", want, joined)
		}
	}
	if strings.Contains(out, "Draft-ID:") || strings.Contains(out, "Attachments:") || strings.Contains(out, "Empty draft") {
		t.Fatalf("plain mode leaked prose: %q", out)
	}
}

func TestExecute_GmailDraftsGet_Plain_EmptyDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	stubGmailService(t, srv)

	stdout := captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "drafts", "get", "d1",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if strings.Contains(stderr, "Empty draft") {
			t.Fatalf("plain empty draft should not print narrative stderr: %q", stderr)
		}
	})

	want := "RECORD_TYPE\tDRAFT_ID\tMESSAGE_ID\tNAME\tVALUE\tPATH\tBYTES\tCACHED\n"
	if stdout != want {
		t.Fatalf("plain empty draft mismatch:\nwant %q\ngot  %q", want, stdout)
	}
}

func TestExecute_GmailDraftsGet_Plain_Download(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	attData := []byte("hello-bytes")
	attEncoded := base64.RawURLEncoding.EncodeToString(attData)
	payloadText := base64.RawURLEncoding.EncodeToString([]byte("Body"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m-draft-1",
					"payload": map[string]any{
						"mimeType": "multipart/mixed",
						"headers": []map[string]any{
							{"name": "To", "value": "x@y.com"},
							{"name": "Subject", "value": "S"},
						},
						"parts": []map[string]any{
							{"mimeType": "text/plain", "body": map[string]any{"data": payloadText}},
							{
								"filename": "a.txt",
								"mimeType": "text/plain",
								"body":     map[string]any{"attachmentId": "a-draft-1", "size": len(attData)},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m-draft-1/attachments/a-draft-1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attEncoded})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()
	stubGmailService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "drafts", "get", "d1", "--download",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	wantHeader := "RECORD_TYPE\tDRAFT_ID\tMESSAGE_ID\tNAME\tVALUE\tPATH\tBYTES\tCACHED\n"
	if !strings.HasPrefix(out, wantHeader) {
		t.Fatalf("missing plain header:\nwant prefix %q\ngot %q", wantHeader, out)
	}
	if !strings.Contains(out, "attachment\td1\tm-draft-1\ta.txt\ttext/plain\t\t11\t") {
		t.Fatalf("missing attachment record: %q", out)
	}

	var downloadLine string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if strings.HasPrefix(line, "download\t") {
			downloadLine = line
			break
		}
	}
	if downloadLine == "" {
		t.Fatalf("missing download record: %q", out)
	}
	cols := strings.Split(downloadLine, "\t")
	if len(cols) != 8 {
		t.Fatalf("download columns=%d line=%q", len(cols), downloadLine)
	}
	if cols[0] != "download" || cols[1] != "d1" || cols[2] != "m-draft-1" {
		t.Fatalf("download identity mismatch: %q", downloadLine)
	}
	if cols[3] != "a.txt" {
		t.Fatalf("download name mismatch: %q", downloadLine)
	}
	if cols[5] == "" {
		t.Fatalf("download path empty: %q", downloadLine)
	}
	if cols[6] != "11" {
		t.Fatalf("download bytes mismatch: %q", downloadLine)
	}
	if cols[7] != "false" && cols[7] != "true" {
		t.Fatalf("download cached not bool: %q", downloadLine)
	}
	if strings.Contains(out, "Saved:") || strings.Contains(out, "Cached:") {
		t.Fatalf("plain download leaked prose: %q", out)
	}

	// Second run should hit cache and report exact path/bytes/cached=true.
	out2 := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "drafts", "get", "d1", "--download",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var cachedLine string
	for _, line := range strings.Split(strings.TrimSuffix(out2, "\n"), "\n") {
		if strings.HasPrefix(line, "download\t") {
			cachedLine = line
			break
		}
	}
	if cachedLine == "" {
		t.Fatalf("missing cached download record: %q", out2)
	}
	cachedCols := strings.Split(cachedLine, "\t")
	if len(cachedCols) != 8 || cachedCols[5] != cols[5] || cachedCols[6] != "11" || cachedCols[7] != "true" {
		t.Fatalf("cached download mismatch:\nfirst %q\nsecond %q", downloadLine, cachedLine)
	}
}

func TestGmailDraftsDeleteCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
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

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailDraftsDeleteCmd{}
		if err := runKong(t, cmd, []string{"d1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Deleted bool   `json:"deleted"`
		DraftID string `json:"draftId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if !parsed.Deleted || parsed.DraftID != "d1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsSendCmd_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/send") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
			})
			return
		}
		http.NotFound(w, r)
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
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		cmd := &GmailDraftsSendCmd{}
		if err := runKong(t, cmd, []string{"d1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "message_id\tm1") || !strings.Contains(out, "thread_id\tt1") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestGmailDraftsCreateCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
				},
			})
			return
		}
		http.NotFound(w, r)
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

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsCreateCmd{}, []string{"--to", "a@example.com", "--subject", "S", "--body", "Hello"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		DraftID  string `json:"draftId"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.DraftID != "d1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsCreateCmd_NoTo(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in create")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if strings.Contains(s, "\r\nTo:") {
				t.Fatalf("unexpected To header in raw:\n%s", s)
			}
			if !strings.Contains(s, "Subject: S\r\n") {
				t.Fatalf("missing Subject in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
				},
			})
			return
		}
		http.NotFound(w, r)
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

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsCreateCmd{}, []string{"--subject", "S", "--body", "Hello"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}

func TestGmailDraftsCreateCmd_WithFromAndReply(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachPath := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(attachPath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write attach: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/settings/sendAs/alias@example.com") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"displayName":        "Alias",
				"verificationStatus": "accepted",
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<msg@id>"},
						{"name": "References", "value": "<ref@id>"},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m2",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
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
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
		if err := runKong(t, &GmailDraftsCreateCmd{}, []string{
			"--to", "a@example.com",
			"--subject", "S",
			"--body", "Hello",
			"--from", "alias@example.com",
			"--reply-to-message-id", "m1",
			"--attach", attachPath,
		}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}

func TestGmailDraftsUpdateCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attData := []byte("attachment")
	attachPath := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(attachPath, attData, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m1", "threadId": "t1"},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m1@example.com>"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in update")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "From: a@b.com\r\n") {
				t.Fatalf("missing From in raw:\n%s", s)
			}
			if !strings.Contains(s, "To: a@example.com\r\n") {
				t.Fatalf("missing To in raw:\n%s", s)
			}
			if !strings.Contains(s, "Cc: cc@example.com\r\n") {
				t.Fatalf("missing Cc in raw:\n%s", s)
			}
			if !strings.Contains(s, "Bcc: bcc@example.com\r\n") {
				t.Fatalf("missing Bcc in raw:\n%s", s)
			}
			if !strings.Contains(s, "Subject: Updated\r\n") {
				t.Fatalf("missing Subject in raw:\n%s", s)
			}
			if !strings.Contains(s, "Reply-To: reply@example.com\r\n") {
				t.Fatalf("missing Reply-To in raw:\n%s", s)
			}
			if !strings.Contains(s, "Hello") {
				t.Fatalf("missing body in raw:\n%s", s)
			}
			if !strings.Contains(s, "Content-Disposition: attachment; filename=\"note.txt\"") {
				t.Fatalf("missing attachment header in raw:\n%s", s)
			}
			if !strings.Contains(s, base64.StdEncoding.EncodeToString(attData)) {
				t.Fatalf("missing attachment data in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m2", "threadId": "t1"},
			})
			return
		default:
			http.NotFound(w, r)
			return
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

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
			"d1",
			"--to", "a@example.com",
			"--cc", "cc@example.com",
			"--bcc", "bcc@example.com",
			"--subject", "Updated",
			"--body", "Hello",
			"--reply-to", "reply@example.com",
			"--attach", attachPath,
		}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		DraftID  string `json:"draftId"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.DraftID != "d1" || parsed.ThreadID != "t1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsUpdateCmd_PreservesToWhenNotProvided(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id":       "m1",
					"threadId": "t1",
					"payload": map[string]any{
						"headers": []map[string]any{
							{"name": "To", "value": "keep@example.com"},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m1@example.com>"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in update")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "To: keep@example.com\r\n") {
				t.Fatalf("expected preserved To in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m2", "threadId": "t1"},
			})
			return
		default:
			http.NotFound(w, r)
			return
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

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
			"d1",
			"--subject", "Updated",
			"--body", "Hello",
		}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}
