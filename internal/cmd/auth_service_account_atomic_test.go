package cmd

import (
	"errors"
	"fmt"
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

func TestAuthKeep_StagingFailuresPreservePriorPairAndRemoveTemporaryFiles(t *testing.T) {
	for _, operation := range []string{"write", "short_write", "close", "permission"} {
		for _, failingTemp := range []int{2, 4} {
			t.Run(operation+"/temp-"+fmt.Sprint(failingTemp), func(t *testing.T) {
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
				for path, prior := range map[string][]byte{keepPath: oldKeep, genericPath: oldGeneric} {
					if writeErr := os.WriteFile(path, prior, 0o600); writeErr != nil {
						t.Fatalf("write prior key %q: %v", path, writeErr)
					}
				}

				originalCreateTemp := createServiceAccountTempFile
				t.Cleanup(func() { createServiceAccountTempFile = originalCreateTemp })
				created := 0
				createServiceAccountTempFile = func(dir, pattern string) (serviceAccountTempFile, error) {
					file, createErr := os.CreateTemp(dir, pattern)
					if createErr != nil {
						return nil, createErr
					}
					created++
					if created == failingTemp {
						return &failingServiceAccountTempFile{File: file, operation: operation}, nil
					}
					return file, nil
				}

				err = Execute([]string{"auth", "keep", email, "--key", keyPath})
				if err == nil {
					t.Fatal("Execute succeeded, want injected staging failure")
				}
				for path, want := range map[string][]byte{keepPath: oldKeep, genericPath: oldGeneric} {
					got, readErr := os.ReadFile(path)
					if readErr != nil {
						t.Fatalf("read prior key %q: %v", path, readErr)
					}
					if string(got) != string(want) {
						t.Fatalf("prior key %q = %q, want %q", path, got, want)
					}
				}
				entries, readErr := os.ReadDir(filepath.Dir(keepPath))
				if readErr != nil {
					t.Fatalf("read credential directory: %v", readErr)
				}
				for _, entry := range entries {
					if entry.Name() != filepath.Base(keepPath) && entry.Name() != filepath.Base(genericPath) {
						t.Fatalf("temporary file remains after failure: %s", entry.Name())
					}
				}
			})
		}
	}
}

func TestAuthKeep_SecondReplacementFailureRestoresPriorPairWhenRollbackRenamesFail(t *testing.T) {
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
	originalFallbackRename := fallbackRenameServiceAccountFile
	t.Cleanup(func() {
		renameServiceAccountFile = originalRename
		fallbackRenameServiceAccountFile = originalFallbackRename
	})
	replacement := 0
	rollbackRenameFailed := false
	fallbackRenameFailed := false
	renameServiceAccountFile = func(oldPath, newPath string) error {
		if strings.Contains(oldPath, ".backup-") && newPath == keepPath && !rollbackRenameFailed {
			rollbackRenameFailed = true
			return errors.New("injected initial rollback replacement failure")
		}
		if strings.Contains(oldPath, ".restore-") && newPath == keepPath && !fallbackRenameFailed {
			fallbackRenameFailed = true
			return errors.New("injected fallback rollback replacement failure")
		}
		if !strings.Contains(oldPath, ".backup-") && !strings.Contains(oldPath, ".restore-") {
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
	if !rollbackRenameFailed {
		t.Fatal("rollback backup rename was not attempted")
	}
	if !fallbackRenameFailed {
		t.Fatal("fallback rollback rename was not attempted")
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

func TestAuthKeep_UnrecoverableRollbackPreservesPrivateBackup(t *testing.T) {
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
	originalFallbackRename := fallbackRenameServiceAccountFile
	t.Cleanup(func() {
		renameServiceAccountFile = originalRename
		fallbackRenameServiceAccountFile = originalFallbackRename
	})
	replacements := 0
	renameServiceAccountFile = func(oldPath, newPath string) error {
		switch {
		case strings.Contains(oldPath, ".backup-") && newPath == keepPath:
			return errors.New("injected backup rollback failure")
		case strings.Contains(oldPath, ".restore-") && newPath == keepPath:
			return errors.New("injected staged rollback failure")
		case !strings.Contains(oldPath, ".backup-") && !strings.Contains(oldPath, ".restore-"):
			replacements++
			if replacements == 2 {
				return errors.New("injected second replacement failure")
			}
		}
		return os.Rename(oldPath, newPath)
	}
	fallbackRenameServiceAccountFile = func(_, _ string) error {
		return errors.New("injected platform rollback failure")
	}

	err = Execute([]string{"auth", "keep", email, "--key", keyPath})
	if err == nil || !strings.Contains(err.Error(), "prior credential preserved at") {
		t.Fatalf("Execute error = %v, want recovery path", err)
	}
	gotKeep, readErr := os.ReadFile(keepPath)
	if readErr != nil {
		t.Fatalf("read current Keep key: %v", readErr)
	}
	if string(gotKeep) != string(newData) {
		t.Fatalf("current Keep key = %q, want intact new key %q", gotKeep, newData)
	}
	gotGeneric, readErr := os.ReadFile(genericPath)
	if readErr != nil {
		t.Fatalf("read generic key: %v", readErr)
	}
	if string(gotGeneric) != string(oldGeneric) {
		t.Fatalf("generic key = %q, want prior key %q", gotGeneric, oldGeneric)
	}
	backups, globErr := filepath.Glob(filepath.Join(filepath.Dir(keepPath), "."+filepath.Base(keepPath)+".backup-*"))
	if globErr != nil {
		t.Fatalf("glob backups: %v", globErr)
	}
	if len(backups) != 1 || !strings.Contains(err.Error(), backups[0]) {
		t.Fatalf("recoverable backups = %v, error = %v", backups, err)
	}
	gotBackup, readErr := os.ReadFile(backups[0])
	if readErr != nil {
		t.Fatalf("read backup: %v", readErr)
	}
	if string(gotBackup) != string(oldKeep) {
		t.Fatalf("backup = %q, want prior key %q", gotBackup, oldKeep)
	}
	backupInfo, statErr := os.Stat(backups[0])
	if statErr != nil {
		t.Fatalf("stat backup: %v", statErr)
	}
	if gotMode := backupInfo.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("backup mode = %04o, want 0600", gotMode)
	}
}

func TestAuthServiceAccountSet_RejectsNonRegularDestination(t *testing.T) {
	for _, kind := range []string{"symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
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
			destPath, err := config.ServiceAccountPath(email)
			if err != nil {
				t.Fatalf("ServiceAccountPath: %v", err)
			}

			switch kind {
			case "symlink":
				target := filepath.Join(t.TempDir(), "target.json")
				if writeErr := os.WriteFile(target, []byte("target"), 0o644); writeErr != nil {
					t.Fatalf("write symlink target: %v", writeErr)
				}
				if symlinkErr := os.Symlink(target, destPath); symlinkErr != nil {
					t.Fatalf("create symlink: %v", symlinkErr)
				}
			case "directory":
				if mkdirErr := os.Mkdir(destPath, 0o755); mkdirErr != nil {
					t.Fatalf("create destination directory: %v", mkdirErr)
				}
			}

			err = Execute([]string{"auth", "service-account", "set", email, "--key", keyPath})
			if err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Fatalf("Execute error = %v, want non-regular destination error", err)
			}
			info, statErr := os.Lstat(destPath)
			if statErr != nil {
				t.Fatalf("lstat destination: %v", statErr)
			}
			if kind == "symlink" && info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("destination mode = %v, want symlink", info.Mode())
			}
			if kind == "directory" && !info.IsDir() {
				t.Fatalf("destination mode = %v, want directory", info.Mode())
			}
		})
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
				renameServiceAccountFile = func(_, newPath string) error {
					got, readErr := os.ReadFile(newPath)
					if readErr != nil {
						t.Fatalf("read live key during replacement: %v", readErr)
					}
					if string(got) != string(oldData) {
						t.Fatalf("live key during replacement = %q, want prior key %q", got, oldData)
					}
					return errors.New("injected replacement failure")
				}
			} else {
				created := 0
				createServiceAccountTempFile = func(dir, pattern string) (serviceAccountTempFile, error) {
					file, createErr := os.CreateTemp(dir, pattern)
					if createErr != nil {
						return nil, createErr
					}
					created++
					if created == 2 {
						return &failingServiceAccountTempFile{File: file, operation: operation}, nil
					}
					return file, nil
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
			for _, pattern := range []string{"." + filepath.Base(destPath) + ".tmp-*", "." + filepath.Base(destPath) + ".backup-*"} {
				temps, globErr := filepath.Glob(filepath.Join(filepath.Dir(destPath), pattern))
				if globErr != nil {
					t.Fatalf("glob temporary files: %v", globErr)
				}
				if len(temps) != 0 {
					t.Fatalf("temporary files remain after failure: %v", temps)
				}
			}
		})
	}
}
