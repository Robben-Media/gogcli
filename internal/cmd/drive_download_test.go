package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestDownloadDriveFile_NonGoogleDoc(t *testing.T) {
	body := "hello"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Files.Get(...).Download hits /drive/v3/files/{id}?alt=media
		if !(strings.Contains(r.URL.Path, "/files/") && r.URL.Query().Get("alt") == "media") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
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

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "file.bin")
	outPath, n, err := downloadDriveFile(context.Background(), svc, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "")
	if err != nil {
		t.Fatalf("downloadDriveFile: %v", err)
	}
	if outPath != dest {
		t.Fatalf("unexpected outPath: %q", outPath)
	}
	if n != int64(len(body)) {
		t.Fatalf("unexpected n: %d", n)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != body {
		t.Fatalf("unexpected body: %q", string(b))
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %04o, want 0644", got)
	}
}

func TestDownloadDriveFile_NonGoogleDocFormatRejected(t *testing.T) {
	origDownload := driveDownload
	t.Cleanup(func() { driveDownload = origDownload })

	called := false
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		called = true
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "file.html")
	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "html")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "non-Google Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatalf("download should not be called on format error")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file written, stat=%v", statErr)
	}
}

func TestDownloadDriveFile_GoogleDocExport(t *testing.T) {
	body := "exported"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Files.Export(...).Download hits /drive/v3/files/{id}/export?mimeType=...
		if !(strings.Contains(r.URL.Path, "/export") && strings.Contains(r.URL.Path, "/files/")) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
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

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "doc.txt")
	outPath, n, err := downloadDriveFile(context.Background(), svc, &drive.File{Id: "id1", MimeType: "application/vnd.google-apps.document"}, dest, "")
	if err != nil {
		t.Fatalf("downloadDriveFile: %v", err)
	}
	if !strings.HasSuffix(outPath, ".pdf") {
		t.Fatalf("expected pdf outPath, got: %q", outPath)
	}
	if n != int64(len(body)) {
		t.Fatalf("unexpected n: %d", n)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != body {
		t.Fatalf("unexpected body: %q", string(b))
	}
}

func TestDownloadDriveFile_InterruptedExportPreservesDestination(t *testing.T) {
	origExport := driveExportDownload
	t.Cleanup(func() { driveExportDownload = origExport })
	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.MultiReader(strings.NewReader("partial"), errorReader{err: errors.New("connection lost")})),
		}, nil
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(dest, []byte("original"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	outPath, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/vnd.google-apps.document"}, filepath.Join(dir, "doc.txt"), "")
	if err == nil {
		t.Fatal("expected interrupted export error")
	}
	if outPath != "" {
		t.Fatalf("outPath = %q, want empty on failure", outPath)
	}
	data, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("destination changed: %q", data)
	}
	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatalf("ReadDir: %v", readDirErr)
	}
	if len(entries) != 1 || entries[0].Name() != "doc.pdf" {
		t.Fatalf("temporary artifact left behind: %#v", entries)
	}
}

func TestDownloadDriveFile_HTTPError(t *testing.T) {
	orig := driveDownload
	t.Cleanup(func() { driveDownload = orig })
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "403 Forbidden",
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("nope\n")),
		}, nil
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "file.bin")
	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "download failed") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadDriveFile_InterruptedDownloadPreservesDestination(t *testing.T) {
	orig := driveDownload
	t.Cleanup(func() { driveDownload = orig })
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.MultiReader(strings.NewReader("partial"), errorReader{err: errors.New("connection lost")})),
		}, nil
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "")
	if err == nil {
		t.Fatal("expected interrupted download error")
	}
	data, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("destination changed: %q", data)
	}
	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatalf("ReadDir: %v", readDirErr)
	}
	if len(entries) != 1 || entries[0].Name() != "file.bin" {
		t.Fatalf("temporary artifact left behind: %#v", entries)
	}
}

func TestDownloadDriveFile_InterruptedNewDownloadLeavesNoArtifacts(t *testing.T) {
	orig := driveDownload
	t.Cleanup(func() { driveDownload = orig })
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.MultiReader(strings.NewReader("partial"), errorReader{err: errors.New("connection lost")})),
		}, nil
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "file.bin")
	if _, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, ""); err == nil {
		t.Fatal("expected interrupted download error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed new download left artifacts: %#v", entries)
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type writeErrorTempFile struct {
	file *os.File
}

func (f *writeErrorTempFile) Write(p []byte) (int, error) {
	if len(p) > 0 {
		_, _ = f.file.Write(p[:1])
	}
	return 0, errors.New("disk full")
}

func (f *writeErrorTempFile) Close() error { return f.file.Close() }
func (f *writeErrorTempFile) Name() string { return f.file.Name() }

type closeErrorTempFile struct {
	file       *os.File
	closeCalls int
}

func (f *closeErrorTempFile) Write(p []byte) (int, error) { return f.file.Write(p) }
func (f *closeErrorTempFile) Name() string                { return f.file.Name() }
func (f *closeErrorTempFile) Close() error {
	f.closeCalls++
	if f.closeCalls == 1 {
		return errors.New("close failed")
	}
	return f.file.Close()
}

func TestApplyDownloadModeMask_RespectsRestrictiveUmask(t *testing.T) {
	if got := applyDownloadModeMask(0o644, 0o077); got != 0o600 {
		t.Fatalf("mode = %04o, want 0600", got)
	}
}

func TestWriteDownloadFile_RetriesCloseBeforeCleanup(t *testing.T) {
	originalCreate := createDownloadTempFile
	t.Cleanup(func() { createDownloadTempFile = originalCreate })

	var temp *closeErrorTempFile
	createDownloadTempFile = func(dir, pattern string) (downloadTempFile, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		temp = &closeErrorTempFile{file: file}
		return temp, nil
	}

	dest := filepath.Join(t.TempDir(), "file.bin")
	if _, _, err := writeDownloadFile(dest, 0o644, func(w io.Writer) (int64, error) {
		n, writeErr := io.WriteString(w, "replacement")
		return int64(n), writeErr
	}); err == nil {
		t.Fatal("expected close failure")
	}
	if temp == nil {
		t.Fatal("temporary file was not created")
	}
	if temp.closeCalls != 2 {
		t.Fatalf("close calls = %d, want 2", temp.closeCalls)
	}
}

func TestDownloadDriveFile_CommitFailuresPreserveDestination(t *testing.T) {
	origDownload := driveDownload
	origCreate := createDownloadTempFile
	origReplace := replaceDownloadFile
	t.Cleanup(func() {
		driveDownload = origDownload
		createDownloadTempFile = origCreate
		replaceDownloadFile = origReplace
	})

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("replacement")),
		}, nil
	}

	tests := []struct {
		name   string
		inject func()
	}{
		{
			name: "write",
			inject: func() {
				createDownloadTempFile = func(dir, pattern string) (downloadTempFile, error) {
					f, err := os.CreateTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					return &writeErrorTempFile{file: f}, nil
				}
			},
		},
		{
			name: "close",
			inject: func() {
				createDownloadTempFile = func(dir, pattern string) (downloadTempFile, error) {
					f, err := os.CreateTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					return &closeErrorTempFile{file: f}, nil
				}
			},
		},
		{
			name: "replace",
			inject: func() {
				replaceDownloadFile = func(string, string) error {
					return errors.New("replace failed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createDownloadTempFile = origCreate
			replaceDownloadFile = origReplace
			tt.inject()

			dir := t.TempDir()
			dest := filepath.Join(dir, "file.bin")
			if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			if _, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, ""); err == nil {
				t.Fatal("expected download failure")
			}
			data, err := os.ReadFile(dest)
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
			if len(entries) != 1 || entries[0].Name() != "file.bin" {
				t.Fatalf("temporary artifact left behind: %#v", entries)
			}
		})
	}
}

func TestWriteDownloadFile_FollowsSymlinkAndPreservesTargetMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.bin")
	if err := os.WriteFile(target, []byte("original"), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "link.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, _, err := writeDownloadFile(link, 0o644, func(w io.Writer) (int64, error) {
		n, writeErr := io.WriteString(w, "replacement")
		return int64(n), writeErr
	}); err != nil {
		t.Fatalf("writeDownloadFile: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination symlink was replaced: mode=%v", info.Mode())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "replacement" {
		t.Fatalf("target data = %q, want replacement", data)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if gotMode := targetInfo.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("target mode = %04o, want 0640", gotMode)
	}
}

func TestWriteDownloadFile_FollowsDanglingSymlinkChain(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.bin")
	intermediate := filepath.Join(dir, "intermediate.bin")
	if err := os.Symlink(target, intermediate); err != nil {
		t.Fatalf("create intermediate symlink: %v", err)
	}
	link := filepath.Join(dir, "link.bin")
	if err := os.Symlink(intermediate, link); err != nil {
		t.Fatalf("create destination symlink: %v", err)
	}

	if _, _, err := writeDownloadFile(link, 0o644, func(w io.Writer) (int64, error) {
		n, writeErr := io.WriteString(w, "replacement")
		return int64(n), writeErr
	}); err != nil {
		t.Fatalf("writeDownloadFile: %v", err)
	}
	for _, symlink := range []string{link, intermediate} {
		info, err := os.Lstat(symlink)
		if err != nil {
			t.Fatalf("lstat %s: %v", symlink, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("symlink was replaced: %s", symlink)
		}
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "replacement" {
		t.Fatalf("target data = %q, want replacement", data)
	}
}

func TestWriteDownloadFile_PreservesZeroMode(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
		t.Fatalf("write destination: %v", err)
	}
	if err := os.Chmod(dest, 0); err != nil {
		t.Fatalf("chmod destination: %v", err)
	}

	if _, _, err := writeDownloadFile(dest, 0o644, func(w io.Writer) (int64, error) {
		n, writeErr := io.WriteString(w, "replacement")
		return int64(n), writeErr
	}); err != nil {
		t.Fatalf("writeDownloadFile: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0 {
		t.Fatalf("destination mode = %04o, want 0000", gotMode)
	}
}

func TestDownloadDriveFile_CreateError(t *testing.T) {
	orig := driveDownload
	t.Cleanup(func() { driveDownload = orig })
	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("x")),
		}, nil
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "no-such-dir", "file.bin")
	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{Id: "id1", MimeType: "application/pdf"}, dest, "")
	if err == nil {
		t.Fatalf("expected error")
	}
}
