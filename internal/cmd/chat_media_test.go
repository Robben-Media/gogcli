package cmd

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

func TestExecute_ChatMediaUpload_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The upload endpoint uses /upload/ prefix
		// Accept any path containing "media" since the Go client handles the endpoint
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"attachmentDataRef": map[string]any{
				"resourceName":          "media/abc123",
				"attachmentUploadToken": "FAKE_TOKEN_FOR_TEST",
			},
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	// Create a temp file for testing
	tmpFile, err := os.CreateTemp("", "upload-test-*.txt")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	tmpFile.WriteString("test file content for upload")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "media", "upload", "spaces/test-space", "--file", tmpFile.Name()}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	ref, ok := result["attachmentDataRef"].(map[string]any)
	if !ok {
		t.Fatalf("expected attachmentDataRef object")
	}
	if ref["resourceName"] != "media/abc123" {
		t.Fatalf("expected media/abc123, got %v", ref["resourceName"])
	}
}

func TestExecute_ChatMediaUpload_WithCustomName(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotFilename string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		// Parse multipart to get filename
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse content-type: %v", err)
		}
		if strings.HasPrefix(mediaType, "multipart/") {
			reader := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("read multipart: %v", err)
				}
				if strings.Contains(part.Header.Get("Content-Type"), "application/json") {
					var body map[string]any
					json.NewDecoder(part).Decode(&body)
					gotFilename, _ = body["filename"].(string)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"attachmentDataRef": map[string]any{
				"resourceName":          "media/xyz789",
				"attachmentUploadToken": "FAKE_TOKEN_FOR_TEST",
			},
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	tmpFile, err := os.CreateTemp("", "upload-test-*.txt")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	tmpFile.WriteString("test content")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "media", "upload", "spaces/test", "--file", tmpFile.Name(), "--name", "custom-name.txt"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	if result["filename"] != "custom-name.txt" {
		t.Fatalf("expected custom-name.txt, got %v", result["filename"])
	}
	if gotFilename != "custom-name.txt" {
		t.Fatalf("expected custom-name.txt in request, got %q", gotFilename)
	}
}

func TestExecute_ChatMediaDownload(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	testContent := "downloaded file content"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Download endpoint is GET /v1/media/{name}?alt=media
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/media/")) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(testContent))
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	// Create temp dir for output
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "downloaded.txt")

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr := Execute([]string{"--json", "--account", "a@b.com", "chat", "media", "download", "media/abc123", "--output", outputPath})
			if execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	// Verify file was created
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(content) != testContent {
		t.Fatalf("expected %q, got %q", testContent, string(content))
	}

	// Verify JSON output
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	if result["path"] != outputPath {
		t.Fatalf("expected path %s, got %v", outputPath, result["path"])
	}
}

func TestExecute_ChatMedia_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	// Using @gmail.com account should fail
	err := Execute([]string{"--account", "user@gmail.com", "chat", "media", "download", "media/test"})
	if err == nil {
		t.Fatal("expected error for consumer account")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("expected Workspace error, got: %v", err)
	}
}
