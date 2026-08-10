package tracking

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	errInjectedWriteFailure   = errors.New("injected write failure")
	errInjectedSyncFailure    = errors.New("injected sync failure")
	errInjectedCloseFailure   = errors.New("injected close failure")
	errInjectedReplaceFailure = errors.New("injected replace failure")
)

func TestSaveConfigRetainsAccountsAndOwnerOnlyPerms(t *testing.T) {
	setupTrackingConfigEnv(t)

	if err := SaveConfig("a@example.com", &Config{
		Enabled:   true,
		WorkerURL: "https://a.example",
	}); err != nil {
		t.Fatalf("SaveConfig a: %v", err)
	}

	if err := SaveConfig("b@example.com", &Config{
		Enabled:   true,
		WorkerURL: "https://b.example",
	}); err != nil {
		t.Fatalf("SaveConfig b: %v", err)
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("final permissions = %04o, want 0600", info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var fileCfg fileConfig
	if unmarshalErr := json.Unmarshal(data, &fileCfg); unmarshalErr != nil {
		t.Fatalf("parse tracking config: %v\n%s", unmarshalErr, data)
	}

	if fileCfg.Accounts["a@example.com"] == nil || fileCfg.Accounts["a@example.com"].WorkerURL != "https://a.example" {
		t.Fatalf("missing prior account a: %#v", fileCfg.Accounts)
	}

	if fileCfg.Accounts["b@example.com"] == nil || fileCfg.Accounts["b@example.com"].WorkerURL != "https://b.example" {
		t.Fatalf("missing account b: %#v", fileCfg.Accounts)
	}

	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), "tracking-*.tmp"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}

	if len(temps) != 0 {
		t.Fatalf("temporary artifacts left behind: %v", temps)
	}
}

func TestSaveConfigFailurePreservesPriorBytesAndCleansTemp(t *testing.T) {
	setupTrackingConfigEnv(t)

	if err := SaveConfig("a@example.com", &Config{
		Enabled:   true,
		WorkerURL: "https://original.example",
	}); err != nil {
		t.Fatalf("seed SaveConfig: %v", err)
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}

	prior, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prior: %v", err)
	}

	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "write",
			setup: func(t *testing.T) {
				t.Helper()
				original := writeTrackingConfigFile
				writeTrackingConfigFile = func(f *os.File, data []byte) (int, error) {
					return 0, errInjectedWriteFailure
				}

				t.Cleanup(func() { writeTrackingConfigFile = original })
			},
		},
		{
			name: "sync",
			setup: func(t *testing.T) {
				t.Helper()
				original := syncTrackingConfigFile
				syncTrackingConfigFile = func(*os.File) error {
					return errInjectedSyncFailure
				}

				t.Cleanup(func() { syncTrackingConfigFile = original })
			},
		},
		{
			name: "close",
			setup: func(t *testing.T) {
				t.Helper()
				original := closeTrackingConfigFile
				closeTrackingConfigFile = func(*os.File) error {
					return errInjectedCloseFailure
				}

				t.Cleanup(func() { closeTrackingConfigFile = original })
			},
		},
		{
			name: "replace",
			setup: func(t *testing.T) {
				t.Helper()
				original := replaceTrackingConfigFile
				replaceTrackingConfigFile = func(string, string) error {
					return errInjectedReplaceFailure
				}

				t.Cleanup(func() { replaceTrackingConfigFile = original })
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)

			err := SaveConfig("b@example.com", &Config{
				Enabled:   true,
				WorkerURL: "https://new.example",
			})
			if err == nil {
				t.Fatal("expected SaveConfig to fail")
			}

			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read after failure: %v", readErr)
			}

			if string(got) != string(prior) {
				t.Fatalf("prior config changed on %s failure\nprior:\n%s\ngot:\n%s", tc.name, prior, got)
			}

			entries, listErr := os.ReadDir(filepath.Dir(path))
			if listErr != nil {
				t.Fatalf("ReadDir: %v", listErr)
			}

			for _, entry := range entries {
				name := entry.Name()
				if name == filepath.Base(path) {
					continue
				}

				if strings.Contains(name, "tmp") || strings.HasPrefix(name, "tracking.json.") {
					t.Fatalf("temporary artifact left after %s failure: %s", tc.name, name)
				}
			}
		})
	}
}

func TestSaveConfigTempUsesOwnerOnlyPermissions(t *testing.T) {
	setupTrackingConfigEnv(t)

	var sawTempPerm os.FileMode
	originalWrite := writeTrackingConfigFile
	writeTrackingConfigFile = func(f *os.File, data []byte) (int, error) {
		info, err := f.Stat()
		if err != nil {
			return 0, fmt.Errorf("stat temp tracking config: %w", err)
		}
		sawTempPerm = info.Mode().Perm()

		return originalWrite(f, data)
	}

	t.Cleanup(func() { writeTrackingConfigFile = originalWrite })

	if err := SaveConfig("a@example.com", &Config{
		Enabled:   true,
		WorkerURL: "https://example.com",
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if sawTempPerm != 0o600 {
		t.Fatalf("temp permissions = %04o, want 0600", sawTempPerm)
	}
}
