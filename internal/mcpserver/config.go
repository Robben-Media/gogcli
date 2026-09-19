package mcpserver

import (
	"log/slog"
	"time"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	accountsListName      = "accounts_list"
	defaultName           = "gog-mcp"
	defaultVersion        = "0.10.0"
	defaultRequestTimeout = 30 * time.Second
	defaultMaxConcurrency = 32
	defaultMaxBodyBytes   = 8 << 20
)

// Config is trusted process startup configuration. Principal is never taken
// from MCP client names, request metadata, or tool arguments.
type Config struct {
	Name            string
	Version         string
	Principal       mcpcontract.Principal
	Grants          []mcpcontract.Grant
	Policies        []config.Policy
	AllowOperations []string
	Operations      []mcpcontract.Operation
	Accounts        access.AccountSource
	Logger          *slog.Logger
	RequestTimeout  time.Duration
	MaxConcurrency  int
	MaxBodyBytes    int64
}

func (c Config) snapshot() access.Snapshot {
	return access.Snapshot{
		Grants:          c.Grants,
		Policies:        c.Policies,
		AllowOperations: c.AllowOperations,
	}
}
