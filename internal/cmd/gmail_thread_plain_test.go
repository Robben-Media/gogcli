package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const gmailThreadPlainHeader = "RECORD_TYPE\tTHREAD_ID\tMESSAGE_ID\tNAME\tVALUE\tPATH\tBYTES\tCACHED"

func setupGmailThreadPlainService(t *testing.T, srv *httptest.Server) {
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

func TestGmailThreadGet_PlainTSV_EmptyThreadHeaderOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/threads/empty") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "empty",
				"messages": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	setupGmailThreadPlainService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "gmail", "thread", "get", "empty"}); err != nil {
				t.Fatalf("Execute empty thread plain: %v", err)
			}
		})
	})
	if got, want := out, gmailThreadPlainHeader+"\n"; got != want {
		t.Fatalf("empty plain output = %q, want %q", got, want)
	}
}

func TestGmailThreadGet_PlainTSV_RecordsAndSanitization(t *testing.T) {
	bodyText := "line1\nline2\twith\ttabs\rand more"
	unsafeFrom := "Alice\tSmith <alice\n@example.com>"
	unsafeSubject := "Hello\r\nWorld"
	longBodyRunes := make([]rune, 520)
	for i := range longBodyRunes {
		longBodyRunes[i] = 'あ'
	}
	longBody := string(longBodyRunes)
	attachmentData := []byte("payload")
	attachmentB64 := base64.RawURLEncoding.EncodeToString(attachmentData)

	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id": "m1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": unsafeFrom},
						{"name": "To", "value": "bob@example.com"},
						{"name": "Subject", "value": unsafeSubject},
						{"name": "Date", "value": "Mon, 1 Jan 2025 00:00:00 +0000"},
					},
					"mimeType": "multipart/mixed",
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte(bodyText)),
							},
						},
						{
							"filename": "note\tfile.txt",
							"mimeType": "text/plain",
							"body": map[string]any{
								"attachmentId": "att1",
								"size":         int64(len(attachmentData)),
							},
						},
					},
				},
			},
			{
				"id": "m2",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "carol@example.com"},
						{"name": "To", "value": "dave@example.com"},
						{"name": "Subject", "value": "Reply"},
						{"name": "Date", "value": "Tue, 2 Jan 2025 00:00:00 +0000"},
					},
					"mimeType": "text/plain",
					"body": map[string]any{
						"data": base64.RawURLEncoding.EncodeToString([]byte(longBody)),
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/threads/t1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(threadResp)
			return
		case r.Method == http.MethodGet && path == "/users/me/messages/m1/attachments/att1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentB64})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()
	setupGmailThreadPlainService(t, srv)

	outDir := t.TempDir()
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "thread", "get", "t1",
				"--download", "--out-dir", outDir,
			}); err != nil {
				t.Fatalf("Execute thread get plain: %v", err)
			}
		})
	})

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) == 0 || lines[0] != gmailThreadPlainHeader {
		t.Fatalf("missing exact header row; first line = %q\nout=%q", firstLine(out), out)
	}
	for i, line := range lines {
		if strings.Count(line, "\t") != 7 {
			t.Fatalf("line %d has %d tabs (want 7): %q", i, strings.Count(line, "\t"), line)
		}
		if strings.ContainsAny(line, "\r") {
			t.Fatalf("line %d contains CR: %q", i, line)
		}
		// Free-form fields must not inject physical rows.
		if i > 0 && strings.Contains(line, "\n") {
			t.Fatalf("line %d is multi-line: %q", i, line)
		}
	}

	byType := map[string][][]string{}
	for _, line := range lines[1:] {
		cols := strings.Split(line, "\t")
		if len(cols) != 8 {
			t.Fatalf("expected 8 columns, got %d: %q", len(cols), line)
		}
		if cols[1] != "t1" {
			t.Fatalf("expected THREAD_ID t1 on every row, got %q in %q", cols[1], line)
		}
		byType[cols[0]] = append(byType[cols[0]], cols)
	}

	if len(byType["metadata"]) != 1 {
		t.Fatalf("expected 1 metadata row, got %#v", byType["metadata"])
	}
	meta := byType["metadata"][0]
	if meta[2] != "" || meta[3] != "message_count" || meta[4] != "2" {
		t.Fatalf("unexpected metadata row: %#v", meta)
	}

	headers := byType["header"]
	if len(headers) != 8 {
		t.Fatalf("expected 8 header rows (4 per message), got %d: %#v", len(headers), headers)
	}
	foundUnsafeFrom := false
	foundUnsafeSubject := false
	for _, h := range headers {
		if h[2] == "" {
			t.Fatalf("header row missing MESSAGE_ID: %#v", h)
		}
		if h[2] == "m1" && h[3] == "From" {
			foundUnsafeFrom = true
			if h[4] != "Alice Smith <alice @example.com>" {
				t.Fatalf("From not sanitized: %q", h[4])
			}
		}
		if h[2] == "m1" && h[3] == "Subject" {
			foundUnsafeSubject = true
			if h[4] != "Hello World" {
				t.Fatalf("Subject not sanitized: %q", h[4])
			}
		}
	}
	if !foundUnsafeFrom || !foundUnsafeSubject {
		t.Fatalf("missing sanitized header rows: %#v", headers)
	}

	bodies := byType["body"]
	if len(bodies) != 2 {
		t.Fatalf("expected 2 body rows, got %#v", bodies)
	}
	var bodyM1, bodyM2 []string
	for _, b := range bodies {
		if b[2] == "" {
			t.Fatalf("body row missing MESSAGE_ID: %#v", b)
		}
		switch b[2] {
		case "m1":
			bodyM1 = b
		case "m2":
			bodyM2 = b
		}
	}
	if bodyM1 == nil || bodyM2 == nil {
		t.Fatalf("missing body rows for m1/m2: %#v", bodies)
	}
	if bodyM1[4] != "line1 line2 with tabs and more" {
		t.Fatalf("body m1 not sanitized: %q", bodyM1[4])
	}
	// Default truncation at 500 runes with marker; full mode is covered separately.
	if utf8.RuneCountInString(bodyM2[4]) <= 500 {
		t.Fatalf("expected truncated body for m2 to include marker, got %q", bodyM2[4])
	}
	if !strings.HasSuffix(bodyM2[4], "... [truncated]") {
		t.Fatalf("expected truncation marker on m2 body, got %q", bodyM2[4])
	}
	truncatedRunes := []rune(bodyM2[4])
	if string(truncatedRunes[:500]) != string(longBodyRunes[:500]) {
		t.Fatalf("truncated body prefix mismatch")
	}

	atts := byType["attachment"]
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment row, got %#v", atts)
	}
	att := atts[0]
	if att[2] != "m1" {
		t.Fatalf("attachment MESSAGE_ID = %q, want m1", att[2])
	}
	if att[3] != "note file.txt" {
		t.Fatalf("attachment NAME not sanitized: %q", att[3])
	}
	if att[4] != "text/plain" {
		t.Fatalf("attachment VALUE(mime) = %q", att[4])
	}
	if att[5] != "att1" {
		t.Fatalf("attachment PATH(attachmentId) = %q", att[5])
	}
	if att[6] != strconv.FormatInt(int64(len(attachmentData)), 10) {
		t.Fatalf("attachment BYTES = %q, want %d", att[6], len(attachmentData))
	}

	downloads := byType["download"]
	if len(downloads) != 1 {
		t.Fatalf("expected 1 download row, got %#v", downloads)
	}
	dl := downloads[0]
	if dl[2] != "m1" {
		t.Fatalf("download MESSAGE_ID = %q, want m1", dl[2])
	}
	if dl[3] != "note file.txt" {
		t.Fatalf("download NAME = %q", dl[3])
	}
	// On-disk path may embed delimiter-bearing filenames; TSV PATH is sanitized.
	expectedPath := filepath.Join(outDir, "m1_att1_note\tfile.txt")
	if dl[5] != sanitizePlainField(expectedPath) {
		t.Fatalf("download PATH = %q, want sanitized %q", dl[5], sanitizePlainField(expectedPath))
	}
	st, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("stat download path: %v", err)
	}
	if dl[6] != strconv.FormatInt(st.Size(), 10) || dl[6] != strconv.Itoa(len(attachmentData)) {
		t.Fatalf("download BYTES = %q, want exact %d", dl[6], len(attachmentData))
	}
	if dl[7] != "false" {
		t.Fatalf("download CACHED = %q, want false", dl[7])
	}

	// Second download hits cache; CACHED must flip true with same exact bytes/path.
	cachedOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "thread", "get", "t1",
				"--download", "--out-dir", outDir,
			}); err != nil {
				t.Fatalf("Execute cached plain: %v", err)
			}
		})
	})
	cachedLines := strings.Split(strings.TrimSuffix(cachedOut, "\n"), "\n")
	var cachedDownload []string
	for _, line := range cachedLines[1:] {
		cols := strings.Split(line, "\t")
		if cols[0] == "download" {
			cachedDownload = cols
		}
	}
	if cachedDownload == nil {
		t.Fatalf("missing download row on cache pass: %q", cachedOut)
	}
	if cachedDownload[5] != dl[5] {
		t.Fatalf("cached path %q != first path %q", cachedDownload[5], dl[5])
	}
	if cachedDownload[6] != dl[6] {
		t.Fatalf("cached BYTES %q != first BYTES %q", cachedDownload[6], dl[6])
	}
	if cachedDownload[7] != "true" {
		t.Fatalf("cached CACHED = %q, want true", cachedDownload[7])
	}
}

func TestGmailThreadGet_PlainTSV_FullBodyAndJSONUnchanged(t *testing.T) {
	longBodyRunes := make([]rune, 520)
	for i := range longBodyRunes {
		longBodyRunes[i] = 'x'
	}
	longBody := string(longBodyRunes)
	threadResp := map[string]any{
		"id": "t-full",
		"messages": []map[string]any{
			{
				"id": "m-full",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "a@example.com"},
						{"name": "To", "value": "b@example.com"},
						{"name": "Subject", "value": "Full"},
						{"name": "Date", "value": "Mon, 1 Jan 2025 00:00:00 +0000"},
					},
					"mimeType": "text/plain",
					"body": map[string]any{
						"data": base64.RawURLEncoding.EncodeToString([]byte(longBody)),
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/me/threads/t-full") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(threadResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	setupGmailThreadPlainService(t, srv)

	fullOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "thread", "get", "t-full", "--full",
			}); err != nil {
				t.Fatalf("Execute full plain: %v", err)
			}
		})
	})
	var fullBody string
	for _, line := range strings.Split(strings.TrimSuffix(fullOut, "\n"), "\n")[1:] {
		cols := strings.Split(line, "\t")
		if cols[0] == "body" {
			fullBody = cols[4]
		}
	}
	if fullBody != longBody {
		t.Fatalf("--full body length=%d want %d (truncated? %v)", len(fullBody), len(longBody), strings.Contains(fullBody, "[truncated]"))
	}

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"gmail", "thread", "get", "t-full",
			}); err != nil {
				t.Fatalf("Execute json: %v", err)
			}
		})
	})
	var payload struct {
		Thread     map[string]any   `json:"thread"`
		Downloaded []map[string]any `json:"downloaded"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("json decode: %v\nout=%q", err, jsonOut)
	}
	if payload.Thread == nil || payload.Thread["id"] != "t-full" {
		t.Fatalf("unexpected json payload: %#v", payload)
	}
	if payload.Downloaded == nil {
		// downloaded key present as empty slice is fine; missing is also ok for Go nil
	}

	humanOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"gmail", "thread", "get", "t-full",
			}); err != nil {
				t.Fatalf("Execute human: %v", err)
			}
		})
	})
	if !strings.Contains(humanOut, "Thread contains 1 message(s)") {
		t.Fatalf("human output regresssed: %q", humanOut)
	}
	if !strings.Contains(humanOut, "... [truncated]") {
		t.Fatalf("human default still truncates; got %q", humanOut)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
