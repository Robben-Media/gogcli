// Command google-catalog generates the checked-in Google API method catalog
// from the google.golang.org/api module pinned by go.mod. It does not call
// Google APIs; live discovery URLs are recorded only as source provenance.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/steipete/gogcli/internal/googlecatalog"
)

const (
	modulePath    = "google.golang.org/api"
	moduleVersion = "v0.260.0"
)

type sourceSnapshots struct {
	APIListSHA256 string                   `json:"api_list_sha256"`
	Documents     []googlecatalog.Document `json:"documents"`
}

type rawResource struct {
	Methods   map[string]map[string]any `json:"methods"`
	Resources map[string]rawResource    `json:"resources"`
}

//nolint:tagliatelle // rawDiscovery JSON preserves Google Discovery field names.
type rawDiscovery struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Version     string                     `json:"version"`
	Revision    string                     `json:"revision"`
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	BaseURL     string                     `json:"baseUrl"`
	RootURL     string                     `json:"rootUrl"`
	ServicePath string                     `json:"servicePath"`
	Resources   map[string]rawResource     `json:"resources"`
	Schemas     map[string]json.RawMessage `json:"schemas"`
	Parameters  map[string]map[string]any  `json:"parameters"`
	Auth        struct {
		OAuth2 struct {
			Scopes map[string]map[string]any `json:"scopes"`
		} `json:"oauth2"`
	} `json:"auth"`
}

var (
	serviceByPath = map[string]string{
		"admin/directory/v1/admin-api.json":                                       "admin_directory",
		"admin/reports/v1/admin-api.json":                                         "admin_reports",
		"analyticsadmin/v1alpha/analyticsadmin-api.json":                          "analytics_admin_alpha",
		"analyticsadmin/v1beta/analyticsadmin-api.json":                           "analytics_admin_beta",
		"analyticsdata/v1beta/analyticsdata-api.json":                             "analytics_data",
		"bigquery/v2/bigquery-api.json":                                           "bigquery",
		"calendar/v3/calendar-api.json":                                           "calendar",
		"chat/v1/chat-api.json":                                                   "chat",
		"classroom/v1/classroom-api.json":                                         "classroom",
		"cloudidentity/v1/cloudidentity-api.json":                                 "cloud_identity",
		"docs/v1/docs-api.json":                                                   "docs",
		"drive/v3/drive-api.json":                                                 "drive",
		"gmail/v1/gmail-api.json":                                                 "gmail",
		"keep/v1/keep-api.json":                                                   "keep",
		"mybusinessaccountmanagement/v1/mybusinessaccountmanagement-api.json":     "mybusiness_account_management",
		"mybusinessbusinesscalls/v1/mybusinessbusinesscalls-api.json":             "mybusiness_business_calls",
		"mybusinessbusinessinformation/v1/mybusinessbusinessinformation-api.json": "mybusiness_business_information",
		"mybusinesslodging/v1/mybusinesslodging-api.json":                         "mybusiness_lodging",
		"mybusinessnotifications/v1/mybusinessnotifications-api.json":             "mybusiness_notifications",
		"mybusinessplaceactions/v1/mybusinessplaceactions-api.json":               "mybusiness_place_actions",
		"mybusinessqanda/v1/mybusinessqanda-api.json":                             "mybusiness_qanda",
		"mybusinessverifications/v1/mybusinessverifications-api.json":             "mybusiness_verifications",
		"people/v1/people-api.json":                                               "people",
		"searchconsole/v1/searchconsole-api.json":                                 "searchconsole",
		"sheets/v4/sheets-api.json":                                               "sheets",
		"slides/v1/slides-api.json":                                               "slides",
		"tagmanager/v2/tagmanager-api.json":                                       "tagmanager",
		"tasks/v1/tasks-api.json":                                                 "tasks",
		"youtube/v3/youtube-api.json":                                             "youtube",
	}

	safeReadPOST = map[string]bool{
		"analyticsadmin.accounts.runAccessReport":                    true,
		"analyticsadmin.accounts.searchChangeHistoryEvents":          true,
		"analyticsadmin.properties.runAccessReport":                  true,
		"analyticsdata.properties.audienceExports.query":             true,
		"analyticsdata.properties.runPivotReport":                    true,
		"analyticsdata.properties.runReport":                         true,
		"analyticsdata.properties.batchRunPivotReports":              true,
		"analyticsdata.properties.batchRunReports":                   true,
		"analyticsdata.properties.checkCompatibility":                true,
		"analyticsdata.properties.runRealtimeReport":                 true,
		"bigquery.routines.getIamPolicy":                             true,
		"bigquery.routines.testIamPermissions":                       true,
		"bigquery.rowAccessPolicies.getIamPolicy":                    true,
		"bigquery.rowAccessPolicies.testIamPermissions":              true,
		"bigquery.tables.getIamPolicy":                               true,
		"bigquery.tables.testIamPermissions":                         true,
		"calendar.freebusy.query":                                    true,
		"drive.files.download":                                       true,
		"mybusinessbusinessinformation.googleLocations.search":       true,
		"mybusinessverifications.locations.fetchVerificationOptions": true,
		"searchconsole.urlInspection.index.inspect":                  true,
		"searchconsole.urlTestingTools.mobileFriendlyTest.run":       true,
		"sheets.spreadsheets.developerMetadata.search":               true,
		"sheets.spreadsheets.getByDataFilter":                        true,
		"sheets.spreadsheets.values.batchGetByDataFilter":            true,
		"tagmanager.accounts.containers.workspaces.folders.entities": true,
		"webmasters.searchanalytics.query":                           true,
	}

	identityModes = map[string][]googlecatalog.IdentityMode{
		"admin_directory": {googlecatalog.IdentityUser, googlecatalog.IdentityServiceAccount},
		"admin_reports":   {googlecatalog.IdentityUser, googlecatalog.IdentityServiceAccount},
		"bigquery":        {googlecatalog.IdentityUser, googlecatalog.IdentityServiceAccount},
		"cloud_identity":  {googlecatalog.IdentityServiceAccount},
		"keep":            {googlecatalog.IdentityServiceAccount},
	}

	// Cloud Identity Groups documents delegated user access while other Cloud
	// Identity families remain restricted to service accounts in this catalog.
	identityModesByMethodPrefix = map[string][]googlecatalog.IdentityMode{
		"cloudidentity.groups.": {
			googlecatalog.IdentityUser,
			googlecatalog.IdentityServiceAccount,
		},
		"cloudidentity.devices.": {
			googlecatalog.IdentityUser,
			googlecatalog.IdentityServiceAccount,
		},
	}

	pathParameterPattern = regexp.MustCompile(`\{[^}]+\}`)
	toolNamePattern      = regexp.MustCompile(`^google_[a-z0-9_]+$`)
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "google-catalog:", err)
		os.Exit(1)
	}
}

//nolint:err113,gosec,wrapcheck // CLI diagnostics embed inputs; module and output paths are operator-supplied by design.
func run(args []string) error {
	flags := flag.NewFlagSet("google-catalog", flag.ContinueOnError)
	moduleRoot := flags.String("module-root", os.Getenv("GOG_GOOGLE_API_MODULE_ROOT"), "path to google.golang.org/api@v0.260.0")

	output := flags.String("output", filepath.Join("internal", "googlecatalog", "manifest.json"), "generated manifest path")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *moduleRoot == "" {
		return errors.New("set -module-root or GOG_GOOGLE_API_MODULE_ROOT")
	}

	snapshots, err := loadSourceSnapshots(filepath.Join("scripts", "google-catalog", "source-snapshots.json"))
	if err != nil {
		return err
	}

	apiList, err := os.ReadFile(filepath.Join(*moduleRoot, "api-list.json"))
	if err != nil {
		return err
	}

	if actual := checksum(apiList); actual != snapshots.APIListSHA256 {
		return fmt.Errorf("api-list.json checksum mismatch: got %s, want %s", actual, snapshots.APIListSHA256)
	}

	scopes := make(map[string]googlecatalog.ScopeInfo)
	var methods []googlecatalog.Method
	seenTools := make(map[string]string)

	for snapshotIndex := range snapshots.Documents {
		snapshot := &snapshots.Documents[snapshotIndex]

		service, ok := serviceByPath[snapshot.LocalPath]
		if !ok {
			return fmt.Errorf("no service mapping for %s", snapshot.LocalPath)
		}

		rawData, readErr := os.ReadFile(filepath.Join(*moduleRoot, filepath.FromSlash(snapshot.LocalPath)))
		if readErr != nil {
			return readErr
		}

		if actual := checksum(rawData); actual != snapshot.SHA256 {
			return fmt.Errorf("%s checksum mismatch: got %s, want %s", snapshot.LocalPath, actual, snapshot.SHA256)
		}

		var raw rawDiscovery
		if decodeErr := json.Unmarshal(rawData, &raw); decodeErr != nil {
			return fmt.Errorf("decode %s: %w", snapshot.LocalPath, decodeErr)
		}

		if raw.Revision != snapshot.Revision {
			return fmt.Errorf("%s revision mismatch: got %s, want %s", snapshot.LocalPath, raw.Revision, snapshot.Revision)
		}

		snapshot.DiscoveryID = raw.ID
		snapshot.Service = service
		snapshot.Version = raw.Version
		addServiceScopes(scopes, service, raw)

		generated, schemas, generationErr := methodsFromDocument(service, raw)
		if generationErr != nil {
			return fmt.Errorf("%s: %w", snapshot.LocalPath, generationErr)
		}
		snapshot.Schemas = schemas

		for _, method := range generated {
			if previous, ok := seenTools[method.ToolName]; ok {
				return fmt.Errorf("duplicate tool name %s in %s and %s", method.ToolName, previous, snapshot.LocalPath)
			}
			seenTools[method.ToolName] = snapshot.LocalPath
			methods = append(methods, method)
		}
	}

	for scope, info := range scopes {
		sort.Strings(info.Services)
		scopes[scope] = info
	}

	sort.Slice(methods, func(i, j int) bool { return methods[i].ToolName < methods[j].ToolName })

	manifest := googlecatalog.Manifest{
		SchemaVersion: googlecatalog.SchemaVersion,
		Source: googlecatalog.Source{
			ModulePath:    modulePath,
			ModuleVersion: moduleVersion,
			APIListSHA256: snapshots.APIListSHA256,
			Documents:     snapshots.Documents,
		},
		Scopes:  scopes,
		Methods: methods,
	}
	if validationErr := validateManifest(manifest); validationErr != nil {
		return validationErr
	}

	encoded, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(*output, encoded, 0o644); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "generated %s: %d methods, %d scopes, %d bytes\n", *output, len(methods), len(scopes), len(encoded))

	return nil
}

//nolint:err113,gosec,wrapcheck // source inventory paths and diagnostics are intentional generator inputs.
func loadSourceSnapshots(path string) (sourceSnapshots, error) {
	var snapshots sourceSnapshots

	data, err := os.ReadFile(path)
	if err != nil {
		return snapshots, err
	}

	if err := json.Unmarshal(data, &snapshots); err != nil {
		return snapshots, fmt.Errorf("decode %s: %w", path, err)
	}

	if len(snapshots.Documents) != len(serviceByPath) {
		return snapshots, fmt.Errorf("source snapshot count %d, want %d", len(snapshots.Documents), len(serviceByPath))
	}

	return snapshots, nil
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func addServiceScopes(scopes map[string]googlecatalog.ScopeInfo, service string, raw rawDiscovery) {
	add := func(scope string) {
		info, ok := scopes[scope]
		if !ok {
			info = googlecatalog.ScopeInfo{}
			if metadata, found := raw.Auth.OAuth2.Scopes[scope]; found {
				info.Title = stringMap(metadata)["title"]
				info.Description = stringMap(metadata)["description"]
			}
		}

		info.Services = append(info.Services, service)
		scopes[scope] = info
	}
	for scope := range raw.Auth.OAuth2.Scopes {
		add(scope)
	}
}

func methodsFromDocument(service string, raw rawDiscovery) ([]googlecatalog.Method, map[string]*googlecatalog.Schema, error) {
	var methods []googlecatalog.Method
	apiName := strings.SplitN(raw.ID, ":", 2)[0]

	err := walkResources(service, apiName, raw, nil, &methods)
	if err != nil {
		return nil, nil, err
	}

	schemas, err := schemasFromRaw(raw.Schemas)
	if err != nil {
		return nil, nil, err
	}

	return methods, schemas, nil
}

func schemasFromRaw(raw map[string]json.RawMessage) (map[string]*googlecatalog.Schema, error) {
	schemas := make(map[string]*googlecatalog.Schema, len(raw))
	for name, rawSchema := range raw {
		var value map[string]any
		if err := json.Unmarshal(rawSchema, &value); err != nil {
			return nil, fmt.Errorf("decode schema %s: %w", name, err)
		}
		converted := schemaFromRaw(value)
		schemas[name] = &converted
	}

	return schemas, nil
}

func walkResources(service string, apiName string, raw rawDiscovery, names []string, methods *[]googlecatalog.Method) error {
	root := rawResource{Resources: raw.Resources}
	return walkResource(service, apiName, raw, root, names, methods)
}

func walkResource(service string, apiName string, raw rawDiscovery, resource rawResource, names []string, methods *[]googlecatalog.Method) error {
	methodNames := sortedRawKeys(resource.Methods)
	for _, methodName := range methodNames {
		rawMethod := resource.Methods[methodName]

		*methods = append(*methods, methodFromRaw(service, apiName, raw, names, methodName, rawMethod))
	}

	resourceNames := make([]string, 0, len(resource.Resources))
	for name := range resource.Resources {
		resourceNames = append(resourceNames, name)
	}

	sort.Strings(resourceNames)

	for _, name := range resourceNames {
		if err := walkResource(service, apiName, raw, resource.Resources[name], append(names, name), methods); err != nil {
			return err
		}
	}

	return nil
}

func methodFromRaw(service string, apiName string, raw rawDiscovery, resourceNames []string, methodName string, rawMethod map[string]any) googlecatalog.Method {
	resourcePath := strings.Join(resourceNames, ".")
	discoveryMethod := rawMethod["id"]

	methodID, _ := discoveryMethod.(string)
	if methodID == "" {
		methodID = raw.ID + "." + strings.Join(resourceNames, ".") + "." + methodName
	}

	baseURL := raw.BaseURL
	if baseURL == "" {
		baseURL = raw.RootURL + raw.ServicePath
	}
	path, _ := rawMethod["path"].(string)
	httpMethod, _ := rawMethod["httpMethod"].(string)
	scopes := stringSlice(rawMethod["scopes"])

	toolName := "google_" + service
	for _, name := range resourceNames {
		toolName += "_" + normalizeToken(name)
	}
	toolName += "_" + normalizeToken(methodName)

	action := service + ":" + apiName
	if resourcePath != "" {
		action += "." + resourcePath
	}
	action += "." + methodName

	parameters := make(map[string]googlecatalog.Parameter)
	for name, rawParameter := range raw.Parameters {
		parameters[name] = parameterFromRaw(rawParameter)
	}

	methodParameters, _ := rawMethod["parameters"].(map[string]any)
	for name, rawParameterValue := range methodParameters {
		rawParameter, _ := rawParameterValue.(map[string]any)
		parameters[name] = parameterFromRaw(rawParameter)
	}

	request := schemaFromRef(rawMethod["request"])
	response := schemaFromRef(rawMethod["response"])

	var gates []googlecatalog.Gate
	if len(scopes) == 0 {
		gates = append(gates, googlecatalog.Gate{
			Kind:   googlecatalog.GateMissingScopes,
			Reason: "discovery declares no OAuth scopes; authorization cannot be inferred",
		})
	}

	modes, documented := identityModesForMethod(methodID, service)
	if !documented {
		modes = []googlecatalog.IdentityMode{googlecatalog.IdentityUser}
	}

	identity := googlecatalog.IdentitySupport{Modes: modes, AuthModel: identityAuthModel(methodID, service)}
	if len(modes) == 0 {
		gate := googlecatalog.Gate{Kind: googlecatalog.GateUnsupportedIdentity, Reason: "no authentication mode is documented for this method family"}
		identity.Gate = &gate
		gates = append(gates, gate)
	}

	method := googlecatalog.Method{
		ID:          methodID,
		ToolName:    toolName,
		Service:     service,
		Version:     raw.Version,
		Action:      action,
		Description: stringMap(rawMethod)["description"],
		BaseURL:     baseURL,
		Path:        path,
		HTTPMethod:  httpMethod,
		Scopes:      scopes,
		Parameters:  parameters,
		Request:     request,
		Response:    response,
		ReadOnly:    readOnly(methodID, httpMethod),
		Media:       mediaFromRaw(path, rawMethod),
		Identity:    identity,
		Gates:       gates,
	}

	return method
}

func parameterFromRaw(raw map[string]any) googlecatalog.Parameter {
	return googlecatalog.Parameter{
		Schema:   schemaFromRaw(raw),
		Location: stringMap(raw)["location"],
		Required: boolValue(raw["required"]),
		Repeated: boolValue(raw["repeated"]),
	}
}

func schemaFromRef(raw any) *googlecatalog.Schema {
	refMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	schema := schemaFromRaw(refMap)

	return &schema
}

func schemaFromRaw(raw map[string]any) googlecatalog.Schema {
	schema := googlecatalog.Schema{
		Ref:              stringMap(raw)["$ref"],
		Type:             stringMap(raw)["type"],
		Format:           stringMap(raw)["format"],
		Description:      stringMap(raw)["description"],
		Default:          raw["default"],
		EnumDescriptions: stringSlice(raw["enumDescriptions"]),
		Pattern:          stringMap(raw)["pattern"],
		Nullable:         boolValue(raw["nullable"]),
		ReadOnly:         boolValue(raw["readOnly"]),
	}
	if annotations, ok := raw["annotations"].(map[string]any); ok {
		schema.RequiredFor = stringSlice(annotations["required"])
	}

	if raw["minimum"] != nil {
		minimum := fmt.Sprint(raw["minimum"])
		schema.Minimum = &minimum
	}

	if raw["maximum"] != nil {
		maximum := fmt.Sprint(raw["maximum"])
		schema.Maximum = &maximum
	}

	if values, ok := raw["enum"].([]any); ok {
		schema.Enum = values
	}

	if values, ok := raw["required"].([]any); ok {
		schema.Required = make([]string, 0, len(values))
		for _, value := range values {
			schema.Required = append(schema.Required, fmt.Sprint(value))
		}
	}

	if properties, ok := raw["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*googlecatalog.Schema, len(properties))
		for name, value := range properties {
			property, _ := value.(map[string]any)
			converted := schemaFromRaw(property)
			schema.Properties[name] = &converted
		}
	}

	if items, ok := raw["items"].(map[string]any); ok {
		converted := schemaFromRaw(items)
		schema.Items = &converted
	}

	switch additional := raw["additionalProperties"].(type) {
	case bool:
		if !additional {
			schema.AdditionalPropertiesForbidden = true
		}
	case map[string]any:
		converted := schemaFromRaw(additional)
		schema.AdditionalProperties = &converted
	}

	for _, fieldName := range []string{"anyOf", "oneOf", "allOf"} {
		values, ok := raw[fieldName].([]any)
		if !ok {
			continue
		}

		candidates := make([]*googlecatalog.Schema, 0, len(values))
		for _, value := range values {
			candidateMap, _ := value.(map[string]any)
			converted := schemaFromRaw(candidateMap)
			candidates = append(candidates, &converted)
		}

		switch fieldName {
		case "anyOf":
			schema.AnyOf = candidates
		case "oneOf":
			schema.OneOf = candidates
		case "allOf":
			schema.AllOf = candidates
		}
	}

	return schema
}

func mediaFromRaw(path string, raw map[string]any) *googlecatalog.Media {
	media := googlecatalog.Media{}
	if boolValue(raw["supportsMediaDownload"]) {
		media.Download = &googlecatalog.MediaDownload{
			Path:               path,
			UseDownloadService: boolValue(raw["useMediaDownloadService"]),
		}
	}

	uploadMap, ok := raw["mediaUpload"].(map[string]any)
	if ok {
		protocols, _ := uploadMap["protocols"].(map[string]any)
		simpleMap, _ := protocols["simple"].(map[string]any)
		resumableMap, _ := protocols["resumable"].(map[string]any)
		simplePath := stringMap(simpleMap)["path"]
		resumablePath := stringMap(resumableMap)["path"]

		multipartPath := ""
		if boolValue(simpleMap["multipart"]) {
			multipartPath = simplePath
		}

		upload := googlecatalog.MediaUpload{
			Accept:        stringSlice(uploadMap["accept"]),
			MaxSize:       int64Value(uploadMap["maxSize"]),
			SimplePath:    simplePath,
			MultipartPath: multipartPath,
			ResumablePath: resumablePath,
		}
		media.Upload = &upload
	}

	if media.Upload == nil && media.Download == nil {
		return nil
	}

	return &media
}

func readOnly(methodID string, httpMethod string) bool {
	switch httpMethod {
	case "GET", "HEAD":
		return true
	case "POST":
		return safeReadPOST[methodID]
	default:
		return false
	}
}

func identityModesForMethod(methodID string, service string) ([]googlecatalog.IdentityMode, bool) {
	if modes, ok := methodSpecificIdentityModes(methodID); ok {
		return modes, true
	}

	modes, documented := identityModes[service]

	return modes, documented
}

func methodSpecificIdentityModes(methodID string) ([]googlecatalog.IdentityMode, bool) {
	for prefix, modes := range identityModesByMethodPrefix {
		if strings.HasPrefix(methodID, prefix) {
			return modes, true
		}
	}

	return nil, false
}

func identityAuthModel(methodID string, service string) string {
	if _, verified := methodSpecificIdentityModes(methodID); verified {
		return "OAuth user or service account"
	}

	switch service {
	case "keep", "cloud_identity":
		return "service account"
	case "admin_directory", "admin_reports":
		return "OAuth user or service account"
	default:
		return "OAuth user"
	}
}

//nolint:err113 // validation diagnostics intentionally include discovery values.
func validateManifest(manifest googlecatalog.Manifest) error {
	seenMethods := make(map[string]string)

	for _, method := range manifest.Methods {
		if !toolNamePattern.MatchString(method.ToolName) {
			return fmt.Errorf("invalid tool name %q", method.ToolName)
		}

		methodKey := method.Service + "\x00" + method.Version + "\x00" + method.ID
		if previous, exists := seenMethods[methodKey]; exists {
			return fmt.Errorf("duplicate discovery method id %q in %s and %s", method.ID, previous, method.Service+":"+method.Version)
		}

		seenMethods[methodKey] = method.Service + ":" + method.Version
		if !strings.HasPrefix(method.BaseURL, "https://") {
			return fmt.Errorf("%s has non-HTTPS base URL %q", method.ID, method.BaseURL)
		}

		if method.HTTPMethod == "" {
			return fmt.Errorf("%s has no HTTP method", method.ID)
		}

		for _, scope := range method.Scopes {
			if _, ok := manifest.Scopes[scope]; !ok {
				return fmt.Errorf("%s uses undeclared scope %q", method.ID, scope)
			}
		}

		for _, parameterName := range pathParameters(method.Path) {
			parameter, ok := method.Parameters[parameterName]
			if !ok {
				return fmt.Errorf("%s path parameter %q is not declared", method.ID, parameterName)
			}

			if parameter.Location != "path" {
				return fmt.Errorf("%s path parameter %q has location %q", method.ID, parameterName, parameter.Location)
			}
		}
	}

	return nil
}

func pathParameters(path string) []string {
	out := make([]string, 0, 2)
	for _, match := range pathParameterPattern.FindAllString(path, -1) {
		out = append(out, strings.Trim(match, "{}+"))
	}

	return out
}

func normalizeToken(input string) string {
	var b strings.Builder
	lastUnderscore := true

	for _, char := range input {
		switch {
		case char >= 'a' && char <= 'z' || char >= '0' && char <= '9':
			b.WriteRune(char)
			lastUnderscore = false
		case char >= 'A' && char <= 'Z':
			b.WriteRune(char + 32)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}

	return strings.Trim(b.String(), "_")
}

func sortedRawKeys(input map[string]map[string]any) []string {
	out := make([]string, 0, len(input))
	for key := range input {
		out = append(out, key)
	}

	sort.Strings(out)

	return out
}

func stringMap(input map[string]any) map[string]string {
	out := make(map[string]string)

	for key, value := range input {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}

	return out
}

func stringSlice(input any) []string {
	values, ok := input.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, fmt.Sprint(value))
	}

	return out
}

func boolValue(input any) bool {
	value, _ := input.(bool)
	return value
}

func int64Value(input any) int64 {
	switch value := input.(type) {
	case float64:
		return int64(value)
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	default:
		return 0
	}
}
