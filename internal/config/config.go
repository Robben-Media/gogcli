package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

type File struct {
	KeyringBackend  string            `json:"keyring_backend,omitempty"`
	DefaultTimezone string            `json:"default_timezone,omitempty"`
	AccountAliases  map[string]string `json:"account_aliases,omitempty"`
	AccountClients  map[string]string `json:"account_clients,omitempty"`
	ClientDomains   map[string]string `json:"client_domains,omitempty"`
	// ClientSetup stores transparent per-client setup facts that Google does not
	// expose via supported CLI inspection (selected project pairing and manual
	// Auth Platform acknowledgments). Operational state is always re-inspected.
	ClientSetup map[string]ClientSetup `json:"client_setup,omitempty"`
	Policies    []Policy               `json:"policies,omitempty"`
}

// ClientSetup is minimal durable setup metadata for one OAuth client namespace.
type ClientSetup struct {
	// ProjectID is the Google Cloud project paired with this client.
	ProjectID string `json:"project_id,omitempty"`
	// Manual acknowledgments for Console-only Auth Platform stages.
	// These are never treated as independently Google-verified.
	AcknowledgedBranding   bool `json:"acknowledged_branding,omitempty"`
	AcknowledgedAudience   bool `json:"acknowledged_audience,omitempty"`
	AcknowledgedDataAccess bool `json:"acknowledged_data_access,omitempty"`
	// CredentialsProjectAssociated records explicit confirmation that credentials
	// without project_id belong to ProjectID. It is not implied by pairing a target.
	CredentialsProjectAssociated bool `json:"credentials_project_associated,omitempty"`
	// ReauthorizationRequired prevents tokens issued to replaced credentials from
	// satisfying setup until they have been invalidated or an account reauthorizes.
	ReauthorizationRequired bool `json:"reauthorization_required,omitempty"`
}

// GetClientSetup returns the setup record for client, or a zero value.
func GetClientSetup(cfg File, client string) ClientSetup {
	if cfg.ClientSetup == nil {
		return ClientSetup{}
	}

	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return ClientSetup{}
	}

	return cfg.ClientSetup[normalized]
}

// SetClientSetupProject pairs client with projectID. Changing project resets
// manual acknowledgments.
func SetClientSetupProject(cfg *File, client, projectID string) error {
	if cfg == nil {
		return errNilConfig
	}

	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return err
	}

	projectID = strings.TrimSpace(projectID)

	if cfg.ClientSetup == nil {
		cfg.ClientSetup = make(map[string]ClientSetup)
	}

	cur := cfg.ClientSetup[normalized]
	if cur.ProjectID != "" && projectID != "" && cur.ProjectID != projectID {
		cur.AcknowledgedBranding = false
		cur.AcknowledgedAudience = false
		cur.AcknowledgedDataAccess = false
		cur.CredentialsProjectAssociated = false
	}

	cur.ProjectID = projectID
	cfg.ClientSetup[normalized] = cur

	return nil
}

// SetClientSetupReauthorizationRequired records whether credential replacement
// requires a fresh account authorization before setup can be complete.
func SetClientSetupReauthorizationRequired(cfg *File, client string, required bool) error {
	if cfg == nil {
		return errNilConfig
	}

	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return err
	}

	if cfg.ClientSetup == nil {
		cfg.ClientSetup = make(map[string]ClientSetup)
	}

	cur := cfg.ClientSetup[normalized]
	cur.ReauthorizationRequired = required
	cfg.ClientSetup[normalized] = cur

	return nil
}

// SetClientSetupCredentialsProjectAssociated records explicit association of
// project-less OAuth credentials with the current selected project.
func SetClientSetupCredentialsProjectAssociated(cfg *File, client string, associated bool) error {
	if cfg == nil {
		return errNilConfig
	}

	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return err
	}

	if cfg.ClientSetup == nil {
		cfg.ClientSetup = make(map[string]ClientSetup)
	}

	cur := cfg.ClientSetup[normalized]
	cur.CredentialsProjectAssociated = associated
	cfg.ClientSetup[normalized] = cur

	return nil
}

// SetClientSetupAcknowledgments records explicit user acknowledgments for
// Console-only stages on the client's current project pairing.

func SetClientSetupAcknowledgments(cfg *File, client string, branding, audience, dataAccess bool) error {
	if cfg == nil {
		return errNilConfig
	}

	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return err
	}

	if cfg.ClientSetup == nil {
		cfg.ClientSetup = make(map[string]ClientSetup)
	}

	cur := cfg.ClientSetup[normalized]
	if branding {
		cur.AcknowledgedBranding = true
	}

	if audience {
		cur.AcknowledgedAudience = true
	}

	if dataAccess {
		cur.AcknowledgedDataAccess = true
	}

	cfg.ClientSetup[normalized] = cur

	return nil
}

func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.json"), nil
}

func WriteConfig(cfg File) error {
	_, err := EnsureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config json: %w", err)
	}

	b = append(b, '\n')

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}

	return nil
}

func ConfigExists() (bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return false, err
	}

	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}

		return false, fmt.Errorf("stat config: %w", statErr)
	}

	return true, nil
}

func ReadConfig() (File, error) {
	path, err := ConfigPath()
	if err != nil {
		return File{}, err
	}

	b, err := os.ReadFile(path) //nolint:gosec // config file path
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}

		return File{}, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := json5.Unmarshal(b, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}
