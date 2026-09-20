package googlecatalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed manifest.json
var manifestData []byte

var manifestState struct {
	once      sync.Once
	manifest  Manifest
	toolIndex map[string]int
	err       error
}

// Load returns the generated catalog. Callers must treat returned slices and
// maps as read-only; values are shared to avoid copying large schema trees.
func Load() (Manifest, error) {
	manifestState.once.Do(func() {
		if err := json.Unmarshal(manifestData, &manifestState.manifest); err != nil {
			manifestState.err = fmt.Errorf("decode google catalog: %w", err)
			return
		}

		if manifestState.manifest.SchemaVersion != SchemaVersion {
			//nolint:err113 // startup diagnostics intentionally include the actual version.
			manifestState.err = fmt.Errorf(
				"google catalog schema version %d, want %d",
				manifestState.manifest.SchemaVersion,
				SchemaVersion,
			)

			return
		}

		manifestState.err = validateLoadedManifest(&manifestState.manifest)
		if manifestState.err != nil {
			return
		}

		manifestState.toolIndex = make(map[string]int, len(manifestState.manifest.Methods))
		for index := range manifestState.manifest.Methods {
			manifestState.toolIndex[manifestState.manifest.Methods[index].ToolName] = index
		}
	})

	if manifestState.err != nil {
		return Manifest{}, manifestState.err
	}

	return manifestState.manifest, nil
}

// MustLoad is for startup paths that cannot continue without the catalog.
func MustLoad() Manifest {
	manifest, err := Load()
	if err != nil {
		panic(err)
	}

	return manifest
}

// Methods returns the generated method list.
func Methods() ([]Method, error) {
	manifest, err := Load()
	if err != nil {
		return nil, err
	}

	return manifest.Methods, nil
}

// MethodByToolName returns the generated method with this MCP tool name.
func MethodByToolName(toolName string) (Method, bool, error) {
	if _, err := Load(); err != nil {
		return Method{}, false, err
	}

	index, ok := manifestState.toolIndex[toolName]
	if !ok {
		return Method{}, false, nil
	}

	return manifestState.manifest.Methods[index], true, nil
}

// MethodsByService returns generated methods in generated order.
func MethodsByService(service string) ([]Method, error) {
	methods, err := Methods()
	if err != nil {
		return nil, err
	}
	out := make([]Method, 0)

	for _, method := range methods {
		if method.Service == service {
			out = append(out, method)
		}
	}

	return out, nil
}

// ScopesByService returns OAuth scopes declared by that service in discovery.
func ScopesByService(service string) (map[string]ScopeInfo, error) {
	manifest, err := Load()
	if err != nil {
		return nil, err
	}
	out := make(map[string]ScopeInfo)

	for scope, info := range manifest.Scopes {
		for _, candidate := range info.Services {
			if candidate == service {
				out[scope] = cloneScopeInfo(info)
				break
			}
		}
	}

	return out, nil
}

//nolint:err113 // validation diagnostics intentionally include catalog values.
func validateLoadedManifest(manifest *Manifest) error {
	if len(manifest.Source.Documents) == 0 {
		return fmt.Errorf("google catalog has no source documents")
	}

	documents := make(map[string]Document, len(manifest.Source.Documents))

	paths := make(map[string]bool, len(manifest.Source.Documents))
	for _, document := range manifest.Source.Documents {
		key := documentKey(document)
		if _, exists := documents[key]; exists {
			return fmt.Errorf("google catalog has duplicate document %s", key)
		}

		if paths[document.LocalPath] {
			return fmt.Errorf("google catalog has duplicate source path %q", document.LocalPath)
		}
		documents[key] = document

		paths[document.LocalPath] = true
		if document.Service == "" || document.Version == "" || len(document.Schemas) == 0 {
			return fmt.Errorf("google catalog document %s is incomplete", key)
		}

		for name := range document.Schemas {
			if name == "" {
				return fmt.Errorf("google catalog document %s has an empty schema name", key)
			}
		}
	}

	seenIDs := make(map[string]bool, len(manifest.Methods))
	for index := range manifest.Methods {
		method := &manifest.Methods[index]

		methodKey := method.Service + "\x00" + method.Version + "\x00" + method.ID
		if seenIDs[methodKey] {
			return fmt.Errorf("google catalog has duplicate method id %q in %s", method.ID, method.Service)
		}
		seenIDs[methodKey] = true

		document, ok := documents[documentKey(Document{
			Service: method.Service,
			Version: method.Version,
		})]
		if !ok {
			return fmt.Errorf("google catalog method %s has no source document", method.ID)
		}

		if err := validateSchemaRefs(method.Request, document.Schemas, method.ID); err != nil {
			return err
		}

		if err := validateSchemaRefs(method.Response, document.Schemas, method.ID); err != nil {
			return err
		}
	}

	return nil
}

//nolint:err113 // reference diagnostics intentionally include the method and schema.
func validateSchemaRefs(root *Schema, definitions map[string]*Schema, methodID string) error {
	seen := make(map[*Schema]bool)
	var walk func(*Schema) error
	walk = func(schema *Schema) error {
		if schema == nil || seen[schema] {
			return nil
		}

		seen[schema] = true
		if schema.Ref != "" {
			if _, exists := definitions[schema.Ref]; !exists {
				return fmt.Errorf(
					"google catalog method %s references missing schema %q",
					methodID,
					schema.Ref,
				)
			}
		}

		for _, candidate := range schema.AnyOf {
			if err := walk(candidate); err != nil {
				return err
			}
		}

		for _, candidate := range schema.OneOf {
			if err := walk(candidate); err != nil {
				return err
			}
		}

		for _, candidate := range schema.AllOf {
			if err := walk(candidate); err != nil {
				return err
			}
		}

		if err := walk(schema.Items); err != nil {
			return err
		}

		for _, property := range schema.Properties {
			if err := walk(property); err != nil {
				return err
			}
		}

		return walk(schema.AdditionalProperties)
	}

	return walk(root)
}

func documentKey(document Document) string {
	return document.Service + "\x00" + document.Version
}

func cloneScopeInfo(info ScopeInfo) ScopeInfo {
	out := info
	out.Services = append([]string(nil), info.Services...)

	return out
}
