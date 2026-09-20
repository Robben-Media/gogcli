package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestDownloadAttachmentToPath_MissingOutPath(t *testing.T) {
	if _, _, _, err := downloadAttachmentToPath(context.Background(), nil, "m1", "a1", " ", 0); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadAttachmentToPath_CachedBySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gotPath, cached, bytes, err := downloadAttachmentToPath(context.Background(), nil, "m1", "a1", path, 3)
	if err != nil {
		t.Fatalf("downloadAttachmentToPath: %v", err)
	}
	if gotPath != path || !cached || bytes != 3 {
		t.Fatalf("unexpected result: path=%q cached=%v bytes=%d", gotPath, cached, bytes)
	}
}

func TestDownloadAttachmentToPath_UnknownSizeDoesNotTrustExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.bin")
	if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv := httptestServerForAttachment(t, base64.RawURLEncoding.EncodeToString([]byte("complete")))
	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	gotPath, cached, bytes, err := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", path, -1)
	if err != nil {
		t.Fatalf("downloadAttachmentToPath: %v", err)
	}
	if gotPath != path || cached || bytes != 8 {
		t.Fatalf("unexpected result: path=%q cached=%v bytes=%d", gotPath, cached, bytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "complete" {
		t.Fatalf("unexpected data: %q", data)
	}
}

func TestDownloadAttachmentToPath_Base64Fallback(t *testing.T) {
	srv := httptestServerForAttachment(t, base64.URLEncoding.EncodeToString([]byte("hello")))

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	path := filepath.Join(t.TempDir(), "c.bin")
	gotPath, cached, bytes, err := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", path, 0)
	if err != nil {
		t.Fatalf("downloadAttachmentToPath: %v", err)
	}
	if gotPath != path || cached || bytes != 5 {
		t.Fatalf("unexpected result: path=%q cached=%v bytes=%d", gotPath, cached, bytes)
	}
	if data, readErr := os.ReadFile(path); readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	} else if string(data) != "hello" {
		t.Fatalf("unexpected data: %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %04o, want 0600", got)
	}
}

func TestDownloadAttachmentToPath_ReplaceFailurePreservesDestination(t *testing.T) {
	origReplace := replaceDownloadFile
	t.Cleanup(func() { replaceDownloadFile = origReplace })
	replaceDownloadFile = func(string, string) error { return errors.New("replace failed") }

	srv := httptestServerForAttachment(t, base64.RawURLEncoding.EncodeToString([]byte("complete")))
	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "attachment.bin")
	if writeErr := os.WriteFile(path, []byte("original"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}
	if _, cached, _, downloadErr := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", path, -1); downloadErr == nil {
		t.Fatal("expected replacement error")
	} else if cached {
		t.Fatal("failed download reported as cached")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("destination changed: %q", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "attachment.bin" {
		t.Fatalf("temporary artifact left behind: %#v", entries)
	}
}

func TestDownloadAttachmentToPath_EmptyData(t *testing.T) {
	srv := httptestServerForAttachment(t, "")

	gsvc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	path := filepath.Join(t.TempDir(), "d.bin")
	if _, _, _, err := downloadAttachmentToPath(context.Background(), gsvc, "m1", "a1", path, 0); err == nil {
		t.Fatalf("expected error")
	}
}

func httptestServerForAttachment(t *testing.T, data string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data,
		})
	}))
}
