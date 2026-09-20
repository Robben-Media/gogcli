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
	"strconv"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Plain schemas for thread modify / attachments (issue #78):
// modify: THREAD_ID\tADDED_LABELS\tREMOVED_LABELS (+ one row, resolved IDs)
// attachments: THREAD_ID\tMESSAGE_ID\tATTACHMENT_ID\tFILENAME\tMIME_TYPE\tBYTES\tPATH\tCACHED

const (
	gmailThreadModifyPlainHeader      = "THREAD_ID\tADDED_LABELS\tREMOVED_LABELS"
	gmailThreadAttachmentsPlainHeader = "THREAD_ID\tMESSAGE_ID\tATTACHMENT_ID\tFILENAME\tMIME_TYPE\tBYTES\tPATH\tCACHED"
)

func TestGmailThreadModifyCmd_PlainReceipt(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels")):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "Label_1", "name": "Custom", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/threads/") && strings.HasSuffix(r.URL.Path, "/modify"):
			var body struct {
				AddLabelIds    []string `json:"addLabelIds"`
				RemoveLabelIds []string `json:"removeLabelIds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.AddLabelIds) != 1 || body.AddLabelIds[0] != "INBOX" {
				http.Error(w, "bad addLabelIds", http.StatusBadRequest)
				return
			}
			if len(body.RemoveLabelIds) != 1 || body.RemoveLabelIds[0] != "Label_1" {
				http.Error(w, "bad removeLabelIds", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
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
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
		if runErr := runKong(t, &GmailThreadModifyCmd{}, []string{
			"t1",
			"--add", "INBOX",
			"--remove", "Custom",
		}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	want := gmailThreadModifyPlainHeader + "\nt1\tINBOX\tLabel_1\n"
	if out != want {
		t.Fatalf("plain modify = %q, want %q", out, want)
	}
	if strings.Contains(strings.ToLower(out), "modified") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
}

func TestGmailThreadModifyCmd_JSONUnchanged(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "Label_1", "name": "Custom", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/modify"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
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
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if runErr := runKong(t, &GmailThreadModifyCmd{}, []string{
			"t1", "--add", "INBOX", "--remove", "Custom",
		}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	var parsed struct {
		Modified      string   `json:"modified"`
		AddedLabels   []string `json:"addedLabels"`
		RemovedLabels []string `json:"removedLabels"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Modified != "t1" || len(parsed.AddedLabels) != 1 || parsed.AddedLabels[0] != "INBOX" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
	if len(parsed.RemovedLabels) != 1 || parsed.RemovedLabels[0] != "Label_1" {
		t.Fatalf("unexpected removed labels: %#v", parsed.RemovedLabels)
	}
}

func TestGmailThreadAttachmentsCmd_PlainList(t *testing.T) {
	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id": "m1",
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"parts": []map[string]any{
						{
							"filename": "note\tfile.txt",
							"mimeType": "text/plain",
							"body": map[string]any{
								"attachmentId": "att1",
								"size":         7,
							},
						},
						{
							"filename": "second.bin",
							"mimeType": "application/octet-stream",
							"body": map[string]any{
								"attachmentId": "att2",
								"size":         12,
							},
						},
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
		case r.Method == http.MethodGet && strings.Contains(path, "/attachments/"):
			// list path must not download
			t.Errorf("list should not download attachments: %s", path)
			http.NotFound(w, r)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()
	setupGmailThreadPlainService(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "gmail", "thread", "attachments", "t1"}); err != nil {
				t.Fatalf("Execute attachments list plain: %v", err)
			}
		})
	})

	want := strings.Join([]string{
		gmailThreadAttachmentsPlainHeader,
		"t1\tm1\tatt1\tnote file.txt\ttext/plain\t7\t\t",
		"t1\tm1\tatt2\tsecond.bin\tapplication/octet-stream\t12\t\t",
		"",
	}, "\n")
	if out != want {
		t.Fatalf("plain attachments list = %q, want %q", out, want)
	}
	if strings.Contains(out, "Found") || strings.Contains(out, "attachment\t") {
		t.Fatalf("plain stdout leaked prose/heading: %q", out)
	}
}

func TestGmailThreadAttachmentsCmd_PlainEmptyHeaderOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/threads/empty":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "empty", "messages": []map[string]any{}})
			return
		case r.Method == http.MethodGet && path == "/users/me/threads/noatts":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "noatts",
				"messages": []map[string]any{
					{
						"id": "m2",
						"payload": map[string]any{
							"mimeType": "text/plain",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte("hello")),
							},
						},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()
	setupGmailThreadPlainService(t, srv)

	for _, threadID := range []string{"empty", "noatts"} {
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := Execute([]string{"--plain", "--account", "a@b.com", "gmail", "thread", "attachments", threadID}); err != nil {
					t.Fatalf("Execute attachments %s plain: %v", threadID, err)
				}
			})
		})
		if got, want := out, gmailThreadAttachmentsPlainHeader+"\n"; got != want {
			t.Fatalf("empty attachments plain (%s) = %q, want %q", threadID, got, want)
		}
		if strings.Contains(out, "No attachments") || strings.Contains(out, "Empty thread") {
			t.Fatalf("plain stdout leaked prose for %s: %q", threadID, out)
		}
	}
}

func TestGmailThreadAttachmentsCmd_PlainDownload(t *testing.T) {
	attachmentData := []byte("payload")
	attachmentB64 := base64.RawURLEncoding.EncodeToString(attachmentData)
	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id": "m1",
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"parts": []map[string]any{
						{
							"filename": "note.txt",
							"mimeType": "text/plain",
							"body": map[string]any{
								"attachmentId": "att1",
								"size":         int64(len(attachmentData)),
							},
						},
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
				"gmail", "thread", "attachments", "t1",
				"--download", "--out-dir", outDir,
			}); err != nil {
				t.Fatalf("Execute attachments download plain: %v", err)
			}
		})
	})

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 row, got %d lines: %q", len(lines), out)
	}
	if lines[0] != gmailThreadAttachmentsPlainHeader {
		t.Fatalf("header = %q, want %q", lines[0], gmailThreadAttachmentsPlainHeader)
	}

	fields := strings.Split(lines[1], "\t")
	if len(fields) != 8 {
		t.Fatalf("expected 8 fields, got %d: %q", len(fields), lines[1])
	}
	if fields[0] != "t1" || fields[1] != "m1" || fields[2] != "att1" || fields[3] != "note.txt" || fields[4] != "text/plain" {
		t.Fatalf("unexpected identity fields: %#v", fields)
	}
	wantBytes := strconv.Itoa(len(attachmentData))
	if fields[5] != wantBytes {
		t.Fatalf("BYTES = %q, want exact %q", fields[5], wantBytes)
	}
	if fields[6] == "" {
		t.Fatalf("PATH should be set on download")
	}
	if !strings.HasPrefix(fields[6], filepath.Clean(outDir)+string(os.PathSeparator)) {
		t.Fatalf("PATH %q not under out dir %q", fields[6], outDir)
	}
	if fields[7] != "false" {
		t.Fatalf("CACHED = %q, want false", fields[7])
	}
	if strings.Contains(out, "Saved") || strings.Contains(out, "Cached") || strings.Contains(out, "Found") {
		t.Fatalf("plain download leaked prose: %q", out)
	}

	// Second download hits cache; CACHED must be true, bytes exact.
	cachedOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"gmail", "thread", "attachments", "t1",
				"--download", "--out-dir", outDir,
			}); err != nil {
				t.Fatalf("Execute attachments cached plain: %v", err)
			}
		})
	})
	cachedLines := strings.Split(strings.TrimSuffix(cachedOut, "\n"), "\n")
	if len(cachedLines) != 2 {
		t.Fatalf("cached expected header + 1 row, got %q", cachedOut)
	}
	cachedFields := strings.Split(cachedLines[1], "\t")
	if len(cachedFields) != 8 {
		t.Fatalf("cached expected 8 fields: %q", cachedLines[1])
	}
	if cachedFields[5] != wantBytes {
		t.Fatalf("cached BYTES = %q, want %q", cachedFields[5], wantBytes)
	}
	if cachedFields[7] != "true" {
		t.Fatalf("cached CACHED = %q, want true", cachedFields[7])
	}
}
