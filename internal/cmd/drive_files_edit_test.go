package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
)

// ============================================
// Drive Files Watch Tests
// ============================================

func TestDriveFilesWatchCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/files/file123/watch") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "channel-123",
				"resourceId": "resource-456",
				"kind":       "api#channel",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveFilesWatchCmd{}
		if execErr := runKong(t, cmd, []string{"--address", "https://example.com/webhook", "file123"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["channelId"] != "channel-123" {
		t.Fatalf("expected channelId=channel-123, got: %v", result["channelId"])
	}
	if result["resourceId"] != "resource-456" {
		t.Fatalf("expected resourceId=resource-456, got: %v", result["resourceId"])
	}
}

func TestDriveFilesWatchCmd_MissingAddress(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, err := drive.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()

	cmd := &DriveFilesWatchCmd{}
	if execErr := runKong(t, cmd, []string{"file123"}, ctx, flags); execErr == nil {
		t.Fatal("expected error for missing --address")
	}
}

// ============================================
// Drive Files GenerateIds Tests
// ============================================

func TestDriveFilesGenerateIdsCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/generateIds") {
			if r.URL.Query().Get("count") != "5" {
				t.Fatalf("expected count=5, got: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ids":   []string{"id1", "id2", "id3", "id4", "id5"},
				"space": "drive",
				"kind":  "drive#generatedIds",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveFilesGenerateIdsCmd{}
		if execErr := runKong(t, cmd, []string{"--count", "5"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	ids, ok := result["ids"].([]any)
	if !ok || len(ids) != 5 {
		t.Fatalf("expected ids array with 5 elements, got: %v", result["ids"])
	}
}

func TestDriveFilesGenerateIdsCmd_Count(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/generateIds") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ids":   []string{"id1", "id2"},
				"space": "drive",
				"kind":  "drive#generatedIds",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveFilesGenerateIdsCmd{}
		if execErr := runKong(t, cmd, []string{"--count", "2"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	ids, ok := result["ids"].([]any)
	if !ok || len(ids) != 2 {
		t.Fatalf("expected ids array with 2 elements, got: %v", result["ids"])
	}
}

func TestDriveFilesGenerateIdsCmd_InvalidCount(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, err := drive.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()

	cmd := &DriveFilesGenerateIdsCmd{}
	// Count too high
	if execErr := runKong(t, cmd, []string{"--count", "2000"}, ctx, flags); execErr == nil {
		t.Fatal("expected error for count > 1000")
	}
	// Count too low
	if execErr := runKong(t, cmd, []string{"--count", "0"}, ctx, flags); execErr == nil {
		t.Fatal("expected error for count < 1")
	}
}

func TestDriveFilesGenerateIdsCmd_InvalidSpace(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, err := drive.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()

	cmd := &DriveFilesGenerateIdsCmd{}
	if execErr := runKong(t, cmd, []string{"--space", "invalid"}, ctx, flags); execErr == nil {
		t.Fatal("expected error for invalid --space")
	}
}

// ============================================
// Drive Files EmptyTrash Tests
// ============================================

func TestDriveFilesEmptyTrashCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/files/trash") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	// Use Force: true in RootFlags instead of --force flag
	flags := &RootFlags{Account: "test@example.com", Force: true}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveFilesEmptyTrashCmd{}
		// No args needed - uses global Force from flags
		if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if emptied, ok := result["emptied"].(bool); !ok || !emptied {
		t.Fatalf("expected emptied=true, got: %v", result["emptied"])
	}
}

func TestDriveFilesEmptyTrashCmd_Confirm(t *testing.T) {
	// Test that confirmation is required without --force
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, err := drive.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com", Force: false}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	cmd := &DriveFilesEmptyTrashCmd{}
	// Without force flag and with stdin closed, should fail
	if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr == nil {
		t.Fatal("expected error for missing confirmation")
	}
}

// ============================================
// Drive Drives Admin Tests (CRUD)
// ============================================

func TestDriveDrivesCreateCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drives") {
			// Verify request ID is present
			if r.URL.Query().Get("requestId") == "" {
				t.Fatal("missing requestId")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "drive-123",
				"name":        "My Shared Drive",
				"createdTime": "2024-01-01T00:00:00.000Z",
				"kind":        "drive#drive",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveDrivesCreateCmd{}
		if execErr := runKong(t, cmd, []string{"My Shared Drive"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	driveData, ok := result["drive"].(map[string]any)
	if !ok {
		t.Fatalf("expected drive object, got: %v", result)
	}
	if driveData["id"] != "drive-123" {
		t.Fatalf("expected id=drive-123, got: %v", driveData["id"])
	}
}

func TestDriveDrivesUpdateCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/drives/drive-123") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "drive-123",
				"name": "Updated Drive Name",
				"kind": "drive#drive",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveDrivesUpdateCmd{}
		if execErr := runKong(t, cmd, []string{"--name", "Updated Drive Name", "drive-123"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	driveData, ok := result["drive"].(map[string]any)
	if !ok {
		t.Fatalf("expected drive object, got: %v", result)
	}
	if driveData["id"] != "drive-123" {
		t.Fatalf("expected id=drive-123, got: %v", driveData["id"])
	}
	if driveData["name"] != "Updated Drive Name" {
		t.Fatalf("expected name=Updated Drive Name, got: %v", driveData["name"])
	}
}

func TestDriveDrivesDeleteCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/drives/drive-123") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "drive-123",
				"name": "Drive To Delete",
				"kind": "drive#drive",
			})
			return
		}
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/drives/drive-123") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	// Use Force: true in RootFlags instead of --force flag
	flags := &RootFlags{Account: "test@example.com", Force: true}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveDrivesDeleteCmd{}
		if execErr := runKong(t, cmd, []string{"drive-123"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["id"] != "drive-123" {
		t.Fatalf("expected id=drive-123, got: %v", result["id"])
	}
	if deleted, ok := result["deleted"].(bool); !ok || !deleted {
		t.Fatalf("expected deleted=true, got: %v", result["deleted"])
	}
}

func TestDriveDrivesHideCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drives/drive-123/hide") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "drive-123",
				"name":   "Hidden Drive",
				"hidden": true,
				"kind":   "drive#drive",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveDrivesHideCmd{}
		if execErr := runKong(t, cmd, []string{"drive-123"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	driveData, ok := result["drive"].(map[string]any)
	if !ok {
		t.Fatalf("expected drive object, got: %v", result)
	}
	if hidden, ok := driveData["hidden"].(bool); !ok || !hidden {
		t.Fatalf("expected hidden=true, got: %v", driveData["hidden"])
	}
}

func TestDriveDrivesUnhideCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drives/drive-123/unhide") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "drive-123",
				"name": "Visible Drive",
				"kind": "drive#drive",
				// Note: hidden is false, which may be omitted in JSON
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveDrivesUnhideCmd{}
		if execErr := runKong(t, cmd, []string{"drive-123"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	driveData, ok := result["drive"].(map[string]any)
	if !ok {
		t.Fatalf("expected drive object, got: %v", result)
	}
	if driveData["id"] != "drive-123" {
		t.Fatalf("expected id=drive-123, got: %v", driveData["id"])
	}
}

func TestDriveDrivesUpdateCmd_MissingName(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, err := drive.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()

	cmd := &DriveDrivesUpdateCmd{}
	if execErr := runKong(t, cmd, []string{"drive-123"}, ctx, flags); execErr == nil {
		t.Fatal("expected error for missing --name")
	}
}

func TestDriveDrivesCreateCmd_EmptyName(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	svc, err := drive.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}
	ctx := context.Background()

	cmd := &DriveDrivesCreateCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr == nil {
		t.Fatal("expected error for missing name")
	}
}
