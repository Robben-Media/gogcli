// Package gcloud provides a narrow, injectable wrapper around the gcloud CLI
// for guided auth setup. It never mutates global gcloud config, never runs
// Application Default Credentials login, and always scopes project operations
// with an explicit project ID.
package gcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var (
	errProjectIDRequired = errors.New("project id required")
	errProjectListLimit  = errors.New("project list limit must be positive")
	errGCloudCommand     = errors.New("gcloud command failed")
	errGCloudParse       = errors.New("parse gcloud output")
)

// Runner executes a gcloud-style command. Tests inject a fake implementation.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// InteractiveRunner executes commands that require a human terminal. Its output
// must not be captured because gcloud login may prompt in a browser or terminal.
type InteractiveRunner interface {
	RunInteractive(ctx context.Context, name string, args ...string) (exitCode int, err error)
}

// ExecRunner runs real subprocesses via os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0

	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdout.String(), stderr.String(), ee.ExitCode(), nil
		}

		return stdout.String(), stderr.String(), -1, err
	}

	return stdout.String(), stderr.String(), exitCode, nil
}

// RunInteractive attaches stdin and sends all gcloud login output to stderr so
// setup's stdout remains structured and parseable.
func (ExecRunner) RunInteractive(ctx context.Context, name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}

		return -1, fmt.Errorf("run interactive gcloud: %w", err)
	}

	return 0, nil
}

// Client is a typed gcloud integration used by auth setup.
type Client struct {
	Runner Runner
	Binary string
}

func New(runner Runner) *Client {
	if runner == nil {
		runner = ExecRunner{}
	}

	return &Client{Runner: runner, Binary: "gcloud"}
}

// Result is a raw command outcome with sanitized stderr.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	// Kind classifies common blocker categories when ExitCode != 0.
	Kind BlockerKind
}

// BlockerKind is a best-effort classification of gcloud failures.
type BlockerKind string

const (
	BlockerNone          BlockerKind = ""
	BlockerNotInstalled  BlockerKind = "not_installed"
	BlockerNotLoggedIn   BlockerKind = "not_logged_in"
	BlockerPermission    BlockerKind = "permission"
	BlockerQuota         BlockerKind = "quota"
	BlockerNotFound      BlockerKind = "not_found"
	BlockerAlreadyExists BlockerKind = "already_exists"
	BlockerInvalidInput  BlockerKind = "invalid_input"
	BlockerUnknown       BlockerKind = "unknown"
)

// Account is the active gcloud account identity.
type Account struct {
	Account string `json:"account"`
	Status  string `json:"status,omitempty"`
}

// Project is a Cloud project summary.
// JSON field names match gcloud --format=json output.
type Project struct {
	ProjectID      string `json:"projectId"` //nolint:tagliatelle // gcloud wire format
	Name           string `json:"name,omitempty"`
	ProjectNumber  string `json:"projectNumber,omitempty"`  //nolint:tagliatelle // gcloud wire format
	LifecycleState string `json:"lifecycleState,omitempty"` //nolint:tagliatelle // gcloud wire format
	Parent         string `json:"parent,omitempty"`
}

// ServiceState describes whether a Service Usage API is enabled.
type ServiceState struct {
	ServiceID string `json:"service_id"`
	State     string `json:"state"`
	Enabled   bool   `json:"enabled"`
}

func (c *Client) binary() string {
	if strings.TrimSpace(c.Binary) == "" {
		return "gcloud"
	}

	return c.Binary
}

func (c *Client) run(ctx context.Context, args ...string) Result {
	stdout, stderr, exitCode, err := c.Runner.Run(ctx, c.binary(), args...)

	res := Result{
		Stdout:   stdout,
		Stderr:   sanitize(stderr),
		ExitCode: exitCode,
	}

	if err != nil {
		// Runner failed before producing an exit code (e.g. binary missing).
		res.ExitCode = -1
		if res.Stderr == "" {
			res.Stderr = sanitize(err.Error())
		}

		if isNotInstalled(err.Error()) || isNotInstalled(res.Stderr) {
			res.Kind = BlockerNotInstalled
		} else {
			res.Kind = BlockerUnknown
		}

		return res
	}

	if exitCode != 0 {
		res.Kind = classify(res.Stderr, exitCode)
	}

	return res
}

// Installed reports whether the gcloud binary can be executed.
func (c *Client) Installed(ctx context.Context) (bool, Result) {
	res := c.run(ctx, "version", "--format=json")
	if res.ExitCode == 0 {
		return true, res
	}

	if res.Kind == BlockerNotInstalled || isNotInstalled(res.Stderr) {
		res.Kind = BlockerNotInstalled

		return false, res
	}
	// Some environments return non-json but still have gcloud; treat any response as installed unless missing.
	if res.ExitCode == 127 || strings.Contains(strings.ToLower(res.Stderr), "not found") {
		res.Kind = BlockerNotInstalled

		return false, res
	}
	// If binary ran, consider installed even when version parsing fails.
	if res.ExitCode >= 0 && res.Kind != BlockerNotInstalled {
		return true, res
	}

	return false, res
}

// ActiveAccount returns the active authenticated account, if any.
func (c *Client) ActiveAccount(ctx context.Context) (Account, Result, error) {
	res := c.run(ctx, "auth", "list", "--filter=status:ACTIVE", "--format=json")
	if res.ExitCode != 0 {
		return Account{}, res, fmt.Errorf("%w: auth list exit %d: %s", errGCloudCommand, res.ExitCode, res.Stderr)
	}

	var rows []struct {
		Account string `json:"account"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &rows); err != nil {
		// empty list often prints []
		if strings.TrimSpace(res.Stdout) == "" || strings.TrimSpace(res.Stdout) == "[]" {
			res.Kind = BlockerNotLoggedIn

			return Account{}, res, nil
		}

		return Account{}, res, fmt.Errorf("%w: auth list: %w", errGCloudParse, err)
	}

	if len(rows) == 0 {
		res.Kind = BlockerNotLoggedIn

		return Account{}, res, nil
	}

	return Account{Account: rows[0].Account, Status: rows[0].Status}, res, nil
}

// Login runs interactive user login. Callers must not invoke this under --no-input.
// Does not set Application Default Credentials and does not mutate config.
func (c *Client) Login(ctx context.Context) Result {
	args := []string{"auth", "login", "--brief"} // Do not pass ADC flags.
	if runner, ok := c.Runner.(InteractiveRunner); ok {
		exitCode, err := runner.RunInteractive(ctx, c.binary(), args...)

		res := Result{ExitCode: exitCode}
		if err != nil {
			res.ExitCode = -1

			res.Stderr = sanitize(err.Error())
			if isNotInstalled(res.Stderr) {
				res.Kind = BlockerNotInstalled
			} else {
				res.Kind = BlockerUnknown
			}
		} else if exitCode != 0 {
			res.Kind = BlockerUnknown
		}

		return res
	}
	// Test runners that do not implement InteractiveRunner retain the captured seam.
	return c.run(ctx, args...)
}

// ListProjects returns the projects returned by one gcloud list call, capped by limit.
// Callers that need to detect truncation should request their display limit plus one.
func (c *Client) ListProjects(ctx context.Context, limit int) ([]Project, Result, error) {
	if limit <= 0 {
		return nil, Result{Kind: BlockerInvalidInput}, errProjectListLimit
	}

	// Filter before limiting so deleted/pending projects do not consume the caller's display budget.
	res := c.run(ctx, "projects", "list", "--format=json", "--filter=lifecycleState:ACTIVE", "--limit", fmt.Sprint(limit))
	if res.ExitCode != 0 {
		return nil, res, fmt.Errorf("%w: projects list exit %d: %s", errGCloudCommand, res.ExitCode, res.Stderr)
	}

	var rows []Project
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &rows); err != nil {
		if strings.TrimSpace(res.Stdout) == "" || strings.TrimSpace(res.Stdout) == "[]" {
			return []Project{}, res, nil
		}

		return nil, res, fmt.Errorf("%w: projects list: %w", errGCloudParse, err)
	}

	return rows, res, nil
}

// ActiveProjectID discovers the active configuration project without changing it.
func (c *Client) ActiveProjectID(ctx context.Context) (string, Result, error) {
	res := c.run(ctx, "config", "get-value", "project", "--format=json")
	if res.ExitCode != 0 {
		return "", res, fmt.Errorf("%w: config get-value project exit %d: %s", errGCloudCommand, res.ExitCode, res.Stderr)
	}

	raw := strings.TrimSpace(res.Stdout)
	if raw == "" || raw == "null" || raw == `""` {
		return "", res, nil
	}

	var value string
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		// non-json plain value
		value = strings.Trim(raw, "\"")
	}

	if value == "(unset)" {
		return "", res, nil
	}

	return strings.TrimSpace(value), res, nil
}

// CreateProject creates a project. parent is optional (folders/123 or organizations/123).
// Never uses --set-as-default.
func (c *Client) CreateProject(ctx context.Context, projectID, name, parent string) (Project, Result, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Project{}, Result{Kind: BlockerInvalidInput}, errProjectIDRequired
	}

	args := []string{"projects", "create", projectID, "--format=json"}
	if strings.TrimSpace(name) != "" {
		args = append(args, "--name", name)
	}

	parent = strings.TrimSpace(parent)
	switch {
	case parent == "":
	case strings.HasPrefix(parent, "organizations/"):
		args = append(args, "--organization", strings.TrimPrefix(parent, "organizations/"))
	case strings.HasPrefix(parent, "folders/"):
		args = append(args, "--folder", strings.TrimPrefix(parent, "folders/"))
	case strings.HasPrefix(parent, "folder/"):
		args = append(args, "--folder", strings.TrimPrefix(parent, "folder/"))
	default:
		// Bare numeric/id: treat as folder ID for convenience.
		args = append(args, "--folder", parent)
	}

	res := c.run(ctx, args...)
	if res.ExitCode != 0 {
		return Project{}, res, fmt.Errorf("%w: projects create exit %d: %s", errGCloudCommand, res.ExitCode, res.Stderr)
	}

	var p Project
	// Best-effort parse; some gcloud versions return empty bodies.
	_ = json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &p)
	if p.ProjectID == "" {
		p.ProjectID = projectID
	}

	if p.Name == "" {
		p.Name = name
	}

	return p, res, nil
}

// DescribeProject retrieves one explicit project without changing gcloud config.
func (c *Client) DescribeProject(ctx context.Context, projectID string) (Project, Result, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Project{}, Result{Kind: BlockerInvalidInput}, errProjectIDRequired
	}

	res := c.run(ctx, "projects", "describe", projectID, "--format=json")
	if res.ExitCode != 0 {
		return Project{}, res, fmt.Errorf("%w: projects describe exit %d: %s", errGCloudCommand, res.ExitCode, res.Stderr)
	}

	var project Project
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &project); err != nil {
		return Project{}, res, fmt.Errorf("%w: projects describe: %w", errGCloudParse, err)
	}

	if project.ProjectID == "" {
		project.ProjectID = projectID
	}

	return project, res, nil
}

// ListEnabledServices lists enabled services for an explicit project.
func (c *Client) ListEnabledServices(ctx context.Context, projectID string) ([]ServiceState, Result, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, Result{Kind: BlockerInvalidInput}, errProjectIDRequired
	}

	res := c.run(ctx, "services", "list", "--enabled", "--project", projectID, "--format=json")
	if res.ExitCode != 0 {
		return nil, res, fmt.Errorf("%w: services list exit %d: %s", errGCloudCommand, res.ExitCode, res.Stderr)
	}

	var rows []struct {
		Config *struct {
			Name string `json:"name"`
		} `json:"config"`
		Name  string `json:"name"`
		State string `json:"state"`
	}

	raw := strings.TrimSpace(res.Stdout)
	if raw == "" || raw == "[]" {
		return []ServiceState{}, res, nil
	}

	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, res, fmt.Errorf("%w: services list: %w", errGCloudParse, err)
	}

	out := make([]ServiceState, 0, len(rows))
	for _, row := range rows {
		id := serviceIDFromName(row.Name)
		if row.Config != nil && row.Config.Name != "" {
			id = serviceIDFromName(row.Config.Name)
		}

		if id == "" {
			continue
		}

		state := row.State
		if state == "" {
			state = "ENABLED"
		}

		out = append(out, ServiceState{
			ServiceID: id,
			State:     state,
			Enabled:   strings.EqualFold(state, "ENABLED"),
		})
	}

	return out, res, nil
}

// EnableServices enables the provided Service Usage IDs on projectID and returns
// which IDs remain missing after a verification list.
func (c *Client) EnableServices(ctx context.Context, projectID string, serviceIDs []string) (enabled []string, stillMissing []string, res Result, err error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, nil, Result{Kind: BlockerInvalidInput}, errProjectIDRequired
	}

	wanted := uniqueNonEmpty(serviceIDs)

	stillMissing = make([]string, 0, len(wanted))
	if len(wanted) == 0 {
		return nil, nil, Result{}, nil
	}

	args := make([]string, 0, 4+len(wanted))
	args = append(args, "services", "enable", "--project", projectID)
	args = append(args, wanted...)

	res = c.run(ctx, args...)
	if res.ExitCode != 0 {
		return nil, wanted, res, fmt.Errorf("%w: services enable exit %d: %s", errGCloudCommand, res.ExitCode, res.Stderr)
	}

	// Verify with one follow-up list of the selected project only.
	states, listRes, listErr := c.ListEnabledServices(ctx, projectID)
	if listErr != nil {
		return wanted, nil, listRes, listErr
	}

	have := make(map[string]bool, len(states))
	for _, s := range states {
		if s.Enabled {
			have[s.ServiceID] = true
		}
	}

	for _, id := range wanted {
		if have[id] {
			enabled = append(enabled, id)
		} else {
			stillMissing = append(stillMissing, id)
		}
	}

	return enabled, stillMissing, listRes, nil
}

// MissingServices returns which of wanted are not enabled on projectID.
func (c *Client) MissingServices(ctx context.Context, projectID string, wanted []string) (missing []string, states []ServiceState, res Result, err error) {
	states, res, err = c.ListEnabledServices(ctx, projectID)
	if err != nil {
		return nil, nil, res, err
	}

	have := make(map[string]bool, len(states))
	for _, s := range states {
		if s.Enabled {
			have[s.ServiceID] = true
		}
	}

	for _, id := range uniqueNonEmpty(wanted) {
		if !have[id] {
			missing = append(missing, id)
		}
	}

	return missing, states, res, nil
}

func serviceIDFromName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// projects/x/services/gmail.googleapis.com or bare gmail.googleapis.com
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}

	return name
}

func uniqueNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))

	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		if _, ok := seen[v]; ok {
			continue
		}

		seen[v] = struct{}{}
		out = append(out, v)
	}

	return out
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Drop common environment dumps / tokens if present.
	lower := strings.ToLower(s)
	for _, secret := range []string{"client_secret", "refresh_token", "access_token", "authorization:"} {
		if strings.Contains(lower, secret) {
			return "[redacted gcloud error]"
		}
	}
	// Cap noisy stderr.
	if len(s) > 2000 {
		return s[:2000] + "…"
	}

	return s
}

func isNotInstalled(msg string) bool {
	lower := strings.ToLower(msg)

	return strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "no such file or directory") ||
		strings.Contains(lower, "not found in $path") ||
		strings.Contains(lower, "command not found")
}

func classify(stderr string, exitCode int) BlockerKind {
	lower := strings.ToLower(stderr)
	switch {
	case isNotInstalled(stderr) || exitCode == 127:
		return BlockerNotInstalled
	case strings.Contains(lower, "not logged in") ||
		strings.Contains(lower, "reauthentication required") ||
		strings.Contains(lower, "there was a problem refreshing your current auth tokens") ||
		(strings.Contains(lower, "please run:") && strings.Contains(lower, "gcloud auth login")):
		return BlockerNotLoggedIn
	case strings.Contains(lower, "permission") ||
		strings.Contains(lower, "denied") ||
		strings.Contains(lower, "403") ||
		strings.Contains(lower, "caller does not have permission"):
		return BlockerPermission
	case strings.Contains(lower, "quota") || strings.Contains(lower, "rate limit"):
		return BlockerQuota
	case strings.Contains(lower, "already exists") || strings.Contains(lower, "alreadyexists") || strings.Contains(lower, "already_exists") || strings.Contains(lower, "already in use"):
		return BlockerAlreadyExists
	case strings.Contains(lower, "was not found") || strings.Contains(lower, "not found") || strings.Contains(lower, "404"):
		return BlockerNotFound
	case strings.Contains(lower, "invalid") || strings.Contains(lower, "must match") || exitCode == 2:
		return BlockerInvalidInput
	default:
		return BlockerUnknown
	}
}
