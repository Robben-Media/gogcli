package mcpserver

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	maxDescribeSchemaBytes    = 8 << 10
	maxSchemaPathRunes        = 200
	maxSchemaPathSegments     = 24
	maxSchemaPathSegmentRunes = 64
	maxInspectPaths           = 64
	maxShallowProperties      = 64
	maxSchemaDescriptionRunes = 240
	maxSchemaPatternRunes     = 128
	maxSchemaDefaultBytes     = 256
	maxSchemaEnumBytes        = 512
)

func schemaPathError(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: message, Retryable: false}
}

func describeOperationSchemas(operation mcpcontract.Operation, schemaPath string) (in, out json.RawMessage, path string, inspect []string, truncated bool, err error) {
	path = strings.TrimSpace(schemaPath)
	if path == "" {
		inRaw, inInspect, inTrunc := projectSchema(operation.InputSchema, operation.InputSchema, "input")
		outRaw, outInspect, outTrunc := projectSchema(operation.OutputSchema, operation.OutputSchema, "output")

		inspect = append([]string(nil), inInspect...)

		inspect = append(inspect, outInspect...)
		if len(inspect) > maxInspectPaths {
			inspect = inspect[:maxInspectPaths]
		}

		return inRaw, outRaw, "", inspect, inTrunc || outTrunc, nil
	}

	if utf8.RuneCountInString(path) > maxSchemaPathRunes {
		return nil, nil, "", nil, false, schemaPathError("schema_path exceeds the 200 character limit")
	}

	parts := strings.Split(path, ".")
	if err := validateSchemaPathParts(parts); err != nil {
		return nil, nil, "", nil, false, err
	}

	switch parts[0] {
	case "input", "input_schema":
		parts[0] = "input"

		node, walkErr := walkSchemaPath(operation.InputSchema, parts[1:])
		if walkErr != nil {
			return nil, nil, "", nil, false, walkErr
		}
		normalized := strings.Join(parts, ".")
		raw, inspect, truncated := projectSchema(operation.InputSchema, node, normalized)

		return raw, json.RawMessage(`{}`), normalized, inspect, truncated, nil
	case "output", "output_schema":
		parts[0] = "output"

		node, walkErr := walkSchemaPath(operation.OutputSchema, parts[1:])
		if walkErr != nil {
			return nil, nil, "", nil, false, walkErr
		}
		normalized := strings.Join(parts, ".")
		raw, inspect, truncated := projectSchema(operation.OutputSchema, node, normalized)

		return json.RawMessage(`{}`), raw, normalized, inspect, truncated, nil
	default:
		return nil, nil, "", nil, false, schemaPathError("schema_path must start with input or output")
	}
}

func validateSchemaPathParts(parts []string) error {
	if len(parts) == 0 {
		return schemaPathError("schema_path is required")
	}

	if len(parts) > maxSchemaPathSegments {
		return schemaPathError("schema_path has too many segments")
	}

	for _, part := range parts {
		if part == "" {
			return schemaPathError("schema_path is invalid")
		}

		if utf8.RuneCountInString(part) > maxSchemaPathSegmentRunes {
			return schemaPathError("schema_path is invalid")
		}

		for _, r := range part {
			if unicode.IsSpace(r) || r == '/' || r == '\\' || unicode.IsControl(r) {
				return schemaPathError("schema_path is invalid")
			}
		}
	}

	return nil
}

func nextSchemaPathPart(parts []string, i int, message string) (string, int, error) {
	next := i + 1
	if next >= len(parts) {
		return "", i, schemaPathError(message)
	}

	return parts[next], next, nil
}

func walkSchemaPath(root *jsonschema.Schema, parts []string) (*jsonschema.Schema, error) {
	current, _ := resolveSchemaRef(root, root, nil)
	for i := 0; i < len(parts); i++ {
		if current == nil {
			return nil, schemaPathError("schema_path does not exist")
		}

		part := parts[i]
		switch part {
		case "properties":
			field, next, pathErr := nextSchemaPathPart(parts, i, "schema_path properties requires a field name")
			if pathErr != nil {
				return nil, pathErr
			}
			i = next

			child, ok := current.Properties[field]
			if !ok || child == nil {
				return nil, schemaPathError("schema_path does not exist")
			}
			current, _ = resolveSchemaRef(root, child, nil)
		case "items":
			if current.Items != nil {
				current, _ = resolveSchemaRef(root, current.Items, nil)
				continue
			}

			child, ok := current.Properties[part]
			if !ok || child == nil {
				return nil, schemaPathError("schema_path does not exist")
			}
			current, _ = resolveSchemaRef(root, child, nil)
		case "additionalProperties":
			if current.AdditionalProperties != nil {
				current, _ = resolveSchemaRef(root, current.AdditionalProperties, nil)
				continue
			}

			child, ok := current.Properties[part]
			if !ok || child == nil {
				return nil, schemaPathError("schema_path does not exist")
			}
			current, _ = resolveSchemaRef(root, child, nil)
		case "defs", "definitions":
			field, next, pathErr := nextSchemaPathPart(parts, i, "schema_path "+part+" requires a definition name")
			if pathErr != nil {
				return nil, pathErr
			}
			i = next

			child := lookupDef(root, field)
			if child == nil {
				return nil, schemaPathError("schema_path does not exist")
			}
			current, _ = resolveSchemaRef(root, child, nil)
		case "not":
			if current.Not != nil {
				current, _ = resolveSchemaRef(root, current.Not, nil)
				continue
			}

			child, ok := current.Properties[part]
			if !ok || child == nil {
				return nil, schemaPathError("schema_path does not exist")
			}
			current, _ = resolveSchemaRef(root, child, nil)
		case "anyOf", "oneOf", "allOf", "prefixItems":
			field, next, pathErr := nextSchemaPathPart(parts, i, "schema_path "+part+" requires an index")
			if pathErr != nil {
				return nil, pathErr
			}
			i = next

			index, ok := parseSchemaIndex(field)
			if !ok {
				return nil, schemaPathError("schema_path does not exist")
			}

			list := schemaList(current, part)
			if index < 0 || index >= len(list) || list[index] == nil {
				return nil, schemaPathError("schema_path does not exist")
			}
			current, _ = resolveSchemaRef(root, list[index], nil)
		default:
			if child, ok := current.Properties[part]; ok && child != nil {
				current, _ = resolveSchemaRef(root, child, nil)
				continue
			}

			return nil, schemaPathError("schema_path does not exist")
		}
	}

	if current == nil {
		return nil, schemaPathError("schema_path does not exist")
	}

	return current, nil
}

func schemaList(schema *jsonschema.Schema, keyword string) []*jsonschema.Schema {
	if schema == nil {
		return nil
	}

	switch keyword {
	case "anyOf":
		return schema.AnyOf
	case "oneOf":
		return schema.OneOf
	case "allOf":
		return schema.AllOf
	case "prefixItems":
		return schema.PrefixItems
	default:
		return nil
	}
}

func parseSchemaIndex(value string) (int, bool) {
	if value == "" {
		return 0, false
	}

	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > 1024 {
		return 0, false
	}

	return n, true
}

func projectSchema(root, schema *jsonschema.Schema, path string) (json.RawMessage, []string, bool) {
	resolved, cyclic := resolveSchemaRef(root, schema, nil)
	if cyclic {
		return marshalSchema(schemaOutline(schema)), nil, false
	}

	exact := marshalSchema(resolved)
	if len(exact) <= maxDescribeSchemaBytes && !hasExpandableRefs(root, resolved, nil) {
		return exact, nil, false
	}

	inspect := make([]string, 0, 8)
	summarized := summarizeSchema(root, resolved, path, &inspect, nil)
	raw, inspect := boundSummarizedSchema(summarized, path, inspect)

	return raw, inspect, true
}

func boundSummarizedSchema(schema *jsonschema.Schema, path string, inspect []string) (json.RawMessage, []string) {
	raw := marshalSchema(schema)
	if len(raw) <= maxDescribeSchemaBytes {
		return capInspectPaths(raw, inspect)
	}

	stripSchemaText(schema)

	raw = marshalSchema(schema)
	if len(raw) <= maxDescribeSchemaBytes {
		return capInspectPaths(raw, inspect)
	}

	if schema != nil && len(schema.Properties) > 0 {
		names := orderedPropertyNames(schema)
		for len(names) > 0 && len(raw) > maxDescribeSchemaBytes {
			last := names[len(names)-1]
			names = names[:len(names)-1]

			delete(schema.Properties, last)
			inspect = appendInspect(inspect, joinSchemaPath(path, "properties", last))
			raw = marshalSchema(schema)
		}
	}

	if len(raw) > maxDescribeSchemaBytes && schema != nil {
		dropIndexed := func(keyword string, children []*jsonschema.Schema) {
			for i := range children {
				inspect = appendInspect(inspect, joinSchemaPath(path, keyword, strconv.Itoa(i)))
			}
		}
		dropIndexed("anyOf", schema.AnyOf)
		dropIndexed("oneOf", schema.OneOf)
		dropIndexed("allOf", schema.AllOf)
		dropIndexed("prefixItems", schema.PrefixItems)
		schema.AnyOf = nil
		schema.OneOf = nil
		schema.AllOf = nil
		schema.PrefixItems = nil
		schema.Not = nil
		schema.Items = nil
		schema.AdditionalProperties = nil
		raw = marshalSchema(schema)
	}

	if len(raw) > maxDescribeSchemaBytes {
		schema = &jsonschema.Schema{Type: schemaTypeObject, Description: "Schema summarized; pass schema_path from inspect_paths to inspect one field."}
		raw = marshalSchema(schema)
	}

	return capInspectPaths(raw, inspect)
}

func capInspectPaths(raw json.RawMessage, inspect []string) (json.RawMessage, []string) {
	if len(inspect) > maxInspectPaths {
		inspect = inspect[:maxInspectPaths]
	}

	return raw, inspect
}

func appendInspect(inspect []string, childPath string) []string {
	if childPath == "" || len(inspect) >= maxInspectPaths {
		return inspect
	}

	for _, existing := range inspect {
		if existing == childPath {
			return inspect
		}
	}

	return append(inspect, childPath)
}

func stripSchemaText(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}

	schema.Title = ""

	schema.Description = ""
	for _, child := range schema.Properties {
		stripSchemaText(child)
	}

	stripSchemaText(schema.Items)
	stripSchemaText(schema.AdditionalProperties)
	stripSchemaText(schema.Not)

	for _, child := range schema.AnyOf {
		stripSchemaText(child)
	}

	for _, child := range schema.OneOf {
		stripSchemaText(child)
	}

	for _, child := range schema.AllOf {
		stripSchemaText(child)
	}

	for _, child := range schema.PrefixItems {
		stripSchemaText(child)
	}
}

func marshalSchema(schema *jsonschema.Schema) json.RawMessage {
	if schema == nil {
		return json.RawMessage(`{}`)
	}

	data, err := json.Marshal(schema)
	if err != nil || len(data) == 0 || string(data) == "true" {
		return json.RawMessage(`{}`)
	}

	return data
}

func summarizeSchema(root, schema *jsonschema.Schema, path string, inspect *[]string, stack []string) *jsonschema.Schema {
	resolved, cyclic := resolveSchemaRef(root, schema, stack)
	if cyclic || resolved == nil {
		return schemaOutline(schema)
	}

	if name := parseRefName(schema.Ref); name != "" {
		stack = append(stack, name)
	}

	out := schemaOutline(resolved)
	if schema.Ref != "" {
		out.Ref = schema.Ref
	}

	addInspect := func(childPath string) {
		if inspect == nil || len(*inspect) >= maxInspectPaths {
			return
		}

		for _, existing := range *inspect {
			if existing == childPath {
				return
			}
		}
		*inspect = append(*inspect, childPath)
	}

	if len(resolved.Properties) > 0 {
		names := orderedPropertyNames(resolved)
		limit := min(len(names), maxShallowProperties)

		out.Properties = make(map[string]*jsonschema.Schema, limit)
		if len(resolved.PropertyOrder) > 0 {
			out.PropertyOrder = append([]string(nil), names[:limit]...)
		}

		for i, name := range names {
			childPath := joinSchemaPath(path, "properties", name)
			if i >= maxShallowProperties {
				addInspect(childPath)
				continue
			}
			child := resolved.Properties[name]

			out.Properties[name] = childOutline(root, child, stack)
			if !isLeafIn(root, child, stack) {
				addInspect(childPath)
			}
		}
	}

	if resolved.Items != nil {
		out.Items = childOutline(root, resolved.Items, stack)
		if !isLeafIn(root, resolved.Items, stack) {
			addInspect(joinSchemaPath(path, "items"))
		}
	}

	if resolved.AdditionalProperties != nil {
		out.AdditionalProperties = childOutline(root, resolved.AdditionalProperties, stack)
		if !isLeafIn(root, resolved.AdditionalProperties, stack) {
			addInspect(joinSchemaPath(path, "additionalProperties"))
		}
	}

	copyIndexed := func(keyword string, children []*jsonschema.Schema) []*jsonschema.Schema {
		if len(children) == 0 {
			return nil
		}

		copied := make([]*jsonschema.Schema, 0, len(children))
		for i, child := range children {
			copied = append(copied, childOutline(root, child, stack))
			if !isLeafIn(root, child, stack) {
				addInspect(joinSchemaPath(path, keyword, strconv.Itoa(i)))
			}
		}

		return copied
	}
	out.AnyOf = copyIndexed("anyOf", resolved.AnyOf)
	out.OneOf = copyIndexed("oneOf", resolved.OneOf)
	out.AllOf = copyIndexed("allOf", resolved.AllOf)

	out.PrefixItems = copyIndexed("prefixItems", resolved.PrefixItems)
	if resolved.Not != nil {
		out.Not = childOutline(root, resolved.Not, stack)
		if !isLeafIn(root, resolved.Not, stack) {
			addInspect(joinSchemaPath(path, "not"))
		}
	}

	return out
}

func childOutline(root, child *jsonschema.Schema, stack []string) *jsonschema.Schema {
	resolved, cyclic := resolveSchemaRef(root, child, stack)
	if cyclic || resolved == nil {
		return schemaOutline(child)
	}

	out := schemaOutline(resolved)
	if child != nil && child.Ref != "" {
		out.Ref = child.Ref
	}

	return out
}

func schemaOutline(schema *jsonschema.Schema) *jsonschema.Schema {
	if schema == nil {
		return &jsonschema.Schema{Type: schemaTypeObject}
	}

	out := &jsonschema.Schema{
		Type:          schema.Type,
		Types:         slices.Clone(schema.Types),
		Title:         clipRunes(schema.Title, maxSchemaDescriptionRunes),
		Description:   clipRunes(schema.Description, maxSchemaDescriptionRunes),
		Format:        schema.Format,
		Pattern:       clipRunes(schema.Pattern, maxSchemaPatternRunes),
		Required:      slices.Clone(schema.Required),
		Deprecated:    schema.Deprecated,
		ReadOnly:      schema.ReadOnly,
		WriteOnly:     schema.WriteOnly,
		Ref:           schema.Ref,
		MinLength:     schema.MinLength,
		MaxLength:     schema.MaxLength,
		Minimum:       schema.Minimum,
		Maximum:       schema.Maximum,
		MinItems:      schema.MinItems,
		MaxItems:      schema.MaxItems,
		MinProperties: schema.MinProperties,
		MaxProperties: schema.MaxProperties,
		UniqueItems:   schema.UniqueItems,
	}
	if len(schema.Default) > 0 && len(schema.Default) <= maxSchemaDefaultBytes {
		out.Default = append(json.RawMessage(nil), schema.Default...)
	}

	if enum := copySmallEnum(schema.Enum); len(enum) > 0 {
		out.Enum = enum
	}

	return out
}

func copySmallEnum(enum []any) []any {
	if len(enum) == 0 {
		return nil
	}

	data, err := json.Marshal(enum)
	if err != nil || len(data) > maxSchemaEnumBytes {
		return nil
	}

	return slices.Clone(enum)
}

func isLeafIn(root, schema *jsonschema.Schema, stack []string) bool {
	resolved, cyclic := resolveSchemaRef(root, schema, stack)
	if cyclic || resolved == nil {
		return true
	}

	return isLeafSchema(resolved)
}

func isLeafSchema(schema *jsonschema.Schema) bool {
	if schema == nil {
		return true
	}

	if schema.Items != nil || len(schema.ItemsArray) > 0 || schema.AdditionalItems != nil || schema.Contains != nil || schema.UnevaluatedItems != nil {
		return false
	}

	if len(schema.Properties) > 0 || len(schema.PatternProperties) > 0 || schema.AdditionalProperties != nil || schema.UnevaluatedProperties != nil || schema.PropertyNames != nil {
		return false
	}

	if len(schema.DependentSchemas) > 0 {
		return false
	}

	if len(schema.AllOf) > 0 || len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 || schema.Not != nil || schema.If != nil || schema.Then != nil || schema.Else != nil {
		return false
	}

	if len(schema.PrefixItems) > 0 || schema.ContentSchema != nil {
		return false
	}

	return true
}

func hasExpandableRefs(root, schema *jsonschema.Schema, stack []string) bool {
	if schema == nil {
		return false
	}

	resolved, cyclic := resolveSchemaRef(root, schema, stack)
	if cyclic || resolved == nil {
		return false
	}

	if name := parseRefName(schema.Ref); name != "" {
		if !isLeafSchema(resolved) {
			return true
		}

		stack = append(stack, name)
	}

	for _, child := range resolved.Properties {
		if hasExpandableRefs(root, child, stack) {
			return true
		}
	}

	if hasExpandableRefs(root, resolved.Items, stack) || hasExpandableRefs(root, resolved.AdditionalProperties, stack) {
		return true
	}

	for _, child := range resolved.AnyOf {
		if hasExpandableRefs(root, child, stack) {
			return true
		}
	}

	for _, child := range resolved.OneOf {
		if hasExpandableRefs(root, child, stack) {
			return true
		}
	}

	for _, child := range resolved.AllOf {
		if hasExpandableRefs(root, child, stack) {
			return true
		}
	}

	for _, child := range resolved.PrefixItems {
		if hasExpandableRefs(root, child, stack) {
			return true
		}
	}

	return false
}

func resolveSchemaRef(root, schema *jsonschema.Schema, stack []string) (*jsonschema.Schema, bool) {
	if schema == nil {
		return nil, false
	}

	name := parseRefName(schema.Ref)
	if name == "" {
		return schema, false
	}

	if slices.Contains(stack, name) {
		return schema, true
	}

	def := lookupDef(root, name)
	if def == nil {
		return schema, false
	}

	return resolveSchemaRef(root, def, append(stack, name))
}

func lookupDef(root *jsonschema.Schema, name string) *jsonschema.Schema {
	if root == nil || name == "" {
		return nil
	}

	if def, ok := root.Defs[name]; ok && def != nil {
		return def
	}

	if def, ok := root.Definitions[name]; ok && def != nil {
		return def
	}

	return nil
}

func parseRefName(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}

	for _, prefix := range []string{"#/$defs/", "#/definitions/", "#/defs/"} {
		if name, ok := strings.CutPrefix(ref, prefix); ok {
			return name
		}
	}

	if strings.HasPrefix(ref, "#") {
		return ""
	}

	if strings.Contains(ref, "/") {
		return ""
	}

	return ref
}

func orderedPropertyNames(schema *jsonschema.Schema) []string {
	if schema == nil || len(schema.Properties) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(schema.Properties))

	names := make([]string, 0, len(schema.Properties))
	for _, name := range schema.PropertyOrder {
		if _, ok := schema.Properties[name]; !ok {
			continue
		}

		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	rest := make([]string, 0, len(schema.Properties)-len(seen))
	for name := range schema.Properties {
		if _, ok := seen[name]; ok {
			continue
		}
		rest = append(rest, name)
	}

	slices.Sort(rest)

	return append(names, rest...)
}

func joinSchemaPath(base string, segs ...string) string {
	parts := make([]string, 0, 1+len(segs))
	if strings.TrimSpace(base) != "" {
		parts = append(parts, base)
	}

	parts = append(parts, segs...)

	return strings.Join(parts, ".")
}

func clipRunes(value string, limit int) string {
	if limit <= 0 || value == "" || utf8.RuneCountInString(value) <= limit {
		return value
	}

	return string([]rune(value)[:limit])
}

func compactGuidance(required []string, truncated bool) string {
	var b strings.Builder
	b.WriteString("Call capabilities_execute with this operation name. arguments.account_id is required and must come from accounts_list.")

	if len(required) > 0 {
		b.WriteString(" Required fields: ")
		b.WriteString(strings.Join(required, ", "))
		b.WriteByte('.')
	}

	if truncated {
		b.WriteString(" Nested schema is summarized; pass schema_path from inspect_paths to inspect one field.")
	}

	return b.String()
}
