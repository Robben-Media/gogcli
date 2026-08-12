package cmd

import (
	"context"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
)

const schemaVersion = "1"

type SchemaCmd struct{}

type schemaDocument struct {
	SchemaVersion string           `json:"schema_version"`
	Commands      []schemaCommand  `json:"commands"`
	GlobalFlags   []schemaValue    `json:"global_flags"`
	ExitCodes     []schemaExitCode `json:"exit_codes"`
	Automation    schemaAutomation `json:"automation"`
}

type schemaExitCode struct {
	Code        int    `json:"code"`
	Class       string `json:"class"`
	Description string `json:"description"`
}

type schemaAutomation struct {
	JSON                bool               `json:"json"`
	Plain               bool               `json:"plain"`
	WrapUntrusted       bool               `json:"wrap_untrusted"`
	NoInput             bool               `json:"no_input"`
	Force               bool               `json:"force"`
	AccountInput        string             `json:"account_input"`
	ClientInput         string             `json:"client_input"`
	Account             string             `json:"account"`
	Client              string             `json:"client"`
	EnabledCommands     schemaEnabledState `json:"enabled_commands"`
	EnabledCommandPaths schemaEnabledState `json:"enabled_command_paths"`
	Policy              schemaPolicyState  `json:"policy"`
}

type schemaEnabledState struct {
	Input        string   `json:"input"`
	Restricted   bool     `json:"restricted"`
	Allowed      []string `json:"allowed"`
	Unrecognized []string `json:"unrecognized"`
}

type schemaPolicyState struct {
	Status           string               `json:"status"`
	UnresolvedReason string               `json:"unresolved_reason,omitempty"`
	Effects          []schemaPolicyEffect `json:"effects"`
}

type schemaPolicyEffect struct {
	Name   string   `json:"name"`
	Allow  []string `json:"allow"`
	Deny   []string `json:"deny"`
	Reason string   `json:"reason,omitempty"`
}

type schemaCommand struct {
	Path      string        `json:"path"`
	Aliases   []string      `json:"aliases"`
	Help      string        `json:"help"`
	Arguments []schemaValue `json:"arguments"`
	Flags     []schemaValue `json:"flags"`
}

type schemaValue struct {
	Name       string   `json:"name"`
	Aliases    []string `json:"aliases"`
	Type       string   `json:"type"`
	Required   bool     `json:"required"`
	Repeatable bool     `json:"repeatable"`
	Default    string   `json:"default"`
	Help       string   `json:"help"`
}

func (c *SchemaCmd) Run(ctx context.Context, flags *RootFlags) error {
	parser, _, err := newParser(baseDescription())
	if err != nil {
		return err
	}
	document := buildSchemaDocument(parser.Model.Node, flags)
	return outfmt.WriteJSON(ctx, os.Stdout, document)
}

func buildSchemaDocument(root *kong.Node, flags *RootFlags) schemaDocument {
	document := schemaDocument{
		SchemaVersion: schemaVersion,
		GlobalFlags:   schemaFlags(root.Flags),
		ExitCodes: []schemaExitCode{
			{Code: 0, Class: "success", Description: "Command completed successfully"},
			{Code: 1, Class: "runtime", Description: "Command failed during execution"},
			{Code: 2, Class: "usage", Description: "Arguments or flags could not be parsed or validated"},
		},
		Automation: buildSchemaAutomation(root, flags),
	}
	appendSchemaCommands(root, &document.Commands)
	sort.Slice(document.Commands, func(i, j int) bool {
		return document.Commands[i].Path < document.Commands[j].Path
	})
	return document
}

func buildSchemaAutomation(root *kong.Node, flags *RootFlags) schemaAutomation {
	if flags == nil {
		flags = &RootFlags{}
	}
	account := strings.TrimSpace(flags.Account)
	if account == "" {
		account = strings.TrimSpace(os.Getenv("GOG_ACCOUNT"))
	}
	client := strings.TrimSpace(flags.Client)

	cfg, readErr := config.ReadConfig()
	resolvedAccount, resolvedClient, resolutionErr := resolveSchemaSelection(cfg, account, client, readErr)
	return schemaAutomation{
		JSON:                flags.JSON,
		Plain:               flags.Plain,
		WrapUntrusted:       flags.WrapUntrusted,
		NoInput:             flags.NoInput,
		Force:               flags.Force,
		AccountInput:        account,
		ClientInput:         client,
		Account:             resolvedAccount,
		Client:              resolvedClient,
		EnabledCommands:     buildSchemaEnabledState(flags.EnableCommands, visibleTopLevelCommands(root)),
		EnabledCommandPaths: buildSchemaEnabledPathState(root, flags.EnableCommandPaths),
		Policy:              buildSchemaPolicyState(cfg.Policies, resolvedAccount, resolvedClient, readErr, resolutionErr),
	}
}

func resolveSchemaSelection(cfg config.File, accountInput string, clientInput string, readErr error) (string, string, string) {
	if readErr != nil {
		return "", "", "configuration could not be read"
	}
	account := strings.TrimSpace(accountInput)
	if account == "" || shouldAutoSelectAccount(account) {
		return "", "", "account selection required"
	}
	if !strings.Contains(account, "@") {
		resolved, ok := cfg.AccountAliases[config.NormalizeAccountAlias(account)]
		if !ok || strings.TrimSpace(resolved) == "" {
			return "", "", "account alias could not be resolved"
		}
		account = strings.TrimSpace(resolved)
	}
	client, err := config.ResolveClientForAccount(cfg, account, clientInput)
	if err != nil {
		return account, "", "client selection could not be resolved"
	}
	return account, client, ""
}

func buildSchemaEnabledState(input string, known map[string]bool) schemaEnabledState {
	input = strings.TrimSpace(input)
	allowedSet, _ := parseEnabledTopLevelCommands(input)
	if input == "" || len(allowedSet) == 0 || allowedSet["*"] || allowedSet["all"] {
		return schemaEnabledState{Input: input, Allowed: []string{}, Unrecognized: []string{}}
	}
	allowed := make([]string, 0, len(allowedSet))
	unrecognized := make([]string, 0, len(allowedSet))
	for command := range allowedSet {
		if known[command] {
			allowed = append(allowed, command)
		} else {
			unrecognized = append(unrecognized, command)
		}
	}
	sort.Strings(allowed)
	sort.Strings(unrecognized)
	return schemaEnabledState{Input: input, Restricted: true, Allowed: allowed, Unrecognized: unrecognized}
}

func buildSchemaEnabledPathState(root *kong.Node, input string) schemaEnabledState {
	input = strings.TrimSpace(input)
	allowedSet := map[string]bool{}
	configured := false
	for _, part := range strings.Split(input, ",") {
		segments := strings.Fields(strings.ToLower(strings.TrimSpace(part)))
		if len(segments) == 0 {
			continue
		}
		configured = true
		if path, ok := resolveCommandPath(root, segments); ok {
			allowedSet[strings.Join(path, " ")] = true
		} else {
			allowedSet[strings.Join(segments, " ")] = true
		}
	}
	if !configured {
		return schemaEnabledState{Input: input, Allowed: []string{}, Unrecognized: []string{}}
	}
	known := schemaCanonicalCommandPaths(root)
	allowed := make([]string, 0, len(allowedSet))
	unrecognized := make([]string, 0, len(allowedSet))
	for command := range allowedSet {
		if known[command] {
			allowed = append(allowed, command)
		} else {
			unrecognized = append(unrecognized, command)
		}
	}
	sort.Strings(allowed)
	sort.Strings(unrecognized)
	return schemaEnabledState{Input: input, Restricted: true, Allowed: allowed, Unrecognized: unrecognized}
}

func schemaCanonicalCommandPaths(root *kong.Node) map[string]bool {
	paths := map[string]bool{}
	var walk func(*kong.Node)
	walk = func(node *kong.Node) {
		for _, child := range node.Children {
			if child.Hidden {
				continue
			}
			if child.Type == kong.CommandNode {
				paths[canonicalCommandPath(child)] = true
			}
			walk(child)
		}
	}
	walk(root)
	return paths
}

func visibleTopLevelCommands(root *kong.Node) map[string]bool {
	commands := map[string]bool{}
	for _, child := range root.Children {
		if child.Type == kong.CommandNode && !child.Hidden {
			commands[strings.ToLower(child.Name)] = true
		}
	}
	return commands
}

func buildSchemaPolicyState(policies []config.Policy, account string, client string, readErr error, resolutionErr string) schemaPolicyState {
	if readErr != nil {
		return schemaPolicyState{Status: "unresolved", UnresolvedReason: "configuration could not be read", Effects: []schemaPolicyEffect{}}
	}
	if len(policies) == 0 {
		return schemaPolicyState{Status: "not_configured", Effects: []schemaPolicyEffect{}}
	}
	if resolutionErr != "" {
		return schemaPolicyState{Status: "unresolved", UnresolvedReason: resolutionErr, Effects: []schemaPolicyEffect{}}
	}

	applicable := mostSpecificApplicablePolicies(policies, account, client)
	effects := make([]schemaPolicyEffect, 0, len(applicable))
	for _, policy := range applicable {
		effects = append(effects, schemaPolicyEffect{
			Name:   policy.Name,
			Allow:  sortedStrings(policy.Allow),
			Deny:   sortedStrings(policy.Deny),
			Reason: policy.Reason,
		})
	}
	sort.Slice(effects, func(i, j int) bool {
		return effects[i].Name < effects[j].Name
	})
	return schemaPolicyState{Status: "resolved", Effects: effects}
}

func appendSchemaCommands(node *kong.Node, commands *[]schemaCommand) {
	for _, child := range node.Children {
		if child.Hidden {
			continue
		}
		if child.Type == kong.CommandNode {
			*commands = append(*commands, schemaCommand{
				Path:      canonicalCommandPath(child),
				Aliases:   sortedStrings(child.Aliases),
				Help:      child.Help,
				Arguments: schemaArguments(child.Positional),
				Flags:     schemaFlags(child.Flags),
			})
		}
		appendSchemaCommands(child, commands)
	}
}

func canonicalCommandPath(node *kong.Node) string {
	parts := make([]string, 0, node.Depth()+1)
	for current := node; current != nil && current.Type != kong.ApplicationNode; current = current.Parent {
		if current.Type == kong.CommandNode {
			parts = append(parts, current.Name)
		}
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.Join(parts, " ")
}

func schemaArguments(arguments []*kong.Positional) []schemaValue {
	values := make([]schemaValue, 0, len(arguments))
	for _, argument := range arguments {
		values = append(values, schemaValueFromValue(argument, nil))
	}
	return values
}

func schemaFlags(flags []*kong.Flag) []schemaValue {
	values := make([]schemaValue, 0, len(flags))
	for _, flag := range flags {
		if flag.Hidden {
			continue
		}
		aliases := append([]string(nil), flag.Aliases...)
		if flag.Short != 0 {
			aliases = append(aliases, string(flag.Short))
		}
		values = append(values, schemaValueFromValue(flag.Value, aliases))
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].Name < values[j].Name
	})
	return values
}

func schemaValueFromValue(value *kong.Value, aliases []string) schemaValue {
	return schemaValue{
		Name:       value.Name,
		Aliases:    sortedStrings(aliases),
		Type:       schemaValueType(value.Target),
		Required:   value.Required,
		Repeatable: value.IsCumulative(),
		Default:    schemaDefault(value),
		Help:       value.Help,
	}
}

func schemaDefault(value *kong.Value) string {
	if value.HasDefault {
		return value.Default
	}
	if value.Tag != nil {
		if envName := value.Tag.Get("schema-default-env"); envName != "" {
			return strings.TrimSpace(os.Getenv(envName))
		}
	}
	if !value.Target.IsValid() {
		return ""
	}
	switch value.Target.Kind() {
	case reflect.Bool:
		return boolFalse
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return "0"
	case reflect.Slice, reflect.Array:
		return "[]"
	case reflect.Map:
		return "{}"
	default:
		return ""
	}
}

func schemaValueType(value reflect.Value) string {
	if !value.IsValid() {
		return "unknown"
	}
	return value.Type().String()
}

func sortedStrings(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}
