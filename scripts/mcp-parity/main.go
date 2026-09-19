// Command mcp-parity inventories implemented CLI leaves without executing them.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/cmd"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type entry struct {
	Command string `json:"command"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Tool    string `json:"tool,omitempty"`
	Reason  string `json:"reason"`
}

func main() {
	parser, err := kong.New(&cmd.CLI{}, kong.Vars{"auth_services": "", "color": "auto", "calendar_weekday": "false", "client": "", "enabled_commands": "", "json": "false", "plain": "false", "version": "inventory"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	byAction := map[string]string{}

	for _, d := range mcpcontract.Catalog() {
		for _, a := range d.Actions {
			byAction[a] = d.Name
		}
	}
	var rows []entry
	var walk func(*kong.Node, []string)
	walk = func(n *kong.Node, path []string) {
		if n.Type == kong.CommandNode {
			path = append(append([]string(nil), path...), n.Name)
		}

		if len(n.Children) > 0 {
			for _, child := range n.Children {
				walk(child, path)
			}

			return
		}

		if len(path) == 0 {
			return
		}
		service := strings.ReplaceAll(path[0], "-", "")

		rest := strings.Join(path[1:], ".")
		if service == "gmail" {
			rest = strings.TrimPrefix(rest, "settings.")
		}
		action := service + ":" + rest
		row := entry{Command: strings.Join(path, " "), Action: action, Status: "deferred", Reason: "Retained CLI operation; migrate after measured pilot and named consumer need."}

		switch service {
		case "auth", "config", "policy", "update", "skills", "version", "completion", "__complete", "time":
			row.Status = "administrative_cli_only"
			row.Reason = "Existing administrative/local CLI remains supported."
		}

		if tool, ok := byAction[action]; ok {
			row.Status = "native_pilot"
			row.Tool = tool
			row.Reason = "Native read primitive covers this action; CLI flags and output remain separately supported."
		}

		rows = append(rows, row)
	}
	walk(parser.Model.Node, nil)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if err := enc.Encode(rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
