package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const gmailGetPlainHeader = "RECORD_TYPE\tMESSAGE_ID\tTHREAD_ID\tNAME\tVALUE\n"

func stubGmailGetService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }
}

func TestExecute_GmailGet_Plain_Full_FramedTSV(t *testing.T) {
	bodyText := "line1\nline2\twith\ttabs\r\nand more"
	bodyData := base64.RawURLEncoding.EncodeToString([]byte(bodyText))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != gmailFormatFull {
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX", "UNREAD"},
			"payload": map[string]any{
				"mimeType": "multipart/mixed",
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Cc", "value": "c@example.com"},
					{"name": "Bcc", "value": "d@example.com"},
					{"name": "Subject", "value": "Hello\tWorld\nX"},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
					{"name": "List-Unsubscribe", "value": "<mailto:unsub@example.com>"},
				},
				"parts": []map[string]any{
					{
						"mimeType": "text/plain",
						"body":     map[string]any{"data": bodyData},
					},
					{
						"mimeType": "application/pdf",
						"filename": "report\t1.pdf",
						"body": map[string]any{
							"attachmentId": "att\t1",
							"size":         54321,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	stubGmailGetService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain",
				"--account", "a@b.com",
				"gmail", "get", "m1",
				"--format", gmailFormatFull,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	want := gmailGetPlainHeader +
		"metadata\tm1\tt1\tid\tm1\n" +
		"metadata\tm1\tt1\tthread_id\tt1\n" +
		"metadata\tm1\tt1\tlabel_ids\tINBOX,UNREAD\n" +
		"header\tm1\tt1\tfrom\ta@example.com\n" +
		"header\tm1\tt1\tto\tb@example.com\n" +
		"header\tm1\tt1\tcc\tc@example.com\n" +
		"header\tm1\tt1\tbcc\td@example.com\n" +
		"header\tm1\tt1\tsubject\tHello World X\n" +
		"header\tm1\tt1\tdate\tFri, 26 Dec 2025 10:00:00 +0000\n" +
		"header\tm1\tt1\tunsubscribe\tmailto:unsub@example.com\n" +
		"attachment\tm1\tt1\tfilename\treport 1.pdf\n" +
		"attachment\tm1\tt1\tsize_human\t" + formatBytes(54321) + "\n" +
		"attachment\tm1\tt1\tmime_type\tapplication/pdf\n" +
		"attachment\tm1\tt1\tattachment_id\tatt 1\n" +
		"body\tm1\tt1\t\tline1 line2 with tabs and more\n"
	if out != want {
		t.Fatalf("plain full output mismatch:\nwant %q\ngot  %q", want, out)
	}

	// Body and free-form values must not inject physical rows/columns.
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	for i, line := range lines {
		if n := strings.Count(line, "\t"); n != 4 {
			t.Fatalf("line %d has %d tabs (want 4): %q", i, n, line)
		}
	}
}

func TestExecute_GmailGet_Plain_Metadata_NoBody(t *testing.T) {
	bodyData := base64.RawURLEncoding.EncodeToString([]byte("should not appear"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != "metadata" {
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"mimeType": "multipart/mixed",
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Subject", "value": "Meta"},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
				},
				"parts": []map[string]any{
					{"mimeType": "text/plain", "body": map[string]any{"data": bodyData}},
					{
						"mimeType": "application/pdf",
						"filename": "meta.pdf",
						"body": map[string]any{
							"attachmentId": "meta-att",
							"size":         4096,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	stubGmailGetService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain",
				"--account", "a@b.com",
				"gmail", "get", "m1",
				"--format", "metadata",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.HasPrefix(out, gmailGetPlainHeader) {
		t.Fatalf("missing plain header: %q", out)
	}
	if strings.Contains(out, "should not appear") {
		t.Fatalf("metadata plain output leaked body: %q", out)
	}
	if strings.Contains(out, "\tbody\t") || strings.HasPrefix(out, "body\t") || strings.Contains(out, "\nbody\t") {
		t.Fatalf("metadata plain output should not include body records: %q", out)
	}
	if !strings.Contains(out, "attachment\tm1\tt1\tfilename\tmeta.pdf\n") {
		t.Fatalf("expected attachment record: %q", out)
	}
	if !strings.Contains(out, "metadata\tm1\tt1\tid\tm1\n") {
		t.Fatalf("expected metadata id record: %q", out)
	}
	for i, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if n := strings.Count(line, "\t"); n != 4 {
			t.Fatalf("line %d has %d tabs (want 4): %q", i, n, line)
		}
	}
}

func TestExecute_GmailGet_Plain_Raw_SanitizesContent(t *testing.T) {
	raw := "Subject: hi\r\nX-Tab: a\tb\n\nbody\tline1\nline2"
	rawEncoded := base64.RawURLEncoding.EncodeToString([]byte(raw))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("format"); got != "raw" {
			t.Errorf("format=%q", got)
			http.Error(w, "bad format", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"raw":      rawEncoded,
		})
	}))
	defer srv.Close()
	stubGmailGetService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain",
				"--account", "a@b.com",
				"gmail", "get", "m1",
				"--format", "raw",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	// CRLF collapses to a single space via the shared plain sanitizer.
	wantValue := "Subject: hi X-Tab: a b  body line1 line2"
	want := gmailGetPlainHeader +
		"metadata\tm1\tt1\tid\tm1\n" +
		"metadata\tm1\tt1\tthread_id\tt1\n" +
		"metadata\tm1\tt1\tlabel_ids\tINBOX\n" +
		"raw\tm1\tt1\t\t" + wantValue + "\n"
	if out != want {
		t.Fatalf("plain raw output mismatch:\nwant %q\ngot  %q", want, out)
	}
}

func TestExecute_GmailGet_Plain_JSONUnchanged(t *testing.T) {
	// Guard: --plain path must not affect JSON payload shape.
	bodyData := base64.RawURLEncoding.EncodeToString([]byte("hello"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"labelIds": []string{"INBOX"},
			"payload": map[string]any{
				"mimeType": "text/plain",
				"body":     map[string]any{"data": bodyData},
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "Subject", "value": "S"},
				},
			},
		})
	}))
	defer srv.Close()
	stubGmailGetService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"gmail", "get", "m1",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed["body"] != "hello" {
		t.Fatalf("unexpected body: %v", parsed["body"])
	}
	if _, ok := parsed["message"]; !ok {
		t.Fatalf("expected message key in JSON")
	}
}
