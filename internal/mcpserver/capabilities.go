package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/workflowguide"
)

type capabilitiesSearchInput struct {
	Query string `json:"query" jsonschema:"Task or service words to match granted operations"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum matches to return; defaults to 8 and is capped at 20"`
}

type capabilitySummary struct {
	Name        string `json:"name"`
	Service     string `json:"service"`
	Description string `json:"description"`
	Retry       string `json:"retry"`
}

type capabilitiesSearchOutput struct {
	Operations []capabilitySummary `json:"operations"`
	Truncated  bool                `json:"truncated"`
}

type capabilitiesDescribeInput struct {
	Name       string `json:"name" jsonschema:"Granted operation name returned by capabilities_search"`
	SchemaPath string `json:"schema_path,omitempty" jsonschema:"Optional dotted path into input or output, such as input.properties.requests.items"`
}

type capabilitiesDescribeOutput struct {
	Name            string              `json:"name"`
	Service         string              `json:"service"`
	Description     string              `json:"description"`
	Retry           string              `json:"retry"`
	Required        []string            `json:"required"`
	Guidance        string              `json:"guidance"`
	ServiceGuidance workflowguide.Guide `json:"service_guidance"`
	InputSchema     json.RawMessage     `json:"input_schema"`
	OutputSchema    json.RawMessage     `json:"output_schema"`
	SchemaPath      string              `json:"schema_path,omitempty"`
	Truncated       bool                `json:"truncated"`
	InspectPaths    []string            `json:"inspect_paths,omitempty"`
}

type capabilitiesExecuteInput struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func compactToolSchemas(name string) (*jsonschema.Schema, *jsonschema.Schema) {
	switch name {
	case capabilitiesSearchName:
		return mustSchema[capabilitiesSearchInput](), mustSchema[capabilitiesSearchOutput]()
	case capabilitiesDescribeName:
		return mustSchema[capabilitiesDescribeInput](), describeOutputSchema()
	case capabilitiesExecuteName:
		return executeInputSchema(), nil
	default:
		return nil, nil
	}
}

func mustSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic("compact tool schema: " + err.Error())
	}

	return schema
}

func describeOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: schemaTypeObject,
		Properties: map[string]*jsonschema.Schema{
			"name":             {Type: schemaTypeString},
			"service":          {Type: schemaTypeString},
			"description":      {Type: schemaTypeString},
			"retry":            {Type: schemaTypeString},
			"required":         {Type: schemaTypeArray, Items: &jsonschema.Schema{Type: schemaTypeString}},
			"guidance":         {Type: schemaTypeString},
			"service_guidance": {Type: schemaTypeObject},
			"input_schema":     {Type: schemaTypeObject},
			"output_schema":    {Type: schemaTypeObject},
			"schema_path":      {Type: schemaTypeString},
			"truncated":        {Type: schemaTypeBoolean},
			"inspect_paths":    {Type: schemaTypeArray, Items: &jsonschema.Schema{Type: schemaTypeString}},
		},
		Required: []string{"name", "service", "description", "retry", "required", "guidance", "service_guidance", "input_schema", "output_schema", "truncated"},
	}
}

func executeInputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: schemaTypeObject,
		Properties: map[string]*jsonschema.Schema{
			"name": {
				Type:        schemaTypeString,
				Description: "Native Google operation name returned by capabilities_search",
			},
			"arguments": {
				Type:        schemaTypeObject,
				Description: "Exact arguments for the selected operation, including account_id",
			},
		},
		Required: []string{"name", "arguments"},
	}
}

func (rt *Runtime) addCompactTool(name string) bool {
	input, output := compactToolSchemas(name)
	if input == nil {
		return false
	}

	switch name {
	case capabilitiesSearchName:
		rt.server.AddTool(&mcp.Tool{
			Name:         name,
			Description:  "Search granted Google operations by task. Returns bounded name/description summaries, not schemas.",
			InputSchema:  input,
			OutputSchema: output,
			Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: true, Title: "Search Google capabilities"},
		}, rt.handleCapabilitiesSearch)
	case capabilitiesDescribeName:
		rt.server.AddTool(&mcp.Tool{
			Name:         name,
			Description:  "Describe one granted Google operation, including selected schema, required inputs, and service guidance.",
			InputSchema:  input,
			OutputSchema: output,
			Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: true, Title: "Describe a Google capability"},
		}, rt.handleCapabilitiesDescribe)
	case capabilitiesExecuteName:
		rt.server.AddTool(&mcp.Tool{
			Name:        name,
			Description: "Execute one granted Google operation. Pass account_id from accounts_list in arguments.",
			InputSchema: input,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: !rt.enableWrites, Title: "Execute a Google capability"},
		}, rt.handleCapabilitiesExecute)
	default:
		return false
	}

	return true
}

func (rt *Runtime) handleCapabilitiesSearch(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	started := time.Now()
	traceID := newTraceID()

	ctx, cleanup, err := rt.beginLocal(ctx)
	if err != nil {
		return rt.finish(ctx, capabilitiesSearchName, traceID, started, toolErrorResult(err)), nil
	}
	defer cleanup()

	var input capabilitiesSearchInput
	if decodeErr := rt.decodeLocal(request, &input); decodeErr != nil {
		return rt.finish(ctx, capabilitiesSearchName, traceID, started, toolErrorResult(decodeErr)), nil
	}

	query := strings.TrimSpace(input.Query)
	if query == "" {
		return rt.finish(ctx, capabilitiesSearchName, traceID, started, toolErrorResult(searchQueryError("query is required"))), nil
	}

	if utf8.RuneCountInString(query) > maxCapabilityQueryRunes {
		return rt.finish(ctx, capabilitiesSearchName, traceID, started, toolErrorResult(searchQueryError("query exceeds the 200 character limit"))), nil
	}

	if len(tokenizeQuery(query)) == 0 {
		return rt.finish(ctx, capabilitiesSearchName, traceID, started, toolErrorResult(searchQueryError("query must contain a service or task word"))), nil
	}

	matches, truncated, err := rt.searchOperations(ctx, rt.authorizer, query, input.Limit)
	if err != nil {
		return rt.finish(ctx, capabilitiesSearchName, traceID, started, toolErrorResult(err)), nil
	}

	if len(matches) == 0 {
		return rt.finish(ctx, capabilitiesSearchName, traceID, started, toolErrorResult(searchNoMatchError())), nil
	}

	summaries := make([]capabilitySummary, 0, len(matches))
	for _, operation := range matches {
		summaries = append(summaries, capabilitySummary{
			Name:        operation.Definition.Name,
			Service:     servicePrefix(operation),
			Description: clipRunes(operation.Definition.Description, maxSearchDescriptionRunes),
			Retry:       string(operation.Definition.Retry),
		})
	}

	result, err := rt.resultFor(boundSearchOutput(capabilitiesSearchOutput{Operations: summaries, Truncated: truncated}))
	if err != nil {
		result = toolErrorResult(err)
	}

	return rt.finish(ctx, capabilitiesSearchName, traceID, started, result), nil
}

func (rt *Runtime) handleCapabilitiesDescribe(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	started := time.Now()
	traceID := newTraceID()

	ctx, cleanup, err := rt.beginLocal(ctx)
	if err != nil {
		return rt.finish(ctx, capabilitiesDescribeName, traceID, started, toolErrorResult(err)), nil
	}
	defer cleanup()

	var input capabilitiesDescribeInput
	if decodeErr := rt.decodeLocal(request, &input); decodeErr != nil {
		return rt.finish(ctx, capabilitiesDescribeName, traceID, started, toolErrorResult(decodeErr)), nil
	}

	if strings.TrimSpace(input.Name) == "" {
		return rt.finish(ctx, capabilitiesDescribeName, traceID, started, toolErrorResult(&mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "name is required"})), nil
	}

	operation, err := rt.selectedOperation(ctx, input.Name)
	if err != nil {
		return rt.finish(ctx, capabilitiesDescribeName, traceID, started, toolErrorResult(err)), nil
	}

	required := schemaRequired(operation.InputSchema)

	inputSchema, outputSchema, schemaPath, inspectPaths, truncated, err := describeOperationSchemas(operation, input.SchemaPath)
	if err != nil {
		return rt.finish(ctx, capabilitiesDescribeName, traceID, started, toolErrorResult(err)), nil
	}

	action := ""
	if len(operation.Definition.Actions) > 0 {
		action = operation.Definition.Actions[0]
	}

	result, err := rt.resultFor(capabilitiesDescribeOutput{
		Name:            operation.Definition.Name,
		Service:         servicePrefix(operation),
		Description:     clipRunes(operation.Definition.Description, maxDescribeDescriptionRunes),
		Retry:           string(operation.Definition.Retry),
		Required:        required,
		Guidance:        compactGuidance(required, truncated),
		ServiceGuidance: workflowguide.ForAction(action),
		InputSchema:     inputSchema,
		OutputSchema:    outputSchema,
		SchemaPath:      schemaPath,
		Truncated:       truncated,
		InspectPaths:    inspectPaths,
	})
	if err != nil {
		result = toolErrorResult(err)
	}

	return rt.finish(ctx, capabilitiesDescribeName, traceID, started, result), nil
}

func (rt *Runtime) handleCapabilitiesExecute(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	started := time.Now()
	traceID := newTraceID()

	var input capabilitiesExecuteInput
	if err := rt.decodeLocal(request, &input); err != nil {
		ctx, _ = googleapi.WithUpstreamCounter(ctx)
		ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

		return rt.finish(ctx, capabilitiesExecuteName, traceID, started, toolErrorResult(err)), nil
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		ctx, _ = googleapi.WithUpstreamCounter(ctx)
		ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

		return rt.finish(ctx, capabilitiesExecuteName, traceID, started, toolErrorResult(&mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "name is required"})), nil
	}

	if len(bytes.TrimSpace(input.Arguments)) == 0 {
		ctx, _ = googleapi.WithUpstreamCounter(ctx)
		ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

		return rt.finish(ctx, capabilitiesExecuteName, traceID, started, toolErrorResult(&mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "arguments is required"})), nil
	}

	if !rawJSONObject(input.Arguments) {
		ctx, _ = googleapi.WithUpstreamCounter(ctx)
		ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

		return rt.finish(ctx, capabilitiesExecuteName, traceID, started, toolErrorResult(&mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "arguments must be a JSON object"})), nil
	}

	if gatewayTool(name) {
		ctx, _ = googleapi.WithUpstreamCounter(ctx)
		ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

		return rt.finish(ctx, capabilitiesExecuteName, traceID, started, toolErrorResult(gatewayExecuteError())), nil
	}

	operation, ok := rt.lookupOperation(name)
	if !ok || (!rt.enableWrites && writeOperation(operation)) {
		ctx, _ = googleapi.WithUpstreamCounter(ctx)
		ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

		return rt.finish(ctx, capabilitiesExecuteName, traceID, started, toolErrorResult(operationUnavailableError())), nil
	}

	inner := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: operation.Definition.Name, Arguments: append(json.RawMessage(nil), input.Arguments...)}}
	if request != nil {
		inner.Session = request.Session
		inner.Extra = request.Extra
	}

	return rt.handlerFor(operation)(ctx, inner)
}

func rawJSONObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '{' && json.Valid(raw)
}

func (rt *Runtime) selectedOperation(ctx context.Context, name string) (mcpcontract.Operation, error) {
	if gatewayTool(name) {
		return mcpcontract.Operation{}, operationUnavailableError()
	}

	operation, ok := rt.lookupOperation(name)
	if !ok {
		return mcpcontract.Operation{}, operationUnavailableError()
	}

	visible, err := rt.operationDiscoverable(ctx, rt.authorizer, operation)
	if err != nil {
		return mcpcontract.Operation{}, err
	}

	if !visible {
		return mcpcontract.Operation{}, operationUnavailableError()
	}

	return operation, nil
}

func (rt *Runtime) beginLocal(ctx context.Context) (context.Context, func(), error) {
	ctx, _ = googleapi.WithUpstreamCounter(ctx)
	ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

	ctx, cancel := rt.withTimeout(ctx)
	if err := rt.acquire(ctx); err != nil {
		cancel()
		return ctx, func() {}, err
	}

	return ctx, func() {
		cancel()
		rt.release()
	}, nil
}

func (rt *Runtime) decodeLocal(request *mcp.CallToolRequest, dest any) error {
	raw := json.RawMessage(`{}`)
	if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
		raw = request.Params.Arguments
	}

	if err := rt.checkSize(int64(len(raw)), "request"); err != nil {
		return err
	}

	return decodeStrict(raw, dest)
}

func schemaRequired(schema *jsonschema.Schema) []string {
	if schema == nil {
		return []string{}
	}

	return append([]string(nil), schema.Required...)
}
