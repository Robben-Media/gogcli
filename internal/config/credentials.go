package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

var (
	errInvalidCredentials = errors.New("invalid credentials.json (expected installed/web client_id and client_secret)")
	errMissingClientID    = errors.New("stored credentials.json is missing client_id/client_secret")
)

type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	// ProjectID is the Google Cloud project_id from the downloaded OAuth client
	// JSON when present. Optional for backwards compatibility.
	ProjectID string `json:"project_id,omitempty"`
}

type googleCredentialsFile struct {
	Installed *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		ProjectID    string `json:"project_id"`
	} `json:"installed"`
	Web *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		ProjectID    string `json:"project_id"`
	} `json:"web"`
}

func ParseGoogleOAuthClientJSON(b []byte) (ClientCredentials, error) {
	return parseGoogleOAuthClientJSON(b, false)
}

// ParseGoogleInstalledOAuthClientJSON accepts only a Desktop/installed OAuth
// client. Guided setup requires this redirect-capable client type; standalone
// credential installation retains historical web-client compatibility.
func ParseGoogleInstalledOAuthClientJSON(b []byte) (ClientCredentials, error) {
	return parseGoogleOAuthClientJSON(b, true)
}

func parseGoogleOAuthClientJSON(b []byte, requireInstalled bool) (ClientCredentials, error) {
	var f googleCredentialsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode credentials json: %w", err)
	}

	var clientID, clientSecret, projectID string
	if f.Installed != nil {
		clientID, clientSecret, projectID = f.Installed.ClientID, f.Installed.ClientSecret, f.Installed.ProjectID
	} else if !requireInstalled && f.Web != nil {
		clientID, clientSecret, projectID = f.Web.ClientID, f.Web.ClientSecret, f.Web.ProjectID
	}

	if clientID == "" || clientSecret == "" {
		return ClientCredentials{}, errInvalidCredentials
	}

	return ClientCredentials{ClientID: clientID, ClientSecret: clientSecret, ProjectID: projectID}, nil
}

func WriteClientCredentials(c ClientCredentials) error {
	return WriteClientCredentialsFor(DefaultClientName, c)
}

func WriteClientCredentialsFor(client string, c ClientCredentials) error {
	_, err := EnsureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return fmt.Errorf("resolve credentials path: %w", err)
	}

	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials json: %w", err)
	}

	b = append(b, '\n')

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit credentials: %w", err)
	}

	return nil
}

func ReadClientCredentials() (ClientCredentials, error) {
	return ReadClientCredentialsFor(DefaultClientName)
}

func ReadClientCredentialsFor(client string) (ClientCredentials, error) {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return ClientCredentials{}, fmt.Errorf("resolve credentials path: %w", err)
	}
	var b []byte

	if b, err = os.ReadFile(path); err != nil { //nolint:gosec // user-provided path
		if os.IsNotExist(err) {
			return ClientCredentials{}, &CredentialsMissingError{Path: path, Cause: err}
		}

		return ClientCredentials{}, fmt.Errorf("read credentials: %w", err)
	}

	var c ClientCredentials
	if err := json.Unmarshal(b, &c); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode credentials: %w", err)
	}

	if c.ClientID == "" || c.ClientSecret == "" {
		return ClientCredentials{}, errMissingClientID
	}

	return c, nil
}

func ClientCredentialsExists(client string) (bool, error) {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("stat credentials: %w", err)
	}

	return true, nil
}

// SameClientCredentials reports whether two stored credential sets match for
// idempotent install (client id/secret/project).
func SameClientCredentials(a, b ClientCredentials) bool {
	return a.ClientID == b.ClientID && a.ClientSecret == b.ClientSecret && a.ProjectID == b.ProjectID
}

type CredentialsMissingError struct {
	Path  string
	Cause error
}

func (e *CredentialsMissingError) Error() string {
	return "oauth credentials missing"
}

func (e *CredentialsMissingError) Unwrap() error {
	return e.Cause
}
