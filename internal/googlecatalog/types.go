// Package googlecatalog contains normalized Google discovery metadata.
//
// The package is intentionally data-only: it has no authorization, transport,
// or CLI dependencies. A gate means that a method must not execute until the
// cited limitation is resolved; gates never imply that an account is selected.
package googlecatalog

// SchemaVersion identifies the compatibility contract of Manifest.
const SchemaVersion = 1

// Gate kinds used by generated availability metadata.
const (
	GateMissingScopes           = "missing_scopes"
	GateUnsupportedIdentity     = "unsupported_identity"
	GateExecutionNotImplemented = "execution_not_implemented"
)

// Manifest is the checked-in, normalized view of pinned Google discovery data.
type Manifest struct {
	SchemaVersion int                  `json:"schema_version"`
	Source        Source               `json:"source"`
	Scopes        map[string]ScopeInfo `json:"scopes"`
	Methods       []Method             `json:"methods"`
}

// Source records the exact inputs used to generate the manifest.
type Source struct {
	ModulePath    string     `json:"module_path"`
	ModuleVersion string     `json:"module_version"`
	APIListSHA256 string     `json:"api_list_sha256"`
	Documents     []Document `json:"documents"`
}

// Document records a pinned discovery document and the endpoint it mirrors.
type Document struct {
	DiscoveryID       string             `json:"discovery_id"`
	Service           string             `json:"service"`
	Version           string             `json:"version"`
	LocalPath         string             `json:"local_path"`
	OfficialURL       string             `json:"official_url"`
	Revision          string             `json:"revision"`
	SHA256            string             `json:"sha256"`
	OfficialStatus    string             `json:"official_status"`
	OfficialRevision  *string            `json:"official_revision,omitempty"`
	OfficialSHA256    *string            `json:"official_sha256,omitempty"`
	OfficialFetchedAt string             `json:"official_fetched_at"`
	OfficialNote      string             `json:"official_note,omitempty"`
	Schemas           map[string]*Schema `json:"schemas,omitempty"`
}

// Official endpoint status values.
const (
	OfficialMatch          = "official_match"
	OfficialRevisionDrift  = "official_revision_drift"
	OfficialUnavailable404 = "official_unavailable_404"
	OfficialNotInDirectory = "official_not_in_discovery_directory"
)

// ScopeInfo is aggregated OAuth scope metadata from discovery documents.
type ScopeInfo struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Services    []string `json:"services"`
}

// Method is one normalized Google REST method.
type Method struct {
	ID          string               `json:"id"`
	ToolName    string               `json:"tool_name"`
	Service     string               `json:"service"`
	Version     string               `json:"version"`
	Action      string               `json:"action"`
	Description string               `json:"description"`
	BaseURL     string               `json:"base_url"`
	Path        string               `json:"path"`
	HTTPMethod  string               `json:"http_method"`
	Scopes      []string             `json:"scopes"`
	Parameters  map[string]Parameter `json:"parameters"`
	Request     *Schema              `json:"request,omitempty"`
	Response    *Schema              `json:"response,omitempty"`
	ReadOnly    bool                 `json:"read_only"`
	Media       *Media               `json:"media,omitempty"`
	Identity    IdentitySupport      `json:"identity"`
	Gates       []Gate               `json:"gates"`
}

// Parameter is a discovery path or query parameter.
type Parameter struct {
	Schema
	Location string `json:"location"`
	Required bool   `json:"required"`
	Repeated bool   `json:"repeated"`
}

// Schema is a JSON-discovery schema. Ref values resolve in Document.Schemas.
//
//nolint:tagliatelle // Schema JSON preserves Google Discovery field names.
type Schema struct {
	Ref                           string             `json:"$ref,omitempty"`
	Type                          string             `json:"type,omitempty"`
	Format                        string             `json:"format,omitempty"`
	Description                   string             `json:"description,omitempty"`
	Default                       any                `json:"default,omitempty"`
	Required                      []string           `json:"required,omitempty"`
	RequiredFor                   []string           `json:"required_for,omitempty"`
	Enum                          []any              `json:"enum,omitempty"`
	EnumDescriptions              []string           `json:"enumDescriptions,omitempty"`
	Items                         *Schema            `json:"items,omitempty"`
	Properties                    map[string]*Schema `json:"properties,omitempty"`
	AdditionalProperties          *Schema            `json:"additionalProperties,omitempty"`
	AdditionalPropertiesForbidden bool               `json:"additionalPropertiesForbidden,omitempty"`
	Minimum                       *string            `json:"minimum,omitempty"`
	Maximum                       *string            `json:"maximum,omitempty"`
	Pattern                       string             `json:"pattern,omitempty"`
	ReadOnly                      bool               `json:"readOnly,omitempty"`
	Nullable                      bool               `json:"nullable,omitempty"`
	AnyOf                         []*Schema          `json:"anyOf,omitempty"`
	OneOf                         []*Schema          `json:"oneOf,omitempty"`
	AllOf                         []*Schema          `json:"allOf,omitempty"`
}

// Media records upload and download capabilities from discovery.
type Media struct {
	Upload   *MediaUpload   `json:"upload,omitempty"`
	Download *MediaDownload `json:"download,omitempty"`
}

// MediaUpload describes Google discovery upload protocol endpoints. The
// multipart endpoint is the simple protocol path when simple.multipart is true.
type MediaUpload struct {
	Accept        []string `json:"accept"`
	MaxSize       int64    `json:"max_size"`
	SimplePath    string   `json:"simple_path,omitempty"`
	MultipartPath string   `json:"multipart_path,omitempty"`
	ResumablePath string   `json:"resumable_path,omitempty"`
}

// MediaDownload describes direct and alternate download-service endpoints.
type MediaDownload struct {
	Path               string `json:"path"`
	UseDownloadService bool   `json:"use_download_service"`
}

// IdentitySupport records modes verified from auth/CLI support, not scopes.
type IdentitySupport struct {
	Modes     []IdentityMode `json:"modes"`
	AuthModel string         `json:"auth_model,omitempty"`
	Gate      *Gate          `json:"gate,omitempty"`
}

// IdentityMode is an authentication principal supported by a method family.
type IdentityMode string

// Supported identity modes.
const (
	IdentityUser           IdentityMode = "user_oauth"
	IdentityServiceAccount IdentityMode = "service_account"
)

// Gate prevents execution for a documented reason.
type Gate struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// Supported reports whether every generated authorization/execution gate is clear.
func (m Method) Supported() bool { return len(m.Gates) == 0 }

// SupportedForIdentity reports whether the method supports the requested mode
// and has no unrelated gates. The returned reason is empty when supported.
func (m Method) SupportedForIdentity(mode IdentityMode) (bool, string) {
	supportedIdentity := false

	for _, candidate := range m.Identity.Modes {
		if candidate == mode {
			supportedIdentity = true
			break
		}
	}

	if !supportedIdentity {
		reason := "identity mode " + string(mode) + " is not documented for this method family"
		if m.Identity.Gate != nil {
			reason = m.Identity.Gate.Reason
		}

		return false, reason
	}

	for _, gate := range m.Gates {
		if gate.Kind != GateUnsupportedIdentity {
			return false, gate.Reason
		}
	}

	return true, ""
}

// UnavailableReason returns the first generated gate reason.
func (m Method) UnavailableReason() string {
	if m.Supported() {
		return ""
	}

	return m.Gates[0].Reason
}
