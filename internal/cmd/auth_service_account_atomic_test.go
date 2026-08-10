package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

type failingServiceAccountTempFile struct {
	*os.File
	operation string
}

func (f *failingServiceAccountTempFile) Write(p []byte) (int, error) {
	switch f.operation {
	case "write":
		n, _ := f.File.Write(p[:len(p)/2])
		return n, errors.New("injected write failure")
	case "short_write":
		return f.File.Write(p[:len(p)/2])
	default:
		return f.File.Write(p)
	}
}

func (f *failingServiceAccountTempFile) Chmod(mode os.FileMode) error {
	if f.operation == "permission" {
		return errors.New("injected permission failure")
	}
	return f.File.Chmod(mode)
}

func (f *failingServiceAccountTempFile) Close() error {
	err := f.File.Close()
	if f.operation == "close" {
		return errors.New("injected close failure")
	}
	return err
}

func TestAuthServiceAccountSet_ReplacesPermissiveFilePrivately(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	const email = "user@example.com"
	oldData := []byte(`{"type":"service_account","client_email":"old@example.com"}`)
	newData := []byte(`{"type":"service_account","client_email":"new@example.com"}`)

	keyPath := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(keyPath, newData, 0o600); err != nil {
		t.Fatalf("write source key: %v", err)
	}
	if _, err := config.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	destPath, err := config.ServiceAccountPath(email)
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	if writeErr := os.WriteFile(destPath, oldData, 0o644); writeErr != nil {
		t.Fatalf("write prior key: %v", writeErr)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if executeErr := Execute([]string{"auth", "service-account", "set", email, "--key", keyPath}); executeErr != nil {
				t.Fatalf("Execute: %v", executeErr)
			}
		})
	})

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read installed key: %v", err)
	}
	if string(got) != string(newData) {
		t.Fatalf("installed key = %q, want %q", got, newData)
	}
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat installed key: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("installed key mode = %04o, want 0600", gotMode)
	}
}

func TestAuthKeep_InstallsMatchingPrivateCredentialFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	const email = "user@example.com"
	newData := []byte(`{"type":"service_account","client_email":"new@example.com"}`)
	keyPath := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(keyPath, newData, 0o600); err != nil {
		t.Fatalf("write source key: %v", err)
	}
	if _, err := config.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	keepPath, err := config.KeepServiceAccountPath(email)
	if err != nil {
		t.Fatalf("KeepServiceAccountPath: %v", err)
	}
	genericPath, err := config.ServiceAccountPath(email)
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	for _, path := range []string{keepPath, genericPath} {
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatalf("write prior key %q: %v", path, err)
		}
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "keep", email, "--key", keyPath}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	for _, path := range []string{keepPath, genericPath} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read installed key %q: %v", path, err)
		}
		if string(got) != string(newData) {
			t.Fatalf("installed key %q = %q, want %q", path, got, newData)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat installed key %q: %v", path, err)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o600 {
			t.Fatalf("installed key %q mode = %04o, want 0600", path, gotMode)
		}
	}
}

func TestAuthKeep_SecondReplacementFailureRestoresPriorPairAndRemovesTemporaryFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	const email = "user@example.com"
	oldKeep := []byte(`{"type":"service_account","client_email":"old-keep@example.com"}`)
	oldGeneric := []byte(`{"type":"service_account","client_email":"old-generic@example.com"}`)
	newData := []byte(`{"type":"service_account","client_email":"new@example.com"}`)
	keyPath := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(keyPath, newData, 0o600); err != nil {
		t.Fatalf("write source key: %v", err)
	}
	if _, err := config.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	keepPath, err := config.KeepServiceAccountPath(email)
	if err != nil {
		t.Fatalf("KeepServiceAccountPath: %v", err)
	}
	genericPath, err := config.ServiceAccountPath(email)
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	if writeErr := os.WriteFile(keepPath, oldKeep, 0o600); writeErr != nil {
		t.Fatalf("write prior Keep key: %v", writeErr)
	}
	if writeErr := os.WriteFile(genericPath, oldGeneric, 0o600); writeErr != nil {
		t.Fatalf("write prior generic key: %v", writeErr)
	}

	originalRename := renameServiceAccountFile
	t.Cleanup(func() { renameServiceAccountFile = originalRename })
	replacement := 0
	renameServiceAccountFile = func(oldPath, newPath string) error {
		if oldPath != keepPath && oldPath != genericPath {
			replacement++
			if replacement == 2 {
				return errors.New("injected second replacement failure")
			}
		}
		return os.Rename(oldPath, newPath)
	}

	err = Execute([]string{"auth", "keep", email, "--key", keyPath})
	if err == nil {
		t.Fatal("Execute succeeded, want injected replacement failure")
	}
	for path, want := range map[string][]byte{keepPath: oldKeep, genericPath: oldGeneric} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read restored key %q: %v", path, readErr)
		}
		if string(got) != string(want) {
			t.Fatalf("restored key %q = %q, want %q", path, got, want)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(keepPath))
	if err != nil {
		t.Fatalf("read credential directory: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(keepPath) && entry.Name() != filepath.Base(genericPath) {
			t.Fatalf("temporary file remains after failure: %s", entry.Name())
		}
	}
}

func TestAuthKeep_RollbackFailureIsReportedAndPreservesBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	const email = "user@example.com"
	oldKeep := []byte(`{"type":"service_account","client_email":"old-keep@example.com"}`)
	oldGeneric := []byte(`{"type":"service_account","client_email":"old-generic@example.com"}`)
	newData := []byte(`{"type":"service_account","client_email":"new@example.com"}`)
	keyPath := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(keyPath, newData, 0o600); err != nil {
		t.Fatalf("write source key: %v", err)
	}
	if _, err := config.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	keepPath, err := config.KeepServiceAccountPath(email)
	if err != nil {
		t.Fatalf("KeepServiceAccountPath: %v", err)
	}
	genericPath, err := config.ServiceAccountPath(email)
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	if writeErr := os.WriteFile(keepPath, oldKeep, 0o600); writeErr != nil {
		t.Fatalf("write prior Keep key: %v", writeErr)
	}
	if writeErr := os.WriteFile(genericPath, oldGeneric, 0o600); writeErr != nil {
		t.Fatalf("write prior generic key: %v", writeErr)
	}

	originalRename := renameServiceAccountFile
	t.Cleanup(func() { renameServiceAccountFile = originalRename })
	replacements := 0
	renameServiceAccountFile = func(oldPath, newPath string) error {
		if strings.Contains(oldPath, ".backup-") && newPath == keepPath {
			return errors.New("injected rollback failure")
		}
		if !strings.Contains(oldPath, ".backup-") && oldPath != keepPath && oldPath != genericPath {
			replacements++
			if replacements == 2 {
				return errors.New("injected second replacement failure")
			}
		}
		return os.Rename(oldPath, newPath)
	}

	err = Execute([]string{"auth", "keep", email, "--key", keyPath})
	if err == nil || !strings.Contains(err.Error(), "restore") || !strings.Contains(err.Error(), "injected rollback failure") {
		t.Fatalf("Execute error = %v, want replacement and rollback failure", err)
	}
	if _, statErr := os.Stat(keepPath); statErr != nil {
		t.Fatalf("final Keep credential missing after rollback failure: %v", statErr)
	}
	backups, globErr := filepath.Glob(filepath.Join(filepath.Dir(keepPath), "."+filepath.Base(keepPath)+".backup-*"))
	if globErr != nil {
		t.Fatalf("glob backups: %v", globErr)
	}
	if len(backups) != 1 {
		t.Fatalf("recoverable backup count = %d, want 1: %v", len(backups), backups)
	}
	gotBackup, readErr := os.ReadFile(backups[0])
	if readErr != nil {
		t.Fatalf("read recoverable backup: %v", readErr)
	}
	if string(gotBackup) != string(oldKeep) {
		t.Fatalf("backup = %q, want prior Keep key %q", gotBackup, oldKeep)
	}
}

func TestAuthServiceAccountSet_FailuresPreservePriorKeyAndRemoveTemporaryFiles(t *testing.T) {
	for _, operation := range []string{"write", "short_write", "close", "permission", "replacement"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

			const email = "user@example.com"
			oldData := []byte(`{"type":"service_account","client_email":"old@example.com"}`)
			newData := []byte(`{"type":"service_account","client_email":"new@example.com"}`)
			keyPath := filepath.Join(t.TempDir(), "service-account.json")
			if err := os.WriteFile(keyPath, newData, 0o600); err != nil {
				t.Fatalf("write source key: %v", err)
			}
			if _, err := config.EnsureDir(); err != nil {
				t.Fatalf("EnsureDir: %v", err)
			}
			destPath, err := config.ServiceAccountPath(email)
			if err != nil {
				t.Fatalf("ServiceAccountPath: %v", err)
			}
			if writeErr := os.WriteFile(destPath, oldData, 0o600); writeErr != nil {
				t.Fatalf("write prior key: %v", writeErr)
			}

			originalCreateTemp := createServiceAccountTempFile
			originalRename := renameServiceAccountFile
			t.Cleanup(func() {
				createServiceAccountTempFile = originalCreateTemp
				renameServiceAccountFile = originalRename
			})
			if operation == "replacement" {
				renameServiceAccountFile = func(_, _ string) error {
					return errors.New("injected replacement failure")
				}
			} else {
				createServiceAccountTempFile = func(dir, pattern string) (serviceAccountTempFile, error) {
					file, createErr := os.CreateTemp(dir, pattern)
					if createErr != nil {
						return nil, createErr
					}
					return &failingServiceAccountTempFile{File: file, operation: operation}, nil
				}
			}

			err = Execute([]string{"auth", "service-account", "set", email, "--key", keyPath})
			if err == nil {
				t.Fatal("Execute succeeded, want injected failure")
			}
			got, readErr := os.ReadFile(destPath)
			if readErr != nil {
				t.Fatalf("read prior key: %v", readErr)
			}
			if string(got) != string(oldData) {
				t.Fatalf("prior key = %q, want %q", got, oldData)
			}
			temps, globErr := filepath.Glob(filepath.Join(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*"))
			if globErr != nil {
				t.Fatalf("glob temporary files: %v", globErr)
			}
			if len(temps) != 0 {
				t.Fatalf("temporary files remain after failure: %v", temps)
			}
		})
	}
}
