package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestConfigCmd_JSONParity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.File{
		KeyringBackend:  "file",
		DefaultTimezone: "UTC",
	}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var list struct {
		Timezone       string `json:"timezone"`
		KeyringBackend string `json:"keyring_backend"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("list json parse: %v\nout=%q", err, listOut)
	}

	getOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "get", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var get struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(getOut), &get); err != nil {
		t.Fatalf("get json parse: %v\nout=%q", err, getOut)
	}
	if get.Key != "timezone" {
		t.Fatalf("expected key timezone, got %q", get.Key)
	}
	if get.Value != list.Timezone {
		t.Fatalf("expected timezone %q, got %q", list.Timezone, get.Value)
	}
}

func TestConfigCmd_JSONEmptyValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config-home"))

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var list struct {
		Timezone       string `json:"timezone"`
		KeyringBackend string `json:"keyring_backend"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("list json parse: %v\nout=%q", err, listOut)
	}
	if list.Timezone != "" {
		t.Fatalf("expected empty timezone, got %q", list.Timezone)
	}
	if list.KeyringBackend != "" {
		t.Fatalf("expected empty keyring_backend, got %q", list.KeyringBackend)
	}

	getOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "get", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var get struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(getOut), &get); err != nil {
		t.Fatalf("get json parse: %v\nout=%q", err, getOut)
	}
	if get.Value != "" {
		t.Fatalf("expected empty value, got %q", get.Value)
	}
}

func TestConfigSet_PlainReceipt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "config", "set", "timezone", "UTC"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if want := "ACTION\tKEY\tVALUE\nset\ttimezone\tUTC\n"; out != want {
		t.Fatalf("plain config set output = %q, want %q", out, want)
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.DefaultTimezone != "UTC" {
		t.Fatalf("persisted timezone = %q, want UTC", cfg.DefaultTimezone)
	}
}

func TestConfigUnset_PlainReceipt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.WriteConfig(config.File{DefaultTimezone: "UTC"}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "config", "unset", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if want := "ACTION\tKEY\tVALUE\nunset\ttimezone\t\n"; out != want {
		t.Fatalf("plain config unset output = %q, want %q", out, want)
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.DefaultTimezone != "" {
		t.Fatalf("persisted timezone = %q, want empty", cfg.DefaultTimezone)
	}
}

func TestConfigList_PlainTSV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.WriteConfig(config.File{
		KeyringBackend:  "file",
		DefaultTimezone: "UTC",
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "config", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.Contains(out, "Config file:") {
		t.Fatalf("expected TSV plain output, got %q", out)
	}
	if strings.Contains(out, "path\t") {
		t.Fatalf("expected only settable config keys in plain output, got %q", out)
	}
	if !strings.Contains(out, "KEY\tVALUE\n") ||
		!strings.Contains(out, "timezone\tUTC\n") ||
		!strings.Contains(out, "keyring_backend\tfile\n") {
		t.Fatalf("unexpected plain config output: %q", out)
	}
}
