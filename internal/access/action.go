package access

import "strings"

// CanonicalAction normalizes a policy action ID so stored aliases and Kong
// dashed command names compare equal to catalog IDs such as searchconsole:query.
func CanonicalAction(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	service, rest, ok := strings.Cut(raw, ":")
	if !ok {
		return CanonicalService(raw)
	}

	service = CanonicalService(service)
	rest = strings.TrimSpace(rest)
	rest = strings.ReplaceAll(rest, "-", ".")
	rest = strings.ReplaceAll(rest, " ", ".")

	for strings.Contains(rest, "..") {
		rest = strings.ReplaceAll(rest, "..", ".")
	}

	rest = strings.Trim(rest, ".")
	if service == serviceGmail {
		if rest == "settings" {
			rest = "*"
		}

		rest = strings.TrimPrefix(rest, "settings.")
	}

	if rest == "" {
		return service + ":*"
	}

	return service + ":" + rest
}

// CanonicalActions drops empty entries after normalization.
func CanonicalActions(actions []string) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if normalized := CanonicalAction(action); normalized != "" {
			out = append(out, normalized)
		}
	}

	return out
}

// MatchAction reports whether a stored pattern covers a canonical action.
func MatchAction(pattern string, action string) bool {
	pattern = CanonicalAction(pattern)
	action = CanonicalAction(action)

	if pattern == "" || action == "" {
		return false
	}

	if pattern == action {
		return true
	}

	patternService, patternRest, ok := strings.Cut(pattern, ":")
	if !ok {
		return false
	}

	actionService, actionRest, ok := strings.Cut(action, ":")
	if !ok {
		return false
	}

	if patternService != actionService {
		return false
	}

	if patternRest == "*" || patternRest == "all" {
		return true
	}

	if strings.HasSuffix(patternRest, ".*") {
		prefix := strings.TrimSuffix(patternRest, ".*")

		return actionRest == prefix || strings.HasPrefix(actionRest, prefix+".")
	}

	if patternRest == "read" {
		return isReadLikeAction(actionService, actionRest)
	}

	if patternRest == "reply" {
		return actionRest == "send" || strings.HasSuffix(actionRest, ".send")
	}

	if !strings.Contains(patternRest, ".") {
		last := actionRest
		if idx := strings.LastIndex(last, "."); idx >= 0 {
			last = last[idx+1:]
		}

		return last == patternRest
	}

	return false
}

func isReadLikeAction(service string, actionRest string) bool {
	if service != serviceGmail {
		return false
	}

	last := actionRest
	if idx := strings.LastIndex(last, "."); idx >= 0 {
		last = last[idx+1:]
	}

	switch last {
	case "attachment", "attachments", "get", "history", "list", "opens", "search", "status", "url":
		return true
	default:
		return false
	}
}

func matchesAnyAction(patterns []string, action string) bool {
	for _, pattern := range patterns {
		if MatchAction(pattern, action) {
			return true
		}
	}

	return false
}
