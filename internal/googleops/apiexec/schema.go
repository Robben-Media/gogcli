package apiexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/steipete/gogcli/internal/googlecatalog"
)

const (
	maxSchemaDepth   = 8
	maxStringBytes   = 8192
	maxPathBytes     = 2048
	maxRepeatedItems = 100
)

const (
	accountField     = "account_id"
	bodyField        = "body"
	schemaTypeNull   = "null"
	schemaTypeAny    = "any"
	schemaTypeString = "string"
	schemaTypeObject = "object"
	schemaTypeArray  = "array"
)

var blockedParameters = map[string]struct{}{
	"access_token":    {},
	"oauth_token":     {},
	"key":             {},
	"callback":        {},
	"alt":             {},
	"uploadType":      {},
	"upload_protocol": {},
	"userIp":          {},
	"quotaUser":       {},
}

type schemaScope struct {
	defs       map[string]*googlecatalog.Schema
	methodID   string
	httpMethod string
}

type preparedCall struct {
	accountID string
	path      map[string]string
	query     url.Values
	body      []byte
}

func scopeFor(method googlecatalog.Method) schemaScope {
	scope := schemaScope{methodID: method.ID, httpMethod: method.HTTPMethod}
	if method.Service == "" || method.Version == "" {
		return scope
	}

	manifest, err := googlecatalog.Load()
	if err != nil {
		return scope
	}

	for _, document := range manifest.Source.Documents {
		if document.Service == method.Service && document.Version == method.Version {
			scope.defs = document.Schemas
			return scope
		}
	}

	return scope
}

func inputSchema(method googlecatalog.Method, scope schemaScope) *jsonschema.Schema {
	ctx := newConvertCtx(scope, convertOpts{})
	properties := map[string]*jsonschema.Schema{
		accountField: {
			Type:        schemaTypeString,
			Description: "Opaque account ID returned by accounts_list; required for every Google operation",
		},
	}
	required := []string{accountField}
	order := []string{accountField}

	names := parameterNames(method)
	for _, name := range names {
		param := method.Parameters[name]
		properties[name] = parameterSchema(param, ctx)

		order = append(order, name)
		if param.Required && param.Default == nil {
			required = append(required, name)
		}
	}

	if includeBody(method) {
		ctx.opts.skipReadOnly = true

		body := ctx.convert(method.Request)
		if body == nil {
			body = &jsonschema.Schema{Type: schemaTypeObject, AdditionalProperties: additionalPropertiesFalse()}
		}
		body.Description = "JSON request body for this method"
		properties[bodyField] = body

		order = append(order, bodyField)
		if bodyRequired(method) {
			required = append(required, bodyField)
		}
	}

	return &jsonschema.Schema{
		Type:                 schemaTypeObject,
		Properties:           properties,
		Required:             required,
		AdditionalProperties: additionalPropertiesFalse(),
		PropertyOrder:        order,
		Defs:                 ctx.defs,
	}
}

func outputSchema() *jsonschema.Schema {
	failure := &jsonschema.Schema{
		Type: schemaTypeObject,
		Properties: map[string]*jsonschema.Schema{
			"source_id": {Type: schemaTypeString},
			"category":  {Type: schemaTypeString},
			"message":   {Type: schemaTypeString},
		},
		Required:             []string{"source_id", "category", "message"},
		AdditionalProperties: additionalPropertiesFalse(),
	}

	return &jsonschema.Schema{
		Type: schemaTypeObject,
		Properties: map[string]*jsonschema.Schema{
			"account_id":      {Type: schemaTypeString},
			"account_label":   {Type: schemaTypeString},
			"data":            {},
			"next_page_token": {Type: schemaTypeString},
			"truncated":       {Type: "boolean"},
			"fetched_at":      {Type: schemaTypeString},
			"partial_failures": {
				Types: []string{schemaTypeNull, schemaTypeArray},
				Items: failure,
			},
		},
		Required:             []string{"account_id", "account_label", "data", "truncated", "fetched_at"},
		AdditionalProperties: additionalPropertiesFalse(),
		PropertyOrder:        []string{"account_id", "account_label", "data", "next_page_token", "truncated", "fetched_at", "partial_failures"},
	}
}

func additionalPropertiesFalse() *jsonschema.Schema {
	return &jsonschema.Schema{Not: &jsonschema.Schema{}}
}

func includeBody(method googlecatalog.Method) bool {
	if method.Request == nil {
		return false
	}

	switch strings.ToUpper(method.HTTPMethod) {
	case httpMethodGet, httpMethodHead:
		return false
	default:
		return true
	}
}

func bodyRequired(method googlecatalog.Method) bool {
	if !includeBody(method) {
		return false
	}

	switch strings.ToUpper(method.HTTPMethod) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

const (
	httpMethodGet  = "GET"
	httpMethodHead = "HEAD"
)

func parameterNames(method googlecatalog.Method) []string {
	names := make([]string, 0, len(method.Parameters))
	for name, param := range method.Parameters {
		if _, blocked := blockedParameters[name]; blocked {
			continue
		}

		if param.Location != "path" && param.Location != "query" {
			continue
		}

		names = append(names, name)
	}

	sortStrings(names)

	return names
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}

type convertOpts struct {
	skipReadOnly bool
}

type convertCtx struct {
	scope    schemaScope
	opts     convertOpts
	defs     map[string]*jsonschema.Schema
	building map[string]bool
	named    map[*googlecatalog.Schema]string
	anon     int
}

func newConvertCtx(scope schemaScope, opts convertOpts) *convertCtx {
	return &convertCtx{
		scope:    scope,
		opts:     opts,
		defs:     make(map[string]*jsonschema.Schema),
		building: make(map[string]bool),
		named:    make(map[*googlecatalog.Schema]string),
	}
}

func parameterSchema(param googlecatalog.Parameter, ctx *convertCtx) *jsonschema.Schema {
	schema := ctx.convert(&param.Schema)
	if param.Repeated {
		schema = &jsonschema.Schema{Type: schemaTypeArray, Items: schema, MaxItems: intPtr(maxRepeatedItems)}
	}

	if param.Description != "" && schema.Description == "" {
		schema.Description = param.Description
	}

	return schema
}

func (c *convertCtx) convert(schema *googlecatalog.Schema) *jsonschema.Schema {
	schema = c.scope.resolve(schema, nil)
	if schema == nil {
		return &jsonschema.Schema{Type: schemaTypeObject}
	}

	if !namedSchema(schema) {
		return c.convertInline(schema)
	}

	name := c.ensureName(schema)
	c.define(name, schema)

	return &jsonschema.Schema{Ref: defRef(name)}
}

func namedSchema(schema *googlecatalog.Schema) bool {
	return schema.Ref != "" || len(schema.Properties) > 0 || schema.Items != nil || schema.AdditionalProperties != nil || len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 || len(schema.AllOf) > 0
}

func (c *convertCtx) ensureName(schema *googlecatalog.Schema) string {
	if schema.Ref != "" {
		return sanitizeDefName(schema.Ref)
	}

	if name, ok := c.named[schema]; ok {
		return name
	}

	c.anon++
	name := fmt.Sprintf("schema%d", c.anon)
	c.named[schema] = name

	return name
}

func (c *convertCtx) define(name string, schema *googlecatalog.Schema) {
	if _, exists := c.defs[name]; exists || c.building[name] {
		return
	}
	c.building[name] = true
	c.named[schema] = name
	c.defs[name] = c.convertInline(schema)
	delete(c.building, name)
}

func defRef(name string) string {
	return "#/$defs/" + name
}

func sanitizeDefName(name string) string {
	name = strings.TrimPrefix(name, "#/definitions/")

	name = strings.TrimPrefix(name, "#/$defs/")
	if name == "" {
		return "schema"
	}
	var out strings.Builder

	for _, r := range name {
		if r == '_' || r == '-' || r == '.' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			out.WriteRune(r)
			continue
		}

		out.WriteByte('_')
	}

	if out.Len() == 0 {
		return "schema"
	}

	return out.String()
}

func (c *convertCtx) convertInline(schema *googlecatalog.Schema) *jsonschema.Schema {
	out := &jsonschema.Schema{
		Description: schema.Description,
		Format:      schema.Format,
		ReadOnly:    schema.ReadOnly,
		Pattern:     schema.Pattern,
	}
	if schema.Default != nil {
		if raw, err := json.Marshal(schema.Default); err == nil {
			out.Default = raw
		}
	}

	if len(schema.Enum) > 0 {
		out.Enum = append([]any(nil), schema.Enum...)
	}
	out.Minimum = parseBound(schema.Minimum)
	out.Maximum = parseBound(schema.Maximum)

	switch {
	case len(schema.AnyOf) > 0:
		out.AnyOf = c.convertList(schema.AnyOf)
	case len(schema.OneOf) > 0:
		out.OneOf = c.convertList(schema.OneOf)
	case len(schema.AllOf) > 0:
		out.AllOf = c.convertList(schema.AllOf)
	}

	typ := schema.Type
	if typ == "" {
		if len(schema.Properties) > 0 || schema.AdditionalProperties != nil || schema.AdditionalPropertiesForbidden {
			typ = schemaTypeObject
		} else if schema.Items != nil {
			typ = schemaTypeArray
		}
	}

	if typ == schemaTypeAny {
		return out
	}

	switch typ {
	case schemaTypeArray:
		out.Type = schemaTypeArray
		out.Items = c.convert(schema.Items)
	case schemaTypeObject, "":
		if typ == schemaTypeObject || len(schema.Properties) > 0 || schema.AdditionalProperties != nil || schema.AdditionalPropertiesForbidden {
			out.Type = schemaTypeObject
			if len(schema.Properties) > 0 {
				out.Properties = make(map[string]*jsonschema.Schema, len(schema.Properties))
				for name, property := range schema.Properties {
					if property == nil {
						continue
					}

					if c.opts.skipReadOnly && propertyReadOnly(c.scope, schema, name, property) {
						continue
					}
					out.Properties[name] = c.convert(property)
				}
				out.Required = advertisedRequired(schema, c.scope, out.Properties)
			}

			if schema.AdditionalProperties != nil {
				out.AdditionalProperties = c.convert(schema.AdditionalProperties)
			} else {
				out.AdditionalProperties = additionalPropertiesFalse()
			}
		} else if typ != "" && typ != "any" {
			out.Type = typ
		}
	default:
		if typ != "any" {
			out.Type = typ
		}
	}

	return out
}

func (c *convertCtx) convertList(list []*googlecatalog.Schema) []*jsonschema.Schema {
	out := make([]*jsonschema.Schema, 0, len(list))
	for _, item := range list {
		out = append(out, c.convert(item))
	}

	return out
}

func advertisedRequired(schema *googlecatalog.Schema, scope schemaScope, properties map[string]*jsonschema.Schema) []string {
	required := make([]string, 0)

	for _, name := range objectRequired(schema, scope) {
		if _, ok := properties[name]; ok {
			required = append(required, name)
		}
	}

	return required
}

func objectRequired(schema *googlecatalog.Schema, scope schemaScope) []string {
	if schema == nil {
		return nil
	}
	seen := make(map[string]struct{})
	required := make([]string, 0)

	add := func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		required = append(required, name)
	}
	for _, name := range schema.Required {
		add(name)
	}

	for name, property := range schema.Properties {
		if requiredForMethod(scope, schema, name, property) {
			add(name)
		}
	}

	return required
}

func requiredForMethod(scope schemaScope, parent *googlecatalog.Schema, name string, property *googlecatalog.Schema) bool {
	if scope.methodID == "" {
		return false
	}

	if requiredIDsContain(property, scope.methodID) {
		return true
	}

	if parent == nil || parent.Ref == "" || scope.defs == nil {
		return false
	}

	definition := scope.defs[parent.Ref]
	if definition == nil || definition.Properties == nil {
		return false
	}

	return requiredIDsContain(definition.Properties[name], scope.methodID)
}

func requiredIDsContain(schema *googlecatalog.Schema, methodID string) bool {
	if schema == nil {
		return false
	}

	for _, id := range schema.RequiredFor {
		if id == methodID {
			return true
		}
	}

	return false
}

func parseBound(value *string) *float64 {
	if value == nil {
		return nil
	}

	number, err := strconv.ParseFloat(*value, 64)
	if err != nil {
		return nil
	}

	return &number
}

func intPtr(value int) *int {
	return &value
}

func (s schemaScope) resolve(schema *googlecatalog.Schema, stack []string) *googlecatalog.Schema {
	if schema == nil {
		return nil
	}

	resolved := schema
	if schema.Ref != "" && !hasShape(schema) {
		if len(stack) >= maxSchemaDepth {
			return overlayRefWrapper(schema, schema)
		}

		for _, seen := range stack {
			if seen == schema.Ref {
				return overlayRefWrapper(schema, schema)
			}
		}

		if s.defs != nil {
			if def := s.defs[schema.Ref]; def != nil {
				resolved = s.resolve(def, append(append([]string(nil), stack...), schema.Ref))
			}
		}
	}

	return overlayRefWrapper(schema, resolved)
}

func overlayRefWrapper(wrapper, resolved *googlecatalog.Schema) *googlecatalog.Schema {
	if wrapper == nil || resolved == nil {
		return resolved
	}

	if !wrapper.ReadOnly && len(wrapper.RequiredFor) == 0 {
		return resolved
	}

	if wrapper == resolved && wrapper.ReadOnly {
		return wrapper
	}

	out := *resolved
	if wrapper.ReadOnly {
		out.ReadOnly = true
	}

	if len(wrapper.RequiredFor) > 0 {
		out.RequiredFor = append([]string(nil), wrapper.RequiredFor...)
	}

	return &out
}

func propertyReadOnly(scope schemaScope, parent *googlecatalog.Schema, name string, property *googlecatalog.Schema) bool {
	if property != nil && property.ReadOnly {
		return true
	}

	if resolved := scope.resolve(property, nil); resolved != nil && resolved.ReadOnly {
		return true
	}

	if parent == nil || parent.Ref == "" || scope.defs == nil {
		return false
	}

	definition := scope.defs[parent.Ref]
	if definition == nil || definition.Properties == nil {
		return false
	}
	original := definition.Properties[name]

	return original != nil && original.ReadOnly
}

func hasShape(schema *googlecatalog.Schema) bool {
	return schema.Type != "" || len(schema.Properties) > 0 || schema.Items != nil || schema.AdditionalProperties != nil || schema.AdditionalPropertiesForbidden || len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 || len(schema.AllOf) > 0
}

func decodeArgs(method googlecatalog.Method, scope schemaScope, raw json.RawMessage) (preparedCall, error) {
	if int64(len(raw)) > maxRequestBytes {
		return preparedCall{}, invalid("arguments exceed the bounded request size")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var object map[string]json.RawMessage
	if err := dec.Decode(&object); err != nil {
		return preparedCall{}, invalid("arguments must match the tool input schema")
	}

	if err := dec.Decode(new(any)); !isEOF(err) {
		return preparedCall{}, invalid("arguments must contain one JSON object")
	}

	allowed := map[string]struct{}{accountField: {}}
	for _, name := range parameterNames(method) {
		allowed[name] = struct{}{}
	}

	if includeBody(method) {
		allowed[bodyField] = struct{}{}
	}

	for name := range object {
		if _, ok := allowed[name]; !ok {
			return preparedCall{}, invalid("arguments must match the tool input schema")
		}
	}

	accountID, err := decodeAccountID(object[accountField])
	if err != nil {
		return preparedCall{}, err
	}

	prepared := preparedCall{accountID: accountID, path: make(map[string]string), query: url.Values{}}

	for _, name := range parameterNames(method) {
		param := method.Parameters[name]

		value, ok := object[name]
		if !ok {
			switch {
			case param.Required && param.Default != nil:
				encoded, marshalErr := json.Marshal(param.Default)
				if marshalErr != nil {
					return preparedCall{}, invalid(name + " is invalid")
				}

				value = encoded
			case param.Required:
				return preparedCall{}, invalid(name + " is required")
			default:
				continue
			}
		}

		decoded, decodeErr := decodeJSON(value)
		if decodeErr != nil {
			return preparedCall{}, invalid(name + " is invalid")
		}

		decoded = coerceDiscoveryValue(param.Type, decoded)
		if err := assignParameter(method, prepared, name, param, scope, decoded); err != nil {
			return preparedCall{}, err
		}
	}

	if includeBody(method) {
		rawBody, ok := object[bodyField]
		if !ok {
			if bodyRequired(method) {
				return preparedCall{}, invalid("body is required")
			}
		} else {
			if int64(len(rawBody)) > maxRequestBytes {
				return preparedCall{}, invalid("body exceeds the bounded request size")
			}

			decoded, decodeErr := decodeJSON(rawBody)
			if decodeErr != nil {
				return preparedCall{}, invalid("body is invalid")
			}

			if err := validateValue(method.Request, scope, decoded, bodyField, 0, nil); err != nil {
				return preparedCall{}, err
			}

			if err := rejectReadOnly(method.Request, scope, decoded, bodyField, 0, nil); err != nil {
				return preparedCall{}, err
			}

			prepared.body = append([]byte(nil), rawBody...)
		}
	}

	if err := collectPathParams(method, prepared); err != nil {
		return preparedCall{}, err
	}

	return prepared, nil
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF)
}

func decodeAccountID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", invalid("account_id is required")
	}

	var accountID string
	if err := json.Unmarshal(raw, &accountID); err != nil {
		return "", invalid("account_id is required")
	}

	if strings.TrimSpace(accountID) == "" {
		return "", invalid("account_id is required")
	}

	return accountID, nil
}

func decodeJSON(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	return value, nil
}

func assignParameter(method googlecatalog.Method, prepared preparedCall, name string, param googlecatalog.Parameter, scope schemaScope, value any) error {
	if param.Repeated {
		items, ok := value.([]any)
		if !ok {
			return invalid(name + " must be an array")
		}

		if len(items) > maxRepeatedItems {
			return invalid(name + " has too many values")
		}

		for index, item := range items {
			if err := validateValue(&param.Schema, scope, item, fmt.Sprintf("%s[%d]", name, index), 0, nil); err != nil {
				return err
			}

			text, err := stringifyParam(name, item)
			if err != nil {
				return err
			}

			if param.Location == "query" {
				prepared.query.Add(name, text)
			} else {
				return invalid(name + " cannot be repeated in the path")
			}
		}

		return nil
	}

	if err := validateValue(&param.Schema, scope, value, name, 0, nil); err != nil {
		return err
	}

	text, err := stringifyParam(name, value)
	if err != nil {
		return err
	}

	switch param.Location {
	case "path":
		if err := validatePathValue(name, text, pathReserved(method, name)); err != nil {
			return err
		}
		prepared.path[name] = text
	case "query":
		prepared.query.Set(name, text)
	default:
		return invalid(name + " is invalid")
	}

	return nil
}

func pathReserved(method googlecatalog.Method, name string) bool {
	return strings.Contains(method.Path, "{+"+name+"}")
}

func coerceDiscoveryValue(typ string, value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}

	switch typ {
	case "boolean":
		switch text {
		case "true":
			return true
		case "false":
			return false
		}
	case "integer", "number":
		if typ == "integer" {
			if _, err := strconv.ParseInt(text, 10, 64); err == nil {
				return json.Number(text)
			}
		}

		if _, err := strconv.ParseFloat(text, 64); err == nil {
			return json.Number(text)
		}
	}

	return value
}

func stringifyParam(name string, value any) (string, error) {
	switch typed := value.(type) {
	case string:
		if len(typed) > maxStringBytes {
			return "", invalid(name + " is too long")
		}

		return typed, nil
	case bool:
		if typed {
			return "true", nil
		}

		return "false", nil
	case json.Number:
		return typed.String(), nil
	default:
		return "", invalid(name + " is invalid")
	}
}

func collectPathParams(method googlecatalog.Method, prepared preparedCall) error {
	remaining := method.Path
	for {
		start := strings.IndexByte(remaining, '{')
		if start < 0 {
			return nil
		}

		end := strings.IndexByte(remaining[start:], '}')
		if end < 0 {
			return invalid("method path is invalid")
		}
		token := remaining[start+1 : start+end]
		remaining = remaining[start+end+1:]

		token = strings.TrimPrefix(token, "+")

		if token == "" {
			return invalid("method path is invalid")
		}

		if _, ok := prepared.path[token]; ok {
			continue
		}

		param, exists := method.Parameters[token]
		if exists && param.Default != nil {
			decoded := coerceDiscoveryValue(param.Type, param.Default)

			text, err := stringifyParam(token, decoded)
			if err != nil {
				return err
			}

			if err := validatePathValue(token, text, pathReserved(method, token)); err != nil {
				return err
			}
			prepared.path[token] = text

			continue
		}

		return invalid(token + " is required")
	}
}

func validateValue(schema *googlecatalog.Schema, scope schemaScope, value any, path string, depth int, stack []string) error {
	if depth > maxSchemaDepth*2 {
		return invalid(path + " exceeds schema depth")
	}

	schema = scope.resolve(schema, stack)
	if schema == nil {
		return nil
	}

	if value == nil {
		if schema.Nullable || schema.Type == schemaTypeAny {
			return nil
		}

		return invalid(path + " is required")
	}

	if len(schema.Enum) > 0 && !enumContains(schema.Enum, value) {
		return invalid(path + " must be one of the allowed values")
	}

	if len(schema.AnyOf) > 0 {
		return validateOneBranch(schema.AnyOf, scope, value, path, depth, stack, false)
	}

	if len(schema.OneOf) > 0 {
		return validateOneBranch(schema.OneOf, scope, value, path, depth, stack, true)
	}

	if len(schema.AllOf) > 0 {
		for _, candidate := range schema.AllOf {
			if err := validateValue(candidate, scope, value, path, depth+1, stack); err != nil {
				return err
			}
		}
	}

	typ := schema.Type
	if typ == "" {
		if len(schema.Properties) > 0 || schema.AdditionalProperties != nil || schema.AdditionalPropertiesForbidden {
			typ = schemaTypeObject
		} else if schema.Items != nil {
			typ = schemaTypeArray
		}
	}

	if typ == "any" {
		return validateAnyValue(value, path)
	}

	switch typ {
	case schemaTypeString:
		text, ok := value.(string)
		if !ok {
			if schema.Format == "int64" || schema.Format == "uint64" {
				if _, ok := asNumber(value); ok {
					return checkBounds(schema, value, path)
				}
			}

			return invalid(path + " must be a string")
		}

		if len(text) > maxStringBytes {
			return invalid(path + " is too long")
		}

		if schema.Pattern != "" {
			matched, err := regexp.MatchString(schema.Pattern, text)
			if err != nil || !matched {
				return invalid(path + " does not match the required pattern")
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return invalid(path + " must be a boolean")
		}
	case "integer":
		if _, ok := asInteger(value); !ok {
			return invalid(path + " must be an integer")
		}

		if err := checkBounds(schema, value, path); err != nil {
			return err
		}
	case "number":
		if _, ok := asNumber(value); !ok {
			return invalid(path + " must be a number")
		}

		if err := checkBounds(schema, value, path); err != nil {
			return err
		}
	case schemaTypeArray:
		return validateArray(schema, scope, value, path, depth, stack)
	case schemaTypeObject:
		return validateObject(schema, scope, value, path, depth, stack)
	}

	return nil
}

func validateAnyValue(value any, path string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return invalid(path + " is invalid")
	}

	if int64(len(raw)) > maxRequestBytes {
		return invalid(path + " exceeds the bounded request size")
	}

	return nil
}

func validateArray(schema *googlecatalog.Schema, scope schemaScope, value any, path string, depth int, stack []string) error {
	items, ok := value.([]any)
	if !ok {
		return invalid(path + " must be an array")
	}

	if len(items) > maxRepeatedItems*10 {
		return invalid(path + " has too many values")
	}

	for index, item := range items {
		if err := validateValue(schema.Items, scope, item, fmt.Sprintf("%s[%d]", path, index), depth+1, stack); err != nil {
			return err
		}
	}

	return nil
}

func validateObject(schema *googlecatalog.Schema, scope schemaScope, value any, path string, depth int, stack []string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return invalid(path + " must be an object")
	}

	for _, name := range objectRequired(schema, scope) {
		if propertyReadOnly(scope, schema, name, schema.Properties[name]) {
			continue
		}

		if _, ok := object[name]; !ok {
			return invalid(path + "." + name + " is required")
		}
	}

	for name, item := range object {
		property, ok := schema.Properties[name]
		if !ok {
			if schema.AdditionalProperties != nil {
				if err := validateValue(schema.AdditionalProperties, scope, item, path+"."+name, depth+1, stack); err != nil {
					return err
				}

				continue
			}

			return invalid(path + " does not allow field " + name)
		}

		if err := validateValue(property, scope, item, path+"."+name, depth+1, stack); err != nil {
			return err
		}
	}

	return nil
}

func rejectReadOnly(schema *googlecatalog.Schema, scope schemaScope, value any, path string, depth int, stack []string) error {
	if depth > maxSchemaDepth*2 {
		return invalid(path + " exceeds schema depth")
	}

	if schema != nil && schema.ReadOnly {
		return invalid(path + " is read-only")
	}

	schema = scope.resolve(schema, stack)
	if schema == nil || value == nil {
		return nil
	}

	if schema.ReadOnly {
		return invalid(path + " is read-only")
	}

	for _, branch := range schema.AnyOf {
		if err := rejectReadOnly(branch, scope, value, path, depth+1, stack); err != nil {
			return err
		}
	}

	for _, branch := range schema.OneOf {
		if err := rejectReadOnly(branch, scope, value, path, depth+1, stack); err != nil {
			return err
		}
	}

	for _, branch := range schema.AllOf {
		if err := rejectReadOnly(branch, scope, value, path, depth+1, stack); err != nil {
			return err
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		for name, item := range typed {
			child := path + "." + name
			if property, ok := schema.Properties[name]; ok {
				if propertyReadOnly(scope, schema, name, property) {
					return invalid(child + " is read-only")
				}

				if err := rejectReadOnly(property, scope, item, child, depth+1, stack); err != nil {
					return err
				}

				continue
			}

			if schema.AdditionalProperties != nil {
				if err := rejectReadOnly(schema.AdditionalProperties, scope, item, child, depth+1, stack); err != nil {
					return err
				}
			}
		}
	case []any:
		if schema.Items == nil {
			return nil
		}

		if len(typed) > maxRepeatedItems*10 {
			return invalid(path + " has too many values")
		}

		for index, item := range typed {
			if err := rejectReadOnly(schema.Items, scope, item, fmt.Sprintf("%s[%d]", path, index), depth+1, stack); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateOneBranch(branches []*googlecatalog.Schema, scope schemaScope, value any, path string, depth int, stack []string, exactlyOne bool) error {
	matched := 0
	var last error

	for _, branch := range branches {
		err := validateValue(branch, scope, value, path, depth+1, stack)
		if err == nil {
			matched++
		} else {
			last = err
		}
	}

	if exactlyOne && matched != 1 {
		return invalid(path + " must match one schema")
	}

	if !exactlyOne && matched == 0 {
		if last != nil {
			return last
		}

		return invalid(path + " is invalid")
	}

	return nil
}

func enumContains(values []any, got any) bool {
	for _, want := range values {
		if jsonEqual(want, got) {
			return true
		}
	}

	return false
}

func jsonEqual(left any, right any) bool {
	leftRaw, leftErr := json.Marshal(normalizeJSON(left))

	rightRaw, rightErr := json.Marshal(normalizeJSON(right))
	if leftErr != nil || rightErr != nil {
		return false
	}

	return bytes.Equal(leftRaw, rightRaw)
}

func normalizeJSON(value any) any {
	switch typed := value.(type) {
	case json.Number:
		if i, err := typed.Int64(); err == nil {
			return i
		}

		if f, err := typed.Float64(); err == nil {
			return f
		}

		return typed.String()
	default:
		return value
	}
}

func asInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		i, err := typed.Int64()
		return i, err == nil
	case float64:
		i := int64(typed)
		return i, float64(i) == typed
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case string:
		i, err := strconv.ParseInt(typed, 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
}

func asNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		f, err := typed.Float64()
		return f, err == nil
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		f, err := strconv.ParseFloat(typed, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func checkBounds(schema *googlecatalog.Schema, value any, path string) error {
	number, ok := asNumber(value)
	if !ok {
		return invalid(path + " must be a number")
	}

	if schema.Minimum != nil {
		minimum, err := strconv.ParseFloat(*schema.Minimum, 64)
		if err == nil && number < minimum {
			return invalid(path + " is below the minimum")
		}
	}

	if schema.Maximum != nil {
		maximum, err := strconv.ParseFloat(*schema.Maximum, 64)
		if err == nil && number > maximum {
			return invalid(path + " is above the maximum")
		}
	}

	return nil
}
