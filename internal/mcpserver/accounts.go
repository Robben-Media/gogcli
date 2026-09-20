package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type accountsListInput struct{}

type accountInfo struct {
	AccountID    string   `json:"account_id"`
	Label        string   `json:"label"`
	Email        string   `json:"email"`
	ClientName   string   `json:"client_name"`
	AuthMode     string   `json:"auth_mode"`
	Capabilities []string `json:"capabilities"`
}

type accountsListOutput struct {
	Accounts []accountInfo `json:"accounts"`
}

func accountsListSchemas() (*jsonschema.Schema, *jsonschema.Schema) {
	input, err := jsonschema.For[accountsListInput](nil)
	if err != nil {
		panic("accounts_list input schema: " + err.Error())
	}

	output, err := jsonschema.For[accountsListOutput](nil)
	if err != nil {
		panic("accounts_list output schema: " + err.Error())
	}

	return input, output
}

func (rt *Runtime) handleAccountsList(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	started := time.Now()
	traceID := newTraceID()

	ctx, _ = googleapi.WithUpstreamCounter(ctx)
	ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

	ctx, cancel := rt.withTimeout(ctx)
	defer cancel()

	if err := rt.acquire(ctx); err != nil {
		return rt.finish(ctx, accountsListName, traceID, started, toolErrorResult(err)), nil
	}
	defer rt.release()

	raw := json.RawMessage(`{}`)
	if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
		raw = request.Params.Arguments
	}

	if err := rt.checkSize(int64(len(raw)), "request"); err != nil {
		return rt.finish(ctx, accountsListName, traceID, started, toolErrorResult(err)), nil
	}

	if err := decodeStrict(raw, &accountsListInput{}); err != nil {
		return rt.finish(ctx, accountsListName, traceID, started, toolErrorResult(err)), nil
	}

	identities, err := rt.authorizer.ListAccounts(ctx, rt.principal)
	if err != nil {
		return rt.finish(ctx, accountsListName, traceID, started, toolErrorResult(err)), nil
	}

	accounts := make([]accountInfo, 0, len(identities))
	for _, identity := range identities {
		caps, capErr := rt.accountCapabilities(ctx, identity)
		if capErr != nil {
			return rt.finish(ctx, accountsListName, traceID, started, toolErrorResult(capErr)), nil
		}

		accounts = append(accounts, accountInfo{
			AccountID:    identity.AccountID,
			Label:        identity.Label,
			Email:        identity.Email,
			ClientName:   identity.ClientName,
			AuthMode:     identity.AuthMode,
			Capabilities: caps,
		})
	}

	result, err := rt.resultFor(accountsListOutput{Accounts: accounts})
	if err != nil {
		result = toolErrorResult(err)
	}

	return rt.finish(ctx, accountsListName, traceID, started, result), nil
}

func decodeStrict(raw json.RawMessage, dest any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dest); err != nil {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "arguments must match the tool input schema"}
	}

	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "arguments must contain one JSON object"}
	}

	return nil
}

func (rt *Runtime) accountCapabilities(ctx context.Context, identity mcpcontract.Identity) ([]string, error) {
	groups := make(map[string][]mcpcontract.Operation)
	order := make([]string, 0, 8)

	for _, operation := range rt.operations {
		name := capabilityGroup(operation)
		if name == "" {
			continue
		}

		if !rt.enableWrites && writeOperation(operation) {
			continue
		}

		if _, ok := groups[name]; !ok {
			order = append(order, name)
		}
		groups[name] = append(groups[name], operation)
	}

	caps := make([]string, 0, len(order))
	for _, name := range order {
		ok, err := rt.capabilityGroupUsable(ctx, identity, groups[name])
		if err != nil {
			return nil, err
		}

		if ok {
			caps = append(caps, name)
		}
	}

	slices.Sort(caps)

	return caps, nil
}

func (rt *Runtime) capabilityGroupUsable(ctx context.Context, identity mcpcontract.Identity, operations []mcpcontract.Operation) (bool, error) {
	for _, operation := range operations {
		ok, err := rt.operationUsableBy(ctx, identity, operation)
		if err != nil {
			return false, err
		}

		if ok {
			return true, nil
		}
	}

	return false, nil
}

func (rt *Runtime) operationUsableBy(ctx context.Context, identity mcpcontract.Identity, operation mcpcontract.Operation) (bool, error) {
	actions := operation.Definition.Actions
	if operation.Definition.AnyAction {
		for _, action := range actions {
			_, err := rt.authorizer.Authorize(ctx, rt.principal, identity.AccountID, operation.Definition.Name, []string{action})
			if err == nil {
				return true, nil
			}

			if !isAccessDenial(err) {
				return false, fmt.Errorf("authorize %s: %w", operation.Definition.Name, err)
			}
		}

		return false, nil
	}

	_, err := rt.authorizer.Authorize(ctx, rt.principal, identity.AccountID, operation.Definition.Name, actions)
	if err == nil {
		return true, nil
	}

	if isAccessDenial(err) {
		return false, nil
	}

	return false, fmt.Errorf("authorize %s: %w", operation.Definition.Name, err)
}

func isAccessDenial(err error) bool {
	var typed *mcpcontract.Error
	if !errors.As(err, &typed) || typed == nil {
		return false
	}

	switch typed.Category {
	case mcpcontract.Forbidden, mcpcontract.AuthRequired, mcpcontract.InsufficientScope, mcpcontract.InvalidInput, mcpcontract.NotFound:
		return true
	default:
		return false
	}
}
