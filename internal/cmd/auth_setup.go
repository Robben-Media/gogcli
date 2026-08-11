package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/gcloud"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/input"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Injectable seams for tests.
var (
	newGCloudClient = func() *gcloud.Client { return gcloud.New(nil) }
	authSetupOpen   = openSecretsStore
)

var errAuthSetupIncomplete = errors.New("auth setup incomplete")

// AuthSetupCmd is the guided, re-runnable first-time Cloud + OAuth setup flow.
type AuthSetupCmd struct {
	Discover bool `name:"discover" help:"Inspect state only; never mutate Cloud or local credentials"`

	// Project selection / creation
	Project       string `name:"project" help:"Existing Google Cloud project ID to use"`
	CreateProject bool   `name:"create-project" help:"Create a new Google Cloud project (requires --project and --force when non-interactive)"`
	ProjectName   string `name:"project-name" help:"Display name for a new project"`
	ProjectParent string `name:"project-parent" help:"Parent for new project: folders/ID or organizations/ID"`

	// APIs / services
	ServicesCSV string `name:"services" help:"Services to enable APIs for: user|all or comma-separated ${auth_services}" default:"user"`
	EnableAPIs  bool   `name:"enable-apis" help:"Enable missing APIs on the selected project (requires --force when non-interactive)"`
	GCloudLogin bool   `name:"gcloud-login" help:"Run gcloud auth login when no active account (interactive only; ignored under --no-input)"`

	// Manual Auth Platform stages
	CredentialsPath string `name:"credentials" help:"Path to downloaded Desktop OAuth client JSON (or '-' for stdin)"`
	AccountEmail    string `name:"email" help:"Email for first-account authorization"`
	ManualOAuth     bool   `name:"manual" help:"Use browserless OAuth paste flow for first-account authorization"`

	// Acknowledgments for Console-only stages (persist as acknowledged, not verified)
	AckBranding   bool `name:"ack-branding" help:"Acknowledge OAuth branding/consent configuration for the selected project"`
	AckAudience   bool `name:"ack-audience" help:"Acknowledge OAuth audience/test users configuration for the selected project"`
	AckDataAccess bool `name:"ack-data-access" help:"Acknowledge OAuth data-access/scopes configuration for the selected project"`
}

// Stage identifiers.
const (
	stageGCloudInstall = "gcloud_install"
	stageGCloudAuth    = "gcloud_auth"
	stageProject       = "project"
	stageAPIs          = "apis"
	stageBranding      = "branding"
	stageAudience      = "audience"
	stageDataAccess    = "data_access"
	stageDesktopClient = "desktop_client"
	stageCredentials   = "credentials"
	stageAccount       = "account"
)

// Stage status values.
const (
	stageStatusOK           = "ok"
	stageStatusMissing      = "missing"
	stageStatusBlocked      = "blocked"
	stageStatusManual       = "manual"
	stageStatusAcknowledged = "acknowledged"
	stageStatusSkipped      = "skipped"
	stageStatusPartial      = "partial"
)

// Action kinds.
const (
	actionNone     = "none"
	actionCommand  = "command"
	actionConsole  = "console"
	actionConfirm  = "confirm"
	actionContinue = "continue"
)

// SetupStage is one inspectable step in the setup report.
type SetupStage struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	ActionKind string `json:"action_kind,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Blocker    string `json:"blocker,omitempty"`
	Resumable  bool   `json:"resumable,omitempty"`
	ConsoleURL string `json:"console_url,omitempty"`
	Command    string `json:"command,omitempty"`
	NextAction string `json:"next_action,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// SetupReport is the structured setup outcome.
type SetupReport struct {
	Complete        bool         `json:"complete"`
	DiscoveryOnly   bool         `json:"discovery_only,omitempty"`
	Client          string       `json:"client"`
	ProjectID       string       `json:"project_id,omitempty"`
	GCloudAccount   string       `json:"gcloud_account,omitempty"`
	Services        []string     `json:"services,omitempty"`
	ServiceUsageIDs []string     `json:"service_usage_ids,omitempty"`
	MissingAPIs     []string     `json:"missing_apis,omitempty"`
	Stages          []SetupStage `json:"stages"`
	Next            string       `json:"next,omitempty"`
	ContinueCmd     string       `json:"continue_command,omitempty"`
}

type setupRuntime struct {
	cmd         *AuthSetupCmd
	flags       *RootFlags
	u           *ui.UI
	gc          *gcloud.Client
	client      string
	interactive bool
	force       bool
	discover    bool
	services    []googleauth.Service
	usageIDs    []string
	serviceCSV  string
	report      SetupReport
	cfg         config.File
	setupRec    config.ClientSetup
}

func (c *AuthSetupCmd) Run(ctx context.Context, flags *RootFlags) error {
	rt, err := c.newSetupRuntime(ctx, flags)
	if err != nil {
		return err
	}

	if stop, runErr := rt.runGCloudInstall(ctx); stop {
		return runErr
	}
	if stop, runErr := rt.runGCloudAuth(ctx); stop {
		return runErr
	}
	if stop, runErr := rt.runProject(ctx); stop {
		return runErr
	}
	if stop, runErr := rt.runAPIs(ctx); stop {
		return runErr
	}
	if runErr := rt.runManualStages(ctx); runErr != nil {
		return runErr
	}
	if stop, runErr := rt.runCredentials(ctx); stop {
		return runErr
	}
	if stop, runErr := rt.runAccount(ctx); stop {
		return runErr
	}

	return emitSetupReport(ctx, rt.u, rt.flags, rt.report)
}

func (c *AuthSetupCmd) newSetupRuntime(ctx context.Context, flags *RootFlags) (*setupRuntime, error) {
	u := ui.FromContext(ctx)
	client, err := normalizeClientForFlag(authclient.ClientOverrideFromContext(ctx))
	if err != nil {
		return nil, err
	}

	interactive := flags != nil && !flags.NoInput && term.IsTerminal(int(os.Stdin.Fd()))
	force := flags != nil && flags.Force
	discover := c.Discover

	services, err := parseAuthServices(c.ServicesCSV)
	if err != nil {
		return nil, err
	}

	usageIDs, err := googleauth.ServiceUsageIDsForServices(services)
	if err != nil {
		return nil, err
	}

	if valErr := c.validateNonInteractive(interactive, force, discover); valErr != nil {
		return nil, valErr
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		return nil, err
	}

	setupRec := config.GetClientSetup(cfg, client)
	projectID := strings.TrimSpace(c.Project)
	if projectID == "" {
		projectID = strings.TrimSpace(setupRec.ProjectID)
	}

	return &setupRuntime{
		cmd:         c,
		flags:       flags,
		u:           u,
		gc:          newGCloudClient(),
		client:      client,
		interactive: interactive,
		force:       force,
		discover:    discover,
		services:    services,
		usageIDs:    usageIDs,
		serviceCSV:  c.ServicesCSV,
		cfg:         cfg,
		setupRec:    setupRec,
		report: SetupReport{
			DiscoveryOnly:   discover,
			Client:          client,
			ProjectID:       projectID,
			Services:        authServiceNames(services),
			ServiceUsageIDs: usageIDs,
		},
	}, nil
}

func (c *AuthSetupCmd) validateNonInteractive(interactive, force, discover bool) error {
	if interactive {
		return nil
	}
	if c.GCloudLogin {
		return usage("--gcloud-login requires an interactive TTY (omit under --no-input)")
	}
	if c.CreateProject && strings.TrimSpace(c.Project) == "" {
		return usage("--create-project requires --project <id>")
	}
	if c.CreateProject && !force && !discover {
		return usage("--create-project requires --force in non-interactive mode")
	}
	if c.EnableAPIs && !force && !discover {
		return usage("--enable-apis requires --force in non-interactive mode")
	}

	return nil
}

func (rt *setupRuntime) appendStage(stages ...SetupStage) {
	rt.report.Stages = append(rt.report.Stages, stages...)
}

func (rt *setupRuntime) emit(ctx context.Context) error {
	return emitSetupReport(ctx, rt.u, rt.flags, rt.report)
}

func (rt *setupRuntime) continueCmd(projectID string) string {
	return continueSetupCmd(rt.client, rt.cmd, projectID, rt.force)
}

func (rt *setupRuntime) runGCloudInstall(ctx context.Context) (stop bool, err error) {
	installed, installRes := rt.gc.Installed(ctx)
	if !installed {
		rt.appendStage(SetupStage{
			ID:         stageGCloudInstall,
			Status:     stageStatusMissing,
			ActionKind: actionConsole,
			Summary:    "gcloud CLI not found",
			Blocker:    firstNonEmpty(installRes.Stderr, "install Google Cloud SDK (gcloud)"),
			Resumable:  true,
			ConsoleURL: "https://cloud.google.com/sdk/docs/install",
			NextAction: "Install gcloud, then re-run gog auth setup",
			Command:    rt.continueCmd(rt.report.ProjectID),
		})

		return true, rt.emit(ctx)
	}

	rt.appendStage(SetupStage{
		ID:         stageGCloudInstall,
		Status:     stageStatusOK,
		Summary:    "gcloud CLI available",
		ActionKind: actionNone,
	})

	return false, nil
}

func (rt *setupRuntime) runGCloudAuth(ctx context.Context) (stop bool, err error) {
	acct, acctRes, acctErr := rt.gc.ActiveAccount(ctx)
	if acctErr != nil && acctRes.Kind == gcloud.BlockerNotLoggedIn {
		acctErr = nil
	}
	if acctErr != nil {
		rt.appendStage(SetupStage{
			ID:         stageGCloudAuth,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "failed to inspect gcloud auth",
			Blocker:    acctErr.Error(),
			Resumable:  true,
			Command:    "gcloud auth list",
			NextAction: "Fix gcloud authentication, then re-run",
		})

		return true, rt.emit(ctx)
	}

	if acct.Account == "" {
		return rt.handleMissingGCloudAccount(ctx)
	}

	rt.report.GCloudAccount = acct.Account
	rt.appendStage(SetupStage{
		ID:         stageGCloudAuth,
		Status:     stageStatusOK,
		Summary:    "gcloud account " + acct.Account,
		ActionKind: actionNone,
		Detail:     acct.Account,
	})

	return false, nil
}

func (rt *setupRuntime) handleMissingGCloudAccount(ctx context.Context) (stop bool, err error) {
	if !(rt.cmd.GCloudLogin && rt.interactive && !rt.discover) {
		rt.appendStage(SetupStage{
			ID:         stageGCloudAuth,
			Status:     stageStatusMissing,
			ActionKind: actionCommand,
			Summary:    "no active gcloud account",
			Blocker:    "run gcloud auth login (or re-run with --gcloud-login interactively)",
			Resumable:  true,
			Command:    "gcloud auth login",
			NextAction: "Authenticate gcloud, then re-run gog auth setup",
		})

		return true, rt.emit(ctx)
	}

	rt.u.Err().Println("Running gcloud auth login (does not change gcloud default project/config beyond auth)…")
	loginRes := rt.gc.Login(ctx)
	if loginRes.ExitCode != 0 {
		rt.appendStage(SetupStage{
			ID:         stageGCloudAuth,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "gcloud auth login failed",
			Blocker:    firstNonEmpty(loginRes.Stderr, "gcloud auth login failed"),
			Resumable:  true,
			Command:    "gcloud auth login",
		})

		return true, rt.emit(ctx)
	}

	acct, _, acctErr := rt.gc.ActiveAccount(ctx)
	if acctErr != nil || acct.Account == "" {
		rt.appendStage(SetupStage{
			ID:         stageGCloudAuth,
			Status:     stageStatusMissing,
			ActionKind: actionCommand,
			Summary:    "still not logged in to gcloud",
			Blocker:    "no active gcloud account after login",
			Resumable:  true,
			Command:    "gcloud auth login",
		})

		return true, rt.emit(ctx)
	}

	rt.report.GCloudAccount = acct.Account
	rt.appendStage(SetupStage{
		ID:         stageGCloudAuth,
		Status:     stageStatusOK,
		Summary:    "gcloud account " + acct.Account,
		ActionKind: actionNone,
		Detail:     acct.Account,
	})

	return false, nil
}

func (rt *setupRuntime) runProject(ctx context.Context) (stop bool, err error) {
	projects, _, listErr := rt.gc.ListProjects(ctx)
	if listErr != nil {
		rt.appendStage(SetupStage{
			ID:         stageProject,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "failed to list projects",
			Blocker:    listErr.Error(),
			Resumable:  true,
		})

		return true, rt.emit(ctx)
	}

	projectID := rt.report.ProjectID
	if rt.cmd.CreateProject && !rt.discover {
		createdID, created, createErr := rt.createProject(ctx)
		if createErr != nil {
			return true, createErr
		}
		if !created {
			return true, nil
		}

		projectID = createdID
		rt.report.ProjectID = projectID
	}

	if projectID == "" && rt.interactive && !rt.discover {
		picked, pickErr := rt.pickProjectInteractive(ctx, projects)
		if pickErr != nil {
			return true, pickErr
		}
		if picked == "" {
			return true, nil
		}

		projectID = picked
		rt.report.ProjectID = projectID
	}

	if projectID == "" {
		active, _, _ := rt.gc.ActiveProjectID(ctx)
		detail := ""
		if active != "" {
			detail = "gcloud active project (not modified): " + active
		}

		rt.appendStage(SetupStage{
			ID:         stageProject,
			Status:     stageStatusMissing,
			ActionKind: actionCommand,
			Summary:    "no project selected",
			Blocker:    "pass --project <id> or run interactively",
			Resumable:  true,
			Detail:     detail,
			Command:    rt.continueCmd(""),
			NextAction: "Re-run with --project <id>",
		})

		return true, rt.emit(ctx)
	}

	if !rt.discover {
		if err := config.SetClientSetupProject(&rt.cfg, rt.client, projectID); err != nil {
			return true, err
		}
		if err := config.WriteConfig(rt.cfg); err != nil {
			return true, err
		}

		rt.setupRec = config.GetClientSetup(rt.cfg, rt.client)
	}

	rt.report.ProjectID = projectID
	rt.appendStage(SetupStage{
		ID:         stageProject,
		Status:     stageStatusOK,
		Summary:    "project " + projectID,
		ActionKind: actionNone,
		Detail:     projectID,
		ConsoleURL: projectConsoleURL(projectID, ""),
	})

	return false, nil
}

func (rt *setupRuntime) createProject(ctx context.Context) (projectID string, created bool, err error) {
	if strings.TrimSpace(rt.cmd.Project) == "" {
		return "", false, usage("--create-project requires --project <id>")
	}
	if !rt.force && rt.interactive {
		if confErr := confirmDestructive(ctx, rt.flags, fmt.Sprintf("create Google Cloud project %q", rt.cmd.Project)); confErr != nil {
			return "", false, confErr
		}
	} else if !rt.force {
		return "", false, usage("--create-project requires --force in non-interactive mode")
	}

	rt.u.Err().Printf("Creating project %s…\n", rt.cmd.Project)
	createdProj, _, createErr := rt.gc.CreateProject(ctx, rt.cmd.Project, rt.cmd.ProjectName, rt.cmd.ProjectParent)
	if createErr != nil {
		rt.appendStage(SetupStage{
			ID:         stageProject,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "project creation failed",
			Blocker:    createErr.Error(),
			Resumable:  true,
			Command:    rt.continueCmd(rt.cmd.Project),
		})

		return "", false, rt.emit(ctx)
	}

	return createdProj.ProjectID, true, nil
}

func (rt *setupRuntime) pickProjectInteractive(ctx context.Context, projects []gcloud.Project) (string, error) {
	if len(projects) == 0 {
		rt.appendStage(SetupStage{
			ID:         stageProject,
			Status:     stageStatusMissing,
			ActionKind: actionCommand,
			Summary:    "no Cloud projects found",
			Blocker:    "create a project with --create-project --project <id> --force",
			Resumable:  true,
			ConsoleURL: "https://console.cloud.google.com/projectcreate",
			Command:    fmt.Sprintf("gog --client %s auth setup --create-project --project <id> --force", rt.client),
		})

		return "", rt.emit(ctx)
	}

	rt.u.Err().Println("Select a Google Cloud project:")
	for i, p := range projects {
		label := p.ProjectID
		if p.Name != "" && p.Name != p.ProjectID {
			label = fmt.Sprintf("%s (%s)", p.ProjectID, p.Name)
		}

		rt.u.Err().Printf("  %d) %s\n", i+1, label)
	}

	line, readErr := input.PromptLine(ctx, "Project number: ")
	if readErr != nil {
		return "", readErr
	}

	var idx int
	if _, scanErr := fmt.Sscanf(strings.TrimSpace(line), "%d", &idx); scanErr != nil || idx < 1 || idx > len(projects) {
		return "", usage("invalid project selection")
	}

	return projects[idx-1].ProjectID, nil
}

func (rt *setupRuntime) runAPIs(ctx context.Context) (stop bool, err error) {
	projectID := rt.report.ProjectID
	missing, _, _, missErr := rt.gc.MissingServices(ctx, projectID, rt.usageIDs)
	if missErr != nil {
		rt.appendStage(SetupStage{
			ID:         stageAPIs,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "failed to inspect enabled APIs",
			Blocker:    missErr.Error(),
			Resumable:  true,
			ConsoleURL: projectConsoleURL(projectID, "apis/dashboard"),
		})

		return true, rt.emit(ctx)
	}

	rt.report.MissingAPIs = missing
	if len(missing) > 0 && rt.cmd.EnableAPIs && !rt.discover {
		enabledMissing, enableErr := rt.enableMissingAPIs(ctx, projectID, missing)
		if enableErr != nil {
			return true, enableErr
		}

		missing = enabledMissing
		rt.report.MissingAPIs = missing
	}

	if len(missing) > 0 {
		rt.appendStage(SetupStage{
			ID:         stageAPIs,
			Status:     stageStatusMissing,
			ActionKind: actionCommand,
			Summary:    fmt.Sprintf("%d API(s) not enabled", len(missing)),
			Blocker:    "enable missing APIs",
			Detail:     strings.Join(missing, ","),
			Resumable:  true,
			ConsoleURL: projectConsoleURL(projectID, "apis/library"),
			Command: fmt.Sprintf(
				"gog --client %s --force auth setup --project %s --enable-apis --services %s",
				rt.client, projectID, rt.serviceCSV,
			),
			NextAction: "Re-run with --enable-apis --force",
		})

		return false, nil
	}

	rt.appendStage(SetupStage{
		ID:         stageAPIs,
		Status:     stageStatusOK,
		Summary:    "required APIs enabled",
		ActionKind: actionNone,
	})

	return false, nil
}

func (rt *setupRuntime) enableMissingAPIs(ctx context.Context, projectID string, missing []string) ([]string, error) {
	if !rt.force && rt.interactive {
		if confErr := confirmDestructive(ctx, rt.flags, fmt.Sprintf("enable %d API(s) on project %s", len(missing), projectID)); confErr != nil {
			return nil, confErr
		}
	} else if !rt.force {
		return nil, usage("--enable-apis requires --force in non-interactive mode")
	}

	rt.u.Err().Printf("Enabling %d API(s) on %s…\n", len(missing), projectID)
	_, stillMissing, _, enableErr := rt.gc.EnableServices(ctx, projectID, missing)
	if enableErr != nil {
		rt.appendStage(SetupStage{
			ID:         stageAPIs,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "API enablement failed",
			Blocker:    enableErr.Error(),
			Resumable:  true,
			Detail:     strings.Join(missing, ","),
			ConsoleURL: projectConsoleURL(projectID, "apis/library"),
			Command:    rt.continueCmd(projectID),
		})

		return nil, rt.emit(ctx)
	}

	return stillMissing, nil
}

func (rt *setupRuntime) runManualStages(ctx context.Context) error {
	projectID := rt.report.ProjectID

	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}

	rt.cfg = cfg
	rt.setupRec = config.GetClientSetup(cfg, rt.client)

	if !rt.discover {
		if rt.cmd.AckBranding || rt.cmd.AckAudience || rt.cmd.AckDataAccess {
			if err := config.SetClientSetupAcknowledgments(&rt.cfg, rt.client, rt.cmd.AckBranding, rt.cmd.AckAudience, rt.cmd.AckDataAccess); err != nil {
				return err
			}
			if err := config.WriteConfig(rt.cfg); err != nil {
				return err
			}

			rt.setupRec = config.GetClientSetup(rt.cfg, rt.client)
		} else if rt.interactive {
			setupRec, promptErr := maybePromptManualAcks(ctx, rt.u, rt.flags, rt.client, projectID, rt.setupRec)
			if promptErr != nil {
				return promptErr
			}

			rt.setupRec = setupRec
		}
	}

	rt.appendStage(
		manualStage(stageBranding, "OAuth branding / consent screen", rt.setupRec.AcknowledgedBranding,
			projectConsoleURL(projectID, "auth/branding"),
			fmt.Sprintf("gog --client %s auth setup --project %s --ack-branding", rt.client, projectID)),
		manualStage(stageAudience, "OAuth audience / test users", rt.setupRec.AcknowledgedAudience,
			projectConsoleURL(projectID, "auth/audience"),
			fmt.Sprintf("gog --client %s auth setup --project %s --ack-audience", rt.client, projectID)),
		manualStage(stageDataAccess, "OAuth data access / scopes", rt.setupRec.AcknowledgedDataAccess,
			projectConsoleURL(projectID, "auth/scopes"),
			fmt.Sprintf("gog --client %s auth setup --project %s --ack-data-access", rt.client, projectID)),
		SetupStage{
			ID:         stageDesktopClient,
			Status:     stageStatusManual,
			ActionKind: actionConsole,
			Summary:    "create Desktop OAuth client in Google Auth Platform",
			Blocker:    "OAuth client creation is Console-only (gog does not use IAM/IAP OAuth client APIs)",
			Resumable:  true,
			ConsoleURL: projectConsoleURL(projectID, "auth/clients"),
			NextAction: "Create a Desktop app OAuth client, download JSON, then pass --credentials",
			Command:    fmt.Sprintf("gog --client %s auth setup --project %s --credentials ~/Downloads/client_secret.json", rt.client, projectID),
		},
	)

	return nil
}

func (rt *setupRuntime) runCredentials(ctx context.Context) (stop bool, err error) {
	projectID := rt.report.ProjectID
	credsExist, _ := config.ClientCredentialsExists(rt.client)

	if strings.TrimSpace(rt.cmd.CredentialsPath) != "" && !rt.discover {
		return rt.installCredentials(ctx, projectID)
	}
	if credsExist {
		rt.appendCredentialsExisting(projectID)

		return false, nil
	}

	rt.appendStage(SetupStage{
		ID:         stageCredentials,
		Status:     stageStatusMissing,
		ActionKind: actionCommand,
		Summary:    "OAuth client credentials not installed",
		Blocker:    "download Desktop client JSON and pass --credentials",
		Resumable:  true,
		Command:    fmt.Sprintf("gog --client %s auth setup --project %s --credentials <path>", rt.client, projectID),
		ConsoleURL: projectConsoleURL(projectID, "auth/clients"),
	})

	return false, nil
}

func (rt *setupRuntime) installCredentials(ctx context.Context, projectID string) (stop bool, err error) {
	confirmFn := func(action string) error {
		return confirmDestructive(ctx, rt.flags, action)
	}
	result, instErr := InstallClientCredentials(InstallCredentialsOptions{
		Client:                rt.client,
		Path:                  rt.cmd.CredentialsPath,
		ExpectedProjectID:     projectID,
		RequireForceToReplace: true,
		Force:                 rt.force,
		Confirm:               confirmFn,
	})
	if instErr != nil {
		rt.appendStage(SetupStage{
			ID:         stageCredentials,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "credential install failed",
			Blocker:    instErr.Error(),
			Resumable:  true,
		})

		return true, rt.emit(ctx)
	}

	summary := "credentials installed"
	switch {
	case result.Identical:
		summary = "credentials already installed (identical)"
	case result.Replaced:
		summary = "credentials replaced"
	}

	rt.appendStage(SetupStage{
		ID:         stageCredentials,
		Status:     stageStatusOK,
		Summary:    summary,
		ActionKind: actionNone,
		Detail:     result.Path,
	})

	return false, nil
}

func (rt *setupRuntime) appendCredentialsExisting(projectID string) {
	stored, readErr := config.ReadClientCredentialsFor(rt.client)
	if readErr == nil && stored.ProjectID != "" && stored.ProjectID != projectID {
		rt.appendStage(SetupStage{
			ID:         stageCredentials,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "stored credentials project mismatch",
			Blocker:    fmt.Sprintf("credentials project_id %q != selected %q", stored.ProjectID, projectID),
			Resumable:  true,
			Command:    fmt.Sprintf("gog --client %s --force auth setup --project %s --credentials <new.json>", rt.client, projectID),
		})

		return
	}

	rt.appendStage(SetupStage{
		ID:         stageCredentials,
		Status:     stageStatusOK,
		Summary:    "credentials present",
		ActionKind: actionNone,
	})
}

func (rt *setupRuntime) runAccount(ctx context.Context) (stop bool, err error) {
	email := strings.TrimSpace(rt.cmd.AccountEmail)
	if email != "" && !rt.discover {
		return rt.authorizeAccount(ctx, email)
	}

	if clientHasToken(rt.client) {
		rt.appendStage(SetupStage{
			ID:         stageAccount,
			Status:     stageStatusOK,
			Summary:    "at least one account token present for client",
			ActionKind: actionNone,
		})

		return false, nil
	}

	rt.appendStage(SetupStage{
		ID:         stageAccount,
		Status:     stageStatusMissing,
		ActionKind: actionCommand,
		Summary:    "no authorized account for client",
		Blocker:    "authorize with --email <email> or gog auth add",
		Resumable:  true,
		Command:    fmt.Sprintf("gog --client %s auth setup --project %s --email you@example.com", rt.client, rt.report.ProjectID),
	})

	return false, nil
}

func (rt *setupRuntime) authorizeAccount(ctx context.Context, email string) (stop bool, err error) {
	credsExist, _ := config.ClientCredentialsExists(rt.client)
	if !credsExist {
		rt.appendStage(SetupStage{
			ID:         stageAccount,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "cannot authorize account without credentials",
			Blocker:    "install credentials first",
			Resumable:  true,
		})

		return true, rt.emit(ctx)
	}

	if !rt.interactive {
		status := stageStatusManual
		summary := "account authorization deferred (non-interactive)"
		blocker := "run auth add interactively"
		if rt.cmd.ManualOAuth {
			status = stageStatusBlocked
			summary = "account authorization requires interactive input"
			blocker = "omit --no-input for OAuth, or complete with: gog auth add"
		}

		rt.appendStage(SetupStage{
			ID:         stageAccount,
			Status:     status,
			ActionKind: actionCommand,
			Summary:    summary,
			Blocker:    blocker,
			Resumable:  true,
			Command:    fmt.Sprintf("gog --client %s auth add %s --services %s", rt.client, email, rt.serviceCSV),
		})

		return false, nil
	}

	rt.u.Err().Printf("Authorizing %s…\n", email)
	result, authErr := AuthorizeAndStoreAccount(ctx, AuthorizeAccountOptions{
		Email:       email,
		Client:      rt.client,
		ServicesCSV: rt.serviceCSV,
		Manual:      rt.cmd.ManualOAuth,
	})
	if authErr != nil {
		rt.appendStage(SetupStage{
			ID:         stageAccount,
			Status:     stageStatusBlocked,
			ActionKind: actionCommand,
			Summary:    "account authorization failed",
			Blocker:    authErr.Error(),
			Resumable:  true,
			Command:    fmt.Sprintf("gog --client %s auth add %s --services %s", rt.client, email, rt.serviceCSV),
		})

		return true, rt.emit(ctx)
	}

	rt.appendStage(SetupStage{
		ID:         stageAccount,
		Status:     stageStatusOK,
		Summary:    "account authorized: " + result.Email,
		ActionKind: actionNone,
		Detail:     result.Email,
	})

	return false, nil
}

func clientHasToken(client string) bool {
	store, storeErr := authSetupOpen()
	if storeErr != nil {
		return false
	}

	tokens, listTokErr := store.ListTokens()
	if listTokErr != nil {
		return false
	}

	for _, tok := range tokens {
		sameClient := tok.Client == client || (client == config.DefaultClientName && tok.Client == "")
		if sameClient && strings.TrimSpace(tok.Email) != "" {
			return true
		}
	}

	return false
}

func manualStage(id, summary string, acked bool, consoleURL, continueCmd string) SetupStage {
	if acked {
		return SetupStage{
			ID:         id,
			Status:     stageStatusAcknowledged,
			ActionKind: actionNone,
			Summary:    summary + " (acknowledged; not Google-verified)",
			Detail:     "acknowledged",
			ConsoleURL: consoleURL,
		}
	}

	return SetupStage{
		ID:         id,
		Status:     stageStatusManual,
		ActionKind: actionConsole,
		Summary:    summary,
		Blocker:    "complete in Google Cloud Console, then acknowledge",
		Resumable:  true,
		ConsoleURL: consoleURL,
		Command:    continueCmd,
		NextAction: "Open Console URL, complete step, re-run with ack flag",
	}
}

func maybePromptManualAcks(ctx context.Context, u *ui.UI, flags *RootFlags, client, projectID string, rec config.ClientSetup) (config.ClientSetup, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return rec, err
	}

	changed := false
	prompt := func(label string, already bool) (bool, error) {
		if already {
			return true, nil
		}

		u.Err().Printf("%s: %s\n", label, projectConsoleURL(projectID, ""))
		if flags.NoInput || !term.IsTerminal(int(os.Stdin.Fd())) {
			return false, nil
		}

		line, promptErr := input.PromptLine(ctx, fmt.Sprintf("Mark %s as done for this project? [y/N]: ", label))
		if promptErr != nil {
			if errors.Is(promptErr, os.ErrClosed) {
				return false, nil
			}

			return false, promptErr
		}

		ans := strings.TrimSpace(strings.ToLower(line))

		//nolint:goconst // confirmation answer, not send-as value
		return ans == "y" || ans == "yes", nil
	}

	if ok, promptErr := prompt("OAuth branding/consent", rec.AcknowledgedBranding); promptErr != nil {
		return rec, promptErr
	} else if ok && !rec.AcknowledgedBranding {
		rec.AcknowledgedBranding = true
		changed = true
	}

	if ok, promptErr := prompt("OAuth audience/test users", rec.AcknowledgedAudience); promptErr != nil {
		return rec, promptErr
	} else if ok && !rec.AcknowledgedAudience {
		rec.AcknowledgedAudience = true
		changed = true
	}

	if ok, promptErr := prompt("OAuth data access/scopes", rec.AcknowledgedDataAccess); promptErr != nil {
		return rec, promptErr
	} else if ok && !rec.AcknowledgedDataAccess {
		rec.AcknowledgedDataAccess = true
		changed = true
	}

	if !changed {
		return rec, nil
	}

	if err := config.SetClientSetupAcknowledgments(&cfg, client, rec.AcknowledgedBranding, rec.AcknowledgedAudience, rec.AcknowledgedDataAccess); err != nil {
		return rec, err
	}
	if err := config.SetClientSetupProject(&cfg, client, projectID); err != nil {
		return rec, err
	}
	// Re-apply acks after project set (project set may reset if different — same project keeps).
	if err := config.SetClientSetupAcknowledgments(&cfg, client, rec.AcknowledgedBranding, rec.AcknowledgedAudience, rec.AcknowledgedDataAccess); err != nil {
		return rec, err
	}
	if err := config.WriteConfig(cfg); err != nil {
		return rec, err
	}

	return config.GetClientSetup(cfg, client), nil
}

func projectConsoleURL(projectID, path string) string {
	base := "https://console.cloud.google.com"
	if path == "" {
		return fmt.Sprintf("%s/home/dashboard?project=%s", base, projectID)
	}

	return fmt.Sprintf("%s/%s?project=%s", base, strings.TrimPrefix(path, "/"), projectID)
}

func continueSetupCmd(client string, c *AuthSetupCmd, projectID string, force bool) string {
	parts := []string{"gog"}
	if client != "" && client != config.DefaultClientName {
		parts = append(parts, "--client", client)
	}
	if force {
		parts = append(parts, "--force")
	}

	parts = append(parts, "auth", "setup")
	switch {
	case projectID != "":
		parts = append(parts, "--project", projectID)
	case strings.TrimSpace(c.Project) != "":
		parts = append(parts, "--project", c.Project)
	}
	if c.Discover {
		parts = append(parts, "--discover")
	}

	return strings.Join(parts, " ")
}

func emitSetupReport(ctx context.Context, u *ui.UI, flags *RootFlags, report SetupReport) error {
	_ = flags
	report.Complete = setupComplete(report)
	report.Next = firstBlockingNext(report)
	report.ContinueCmd = firstContinueCmd(report)

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(os.Stdout, report); err != nil {
			return err
		}
		if report.Complete {
			return nil
		}

		return &ExitError{Code: 1, Err: errAuthSetupIncomplete}
	}

	if outfmt.IsPlain(ctx) {
		emitPlainSetupReport(ctx, report)
	} else {
		emitHumanSetupReport(u, report)
	}

	if report.Complete {
		return nil
	}

	return &ExitError{Code: 1, Err: errAuthSetupIncomplete}
}

func emitPlainSetupReport(ctx context.Context, report SetupReport) {
	w, flush := tableWriter(ctx)
	defer flush()
	writeTableRow(ctx, w, []string{"STAGE", "STATUS", "SUMMARY", "BLOCKER", "COMMAND"})
	for _, st := range report.Stages {
		writeTableRow(ctx, w, []string{st.ID, st.Status, st.Summary, st.Blocker, st.Command})
	}

	writeTableRow(ctx, w, []string{"meta", "complete", fmt.Sprintf("%t", report.Complete), report.ProjectID, report.ContinueCmd})
}

func emitHumanSetupReport(u *ui.UI, report SetupReport) {
	u.Out().Printf("client\t%s", report.Client)
	if report.ProjectID != "" {
		u.Out().Printf("project\t%s", report.ProjectID)
	}
	if report.GCloudAccount != "" {
		u.Out().Printf("gcloud_account\t%s", report.GCloudAccount)
	}

	u.Out().Printf("complete\t%t", report.Complete)
	for _, st := range report.Stages {
		u.Out().Println(fmt.Sprintf("stage\t%s\t%s\t%s", st.ID, st.Status, st.Summary))
		if st.Blocker != "" {
			u.Err().Printf("  blocker: %s\n", st.Blocker)
		}
		if st.ConsoleURL != "" {
			u.Err().Printf("  console: %s\n", st.ConsoleURL)
		}
		if st.Command != "" {
			u.Err().Printf("  continue: %s\n", st.Command)
		}
	}
	if report.Next != "" {
		u.Err().Printf("next: %s\n", report.Next)
	}
}

func setupComplete(r SetupReport) bool {
	// Discovery-only is successful once a report is produced.
	if r.DiscoveryOnly {
		return true
	}

	needOK := map[string]bool{
		stageGCloudInstall: true,
		stageGCloudAuth:    true,
		stageProject:       true,
		stageAPIs:          true,
		stageCredentials:   true,
		stageAccount:       true,
	}
	// Manual stages: acknowledged counts as done for completion.
	needAck := map[string]bool{
		stageBranding:   true,
		stageAudience:   true,
		stageDataAccess: true,
	}
	// Desktop client is guidance-only; completion requires credentials present
	// (proxy that a desktop client was created and downloaded).
	byID := make(map[string]SetupStage, len(r.Stages))
	for _, st := range r.Stages {
		byID[st.ID] = st
	}
	for id := range needOK {
		st, ok := byID[id]
		if !ok || st.Status != stageStatusOK {
			return false
		}
	}
	for id := range needAck {
		st, ok := byID[id]
		if !ok || (st.Status != stageStatusAcknowledged && st.Status != stageStatusOK) {
			return false
		}
	}

	return true
}

func firstBlockingNext(r SetupReport) string {
	priority := []string{
		stageGCloudInstall, stageGCloudAuth, stageProject, stageAPIs,
		stageBranding, stageAudience, stageDataAccess, stageDesktopClient,
		stageCredentials, stageAccount,
	}
	byID := make(map[string]SetupStage, len(r.Stages))
	for _, st := range r.Stages {
		byID[st.ID] = st
	}
	for _, id := range priority {
		st, ok := byID[id]
		if !ok {
			continue
		}
		switch st.Status {
		case stageStatusOK, stageStatusAcknowledged, stageStatusSkipped:
			continue
		default:
			if st.NextAction != "" {
				return st.NextAction
			}
			if st.Blocker != "" {
				return st.Blocker
			}

			return st.Summary
		}
	}

	return ""
}

func firstContinueCmd(r SetupReport) string {
	for _, st := range r.Stages {
		switch st.Status {
		case stageStatusOK, stageStatusAcknowledged, stageStatusSkipped:
			continue
		}
		if st.Command != "" {
			return st.Command
		}
	}

	return ""
}
