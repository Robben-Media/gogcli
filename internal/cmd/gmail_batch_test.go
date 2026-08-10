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

func TestGmailBatchDeleteCmd_Plain(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/messages/batchDelete") && r.Method == http.MethodPost {
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
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})

		if runErr := runKong(t, &GmailBatchDeleteCmd{}, []string{"msg1", "msg2", "msg3"}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	const want = "ACTION\tCOUNT\tADDED_LABELS\tREMOVED_LABELS\ndelete\t3\t\t\n"
	if out != want {
		t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", want, out)
	}
	if strings.Contains(strings.ToLower(out), "deleted") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
}

func TestGmailBatchModifyCmd_Plain(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Work", "type": "user"},
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "SPAM", "name": "SPAM", "type": "system"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/messages/batchModify") && r.Method == http.MethodPost:
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
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})

		if runErr := runKong(t, &GmailBatchModifyCmd{}, []string{
			"msg1", "msg2",
			"--add", "Work,INBOX",
			"--remove", "SPAM",
		}, ctx, flags); runErr != nil {
			t.Fatalf("execute: %v", runErr)
		}
	})

	const want = "ACTION\tCOUNT\tADDED_LABELS\tREMOVED_LABELS\nmodify\t2\tLabel_1,INBOX\tSPAM\n"
	if out != want {
		t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", want, out)
	}
	if strings.Contains(strings.ToLower(out), "modified") {
		t.Fatalf("plain stdout must not include prose, got %q", out)
	}
}
