package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func compactToolNames() []string {
	return []string{capabilitiesSearchName, capabilitiesDescribeName, capabilitiesExecuteName}
}

func gatewayTool(name string) bool {
	switch strings.TrimSpace(name) {
	case accountsListName, capabilitiesSearchName, capabilitiesDescribeName, capabilitiesExecuteName:
		return true
	default:
		return false
	}
}

func capabilityGroup(operation mcpcontract.Operation) string {
	if operation.Definition.Local || gatewayTool(operation.Definition.Name) {
		return ""
	}

	service := servicePrefix(operation)
	if service == "" {
		return ""
	}

	if writeOperation(operation) {
		return service + ".write"
	}

	return service + ".read"
}

func writeOperation(operation mcpcontract.Operation) bool {
	if operation.Definition.Local {
		return false
	}

	retry := operation.Definition.Retry
	if retry == "" {
		if def, ok := mcpcontract.Lookup(operation.Definition.Name); ok {
			retry = def.Retry
		}
	}

	return retry != mcpcontract.SafeRead
}

func writeDisabledError() error {
	return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: "write operations are disabled", Retryable: false}
}

func operationUnavailableError() error {
	return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: "operation is not available", Retryable: false}
}

func gatewayExecuteError() error {
	return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: "capabilities_execute can only run native Google operations", Retryable: false}
}

func searchQueryError(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: message, Retryable: false}
}

func searchNoMatchError() error {
	return &mcpcontract.Error{Category: mcpcontract.NotFound, Message: "no eligible capability; narrow service/task or check grants", Retryable: false}
}

func (rt *Runtime) operationDiscoverable(ctx context.Context, authorizer *access.Authorizer, operation mcpcontract.Operation) (bool, error) {
	name := strings.TrimSpace(operation.Definition.Name)
	if name == "" || operation.Definition.Local || gatewayTool(name) {
		return false, nil
	}

	if !rt.enableWrites && writeOperation(operation) {
		return false, nil
	}

	ok, err := authorizer.Visible(ctx, rt.principal, name)
	if err != nil {
		return false, fmt.Errorf("google tool visibility: %w", err)
	}

	return ok, nil
}

func (rt *Runtime) lookupOperation(name string) (mcpcontract.Operation, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return mcpcontract.Operation{}, false
	}

	for _, operation := range rt.operations {
		if operation.Definition.Name == name {
			return operation, true
		}
	}

	return mcpcontract.Operation{}, false
}

type rankedOperation struct {
	operation mcpcontract.Operation
	score     int
	order     int
}

func (rt *Runtime) searchOperations(ctx context.Context, authorizer *access.Authorizer, query string, limit int) ([]mcpcontract.Operation, bool, error) {
	tokens := tokenizeQuery(query)
	ranked := make([]rankedOperation, 0, len(rt.operations))
	seen := make(map[string]int, len(rt.operations))

	for index, operation := range rt.operations {
		ok, err := rt.operationDiscoverable(ctx, authorizer, operation)
		if err != nil {
			return nil, false, err
		}

		if !ok {
			continue
		}

		score := scoreOperation(query, tokens, operation)
		if score <= 0 {
			continue
		}

		name := operation.Definition.Name
		if previous, exists := seen[name]; exists {
			if score > ranked[previous].score {
				ranked[previous] = rankedOperation{operation: operation, score: score, order: index}
			}

			continue
		}

		seen[name] = len(ranked)
		ranked = append(ranked, rankedOperation{operation: operation, score: score, order: index})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}

		return ranked[i].order < ranked[j].order
	})

	if limit <= 0 {
		limit = defaultCapabilitySearchLimit
	}

	if limit > maxCapabilitySearchLimit {
		limit = maxCapabilitySearchLimit
	}

	truncated := len(ranked) > limit
	if truncated {
		ranked = ranked[:limit]
	}

	out := make([]mcpcontract.Operation, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.operation)
	}

	return out, truncated, nil
}

func tokenizeQuery(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(query)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	out := make([]string, 0, len(fields))

	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}

		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}

	return out
}

func scoreOperation(query string, tokens []string, operation mcpcontract.Operation) int {
	name := strings.ToLower(operation.Definition.Name)
	desc := strings.ToLower(operation.Definition.Description)
	service := strings.ToLower(servicePrefix(operation))
	normalized := strings.ToLower(strings.TrimSpace(query))
	score := 0

	if name == normalized {
		score += 1000
	} else if strings.Contains(name, normalized) {
		score += 300
	}

	if service != "" && service == normalized {
		score += 200
	}

	for _, token := range tokens {
		if name == token || strings.HasPrefix(name, token+"_") || strings.HasPrefix(name, "google_"+token+"_") {
			score += 80
		} else if strings.Contains(name, token) {
			score += 40
		}

		if service != "" && service == token {
			score += 50
		}

		if strings.Contains(desc, token) {
			score += 20
		}

		for _, action := range operation.Definition.Actions {
			if strings.Contains(strings.ToLower(action), token) {
				score += 25
				break
			}
		}

		score += taskBonus(token, service)
	}

	return score
}

func taskBonus(token, service string) int {
	want, ok := taskService(token)
	if !ok || want != service {
		return 0
	}

	return 120
}

func taskService(token string) (string, bool) {
	switch token {
	case "mail", "email", "inbox", "message", "messages", "thread", "mailbox", "gmail":
		return "gmail", true
	case "drive", "file", "files", "folder":
		return "drive", true
	case "doc", "docs", "document", "documents":
		return "docs", true
	case "slide", "slides", "presentation", "presentations", "deck":
		return "slides", true
	case "calendar", "event", "events", "busy", "freebusy", "availability":
		return "calendar", true
	case "analytics", "ga4", "property", "properties", "metric", "metrics", "dimension", "report":
		return "analytics", true
	case "searchconsole", "webmaster", "webmasters", "site", "sites":
		return "searchconsole", true
	case "sheet", "sheets", "spreadsheet", "spreadsheets", "range", "cell":
		return "sheets", true
	default:
		return "", false
	}
}

func servicePrefix(operation mcpcontract.Operation) string {
	for _, action := range operation.Definition.Actions {
		if i := strings.IndexByte(action, ':'); i > 0 {
			return action[:i]
		}
	}

	name := operation.Definition.Name

	name = strings.TrimPrefix(name, "google_")
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name[:i]
	}

	return name
}

func boundSearchOutput(out capabilitiesSearchOutput) capabilitiesSearchOutput {
	for len(out.Operations) > 0 {
		data, err := json.Marshal(out)
		if err == nil && len(data) <= maxSearchReplyBytes {
			return out
		}
		out.Operations = out.Operations[:len(out.Operations)-1]
		out.Truncated = true
	}

	return out
}
