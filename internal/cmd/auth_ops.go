package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

var (
	errNoServicesSelected = errors.New("no services selected")
	errAuthorizedEmail    = errors.New("authorized email mismatch")
)

const (
	authServicesDefault = "user"
	authScopeFull       = "full"
)

// InstallCredentialsResult is the outcome of storing OAuth client credentials.
type InstallCredentialsResult struct {
	Saved     bool
	Path      string
	Client    string
	Replaced  bool
	Identical bool
	ProjectID string
}

// InstallCredentialsOptions controls credential installation.
type InstallCredentialsOptions struct {
	// Client is the normalized client namespace.
	Client string
	// Path is a file path, or "-" for stdin.
	Path string
	// Raw overrides Path when non-nil (for tests / in-memory JSON).
	Raw []byte
	// Domains optionally maps domains to this client.
	Domains string
	// ExpectedProjectID, when set, rejects credentials whose project_id differs.
	ExpectedProjectID string
	// RequireInstalledClient rejects Web OAuth clients for guided setup.
	RequireInstalledClient bool
	// RequireProjectIDConfirmation requires an explicit confirmation or --force
	// before accepting credentials that omit project_id.
	RequireProjectIDConfirmation bool
	// RequireForceToReplace requires Force when replacing non-identical credentials.
	RequireForceToReplace bool
	// Force allows replacing different credentials when RequireForceToReplace is set.
	Force bool
	// Confirm is called before replacing different credentials when interactive.
	// If nil and replacement requires confirmation, the operation fails.
	Confirm func(action string) error
}

// InstallClientCredentials stores OAuth client credentials for a client
// namespace. Standalone `auth credentials` and `auth setup` both use this.
func InstallClientCredentials(opts InstallCredentialsOptions) (InstallCredentialsResult, error) {
	client, err := config.NormalizeClientNameOrDefault(opts.Client)
	if err != nil {
		return InstallCredentialsResult{}, err
	}

	var b []byte
	switch {
	case opts.Raw != nil:
		b = opts.Raw
	case strings.TrimSpace(opts.Path) == "-":
		b, err = io.ReadAll(os.Stdin)
		if err != nil {
			return InstallCredentialsResult{}, err
		}
	case strings.TrimSpace(opts.Path) != "":
		inPath, expandErr := config.ExpandPath(opts.Path)
		if expandErr != nil {
			return InstallCredentialsResult{}, expandErr
		}
		b, err = os.ReadFile(inPath) //nolint:gosec // user-provided path
		if err != nil {
			return InstallCredentialsResult{}, err
		}
	default:
		return InstallCredentialsResult{}, usage("credentials path required")
	}

	parseCredentials := config.ParseGoogleOAuthClientJSON
	if opts.RequireInstalledClient {
		parseCredentials = config.ParseGoogleInstalledOAuthClientJSON
	}
	creds, err := parseCredentials(b)
	if err != nil {
		return InstallCredentialsResult{}, err
	}
	if opts.ExpectedProjectID != "" && creds.ProjectID == "" && opts.RequireProjectIDConfirmation && !opts.Force {
		if opts.Confirm == nil {
			return InstallCredentialsResult{}, usage("credentials omit project_id; re-run with --force or confirm interactively")
		}
		if confirmErr := opts.Confirm(fmt.Sprintf("associate OAuth credentials without project_id with project %q", opts.ExpectedProjectID)); confirmErr != nil {
			return InstallCredentialsResult{}, confirmErr
		}
	}

	if opts.ExpectedProjectID != "" && creds.ProjectID != "" &&
		creds.ProjectID != opts.ExpectedProjectID {
		return InstallCredentialsResult{}, usagef(
			"credentials project_id %q does not match selected project %q",
			creds.ProjectID, opts.ExpectedProjectID,
		)
	}

	exists, err := config.ClientCredentialsExists(client)
	if err != nil {
		return InstallCredentialsResult{}, err
	}
	var replaced, identical bool
	if exists {
		existing, readErr := config.ReadClientCredentialsFor(client)
		switch {
		case readErr == nil && config.SameClientCredentials(existing, creds):
			identical = true
		case opts.RequireForceToReplace:
			if !opts.Force {
				if opts.Confirm == nil {
					return InstallCredentialsResult{}, usagef(
						"refusing to replace credentials for client %q without --force",
						client,
					)
				}
				if confErr := opts.Confirm(fmt.Sprintf("replace OAuth credentials for client %q", client)); confErr != nil {
					return InstallCredentialsResult{}, confErr
				}
			}
			replaced = true
		default:
			// Standalone auth credentials always overwrites (historical behavior).
			replaced = true
		}
	}

	if !identical {
		if err := config.WriteClientCredentialsFor(client, creds); err != nil {
			return InstallCredentialsResult{}, err
		}
		// A standalone overwrite does not prove its project association remains
		// valid. Guided setup will require confirmation again when needed.
		if !opts.RequireInstalledClient {
			cfg, cfgErr := config.ReadConfig()
			if cfgErr != nil {
				return InstallCredentialsResult{}, cfgErr
			}
			if err := config.SetClientSetupCredentialsProjectAssociated(&cfg, client, false); err != nil {
				return InstallCredentialsResult{}, err
			}
			if err := config.WriteConfig(cfg); err != nil {
				return InstallCredentialsResult{}, err
			}
		}
	}

	outPath, _ := config.ClientCredentialsPathFor(client)
	if strings.TrimSpace(opts.Domains) != "" {
		cfg, cfgErr := config.ReadConfig()
		if cfgErr != nil {
			return InstallCredentialsResult{}, cfgErr
		}
		for _, domain := range splitCommaList(opts.Domains) {
			if err := config.SetClientDomain(&cfg, domain, client); err != nil {
				return InstallCredentialsResult{}, err
			}
		}
		if err := config.WriteConfig(cfg); err != nil {
			return InstallCredentialsResult{}, err
		}
	}

	return InstallCredentialsResult{
		Saved:     true,
		Path:      outPath,
		Client:    client,
		Replaced:  replaced,
		Identical: identical,
		ProjectID: creds.ProjectID,
	}, nil
}

// AuthorizeAccountOptions controls first-account (or re-auth) OAuth storage.
type AuthorizeAccountOptions struct {
	Email         string
	Client        string
	ServicesCSV   string
	Manual        bool
	ForceConsent  bool
	ReplaceScopes bool
	Readonly      bool
	DriveScope    string
	// SuppressClientMapping preserves an existing account/domain client mapping.
	// Setup uses this when no explicit --client was supplied.
	SuppressClientMapping bool
	GmailScope            string
}

// AuthorizeAccountResult is returned after a successful token store.
type AuthorizeAccountResult struct {
	Stored   bool
	Email    string
	Services []string
	Client   string
	Scopes   []string
}

// AuthorizeAndStoreAccount runs keychain preflight, scope calculation/merging,
// OAuth, email verification, token storage, and optional named-client mapping.
func AuthorizeAndStoreAccount(ctx context.Context, opts AuthorizeAccountOptions) (AuthorizeAccountResult, error) {
	email := strings.TrimSpace(opts.Email)
	if email == "" {
		return AuthorizeAccountResult{}, usage("empty email")
	}

	override := opts.Client
	if strings.TrimSpace(override) == "" {
		override = authclient.ClientOverrideFromContext(ctx)
	}
	client, err := authclient.ResolveClientWithOverride(email, override)
	if err != nil {
		return AuthorizeAccountResult{}, err
	}

	servicesCSV := opts.ServicesCSV
	if strings.TrimSpace(servicesCSV) == "" {
		servicesCSV = authServicesDefault
	}
	services, err := parseAuthServices(servicesCSV)
	if err != nil {
		return AuthorizeAccountResult{}, err
	}
	if len(services) == 0 {
		return AuthorizeAccountResult{}, errNoServicesSelected
	}

	driveScope := strings.ToLower(strings.TrimSpace(opts.DriveScope))
	if driveScope == "" {
		driveScope = authScopeFull
	}
	gmailScope := strings.ToLower(strings.TrimSpace(opts.GmailScope))
	if gmailScope == "" {
		gmailScope = authScopeFull
	}
	if opts.Readonly && driveScope == strFile {
		return AuthorizeAccountResult{}, usage("cannot combine --readonly with --drive-scope=file (file is write-capable)")
	}

	scopes, err := googleauth.ScopesForManageWithOptions(services, googleauth.ScopeOptions{
		Readonly:   opts.Readonly,
		DriveScope: googleauth.DriveScopeMode(driveScope),
		GmailScope: googleauth.GmailScopeMode(gmailScope),
	})
	if err != nil {
		return AuthorizeAccountResult{}, err
	}

	if keychainErr := ensureKeychainAccessIfNeeded(); keychainErr != nil {
		return AuthorizeAccountResult{}, fmt.Errorf("keychain access: %w", keychainErr)
	}
	store, err := openSecretsStore()
	if err != nil {
		return AuthorizeAccountResult{}, err
	}

	existing, existingErr := store.GetToken(client, email)
	hasExisting := existingErr == nil
	if existingErr != nil && !errors.Is(existingErr, keyring.ErrKeyNotFound) {
		return AuthorizeAccountResult{}, fmt.Errorf("read existing token: %w", existingErr)
	}

	if hasExisting && !opts.ReplaceScopes {
		services, scopes = googleauth.MergeAuthGrant(services, scopes, existing.Services, existing.Scopes)
	}

	serviceNames := authServiceNames(services)
	forceConsent := opts.ForceConsent || opts.ReplaceScopes
	disableIncludeGrantedScopes := forceConsent ||
		opts.Readonly ||
		driveScope == "readonly" ||
		driveScope == strFile ||
		gmailScope == "readonly"

	refreshToken, err := authorizeGoogle(ctx, googleauth.AuthorizeOptions{
		Services:                    services,
		Scopes:                      scopes,
		Manual:                      opts.Manual,
		ForceConsent:                forceConsent,
		DisableIncludeGrantedScopes: disableIncludeGrantedScopes,
		Client:                      client,
	})
	if err != nil {
		return AuthorizeAccountResult{}, err
	}

	authorizedEmail, err := fetchAuthorizedEmail(ctx, client, refreshToken, scopes, 15*time.Second)
	if err != nil {
		return AuthorizeAccountResult{}, fmt.Errorf("fetch authorized email: %w", err)
	}
	if normalizeEmail(authorizedEmail) != normalizeEmail(email) {
		return AuthorizeAccountResult{}, fmt.Errorf("%w: authorized as %s, expected %s", errAuthorizedEmail, authorizedEmail, email)
	}

	if err := store.SetToken(client, authorizedEmail, secrets.Token{
		Client:       client,
		Email:        authorizedEmail,
		Services:     serviceNames,
		Scopes:       scopes,
		RefreshToken: refreshToken,
	}); err != nil {
		return AuthorizeAccountResult{}, err
	}
	if strings.TrimSpace(override) != "" && !opts.SuppressClientMapping {
		cfg, cfgErr := config.ReadConfig()
		if cfgErr != nil {
			return AuthorizeAccountResult{}, cfgErr
		}
		if err := config.SetAccountClient(&cfg, authorizedEmail, client); err != nil {
			return AuthorizeAccountResult{}, err
		}
		if err := config.WriteConfig(cfg); err != nil {
			return AuthorizeAccountResult{}, err
		}
	}

	return AuthorizeAccountResult{
		Stored:   true,
		Email:    authorizedEmail,
		Services: serviceNames,
		Client:   client,
		Scopes:   scopes,
	}, nil
}
