package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

var openSecretsInspector = secrets.OpenReadOnlyDefault

const (
	doctorStatusOK   = "ok"
	doctorStatusWarn = "warn"
	doctorStatusFail = "fail"
	doctorStatusSkip = "skip"

	doctorCheckConfig      = "config"
	doctorCheckKeyring     = "keyring"
	doctorCheckCredentials = "credentials"
	doctorCheckAccounts    = "accounts"

	keyringPasswordEnv = "GOG_KEYRING_PASSWORD" //nolint:gosec // env var name, not a credential
)

// AuthDoctorCmd runs a read-only auth health check.
type AuthDoctorCmd struct {
	Timeout time.Duration `name:"timeout" help:"Per-token check timeout" default:"15s"`
}

type doctorCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
	Recovery string `json:"recovery,omitempty"`
}

type doctorReport struct {
	Status  string        `json:"status"`
	Healthy bool          `json:"healthy"`
	Checks  []doctorCheck `json:"checks"`
}

func (c *AuthDoctorCmd) Run(ctx context.Context) error {
	report := runAuthDoctor(ctx, c.Timeout)
	if err := writeAuthDoctorReport(ctx, report); err != nil {
		return err
	}
	if !report.Healthy {
		return &ExitError{Code: 1, Err: errors.New("auth doctor found problems")}
	}
	return nil
}

func runAuthDoctor(ctx context.Context, timeout time.Duration) doctorReport {
	checks := make([]doctorCheck, 0, 8)
	redactSecrets := make([]string, 0, 8)

	checks = append(checks, checkDoctorConfig())

	backendInfo, backendErr := secrets.ResolveKeyringBackendInfo()
	keyringCheck, inspector, keyringOpenErr := checkDoctorKeyring(backendInfo, backendErr)
	checks = append(checks, keyringCheck)

	var inspections []secrets.TokenInspection
	var listTokensErr error
	if inspector != nil {
		inspections, listTokensErr = inspector.InspectTokens()
		for _, inspection := range inspections {
			if inspection.Err != nil {
				continue
			}
			if token := strings.TrimSpace(inspection.Token.RefreshToken); token != "" {
				redactSecrets = append(redactSecrets, token)
			}
		}
	}

	credCheck, usableCreds, clientSecrets := checkDoctorCredentials(oauthTokenClients(inspections))
	checks = append(checks, credCheck)
	redactSecrets = append(redactSecrets, clientSecrets...)

	// Service-account files are independent of keyring access.
	saEmails, listSAErr := config.ListServiceAccountEmails()

	accountsCheck := checkDoctorAccounts(inspector != nil, keyringOpenErr, inspections, listTokensErr, saEmails, listSAErr)
	checks = append(checks, accountsCheck)

	tokenChecks := checkDoctorTokens(ctx, timeout, inspector != nil, keyringOpenErr, inspections, listTokensErr, saEmails, listSAErr, usableCreds, redactSecrets)
	checks = append(checks, tokenChecks...)

	return finalizeDoctorReport(checks)
}

func checkDoctorConfig() doctorCheck {
	path, err := config.ConfigPath()
	if err != nil {
		return doctorCheck{
			ID:       doctorCheckConfig,
			Status:   doctorStatusFail,
			Summary:  "could not resolve config path",
			Detail:   err.Error(),
			Recovery: "Ensure home/config directories are writable, then re-run gog auth doctor.",
		}
	}

	exists, err := config.ConfigExists()
	if err != nil {
		return doctorCheck{
			ID:       doctorCheckConfig,
			Status:   doctorStatusFail,
			Summary:  "config is not readable",
			Detail:   fmt.Sprintf("path=%s; %s", path, err.Error()),
			Recovery: fmt.Sprintf("Fix permissions on %s or remove a corrupt config file.", path),
		}
	}
	if !exists {
		return doctorCheck{
			ID:      doctorCheckConfig,
			Status:  doctorStatusOK,
			Summary: "config file not present; defaults will be used",
			Detail:  fmt.Sprintf("path=%s", path),
		}
	}

	if _, err := config.ReadConfig(); err != nil {
		return doctorCheck{
			ID:       doctorCheckConfig,
			Status:   doctorStatusFail,
			Summary:  "config exists but could not be parsed",
			Detail:   fmt.Sprintf("path=%s; %s", path, err.Error()),
			Recovery: fmt.Sprintf("Fix or remove the corrupt config at %s.", path),
		}
	}

	return doctorCheck{
		ID:      doctorCheckConfig,
		Status:  doctorStatusOK,
		Summary: "config readable",
		Detail:  fmt.Sprintf("path=%s exists=true", path),
	}
}

func checkDoctorKeyring(backendInfo secrets.KeyringBackendInfo, backendErr error) (doctorCheck, secrets.TokenInspector, error) {
	if backendErr != nil {
		return doctorCheck{
			ID:       doctorCheckKeyring,
			Status:   doctorStatusFail,
			Summary:  "could not resolve keyring backend",
			Detail:   backendErr.Error(),
			Recovery: "Check config.json keyring_backend and GOG_KEYRING_BACKEND (auto|keychain|file).",
		}, nil, backendErr
	}

	detail := fmt.Sprintf("backend=%s source=%s", backendInfo.Value, backendInfo.Source)
	inspector, openErr := openSecretsInspector()
	if openErr != nil {
		if secrets.IsKeyringStorageMissing(openErr) {
			return doctorCheck{
				ID:      doctorCheckKeyring,
				Status:  doctorStatusSkip,
				Summary: "no OAuth keyring storage present",
				Detail:  detail,
			}, nil, nil
		}

		return doctorCheck{
			ID:       doctorCheckKeyring,
			Status:   doctorStatusFail,
			Summary:  "keyring is not accessible",
			Detail:   joinDoctorDetail(detail, openErr.Error()),
			Recovery: keyringRecoveryHint(backendInfo, openErr),
		}, nil, openErr
	}

	// Soft warning for headless file-backend setups without a configured password.
	if backendInfo.Value == strFile {
		if strings.TrimSpace(os.Getenv(keyringPasswordEnv)) == "" && !term.IsTerminal(int(os.Stdin.Fd())) {
			return doctorCheck{
				ID:       doctorCheckKeyring,
				Status:   doctorStatusWarn,
				Summary:  "file keyring accessible, but non-interactive use needs a password",
				Detail:   detail,
				Recovery: fmt.Sprintf("Set %s for non-interactive/file keyring use.", keyringPasswordEnv),
			}, inspector, nil
		}
	}

	return doctorCheck{
		ID:      doctorCheckKeyring,
		Status:  doctorStatusOK,
		Summary: "keyring accessible",
		Detail:  detail,
	}, inspector, nil
}

func keyringRecoveryHint(info secrets.KeyringBackendInfo, openErr error) string {
	msg := openErr.Error()
	switch {
	case secrets.IsKeychainLockedError(msg):
		return "Unlock the macOS login keychain (security unlock-keychain), then re-run gog auth doctor."
	case strings.Contains(msg, keyringPasswordEnv) || strings.Contains(msg, "no TTY"):
		return fmt.Sprintf("Set GOG_KEYRING_BACKEND=file and %s=<password> for headless/file keyring use.", keyringPasswordEnv)
	case strings.Contains(msg, "timed out") || strings.Contains(strings.ToLower(msg), "dbus"):
		return fmt.Sprintf("Secret Service may be unavailable. Set GOG_KEYRING_BACKEND=file and %s=<password>.", keyringPasswordEnv)
	case info.Value == "keychain":
		return "Ensure OS keychain access is allowed, or switch with: gog auth keyring file"
	default:
		return fmt.Sprintf("Check keyring backend (%s). For headless environments: gog auth keyring file and set %s.", info.Value, keyringPasswordEnv)
	}
}

func oauthTokenClients(inspections []secrets.TokenInspection) []string {
	clients := make(map[string]struct{}, len(inspections))
	for _, inspection := range inspections {
		if normalizeEmail(inspection.Email) == "" {
			continue
		}
		client := strings.TrimSpace(inspection.Client)
		if client == "" {
			client = config.DefaultClientName
		}
		clients[client] = struct{}{}
	}

	out := make([]string, 0, len(clients))
	for client := range clients {
		out = append(out, client)
	}
	sort.Strings(out)
	return out
}

func checkDoctorCredentials(requiredClients []string) (doctorCheck, []config.ClientCredentialsInfo, []string) {
	storedCreds, listErr := config.ListClientCredentials()
	if listErr != nil {
		return doctorCheck{
			ID:       doctorCheckCredentials,
			Status:   doctorStatusFail,
			Summary:  "could not list OAuth client credentials",
			Detail:   listErr.Error(),
			Recovery: "Ensure the gog config directory is readable, then re-run gog auth doctor.",
		}, nil, nil
	}

	candidates := make(map[string]config.ClientCredentialsInfo, len(storedCreds)+len(requiredClients))
	for _, info := range storedCreds {
		candidates[info.Client] = info
	}
	for _, client := range requiredClients {
		if _, ok := candidates[client]; ok {
			continue
		}
		path, _ := config.ClientCredentialsPathFor(client)
		candidates[client] = config.ClientCredentialsInfo{
			Client:  client,
			Path:    path,
			Default: client == config.DefaultClientName,
		}
	}

	if len(candidates) == 0 {
		return doctorCheck{
			ID:      doctorCheckCredentials,
			Status:  doctorStatusSkip,
			Summary: "no OAuth client credentials stored or required",
			Detail:  "stored=0 required=0",
		}, nil, nil
	}

	clients := make([]string, 0, len(candidates))
	for client := range candidates {
		clients = append(clients, client)
	}
	sort.Strings(clients)

	usable := make([]config.ClientCredentialsInfo, 0, len(clients))
	usableClients := make([]string, 0, len(clients))
	failures := make([]string, 0)
	secretValues := make([]string, 0, len(clients)*2)
	for _, client := range clients {
		stored, err := config.ReadClientCredentialsFor(client)
		if err != nil {
			failures = append(failures, client)
			continue
		}

		usable = append(usable, candidates[client])
		usableClients = append(usableClients, client)
		secretValues = append(secretValues, stored.ClientSecret, stored.ClientID)
	}

	if len(failures) > 0 {
		recovery := "Replace each unusable credential set with: gog --client <client> auth credentials <credentials.json>"
		if len(failures) == 1 && failures[0] == config.DefaultClientName {
			recovery = "Download a Desktop OAuth client JSON from Google Cloud Console, then run: gog auth credentials <credentials.json>"
		}
		return doctorCheck{
			ID:       doctorCheckCredentials,
			Status:   doctorStatusFail,
			Summary:  fmt.Sprintf("%d of %d OAuth client credential set(s) usable", len(usable), len(clients)),
			Detail:   joinDoctorDetail("usable_clients="+strings.Join(usableClients, ","), "unusable_clients="+strings.Join(failures, ",")),
			Recovery: recovery,
		}, usable, secretValues
	}

	return doctorCheck{
		ID:      doctorCheckCredentials,
		Status:  doctorStatusOK,
		Summary: fmt.Sprintf("%d OAuth client credential set(s) available", len(usable)),
		Detail:  fmt.Sprintf("clients=%s", strings.Join(usableClients, ",")),
	}, usable, secretValues
}

func checkDoctorAccounts(
	keyringOpen bool,
	keyringErr error,
	inspections []secrets.TokenInspection,
	listTokensErr error,
	saEmails []string,
	listSAErr error,
) doctorCheck {
	if listSAErr != nil {
		return doctorCheck{
			ID:       doctorCheckAccounts,
			Status:   doctorStatusFail,
			Summary:  "could not list service-account identities",
			Detail:   listSAErr.Error(),
			Recovery: "Ensure the gog config directory is readable.",
		}
	}

	saSet := make(map[string]struct{}, len(saEmails))
	for _, email := range saEmails {
		if email = normalizeEmail(email); email != "" {
			saSet[email] = struct{}{}
		}
	}

	if !keyringOpen {
		if keyringErr == nil {
			if len(saSet) == 0 {
				return doctorCheck{
					ID:       doctorCheckAccounts,
					Status:   doctorStatusFail,
					Summary:  "no stored accounts",
					Detail:   "oauth=0 service_account=0",
					Recovery: "Authorize an account with gog auth add <email>, or configure a Workspace service account with gog auth service-account set <email> --key <path>.",
				}
			}

			return doctorCheck{
				ID:      doctorCheckAccounts,
				Status:  doctorStatusOK,
				Summary: fmt.Sprintf("%d service-account identity(s) stored", len(saSet)),
				Detail:  fmt.Sprintf("total=%d oauth_only=0 service_account_only=%d both=0", len(saSet), len(saSet)),
			}
		}

		return doctorCheck{
			ID:       doctorCheckAccounts,
			Status:   doctorStatusSkip,
			Summary:  fmt.Sprintf("OAuth account listing skipped; %d service-account identity(s) found", len(saSet)),
			Detail:   joinDoctorDetail(errString(keyringErr), fmt.Sprintf("service_account=%d", len(saSet))),
			Recovery: "Restore keyring access, then re-run gog auth doctor.",
		}
	}
	if listTokensErr != nil {
		return doctorCheck{
			ID:       doctorCheckAccounts,
			Status:   doctorStatusFail,
			Summary:  fmt.Sprintf("could not list stored OAuth tokens; %d service-account identity(s) found", len(saSet)),
			Detail:   joinDoctorDetail(listTokensErr.Error(), fmt.Sprintf("service_account=%d", len(saSet))),
			Recovery: "Restore keyring access or re-import tokens with gog auth tokens import.",
		}
	}

	oauthEmails := make(map[string]struct{}, len(inspections))
	for _, inspection := range inspections {
		if email := normalizeEmail(inspection.Email); email != "" {
			oauthEmails[email] = struct{}{}
		}
	}

	all := make(map[string]struct{}, len(oauthEmails)+len(saSet))
	for email := range oauthEmails {
		all[email] = struct{}{}
	}
	for email := range saSet {
		all[email] = struct{}{}
	}

	if len(all) == 0 {
		return doctorCheck{
			ID:       doctorCheckAccounts,
			Status:   doctorStatusFail,
			Summary:  "no stored accounts",
			Detail:   "oauth=0 service_account=0",
			Recovery: "Authorize an account with gog auth add <email>, or configure a Workspace service account with gog auth service-account set <email> --key <path>.",
		}
	}

	oauthOnly, saOnly, both := 0, 0, 0
	for email := range all {
		_, hasOAuth := oauthEmails[email]
		_, hasSA := saSet[email]
		switch {
		case hasOAuth && hasSA:
			both++
		case hasOAuth:
			oauthOnly++
		case hasSA:
			saOnly++
		}
	}

	return doctorCheck{
		ID:      doctorCheckAccounts,
		Status:  doctorStatusOK,
		Summary: fmt.Sprintf("%d account(s) stored", len(all)),
		Detail:  fmt.Sprintf("total=%d oauth_only=%d service_account_only=%d both=%d", len(all), oauthOnly, saOnly, both),
	}
}

func checkDoctorTokens(
	ctx context.Context,
	timeout time.Duration,
	keyringOpen bool,
	keyringErr error,
	inspections []secrets.TokenInspection,
	listTokensErr error,
	saEmails []string,
	listSAErr error,
	creds []config.ClientCredentialsInfo,
	redactSecrets []string,
) []doctorCheck {
	saSet := make(map[string]struct{}, len(saEmails))
	if listSAErr == nil {
		for _, email := range saEmails {
			if email = normalizeEmail(email); email != "" {
				saSet[email] = struct{}{}
			}
		}
	}

	if !keyringOpen {
		oauthCheck := doctorCheck{
			ID:      "tokens",
			Status:  doctorStatusSkip,
			Summary: "no OAuth keyring storage present",
		}
		if keyringErr != nil {
			oauthCheck.Summary = "skipped OAuth token checks because keyring is inaccessible"
			oauthCheck.Detail = errString(keyringErr)
			oauthCheck.Recovery = "Restore keyring access, then re-run gog auth doctor."
		}

		out := make([]doctorCheck, 0, 1+len(saSet))
		out = append(out, oauthCheck)
		for email := range saSet {
			out = append(out, serviceAccountTokenCheck(email))
		}
		sort.Slice(out[1:], func(i, j int) bool { return out[i+1].ID < out[j+1].ID })
		return out
	}
	if listTokensErr != nil {
		out := make([]doctorCheck, 0, 1+len(saSet))
		out = append(out, doctorCheck{
			ID:      "tokens",
			Status:  doctorStatusSkip,
			Summary: "skipped OAuth token checks because tokens could not be listed",
			Detail:  listTokensErr.Error(),
		})
		for email := range saSet {
			out = append(out, serviceAccountTokenCheck(email))
		}
		sort.Slice(out[1:], func(i, j int) bool { return out[i+1].ID < out[j+1].ID })
		return out
	}

	credClients := make(map[string]struct{}, len(creds))
	for _, info := range creds {
		credClients[info.Client] = struct{}{}
	}

	// Stable order: service-account-only identities first by email, then OAuth tokens by email/client.
	type identity struct {
		email      string
		client     string
		inspection *secrets.TokenInspection
		saOnly     bool
	}
	idents := make([]identity, 0, len(inspections)+len(saSet))
	oauthEmails := make(map[string]struct{}, len(inspections))
	for i := range inspections {
		inspection := &inspections[i]
		email := normalizeEmail(inspection.Email)
		if email == "" {
			continue
		}
		client := strings.TrimSpace(inspection.Client)
		if client == "" {
			client = config.DefaultClientName
		}
		oauthEmails[email] = struct{}{}
		idents = append(idents, identity{email: email, client: client, inspection: inspection})
	}
	for email := range saSet {
		if _, ok := oauthEmails[email]; !ok {
			idents = append(idents, identity{email: email, saOnly: true})
		}
	}
	sort.Slice(idents, func(i, j int) bool {
		if idents[i].email != idents[j].email {
			return idents[i].email < idents[j].email
		}
		if idents[i].saOnly != idents[j].saOnly {
			return idents[i].saOnly
		}
		return idents[i].client < idents[j].client
	})

	if len(idents) == 0 {
		return []doctorCheck{{
			ID:      "tokens",
			Status:  doctorStatusSkip,
			Summary: "no accounts available for token checks",
		}}
	}

	out := make([]doctorCheck, 0, len(idents))
	for _, id := range idents {
		if id.saOnly {
			out = append(out, serviceAccountTokenCheck(id.email))
			continue
		}

		checkID := doctorTokenCheckID(id.email, id.client)
		detail := fmt.Sprintf("email=%s client=%s auth=oauth", id.email, id.client)
		if _, ok := saSet[id.email]; ok {
			detail = fmt.Sprintf("email=%s client=%s auth=oauth+service_account", id.email, id.client)
		}

		if id.inspection.Err != nil {
			out = append(out, doctorCheck{
				ID:       checkID,
				Status:   doctorStatusFail,
				Summary:  "stored OAuth token could not be read",
				Detail:   joinDoctorDetail(detail, "token_data=unreadable"),
				Recovery: fmt.Sprintf("Re-authorize the account: gog auth add %s --force-consent", id.email),
			})
			continue
		}

		if _, ok := credClients[id.client]; !ok {
			path, _ := config.ClientCredentialsPathFor(id.client)
			out = append(out, doctorCheck{
				ID:       checkID,
				Status:   doctorStatusFail,
				Summary:  "OAuth client credentials unavailable for token client",
				Detail:   joinDoctorDetail(detail, fmt.Sprintf("credentials_path=%s", path)),
				Recovery: fmt.Sprintf("Store usable credentials for client %q: gog --client %s auth credentials <credentials.json>", id.client, id.client),
			})
			continue
		}

		tok := id.inspection.Token
		if strings.TrimSpace(tok.RefreshToken) == "" {
			out = append(out, doctorCheck{
				ID:       checkID,
				Status:   doctorStatusFail,
				Summary:  "stored OAuth token is missing a refresh token",
				Detail:   detail,
				Recovery: fmt.Sprintf("Re-authorize: gog auth add %s --force-consent", id.email),
			})
			continue
		}

		if err := checkRefreshToken(ctx, id.client, tok.RefreshToken, tok.Scopes, timeout); err != nil {
			out = append(out, doctorCheck{
				ID:       checkID,
				Status:   doctorStatusFail,
				Summary:  "OAuth refresh token is not usable",
				Detail:   joinDoctorDetail(detail, redactDoctorText(err.Error(), redactSecrets...)),
				Recovery: fmt.Sprintf("Re-authorize the account: gog auth add %s --force-consent", id.email),
			})
			continue
		}

		out = append(out, doctorCheck{
			ID:      checkID,
			Status:  doctorStatusOK,
			Summary: "OAuth refresh token is usable",
			Detail:  detail,
		})
	}
	return out
}

func doctorTokenCheckID(email, client string) string {
	if client == config.DefaultClientName {
		return "token:" + email
	}
	return "token:" + email + ":" + client
}

func serviceAccountTokenCheck(email string) doctorCheck {
	return doctorCheck{
		ID:      "token:" + email,
		Status:  doctorStatusSkip,
		Summary: "service-account identity; OAuth refresh-token check not applicable",
		Detail:  "auth=service_account",
	}
}

func finalizeDoctorReport(checks []doctorCheck) doctorReport {
	status := doctorStatusOK
	healthy := true
	for _, c := range checks {
		switch c.Status {
		case doctorStatusFail:
			status = doctorStatusFail
			healthy = false
		case doctorStatusWarn:
			if status == doctorStatusOK {
				status = doctorStatusWarn
			}
		}
	}
	return doctorReport{
		Status:  status,
		Healthy: healthy,
		Checks:  checks,
	}
}

func writeAuthDoctorReport(ctx context.Context, report doctorReport) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, report)
	}

	u := ui.FromContext(ctx)
	if u == nil {
		return nil
	}

	if outfmt.IsPlain(ctx) {
		// --plain keeps a stable four-column check schema for scripts.
		u.Out().Printf("status\t%s", report.Status)
		u.Out().Printf("healthy\t%t", report.Healthy)
		for _, c := range report.Checks {
			u.Out().Printf("check\t%s\t%s\t%s\t%s",
				sanitizePlainField(c.ID),
				sanitizePlainField(c.Status),
				sanitizePlainField(c.Summary),
				sanitizePlainField(c.Detail),
			)
		}
	} else {
		u.Out().Printf("Auth doctor: %s", strings.ToUpper(report.Status))
		for _, c := range report.Checks {
			u.Out().Printf("[%s] %s: %s", strings.ToUpper(c.Status), c.ID, c.Summary)
			if c.Detail != "" {
				u.Out().Printf("  %s", c.Detail)
			}
		}
	}

	var tips []string
	for _, c := range report.Checks {
		if c.Recovery == "" {
			continue
		}
		if c.Status == doctorStatusFail || c.Status == doctorStatusWarn {
			tips = append(tips, fmt.Sprintf("%s: %s", c.ID, c.Recovery))
		}
	}
	if len(tips) > 0 {
		u.Err().Println("Recovery hints:")
		for _, tip := range tips {
			u.Err().Printf("- %s", tip)
		}
	}
	return nil
}

func joinDoctorDetail(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "; ")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func redactDoctorText(s string, secretValues ...string) string {
	out := s
	// Longest first to avoid partial overlaps leaving remnants.
	sort.SliceStable(secretValues, func(i, j int) bool { return len(secretValues[i]) > len(secretValues[j]) })
	for _, secret := range secretValues {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		if strings.Contains(out, secret) {
			out = strings.ReplaceAll(out, secret, "[redacted]")
		}
	}
	return out
}
