package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/selfupdate"
)

type decodedSchema struct {
	SchemaVersion string                  `json:"schema_version"`
	Commands      []decodedSchemaCommand  `json:"commands"`
	GlobalFlags   []decodedSchemaValue    `json:"global_flags"`
	ExitCodes     []decodedSchemaExitCode `json:"exit_codes"`
	Automation    decodedSchemaAutomation `json:"automation"`
}

type decodedSchemaExitCode struct {
	Code  int    `json:"code"`
	Class string `json:"class"`
}

type decodedSchemaAutomation struct {
	JSON            bool                `json:"json"`
	Plain           bool                `json:"plain"`
	NoInput         bool                `json:"no_input"`
	Force           bool                `json:"force"`
	AccountInput    string              `json:"account_input"`
	ClientInput     string              `json:"client_input"`
	Account         string              `json:"account"`
	Client          string              `json:"client"`
	EnabledCommands decodedEnabledState `json:"enabled_commands"`
	Policy          decodedPolicyState  `json:"policy"`
}

type decodedEnabledState struct {
	Restricted bool     `json:"restricted"`
	Allowed    []string `json:"allowed"`
}

type decodedPolicyState struct {
	Status           string                `json:"status"`
	UnresolvedReason string                `json:"unresolved_reason"`
	Effects          []decodedPolicyEffect `json:"effects"`
}

type decodedPolicyEffect struct {
	Name  string   `json:"name"`
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type decodedSchemaCommand struct {
	Path      string               `json:"path"`
	Aliases   []string             `json:"aliases"`
	Help      string               `json:"help"`
	Arguments []decodedSchemaValue `json:"arguments"`
	Flags     []decodedSchemaValue `json:"flags"`
}

type decodedSchemaValue struct {
	Name       string   `json:"name"`
	Aliases    []string `json:"aliases"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	Repeatable bool     `json:"repeatable"`
	Default    string   `json:"default"`
	Help       string   `json:"help"`
}

func TestExecuteSchemaEmitsVersionedJSONWithoutUpdateCheck(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	calls := 0
	original := maybeNotifyUpdate
	maybeNotifyUpdate = func(context.Context, *selfupdate.Client, string, time.Duration) string {
		calls++
		return "unexpected update notice"
	}
	t.Cleanup(func() { maybeNotifyUpdate = original })

	var executeErr error
	stderr := captureStderr(t, func() {
		stdout := captureStdout(t, func() {
			executeErr = Execute([]string{"schema"})
		})
		if executeErr != nil {
			t.Fatalf("Execute(schema): %v", executeErr)
		}

		var document decodedSchema
		if err := json.Unmarshal([]byte(stdout), &document); err != nil {
			t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
		}
		if document.SchemaVersion != "1" {
			t.Fatalf("schema_version = %q, want 1", document.SchemaVersion)
		}
	})

	if calls != 0 {
		t.Fatalf("update notifier calls = %d, want 0", calls)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestExecuteSchemaBypassesPolicyEnforcementForMalformedConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, err := config.EnsureDir(); err != nil {
		t.Fatalf("ensure config dir: %v", err)
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if err := os.WriteFile(path, []byte("{malformed"), 0o600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	document, _ := executeSchema(t, "schema")
	if document.Automation.Policy.Status != "unresolved" || document.Automation.Policy.UnresolvedReason != "configuration could not be read" {
		t.Fatalf("policy state = %+v, want unresolved config read", document.Automation.Policy)
	}

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"time", "now"}); err == nil {
				t.Fatal("ordinary command succeeded with malformed config")
			}
		})
	})
	if !strings.Contains(stderr, "read config") {
		t.Fatalf("ordinary command stderr = %q, want config read failure", stderr)
	}
}

func TestExecuteSchemaIncludesKongCommandArgumentAndFlagMetadata(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_SKIP_UPDATE_CHECK", "1")

	var executeErr error
	stdout := captureStdout(t, func() {
		executeErr = Execute([]string{"schema"})
	})
	if executeErr != nil {
		t.Fatalf("Execute(schema): %v", executeErr)
	}

	var document decodedSchema
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	gmail := findSchemaCommand(t, document.Commands, "gmail")
	if !schemaContainsString(gmail.Aliases, "mail") || !schemaContainsString(gmail.Aliases, "email") {
		t.Fatalf("gmail aliases = %v, want mail and email", gmail.Aliases)
	}

	deleteGuardian := findSchemaCommand(t, document.Commands, "classroom guardians delete")
	if !schemaContainsString(deleteGuardian.Aliases, "rm") {
		t.Fatalf("classroom guardians delete aliases = %v, want rm", deleteGuardian.Aliases)
	}

	get := findSchemaCommand(t, document.Commands, "gmail get")
	if len(get.Arguments) == 0 || get.Arguments[0].Name == "" || get.Arguments[0].Type == "" || get.Arguments[0].Help == "" {
		t.Fatalf("gmail get arguments missing machine metadata: %+v", get.Arguments)
	}
	if len(get.Flags) == 0 {
		t.Fatalf("gmail get flags are empty")
	}
	for _, flag := range get.Flags {
		if flag.Name == "" || flag.Type == "" || flag.Help == "" {
			t.Fatalf("gmail get flag missing machine metadata: %+v", flag)
		}
	}

	account := findSchemaValue(t, document.GlobalFlags, "account")
	if account.Type != "string" || account.Help == "" {
		t.Fatalf("global account flag = %+v", account)
	}
	force := findSchemaValue(t, document.GlobalFlags, "force")
	if force.Default != "false" {
		t.Fatalf("global force default = %q, want false", force.Default)
	}
	calendarCreate := findSchemaCommand(t, document.Commands, "calendar create")
	sendUpdates := findSchemaValue(t, calendarCreate.Flags, "send-updates")
	if sendUpdates.Default != "all" {
		t.Fatalf("calendar create send-updates default = %q, want all", sendUpdates.Default)
	}

	seen := make(map[string]bool, len(document.Commands))
	for i, command := range document.Commands {
		if command.Path == "" || command.Help == "" {
			t.Fatalf("command missing path/help: %+v", command)
		}
		if seen[command.Path] {
			t.Fatalf("duplicate canonical command path %q", command.Path)
		}
		seen[command.Path] = true
		if i > 0 && document.Commands[i-1].Path > command.Path {
			t.Fatalf("commands are not deterministically sorted at %q", command.Path)
		}
	}

	parser, cli, err := newParser(baseDescription())
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	wantPaths := visibleCanonicalCommandPaths(parser.Model.Node)
	if len(seen) != len(wantPaths) {
		t.Fatalf("emitted command count = %d, Kong visible command count = %d", len(seen), len(wantPaths))
	}
	for path := range wantPaths {
		if !seen[path] {
			t.Fatalf("visible Kong command %q missing from schema", path)
		}
	}
	if _, err := parser.Parse([]string{"calendar", "create", "primary"}); err != nil {
		t.Fatalf("parse calendar create defaults: %v", err)
	}
	if cli.Calendar.Create.SendUpdates != scopeAll {
		t.Fatalf("calendar create runtime send-updates default = %q, want %q", cli.Calendar.Create.SendUpdates, scopeAll)
	}
}

func TestExecuteSchemaUsesEffectiveEnvironmentDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_SKIP_UPDATE_CHECK", "1")
	t.Setenv("GOG_JSON", "1")
	t.Setenv("GOG_PLAIN", "")
	t.Setenv("GOG_COLOR", "always")
	t.Setenv("GOG_CLIENT", "env-client")
	t.Setenv("GOG_ACCOUNT", "env@example.com")
	t.Setenv("GOG_ENABLE_COMMANDS", "calendar,gmail")

	document, _ := executeSchema(t, "schema")
	wantDefaults := map[string]string{
		"account":         "env@example.com",
		"client":          "env-client",
		"color":           "always",
		"enable-commands": "calendar,gmail",
		"json":            "true",
		"plain":           "false",
	}
	for name, want := range wantDefaults {
		if got := findSchemaValue(t, document.GlobalFlags, name).Default; got != want {
			t.Fatalf("global %s default = %q, want %q", name, got, want)
		}
	}
	if !document.Automation.JSON || document.Automation.ClientInput != "env-client" || document.Automation.AccountInput != "env@example.com" {
		t.Fatalf("environment automation defaults = %+v", document.Automation)
	}
}

func TestExecuteSchemaUsesEffectivePlainEnvironmentDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_JSON", "")
	t.Setenv("GOG_PLAIN", "1")

	document, _ := executeSchema(t, "schema")
	if got := findSchemaValue(t, document.GlobalFlags, "plain").Default; got != "true" {
		t.Fatalf("global plain default = %q, want true", got)
	}
	if !document.Automation.Plain || document.Automation.JSON {
		t.Fatalf("environment automation defaults = %+v", document.Automation)
	}
}

func visibleCanonicalCommandPaths(node *kong.Node) map[string]bool {
	paths := map[string]bool{}
	var walk func(*kong.Node)
	walk = func(current *kong.Node) {
		for _, child := range current.Children {
			if child.Hidden {
				continue
			}
			if child.Type == kong.CommandNode {
				paths[canonicalCommandPath(child)] = true
			}
			walk(child)
		}
	}
	walk(node)
	return paths
}

func TestExecuteSchemaReportsExitCodesAndEffectiveAutomationState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_JSON", "1")
	t.Setenv("GOG_ENABLE_COMMANDS", "calendar,gmail")
	t.Setenv("GOG_SKIP_UPDATE_CHECK", "1")

	document, _ := executeSchema(t, "--account", "agent@example.com", "--client", "work", "--no-input", "--force", "schema")

	wantExitCodes := map[int]string{0: "success", 1: "runtime", 2: "usage"}
	for _, exitCode := range document.ExitCodes {
		delete(wantExitCodes, exitCode.Code)
	}
	if len(wantExitCodes) != 0 {
		t.Fatalf("missing exit code classes: %v (got %+v)", wantExitCodes, document.ExitCodes)
	}
	if !document.Automation.JSON || document.Automation.Plain || !document.Automation.NoInput || !document.Automation.Force {
		t.Fatalf("automation flags = %+v", document.Automation)
	}
	if document.Automation.AccountInput != "agent@example.com" || document.Automation.ClientInput != "work" {
		t.Fatalf("automation selection inputs = %+v", document.Automation)
	}
	if !document.Automation.EnabledCommands.Restricted {
		t.Fatalf("enabled-command restriction not reported: %+v", document.Automation.EnabledCommands)
	}
	if strings.Join(document.Automation.EnabledCommands.Allowed, ",") != "calendar,gmail" {
		t.Fatalf("enabled commands = %v", document.Automation.EnabledCommands.Allowed)
	}
}

func TestExecuteSchemaReportsPolicyEffectsOrUnresolvedContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_SKIP_UPDATE_CHECK", "1")

	cfg := config.File{Policies: []config.Policy{
		{Name: "agent-mail", Account: "agent@example.com", Client: "work", Allow: []string{"gmail:read"}, Deny: []string{"gmail:send"}},
	}}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config before schema: %v", err)
	}

	unresolved, _ := executeSchema(t, "schema")
	if unresolved.Automation.Policy.Status != "unresolved" || unresolved.Automation.Policy.UnresolvedReason == "" {
		t.Fatalf("policy without context = %+v", unresolved.Automation.Policy)
	}

	resolved, _ := executeSchema(t, "--account", "agent@example.com", "--client", "work", "schema")
	if resolved.Automation.Policy.Status != "resolved" || len(resolved.Automation.Policy.Effects) != 1 {
		t.Fatalf("policy with context = %+v", resolved.Automation.Policy)
	}
	effect := resolved.Automation.Policy.Effects[0]
	if effect.Name != "agent-mail" || strings.Join(effect.Allow, ",") != "gmail:read" || strings.Join(effect.Deny, ",") != "gmail:send" {
		t.Fatalf("policy effect = %+v", effect)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after schema: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("schema modified config file")
	}
}

func TestExecuteSchemaResolvesAliasAndDefaultClientForPolicyEffects(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_SKIP_UPDATE_CHECK", "1")

	cfg := config.File{
		AccountAliases: map[string]string{"team": "agent@example.com"},
		Policies: []config.Policy{
			{Name: "default-mail", Account: "agent@example.com", Client: "default", Deny: []string{"gmail:send"}},
		},
	}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	document, _ := executeSchema(t, "--account", "team", "schema")
	if document.Automation.AccountInput != "team" || document.Automation.Account != "agent@example.com" || document.Automation.Client != "default" {
		t.Fatalf("resolved selection = %+v", document.Automation)
	}
	if document.Automation.Policy.Status != "resolved" || len(document.Automation.Policy.Effects) != 1 || document.Automation.Policy.Effects[0].Name != "default-mail" {
		t.Fatalf("resolved alias/default policy = %+v", document.Automation.Policy)
	}
}

func TestExecuteSchemaIsDeterministicAndExcludesSecretValues(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_SKIP_UPDATE_CHECK", "1")
	t.Setenv("GOG_KEYRING_PASSWORD", "schema-secret-password-152")
	t.Setenv("GITHUB_TOKEN", "schema-secret-token-152")
	t.Setenv("GH_TOKEN", "schema-secret-fallback-token-152")

	_, first := executeSchema(t, "schema")
	_, second := executeSchema(t, "schema")
	if first != second {
		t.Fatalf("schema output changed between identical invocations")
	}
	for _, secret := range []string{"schema-secret-password-152", "schema-secret-token-152", "schema-secret-fallback-token-152"} {
		if strings.Contains(first, secret) {
			t.Fatalf("schema output contains secret value %q", secret)
		}
	}
}

func TestSchemaV1ContractFingerprint(t *testing.T) {
	normalizeSchemaEnvironment(t)

	_, output := executeSchema(t, "schema")
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(output)))
	const want = "35bdf825496c042720ebdf294b49afe0a134d396ea61edf8f623606de4d86afb"
	if got != want {
		t.Fatalf("schema v1 contract fingerprint = %s, want %s; review the contract change and schema version", got, want)
	}
}

func normalizeSchemaEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, key := range []string{
		"GOG_ACCOUNT",
		"GOG_CALENDAR_WEEKDAY",
		"GOG_CLIENT",
		"GOG_COLOR",
		"GOG_ENABLE_COMMANDS",
		"GOG_JSON",
		"GOG_KEYRING_PASSWORD",
		"GOG_PLAIN",
		"GOG_SKIP_UPDATE_CHECK",
		"GOG_UPDATE_REPO",
		"GITHUB_TOKEN",
		"GH_TOKEN",
	} {
		t.Setenv(key, "")
	}
}

func executeSchema(t *testing.T, args ...string) (decodedSchema, string) {
	t.Helper()
	var executeErr error
	stdout := captureStdout(t, func() {
		executeErr = Execute(args)
	})
	if executeErr != nil {
		t.Fatalf("Execute(%v): %v", args, executeErr)
	}
	var document decodedSchema
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return document, stdout
}

func findSchemaCommand(t *testing.T, commands []decodedSchemaCommand, path string) decodedSchemaCommand {
	t.Helper()
	for _, command := range commands {
		if command.Path == path {
			return command
		}
	}
	t.Fatalf("schema command %q not found", path)
	return decodedSchemaCommand{}
}

func findSchemaValue(t *testing.T, values []decodedSchemaValue, name string) decodedSchemaValue {
	t.Helper()
	for _, value := range values {
		if value.Name == name {
			return value
		}
	}
	t.Fatalf("schema value %q not found", name)
	return decodedSchemaValue{}
}

func schemaContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
