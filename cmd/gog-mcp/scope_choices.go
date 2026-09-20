package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/googlecatalog"
)

var errUnknownScope = errors.New("unknown Google OAuth scope")

// Additional consent is chosen by the operator, never inferred from available
// methods. Only scopes declared by the pinned Google discovery sources qualify.
func configuredConnectScopes(raw string) ([]accountconnect.ScopeChoice, error) {
	scopes := splitCSV(raw)
	if len(scopes) == 0 {
		return nil, nil
	}
	known := googlecatalog.MustLoad().Scopes
	out := make([]accountconnect.ScopeChoice, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := known[scope]; !ok {
			return nil, fmt.Errorf("%w %q", errUnknownScope, scope)
		}
		label := strings.TrimPrefix(scope, "https://www.googleapis.com/auth/")
		if scope == "https://mail.google.com/" {
			label = "gmail.full-access"
		}
		out = append(out, accountconnect.ScopeChoice{Capability: "scope." + label, Scope: scope})
	}
	return out, nil
}
