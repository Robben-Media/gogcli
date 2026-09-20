package mcpserver

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	maxHTTPCallers   = 32
	sha256HexLength  = 64
	sha256ByteLength = 32
)

// ErrHTTPConfigInvalid is the root of HTTP config parse and validation failures.
var ErrHTTPConfigInvalid = errors.New("mcpserver: http config")

var (
	errHTTPHostRequired        = fmt.Errorf("%w: host is required", ErrHTTPConfigInvalid)
	errHTTPHostInvalid         = fmt.Errorf("%w: host must be an exact Host header value", ErrHTTPConfigInvalid)
	errHTTPCallersRequired     = fmt.Errorf("%w: at least one caller is required", ErrHTTPConfigInvalid)
	errHTTPTooManyCallers      = fmt.Errorf("%w: at most 32 callers are allowed", ErrHTTPConfigInvalid)
	errHTTPCallerIDRequired    = fmt.Errorf("%w: caller id is required", ErrHTTPConfigInvalid)
	errHTTPPrincipalRequired   = fmt.Errorf("%w: caller principal_id is required", ErrHTTPConfigInvalid)
	errHTTPTokenDigestRequired = fmt.Errorf("%w: caller token_sha256 must be a 64-character hex SHA-256 digest", ErrHTTPConfigInvalid)
	errHTTPGrantPrincipal      = fmt.Errorf("%w: grant principal_id must match caller principal_id", ErrHTTPConfigInvalid)
	errHTTPConfigTrailing      = fmt.Errorf("%w: trailing data", ErrHTTPConfigInvalid)
	errHTTPOriginInvalid       = fmt.Errorf("%w: allowed_origins entries must be exact http(s) origins", ErrHTTPConfigInvalid)
)

// HTTPConfig is the trusted Streamable HTTP caller file. Token SHA-256 digests
// identify harness credentials; principal_id is the account owner.
type HTTPConfig struct {
	Host           string
	AllowedOrigins []string
	Callers        []HTTPCaller
}

// HTTPCaller is one authenticated harness. Multiple callers may share a
// principal_id with narrower grants.
type HTTPCaller struct {
	ID              string
	TokenSHA256     [sha256ByteLength]byte
	PrincipalID     string
	AllowOperations []string
	Grants          []mcpcontract.Grant
}

type httpConfigJSON struct {
	Host           string           `json:"host"`
	AllowedOrigins []string         `json:"allowed_origins"`
	Callers        []httpCallerJSON `json:"callers"`
}

type httpCallerJSON struct {
	ID              string              `json:"id"`
	TokenSHA256     string              `json:"token_sha256"`
	PrincipalID     string              `json:"principal_id"`
	AllowOperations []string            `json:"allow_operations"`
	Grants          []mcpcontract.Grant `json:"grants"`
}

// LoadHTTPConfig reads and validates the HTTP caller file.
func LoadHTTPConfig(path string) (HTTPConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied --http-config path
	if err != nil {
		return HTTPConfig{}, fmt.Errorf("%w: read: %w", ErrHTTPConfigInvalid, err)
	}

	return ParseHTTPConfig(data)
}

// ParseHTTPConfig validates a complete HTTP caller document. Unknown fields,
// duplicate credential ids or token digests, and principal mismatches fail.
func ParseHTTPConfig(data []byte) (HTTPConfig, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var raw httpConfigJSON
	if err := dec.Decode(&raw); err != nil {
		return HTTPConfig{}, fmt.Errorf("%w: %w", ErrHTTPConfigInvalid, err)
	}

	if dec.More() {
		return HTTPConfig{}, errHTTPConfigTrailing
	}

	if err := dec.Decode(&struct{}{}); err != nil && !errors.Is(err, io.EOF) {
		return HTTPConfig{}, errHTTPConfigTrailing
	}

	return compileHTTPConfig(raw)
}

// UniquePrincipalIDs returns owner principals in first-seen order.
func (c HTTPConfig) UniquePrincipalIDs() []string {
	seen := make(map[string]struct{}, len(c.Callers))
	out := make([]string, 0, len(c.Callers))

	for _, caller := range c.Callers {
		if _, ok := seen[caller.PrincipalID]; ok {
			continue
		}

		seen[caller.PrincipalID] = struct{}{}
		out = append(out, caller.PrincipalID)
	}

	return out
}

func compileHTTPConfig(raw httpConfigJSON) (HTTPConfig, error) {
	host := strings.TrimSpace(raw.Host)
	if host == "" {
		return HTTPConfig{}, errHTTPHostRequired
	}

	if strings.Contains(host, "://") || strings.ContainsAny(host, "/ \t\r\n") {
		return HTTPConfig{}, errHTTPHostInvalid
	}

	origins, err := compileAllowedOrigins(raw.AllowedOrigins)
	if err != nil {
		return HTTPConfig{}, err
	}

	if len(raw.Callers) == 0 {
		return HTTPConfig{}, errHTTPCallersRequired
	}

	if len(raw.Callers) > maxHTTPCallers {
		return HTTPConfig{}, errHTTPTooManyCallers
	}

	callers := make([]HTTPCaller, 0, len(raw.Callers))
	ids := make(map[string]struct{}, len(raw.Callers))
	digests := make(map[[sha256ByteLength]byte]struct{}, len(raw.Callers))

	for _, rawCaller := range raw.Callers {
		caller, compileErr := compileHTTPCaller(rawCaller)
		if compileErr != nil {
			return HTTPConfig{}, compileErr
		}

		if _, ok := ids[caller.ID]; ok {
			return HTTPConfig{}, fmt.Errorf("%w: duplicate caller id %q", ErrHTTPConfigInvalid, caller.ID)
		}

		if _, ok := digests[caller.TokenSHA256]; ok {
			return HTTPConfig{}, fmt.Errorf("%w: duplicate token_sha256", ErrHTTPConfigInvalid)
		}

		ids[caller.ID] = struct{}{}
		digests[caller.TokenSHA256] = struct{}{}
		callers = append(callers, caller)
	}

	return HTTPConfig{
		Host:           host,
		AllowedOrigins: origins,
		Callers:        callers,
	}, nil
}

func compileAllowedOrigins(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, raw := range values {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			return nil, errHTTPOriginInvalid
		}

		if err := validateHTTPOrigin(origin); err != nil {
			return nil, err
		}

		if _, ok := seen[origin]; ok {
			return nil, fmt.Errorf("%w: duplicate origin %q", ErrHTTPConfigInvalid, origin)
		}

		seen[origin] = struct{}{}
		out = append(out, origin)
	}

	return out, nil
}

func validateHTTPOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("%w: %w", errHTTPOriginInvalid, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errHTTPOriginInvalid
	}

	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errHTTPOriginInvalid
	}

	if parsed.Path != "" && parsed.Path != "/" {
		return errHTTPOriginInvalid
	}

	if parsed.Path == "/" {
		return errHTTPOriginInvalid
	}

	if parsed.String() != origin {
		return errHTTPOriginInvalid
	}

	return nil
}

func compileHTTPCaller(raw httpCallerJSON) (HTTPCaller, error) {
	id := strings.TrimSpace(raw.ID)
	if id == "" {
		return HTTPCaller{}, errHTTPCallerIDRequired
	}

	principalID := strings.TrimSpace(raw.PrincipalID)
	if principalID == "" {
		return HTTPCaller{}, errHTTPPrincipalRequired
	}

	digest, err := parseTokenSHA256(raw.TokenSHA256)
	if err != nil {
		return HTTPCaller{}, err
	}

	grants := make([]mcpcontract.Grant, 0, len(raw.Grants))
	for _, grant := range raw.Grants {
		grant.PrincipalID = strings.TrimSpace(grant.PrincipalID)
		if grant.PrincipalID == "" {
			grant.PrincipalID = principalID
		}

		if grant.PrincipalID != principalID {
			return HTTPCaller{}, errHTTPGrantPrincipal
		}

		grants = append(grants, grant)
	}

	allow := make([]string, 0, len(raw.AllowOperations))
	for _, name := range raw.AllowOperations {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		allow = append(allow, name)
	}

	if err := (access.Snapshot{Grants: grants, AllowOperations: allow}).Validate(); err != nil {
		return HTTPCaller{}, fmt.Errorf("%w: caller %s: %w", ErrHTTPConfigInvalid, id, err)
	}

	return HTTPCaller{
		ID:              id,
		TokenSHA256:     digest,
		PrincipalID:     principalID,
		AllowOperations: allow,
		Grants:          grants,
	}, nil
}

func parseTokenSHA256(raw string) ([sha256ByteLength]byte, error) {
	var digest [sha256ByteLength]byte

	value := strings.TrimSpace(raw)
	if len(value) != sha256HexLength {
		return digest, errHTTPTokenDigestRequired
	}

	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256ByteLength {
		return digest, errHTTPTokenDigestRequired
	}

	copy(digest[:], decoded)

	return digest, nil
}
