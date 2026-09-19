package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

	ctx, cancel := rt.withTimeout(ctx)
	defer cancel()

	if err := rt.acquire(ctx); err != nil {
		result := toolErrorResult(err)
		rt.logCall(ctx, accountsListName, traceID, started, result)

		return result, nil
	}
	defer rt.release()

	raw := json.RawMessage(`{}`)
	if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
		raw = request.Params.Arguments
	}

	if err := rt.checkSize(int64(len(raw)), "request"); err != nil {
		result := toolErrorResult(err)
		rt.logCall(ctx, accountsListName, traceID, started, result)

		return result, nil
	}

	if err := decodeStrict(raw, &accountsListInput{}); err != nil {
		result := toolErrorResult(err)
		rt.logCall(ctx, accountsListName, traceID, started, result)

		return result, nil
	}

	identities, err := rt.authorizer.ListAccounts(ctx, rt.principal)
	if err != nil {
		result := toolErrorResult(err)
		rt.logCall(ctx, accountsListName, traceID, started, result)

		return result, nil
	}

	accounts := make([]accountInfo, 0, len(identities))
	for _, identity := range identities {
		accounts = append(accounts, accountInfo{
			AccountID:    identity.AccountID,
			Label:        identity.Label,
			Email:        identity.Email,
			ClientName:   identity.ClientName,
			AuthMode:     identity.AuthMode,
			Capabilities: rt.authorizer.Capabilities(identity),
		})
	}

	result, err := rt.resultFor(accountsListOutput{Accounts: accounts})
	if err != nil {
		result = toolErrorResult(err)
	}

	rt.logCall(ctx, accountsListName, traceID, started, result)

	return result, nil
}

func decodeStrict(raw json.RawMessage, dest any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dest); err != nil {
		return mcpcontract.Invalid("arguments must match the tool input schema")
	}

	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return mcpcontract.Invalid("arguments must contain one JSON object")
	}

	return nil
}
