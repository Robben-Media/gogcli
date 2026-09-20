package access

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const authModeOAuth = "oauth"

// Authorizer binds a trusted principal to granted accounts, clients, operations,
// and existing policies. Callers must pass the startup Principal, never MCP metadata.
type Authorizer struct {
	snapshot atomic.Pointer[Snapshot]
	accounts AccountSource
}

func NewAuthorizer(snapshot Snapshot, accounts AccountSource) (*Authorizer, error) {
	if accounts == nil {
		return nil, invalid("account source is required")
	}

	if err := validateSnapshot(snapshot); err != nil {
		return nil, err
	}

	authorizer := &Authorizer{accounts: accounts}
	authorizer.Replace(snapshot)

	return authorizer, nil
}

func (a *Authorizer) Replace(snapshot Snapshot) {
	cloned := cloneSnapshot(snapshot)
	a.snapshot.Store(&cloned)
}

func (a *Authorizer) Preview(snapshot Snapshot) (*Authorizer, error) {
	return NewAuthorizer(snapshot, a.accounts)
}

func (a *Authorizer) load() Snapshot {
	if a == nil {
		return Snapshot{}
	}

	loaded := a.snapshot.Load()
	if loaded == nil {
		return Snapshot{}
	}

	return *loaded
}

// Visible reports whether tools/list may advertise operation for principal.
// analytics_metadata (AnyAction) is listed when any catalog action is allowed;
// invocation still authorizes every selected Call.Actions.
func (a *Authorizer) Visible(ctx context.Context, principal mcpcontract.Principal, operation string) (bool, error) {
	principalID := strings.TrimSpace(principal.ID)

	operation = strings.TrimSpace(operation)
	if principalID == "" || operation == "" {
		return false, nil
	}

	def, ok := mcpcontract.Lookup(operation)
	if !ok {
		return false, nil
	}

	snapshot := a.load()
	if def.Local {
		return operationAllowed(snapshot.AllowOperations, operation) && a.hasGrant(snapshot, principalID), nil
	}

	if !operationAllowed(snapshot.AllowOperations, operation) {
		return false, nil
	}

	identities, err := a.principalIdentities(ctx, principalID)
	if err != nil {
		return false, err
	}

	for _, identity := range identities {
		if def.AnyAction {
			for _, action := range def.Actions {
				if a.identityEligible(snapshot, principalID, identity, operation, []string{action}) {
					return true, nil
				}
			}

			continue
		}

		if a.identityEligible(snapshot, principalID, identity, operation, def.Actions) {
			return true, nil
		}
	}

	return false, nil
}

// Authorize resolves accountID for principal and authorizes every selected action
// before any upstream call. Decoder actions may only narrow the catalog set.
func (a *Authorizer) Authorize(ctx context.Context, principal mcpcontract.Principal, accountID string, operation string, actions []string) (mcpcontract.Identity, error) {
	principalID := strings.TrimSpace(principal.ID)
	if principalID == "" {
		return mcpcontract.Identity{}, authRequired("principal is required")
	}

	operation = strings.TrimSpace(operation)
	accountID = strings.TrimSpace(accountID)

	if operation == "" {
		return mcpcontract.Identity{}, invalid("operation is required")
	}

	def, ok := mcpcontract.Lookup(operation)
	if !ok {
		return mcpcontract.Identity{}, forbidden("unknown operation")
	}

	if def.Local {
		return mcpcontract.Identity{}, forbidden("operation does not take an account")
	}

	snapshot := a.load()
	if !operationAllowed(snapshot.AllowOperations, operation) {
		return mcpcontract.Identity{}, forbidden("operation is not enabled")
	}

	if accountID == "" {
		return mcpcontract.Identity{}, invalid("account_id is required")
	}

	identity, ok, err := a.accounts.Get(ctx, accountID)
	if err != nil {
		return mcpcontract.Identity{}, fmt.Errorf("lookup account: %w", err)
	}

	if !ok {
		return mcpcontract.Identity{}, forbidden("unknown account")
	}

	identity = identity.Clone()
	if identity.PrincipalID != principalID {
		return mcpcontract.Identity{}, forbidden("account is not granted to this caller")
	}

	selected, err := narrowActions(def.Actions, actions)
	if err != nil {
		return mcpcontract.Identity{}, err
	}

	if !a.identityEligible(snapshot, principalID, identity, operation, selected) {
		if !mcpReady(identity) {
			return mcpcontract.Identity{}, forbidden("account is not ready")
		}

		if identity.AuthMode != authModeOAuth {
			return mcpcontract.Identity{}, forbidden("account is not OAuth-connected")
		}

		if !a.grantMatchesActions(snapshot, principalID, identity, operation, selected) {
			return mcpcontract.Identity{}, forbidden("account is not granted to this caller")
		}

		if !a.actionsAllowed(snapshot, identity, selected) {
			return mcpcontract.Identity{}, forbidden("action is denied by policy")
		}

		if !mcpcontract.ScopesSatisfied(identity.Scopes, def) {
			return mcpcontract.Identity{}, insufficientScope("account is missing required Google scopes")
		}

		return mcpcontract.Identity{}, forbidden("account is not granted to this caller")
	}

	return identity, nil
}

// ListAccounts returns caller-filtered local metadata. No Google HTTP is issued.
func (a *Authorizer) ListAccounts(ctx context.Context, principal mcpcontract.Principal) ([]mcpcontract.Identity, error) {
	principalID := strings.TrimSpace(principal.ID)
	if principalID == "" {
		return nil, authRequired("principal is required")
	}

	snapshot := a.load()
	if !operationAllowed(snapshot.AllowOperations, "accounts_list") {
		return nil, forbidden("operation is not enabled")
	}

	if !a.hasGrant(snapshot, principalID) {
		return nil, forbidden("caller has no account grants")
	}

	records, err := a.accounts.List(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	out := make([]mcpcontract.Identity, 0, len(records))
	for _, identity := range records {
		identity = identity.Clone()
		if identity.PrincipalID != principalID {
			continue
		}

		if !mcpReady(identity) {
			continue
		}

		if !a.grantMatches(snapshot, principalID, identity, "accounts_list") && !a.grantMatchesAnyOperation(snapshot, principalID, identity) {
			continue
		}

		out = append(out, identity)
	}

	return out, nil
}

func (a *Authorizer) hasGrant(snapshot Snapshot, principalID string) bool {
	for _, grant := range snapshot.Grants {
		if grant.PrincipalID == principalID {
			return true
		}
	}

	return false
}

func (a *Authorizer) principalIdentities(ctx context.Context, principalID string) ([]mcpcontract.Identity, error) {
	records, err := a.accounts.List(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	out := make([]mcpcontract.Identity, 0, len(records))
	for _, identity := range records {
		identity = identity.Clone()
		if identity.PrincipalID != principalID || !mcpReady(identity) {
			continue
		}

		out = append(out, identity)
	}

	return out, nil
}

func (a *Authorizer) grantMatches(snapshot Snapshot, principalID string, identity mcpcontract.Identity, operation string) bool {
	def, _ := mcpcontract.Lookup(operation)
	return a.grantMatchesActions(snapshot, principalID, identity, operation, def.Actions)
}

func (a *Authorizer) grantMatchesActions(snapshot Snapshot, principalID string, identity mcpcontract.Identity, operation string, actions []string) bool {
	for _, grant := range snapshot.Grants {
		if grant.PrincipalID != principalID {
			continue
		}

		if !containsExact(grant.AccountIDs, identity.AccountID) {
			continue
		}

		// Client names are canonicalized lowercase by config.NormalizeClientName.
		if !containsFold(grant.ClientNames, identity.ClientName) {
			continue
		}

		if grantCovers(grant, operation, actions) {
			return true
		}
	}

	return false
}

func grantCovers(grant mcpcontract.Grant, operation string, actions []string) bool {
	if len(grant.Operations) == 0 {
		return false
	}

	if containsExact(grant.Operations, operation) {
		return true
	}

	if operation == "accounts_list" {
		return true
	}

	if len(actions) == 0 {
		return false
	}

	for _, action := range actions {
		if !matchesAnyAction(grant.Operations, action) {
			return false
		}
	}

	return true
}

func (a *Authorizer) grantMatchesAnyOperation(snapshot Snapshot, principalID string, identity mcpcontract.Identity) bool {
	for _, grant := range snapshot.Grants {
		if grant.PrincipalID != principalID {
			continue
		}

		if !containsExact(grant.AccountIDs, identity.AccountID) {
			continue
		}

		// Client names are canonicalized lowercase by config.NormalizeClientName.
		if !containsFold(grant.ClientNames, identity.ClientName) {
			continue
		}

		if len(grant.Operations) == 0 {
			continue
		}

		return true
	}

	return false
}

func (a *Authorizer) actionsAllowed(snapshot Snapshot, identity mcpcontract.Identity, actions []string) bool {
	for _, action := range actions {
		if !a.actionAllowed(snapshot, identity, action) {
			return false
		}
	}

	return len(actions) > 0
}

func (a *Authorizer) actionAllowed(snapshot Snapshot, identity mcpcontract.Identity, action string) bool {
	decision := evaluate(snapshot.Policies, action, identity.Email, identity.AccountID, identity.ClientName)
	return !decision.Denied
}

func narrowActions(catalog []string, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, invalid("operation actions are required")
	}

	selected := make([]string, 0, len(requested))
	for _, action := range requested {
		canonical := CanonicalAction(action)
		if canonical == "" {
			return nil, invalid("operation actions are required")
		}

		if !slices.Contains(catalog, canonical) {
			return nil, forbidden("action is outside the operation catalog")
		}

		if !slices.Contains(selected, canonical) {
			selected = append(selected, canonical)
		}
	}

	return selected, nil
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if equalFold(value, want) {
			return true
		}
	}

	return false
}

func containsExact(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}

	return false
}
