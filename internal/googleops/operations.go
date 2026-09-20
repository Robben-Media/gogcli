// Package googleops assembles the native read-only Google operation catalog.
package googleops

import (
	"github.com/steipete/gogcli/internal/googleops/analytics"
	"github.com/steipete/gogcli/internal/googleops/calendar"
	"github.com/steipete/gogcli/internal/googleops/docs"
	"github.com/steipete/gogcli/internal/googleops/drive"
	"github.com/steipete/gogcli/internal/googleops/gmail"
	"github.com/steipete/gogcli/internal/googleops/searchconsole"
	"github.com/steipete/gogcli/internal/googleops/sheets"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

// Operations excludes caller-local accounts_list, which the runtime supplies.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	operations := make([]mcpcontract.Operation, 0, 16)
	for _, group := range [][]mcpcontract.Operation{gmail.Operations(provider), drive.Operations(provider), docs.Operations(provider), calendar.Operations(provider), analytics.Operations(provider), searchconsole.Operations(provider), sheets.Operations(provider)} {
		operations = append(operations, group...)
	}

	return operations
}
