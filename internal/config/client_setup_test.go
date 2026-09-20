package config

import (
	"path/filepath"
	"testing"
)

func TestClientSetup_ProjectChangeResetsAcks(t *testing.T) {
	cfg := File{}
	if err := SetClientSetupProject(&cfg, "default", "p1"); err != nil {
		t.Fatalf("set: %v", err)
	}

	if err := SetClientSetupAcknowledgments(&cfg, "default", true, true, true); err != nil {
		t.Fatalf("acks: %v", err)
	}

	got := GetClientSetup(cfg, "default")
	if !got.AcknowledgedBranding || got.ProjectID != "p1" {
		t.Fatalf("got %#v", got)
	}

	if err := SetClientSetupProject(&cfg, "default", "p2"); err != nil {
		t.Fatalf("set2: %v", err)
	}

	got = GetClientSetup(cfg, "default")
	if got.ProjectID != "p2" {
		t.Fatalf("project=%q", got.ProjectID)
	}

	if got.AcknowledgedBranding || got.AcknowledgedAudience || got.AcknowledgedDataAccess {
		t.Fatalf("acks should reset on project change: %#v", got)
	}
}

func TestClientSetup_RoundtripConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	cfg := File{}
	if err := SetClientSetupProject(&cfg, "work", "cloud-proj"); err != nil {
		t.Fatalf("set: %v", err)
	}

	if err := SetClientSetupAcknowledgments(&cfg, "work", true, false, true); err != nil {
		t.Fatalf("acks: %v", err)
	}

	if err := WriteConfig(cfg); err != nil {
		t.Fatalf("write: %v", err)
	}

	read, err := ReadConfig()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	got := GetClientSetup(read, "work")
	if got.ProjectID != "cloud-proj" || !got.AcknowledgedBranding || got.AcknowledgedAudience || !got.AcknowledgedDataAccess {
		t.Fatalf("got %#v", got)
	}
}
