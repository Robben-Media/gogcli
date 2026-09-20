package mcpserver

import (
	"log/slog"
	"time"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mediaartifact"
)

// DiscoveryMode selects how Google operations are advertised over MCP.
type DiscoveryMode string

const (
	// DiscoveryExpanded advertises accounts_list plus each eligible native tool.
	DiscoveryExpanded DiscoveryMode = "expanded"
	// DiscoveryCompact advertises accounts_list plus search/describe/execute gateways.
	DiscoveryCompact DiscoveryMode = "compact"

	accountsListName             = "accounts_list"
	capabilitiesSearchName       = "capabilities_search"
	capabilitiesDescribeName     = "capabilities_describe"
	capabilitiesExecuteName      = "capabilities_execute"
	defaultName                  = "gog-mcp"
	defaultVersion               = "0.10.0"
	defaultRequestTimeout        = 30 * time.Second
	defaultMaxConcurrency        = 32
	defaultMaxBodyBytes          = 8 << 20
	minMaxBodyBytes              = 512
	defaultMaxUpstreamCalls      = 32
	maxMaxUpstreamCalls          = 256
	defaultCapabilitySearchLimit = 8
	maxCapabilitySearchLimit     = 20
	maxCapabilityQueryRunes      = 200
	maxSearchDescriptionRunes    = 320
	maxSearchReplyBytes          = 8 << 10
	maxDescribeDescriptionRunes  = 1200

	schemaTypeObject  = "object"
	schemaTypeString  = "string"
	schemaTypeArray   = "array"
	schemaTypeBoolean = "boolean"
)

// Config is trusted process startup configuration. Principal is never taken
// from MCP client names, request metadata, or tool arguments.
type Config struct {
	MediaArtifacts   *mediaartifact.Store
	Name             string
	Version          string
	Principal        mcpcontract.Principal
	Grants           []mcpcontract.Grant
	Policies         []config.Policy
	AllowOperations  []string
	Operations       []mcpcontract.Operation
	Accounts         access.AccountSource
	Logger           *slog.Logger
	RequestTimeout   time.Duration
	MaxConcurrency   int
	MaxBodyBytes     int64
	DiscoveryMode    DiscoveryMode
	EnableWrites     bool
	MaxUpstreamCalls int64
}

func (c Config) snapshot() access.Snapshot {
	return access.Snapshot{
		Grants:          c.Grants,
		Policies:        c.Policies,
		AllowOperations: c.AllowOperations,
	}
}
