package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

// enforceEnabledCommands applies invocation-scoped enablement.
//
// Composition:
//   - unrestricted when neither top-level nor exact-path input is configured
//   - a match in either list permits the command (OR)
//   - parent exact paths do not implicitly allow descendants
//   - persisted policies remain a separate subsequent gate
func enforceEnabledCommands(kctx *kong.Context, enabledTop string, enabledPaths string) error {
	topAllow, topConfigured := parseEnabledTopLevelCommands(enabledTop)
	pathAllow, pathConfigured := parseEnabledCommandPaths(kctx, enabledPaths)
	if !topConfigured && !pathConfigured {
		return nil
	}

	path := commandPath(kctx)
	if len(path) == 0 {
		return nil
	}

	if topConfigured && topAllowsCommand(topAllow, path[0]) {
		return nil
	}
	if pathConfigured && pathAllow[strings.Join(path, " ")] {
		return nil
	}
	if isSchemaCommand(kctx.Command()) {
		return nil
	}

	display := strings.Join(path, " ")
	if pathConfigured && !topConfigured {
		return usagef("command %q is not enabled (set --enable-command-paths to allow it)", display)
	}
	if topConfigured && !pathConfigured {
		return usagef("command %q is not enabled (set --enable-commands to allow it)", path[0])
	}
	return usagef("command %q is not enabled (set --enable-commands or --enable-command-paths to allow it)", display)
}

// commandPath returns the parser-resolved canonical command identity:
// command segments only (no flags, no positional placeholders/values).
// Kong already rewrites aliases to their primary command names.
func commandPath(kctx *kong.Context) []string {
	if kctx == nil {
		return nil
	}
	rawParts := strings.Fields(strings.TrimSpace(kctx.Command()))
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		// Kong represents positional args as <name> placeholders in Command().
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func parseEnabledTopLevelCommands(value string) (map[string]bool, bool) {
	out := map[string]bool{}
	configured := false
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		configured = true
		out[part] = true
	}
	return out, configured
}

// parseEnabledCommandPaths builds the exact-path allow set.
// When a Kong context is available, each entry is walked through the command
// model so documented aliases collapse to the same canonical path identity.
func parseEnabledCommandPaths(kctx *kong.Context, value string) (map[string]bool, bool) {
	out := map[string]bool{}
	configured := false
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments := strings.Fields(strings.ToLower(part))
		if len(segments) == 0 {
			continue
		}
		configured = true
		out[canonicalizeEnabledPath(kctx, segments)] = true
	}
	return out, configured
}

func canonicalizeEnabledPath(kctx *kong.Context, segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	if kctx != nil && kctx.Model != nil {
		if path, ok := resolveCommandPath(kctx.Model.Node, segments); ok {
			return strings.Join(path, " ")
		}
	}
	// Fallback when the entry cannot be resolved against the model: still
	// normalize the top-level service alias so mail/email/bq entries work.
	segments = append([]string(nil), segments...)
	segments[0] = normalizeCommandService(segments[0])
	return strings.Join(segments, " ")
}

// resolveCommandPath walks the Kong command tree matching each segment by
// primary name or alias. It returns the canonical primary-name path written by
// the user; a final alias that selects a default command includes that leaf.
// An exact-path entry written with a parent primary name does not follow it.
func resolveCommandPath(root *kong.Node, segments []string) ([]string, bool) {
	if root == nil || len(segments) == 0 {
		return nil, false
	}
	node := root
	path := make([]string, 0, len(segments)+1)
	for i, segment := range segments {
		child, isAlias := findCommandChild(node, segment)
		if child == nil {
			return nil, false
		}
		path = append(path, child.Name)
		node = child
		if i == len(segments)-1 && isAlias && node.DefaultCmd != nil {
			path = append(path, node.DefaultCmd.Name)
		}
	}
	return path, true
}

func findCommandChild(node *kong.Node, segment string) (*kong.Node, bool) {
	if node == nil {
		return nil, false
	}
	segment = strings.ToLower(strings.TrimSpace(segment))
	for _, child := range node.Children {
		if child == nil || child.Type != kong.CommandNode {
			continue
		}
		if strings.EqualFold(child.Name, segment) {
			return child, false
		}
		for _, alias := range child.Aliases {
			if strings.EqualFold(alias, segment) {
				return child, true
			}
		}
	}
	return nil, false
}

func topAllowsCommand(allow map[string]bool, top string) bool {
	if allow["*"] || allow["all"] {
		return true
	}
	return allow[top]
}
