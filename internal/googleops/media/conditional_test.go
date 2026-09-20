package media

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestConditionalUpdatePreservesAtomicPrecondition(t *testing.T) {
	for _, scenario := range []string{"success", "stale", "missing-etag", "race"} {
		t.Run(scenario, func(t *testing.T) {
			writes := 0

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if r.URL.Path != "/drive/v2/files/file1" {
						t.Errorf("unexpected metadata path %s", r.URL.Path)
					}

					version, etag := "12", "\"revision-12\""
					if scenario == "stale" {
						version = "13"
					}

					if scenario == "missing-etag" {
						etag = ""
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"id": "file1", "version": version, "etag": etag})

					return
				}

				writes++

				if r.Method != http.MethodPut || r.URL.Path != "/upload/drive/v2/files/file1" || r.Header.Get("If-Match") != "\"revision-12\"" {
					t.Errorf("unguarded write %s %s %q", r.Method, r.URL.Path, r.Header.Get("If-Match"))
				}

				if scenario == "race" {
					w.WriteHeader(http.StatusPreconditionFailed)
					_, _ = io.WriteString(w, `{"error":{"code":412,"message":"stale"}}`)

					return
				}
				_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
				reader := multipart.NewReader(r.Body, params["boundary"])
				part, _ := reader.NextPart()
				var meta map[string]any

				_ = json.NewDecoder(part).Decode(&meta)
				if meta["title"] != "renamed" || meta["name"] != nil {
					t.Errorf("wrong v2 metadata %#v", meta)
				}
				_, _ = io.WriteString(w, `{"id":"file1","title":"renamed"}`)
			}))
			defer server.Close()
			svc := &service{provider: &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}}

			out, err := svc.updateFile(context.Background(), testIdentity("account"), UpdateInput{Selection: mcpcontract.Selection{AccountID: "account"}, FileID: "file1", Name: "renamed", Data: "content", ExpectedVersion: 12})
			if scenario == "success" {
				if err != nil || out.Data.FileID != "file1" || out.Data.Name != "renamed" {
					t.Fatalf("result %#v error %v", out, err)
				}
			} else if err == nil {
				t.Fatal("expected refusal")
			}

			expected := 1
			if scenario == "stale" || scenario == "missing-etag" {
				expected = 0
			}

			if writes != expected {
				t.Fatalf("writes=%d want %d", writes, expected)
			}
		})
	}
}

func TestCreatePredeterminedID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		reader := multipart.NewReader(r.Body, params["boundary"])
		part, _ := reader.NextPart()
		var meta map[string]any

		_ = json.NewDecoder(part).Decode(&meta)
		if meta["id"] != "reserved1" {
			t.Errorf("missing persisted identity %#v", meta)
		}
		_, _ = io.Copy(w, strings.NewReader(`{"id":"reserved1"}`))
	}))
	defer server.Close()

	svc := &service{provider: &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}}
	if _, err := svc.createFile(context.Background(), testIdentity("account"), CreateInput{FileID: "reserved1", Name: "test", Data: "payload"}); err != nil {
		t.Fatal(err)
	}

	if err := svc.validateUpdate(UpdateInput{FileID: "file1", Data: "x", ExpectedVersion: -1}); err == nil {
		t.Fatal("accepted negative version")
	}
}
