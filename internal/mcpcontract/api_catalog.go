package mcpcontract

import (
	"strings"
	"sync"

	"github.com/steipete/gogcli/internal/googlecatalog"
)

var apiDefinitions struct {
	once    sync.Once
	ordered []Definition
	byName  map[string]Definition
}

func loadAPIDefinitions() {
	apiDefinitions.once.Do(func() {
		apiDefinitions.byName = make(map[string]Definition)

		for _, method := range googlecatalog.MustLoad().Methods {
			if !method.Supported() || len(method.Scopes) == 0 {
				continue
			}

			if ok, _ := method.SupportedForIdentity(googlecatalog.IdentityUser); !ok {
				continue
			}

			retry := NonReplayableWrite
			if method.HTTPMethod == "GET" || method.HTTPMethod == "HEAD" || method.ReadOnly {
				retry = SafeRead
			}

			def := Definition{Name: method.ToolName, Description: method.Description, Actions: []string{strings.ToLower(method.Action)}, Scopes: executionScopes(method), AnyScope: true, Retry: retry}
			if !strings.HasPrefix(def.Name, "google_") || method.Action == "" {
				panic("invalid generated Google operation definition")
			}

			if _, exists := apiDefinitions.byName[def.Name]; exists {
				panic("duplicate generated Google operation definition")
			}
			apiDefinitions.byName[def.Name] = def
			apiDefinitions.ordered = append(apiDefinitions.ordered, def)
		}
	})
}

// Discovery can include scopes for referenced content alongside mutation
// authority. These mutators require a scope that permits changing their target;
// a read-only token must never authorize a write in the native server.
func executionScopes(method googlecatalog.Method) []string {
	var allowed []string

	switch method.ID {
	case "slides.presentations.batchUpdate":
		allowed = []string{"https://www.googleapis.com/auth/drive", "https://www.googleapis.com/auth/drive.file", SlidesWriteScope}
	case "drive.files.copy":
		allowed = []string{"https://www.googleapis.com/auth/drive", "https://www.googleapis.com/auth/drive.appdata", "https://www.googleapis.com/auth/drive.file"}
	case "people.otherContacts.copyOtherContactToMyContactsGroup":
		allowed = []string{"https://www.googleapis.com/auth/contacts"}
	default:
		return append([]string(nil), method.Scopes...)
	}

	var scopes []string

	for _, scope := range method.Scopes {
		for _, candidate := range allowed {
			if scope == candidate {
				scopes = append(scopes, scope)
			}
		}
	}

	return scopes
}

// APIDefinitions returns the user-OAuth API catalogue. Loading is lazy so the
// legacy CLI and curated read-only MCP do not parse expanded schemas at startup.
// An executor may impose additional media or protocol support restrictions.
func APIDefinitions() []Definition {
	loadAPIDefinitions()

	out := make([]Definition, 0, len(apiDefinitions.ordered))
	for _, def := range apiDefinitions.ordered {
		out = append(out, cloneDefinition(def))
	}

	return out
}

// AllDefinitions is for trusted grant validation, not a model-context payload.
func AllDefinitions() []Definition {
	return append(append(Catalog(), WorkflowCatalog()...), APIDefinitions()...)
}

func lookupAPI(name string) (Definition, bool) {
	if !strings.HasPrefix(name, "google_") {
		return Definition{}, false
	}

	loadAPIDefinitions()
	def, ok := apiDefinitions.byName[name]

	return cloneDefinition(def), ok
}

func cloneDefinition(def Definition) Definition {
	def.Actions = append([]string(nil), def.Actions...)
	def.Scopes = append([]string(nil), def.Scopes...)

	return def
}
