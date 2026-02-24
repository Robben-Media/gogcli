package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ChatEventsCmd contains subcommands for space events.
type ChatEventsCmd struct {
	Get  ChatEventsGetCmd  `cmd:"" name:"get" help:"Get a space event"`
	List ChatEventsListCmd `cmd:"" name:"list" help:"List space events"`
}

// ChatEventsGetCmd gets a specific space event.
type ChatEventsGetCmd struct {
	Name string `arg:"" name:"name" help:"Space event resource name (spaces/.../spaceEvents/...)"`
}

func (c *ChatEventsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.SpaceEvents.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.EventType != "" {
		u.Out().Printf("eventType\t%s", resp.EventType)
	}
	if resp.EventTime != "" {
		u.Out().Printf("eventTime\t%s", resp.EventTime)
	}
	return nil
}

// ChatEventsListCmd lists space events.
type ChatEventsListCmd struct {
	Parent string `arg:"" name:"parent" help:"Space resource name (spaces/...)"`
	Filter string `name:"filter" help:"Event filter (required)" required:""`
	Max    int64  `name:"max" aliases:"limit" help:"Max results (default: 100)" default:"100"`
	Page   string `name:"page" help:"Page token"`
}

func (c *ChatEventsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	parent, err := normalizeSpace(c.Parent)
	if err != nil {
		return usage("required: parent")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Spaces.SpaceEvents.List(parent).
		Filter(c.Filter).
		PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource  string `json:"resource"`
			EventType string `json:"eventType,omitempty"`
			EventTime string `json:"eventTime,omitempty"`
		}
		items := make([]item, 0, len(resp.SpaceEvents))
		for _, evt := range resp.SpaceEvents {
			if evt == nil {
				continue
			}
			items = append(items, item{
				Resource:  evt.Name,
				EventType: evt.EventType,
				EventTime: evt.EventTime,
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spaceEvents":   items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.SpaceEvents) == 0 {
		u.Err().Println("No space events")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tEVENT_TYPE\tTIME")
	for _, evt := range resp.SpaceEvents {
		if evt == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			evt.Name,
			sanitizeTab(evt.EventType),
			sanitizeTab(evt.EventTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}
