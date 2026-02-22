package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestContactsPhotoDelete(t *testing.T) {
	tests := []struct {
		name         string
		cmd          ContactsPhotoDeleteCmd
		force        bool
		resourceName string
		wantErr      bool
	}{
		{
			name:         "deletes contact photo with force",
			cmd:          ContactsPhotoDeleteCmd{ResourceName: "people/123"},
			force:        true,
			resourceName: "people/123",
			wantErr:      false,
		},
		{
			name:         "deletes contact photo with short ID",
			cmd:          ContactsPhotoDeleteCmd{ResourceName: "123"},
			force:        true,
			resourceName: "people/123",
			wantErr:      false,
		},
		{
			name:    "empty resource name fails",
			cmd:     ContactsPhotoDeleteCmd{ResourceName: ""},
			force:   true,
			wantErr: true,
		},
		{
			name:    "requires confirmation without force",
			cmd:     ContactsPhotoDeleteCmd{ResourceName: "people/123"},
			force:   false,
			wantErr: true, // confirmation will fail in test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/v1/" + tt.resourceName + ":deleteContactPhoto"
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
				}
				if r.Method != http.MethodDelete {
					t.Errorf("unexpected method: %s", r.Method)
				}

				// Return empty response for successful delete
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("{}"))
			}))
			defer srv.Close()

			svc, err := people.NewService(context.Background(),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
			)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			newPeopleContactsService = func(ctx context.Context, email string) (*people.Service, error) {
				return svc, nil
			}

			u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			if err != nil {
				t.Fatalf("failed to create UI: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			flags := &RootFlags{Account: "a@b.com"}
			if tt.force {
				flags.Force = true
			}

			err = tt.cmd.Run(ctx, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContactsPhotoUpdate(t *testing.T) {
	// Create test image data (a small valid PNG-like header for testing)
	testImageData := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01")
	testImageBase64 := base64.StdEncoding.EncodeToString(testImageData)

	tests := []struct {
		name         string
		cmd          ContactsPhotoUpdateCmd
		resourceName string
		setup        func(w http.ResponseWriter, r *http.Request)
		wantErr      bool
	}{
		{
			name:         "updates contact photo from file",
			cmd:          ContactsPhotoUpdateCmd{ResourceName: "people/123", File: "test.png"},
			resourceName: "people/123",
			setup: func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/v1/people/123:updateContactPhoto"
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
				}
				if r.Method != http.MethodPatch {
					t.Errorf("unexpected method: %s", r.Method)
				}

				// Decode and verify the request
				var req people.UpdateContactPhotoRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}
				if req.PhotoBytes != testImageBase64 {
					t.Errorf("photo bytes mismatch")
				}

				resp := &people.UpdateContactPhotoResponse{
					Person: &people.Person{
						ResourceName: "people/123",
						Names:        []*people.Name{{DisplayName: "Test User"}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:         "updates contact photo with short ID",
			cmd:          ContactsPhotoUpdateCmd{ResourceName: "123", File: "test.png"},
			resourceName: "people/123",
			setup: func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/v1/people/123:updateContactPhoto"
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
				}

				resp := &people.UpdateContactPhotoResponse{
					Person: &people.Person{
						ResourceName: "people/123",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:    "empty resource name fails",
			cmd:     ContactsPhotoUpdateCmd{ResourceName: "", File: "test.png"},
			wantErr: true,
		},
		{
			name:    "missing file flag fails",
			cmd:     ContactsPhotoUpdateCmd{ResourceName: "people/123", File: ""},
			wantErr: true,
		},
		{
			name:    "non-existent file fails",
			cmd:     ContactsPhotoUpdateCmd{ResourceName: "people/123", File: "/nonexistent/path/test.png"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.setup != nil {
					tt.setup(w, r)
				} else {
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()

			svc, err := people.NewService(context.Background(),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
			)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			newPeopleContactsService = func(ctx context.Context, email string) (*people.Service, error) {
				return svc, nil
			}

			u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			if err != nil {
				t.Fatalf("failed to create UI: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			flags := &RootFlags{Account: "a@b.com"}

			// For the file test, we need to create a temp file
			if tt.cmd.File == "test.png" {
				tmpFile := createTempImageFile(t, testImageData)
				tt.cmd.File = tmpFile
			}

			err = tt.cmd.Run(ctx, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContactsPhotoUpdateFromStdin(t *testing.T) {
	testImageData := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &people.UpdateContactPhotoResponse{
			Person: &people.Person{
				ResourceName: "people/123",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	svc, err := people.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	newPeopleContactsService = func(ctx context.Context, email string) (*people.Service, error) {
		return svc, nil
	}

	// Create a buffer with test image data for stdin
	stdinBuffer := bytes.NewBuffer(testImageData)

	u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("failed to create UI: %v", err)
	}

	// Replace stdin with our buffer
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.Write(testImageData)
		w.Close()
	}()

	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := ContactsPhotoUpdateCmd{ResourceName: "people/123", File: "-"}

	err = cmd.Run(ctx, flags)
	_ = stdinBuffer // avoid unused variable warning

	if err != nil {
		t.Errorf("Run() error = %v", err)
	}
}

func createTempImageFile(t *testing.T, data []byte) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-image-*.png")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})
	return tmpFile.Name()
}
