package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/tagmanager/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type TagManagerTriggersCreateCmd struct {
	tagManagerWorkspaceFlags
	Name              string   `name:"name" required:"" help:"Trigger name"`
	Type              string   `name:"type" required:"" help:"GTM trigger type"`
	Filter            []string `name:"filter" sep:"none" help:"GTM Condition JSON object (repeatable)"`
	AutoEventFilter   []string `name:"auto-event-filter" sep:"none" help:"GTM auto-event Condition JSON object (repeatable)"`
	CustomEventFilter []string `name:"custom-event-filter" sep:"none" help:"GTM custom-event Condition JSON object (repeatable)"`
	EventName         string   `name:"event-name" help:"GTM timer event-name Parameter JSON object"`
	Interval          string   `name:"interval" help:"GTM timer interval Parameter JSON object"`
	Limit             string   `name:"limit" help:"GTM timer limit Parameter JSON object"`
}

func (c *TagManagerTriggersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	parent, err := c.parent()
	if err != nil {
		return err
	}
	trigger, err := c.input()
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	created, err := svc.Accounts.Containers.Workspaces.Triggers.Create(parent, trigger).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerTrigger(ctx, "Created", created)
}

func (c *TagManagerTriggersCreateCmd) input() (*tagmanager.Trigger, error) {
	name, triggerType := strings.TrimSpace(c.Name), strings.TrimSpace(c.Type)
	if name == "" || triggerType == "" {
		return nil, usage("--name and --type must not be empty")
	}
	filters, err := parseTagManagerJSONObjects[tagmanager.Condition]("filter", c.Filter)
	if err != nil {
		return nil, err
	}
	autoEventFilters, err := parseTagManagerJSONObjects[tagmanager.Condition]("auto-event-filter", c.AutoEventFilter)
	if err != nil {
		return nil, err
	}
	customEventFilters, err := parseTagManagerJSONObjects[tagmanager.Condition]("custom-event-filter", c.CustomEventFilter)
	if err != nil {
		return nil, err
	}
	var eventName, interval, limit *tagmanager.Parameter
	if strings.TrimSpace(c.EventName) != "" {
		eventName, err = parseTagManagerJSONObject[tagmanager.Parameter]("event-name", c.EventName)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(c.Interval) != "" {
		interval, err = parseTagManagerJSONObject[tagmanager.Parameter]("interval", c.Interval)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(c.Limit) != "" {
		limit, err = parseTagManagerJSONObject[tagmanager.Parameter]("limit", c.Limit)
		if err != nil {
			return nil, err
		}
	}
	return &tagmanager.Trigger{
		Name: name, Type: triggerType, Filter: filters, AutoEventFilter: autoEventFilters,
		CustomEventFilter: customEventFilters, EventName: eventName, Interval: interval, Limit: limit,
	}, nil
}

type TagManagerTriggersGetCmd struct {
	Path string `arg:"" name:"path" help:"Full GTM trigger path"`
}

func (c *TagManagerTriggersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "triggers", "TRIGGER_ID")
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	trigger, err := svc.Accounts.Containers.Workspaces.Triggers.Get(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerTrigger(ctx, "Trigger", trigger)
}

type TagManagerTriggersDeleteCmd struct {
	Path string `arg:"" name:"path" help:"Full GTM trigger path"`
}

func (c *TagManagerTriggersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "triggers", "TRIGGER_ID")
	if err != nil {
		return err
	}
	if err = confirmDestructive(ctx, flags, "delete GTM trigger "+path); err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	if err = svc.Accounts.Containers.Workspaces.Triggers.Delete(path).Context(ctx).Do(); err != nil {
		return err
	}
	return writeTagManagerDelete(ctx, "trigger", path)
}

type TagManagerTriggersRevertCmd struct {
	Path string `arg:"" name:"path" help:"Full GTM trigger path"`
}

func (c *TagManagerTriggersRevertCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "triggers", "TRIGGER_ID")
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Accounts.Containers.Workspaces.Triggers.Revert(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerTrigger(ctx, "Reverted", resp.Trigger)
}

type TagManagerTriggersUpdateCmd struct {
	Path              string   `arg:"" name:"path" help:"Full GTM trigger path"`
	Name              string   `name:"name" help:"Trigger name"`
	Type              string   `name:"type" help:"GTM trigger type"`
	Filter            []string `name:"filter" sep:"none" help:"GTM Condition JSON object (repeatable)"`
	AutoEventFilter   []string `name:"auto-event-filter" sep:"none" help:"GTM auto-event Condition JSON object (repeatable)"`
	CustomEventFilter []string `name:"custom-event-filter" sep:"none" help:"GTM custom-event Condition JSON object (repeatable)"`
	EventName         string   `name:"event-name" help:"GTM timer event-name Parameter JSON object"`
	Interval          string   `name:"interval" help:"GTM timer interval Parameter JSON object"`
	Limit             string   `name:"limit" help:"GTM timer limit Parameter JSON object"`
}

func (c *TagManagerTriggersUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	account, path, err := tagManagerResourceRequest(flags, c.Path, "triggers", "TRIGGER_ID")
	if err != nil {
		return err
	}
	if !flagProvidedAny(kctx, "name", "type", "filter", "auto-event-filter", "custom-event-filter", "event-name", "interval", "limit") {
		return usage("at least one trigger field flag is required")
	}
	apply, err := c.buildPatch(kctx)
	if err != nil {
		return err
	}
	svc, err := newTagManagerService(ctx, account)
	if err != nil {
		return err
	}
	trigger, err := svc.Accounts.Containers.Workspaces.Triggers.Get(path).Context(ctx).Do()
	if err != nil {
		return err
	}
	apply(trigger)
	updated, err := svc.Accounts.Containers.Workspaces.Triggers.Update(path, trigger).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeTagManagerTrigger(ctx, "Updated", updated)
}

func (c *TagManagerTriggersUpdateCmd) buildPatch(kctx *kong.Context) (func(*tagmanager.Trigger), error) {
	if flagProvided(kctx, "name") && strings.TrimSpace(c.Name) == "" {
		return nil, usage("--name must not be blank")
	}
	if flagProvided(kctx, "type") && strings.TrimSpace(c.Type) == "" {
		return nil, usage("--type must not be blank")
	}
	conditions := make(map[string][]*tagmanager.Condition)
	for name, values := range map[string][]string{
		"filter": c.Filter, "auto-event-filter": c.AutoEventFilter, "custom-event-filter": c.CustomEventFilter,
	} {
		if flagProvided(kctx, name) {
			parsed, err := parseTagManagerJSONObjects[tagmanager.Condition](name, values)
			if err != nil {
				return nil, err
			}
			conditions[name] = parsed
		}
	}
	parameters := make(map[string]*tagmanager.Parameter)
	for name, value := range map[string]string{"event-name": c.EventName, "interval": c.Interval, "limit": c.Limit} {
		if flagProvided(kctx, name) {
			parsed, err := parseTagManagerJSONObject[tagmanager.Parameter](name, value)
			if err != nil {
				return nil, err
			}
			parameters[name] = parsed
		}
	}
	return func(trigger *tagmanager.Trigger) {
		if flagProvided(kctx, "name") {
			trigger.Name = strings.TrimSpace(c.Name)
		}
		if flagProvided(kctx, "type") {
			trigger.Type = strings.TrimSpace(c.Type)
		}
		if value, ok := conditions["filter"]; ok {
			trigger.Filter = value
		}
		if value, ok := conditions["auto-event-filter"]; ok {
			trigger.AutoEventFilter = value
		}
		if value, ok := conditions["custom-event-filter"]; ok {
			trigger.CustomEventFilter = value
		}
		if value, ok := parameters["event-name"]; ok {
			trigger.EventName = value
		}
		if value, ok := parameters["interval"]; ok {
			trigger.Interval = value
		}
		if value, ok := parameters["limit"]; ok {
			trigger.Limit = value
		}
	}, nil
}

func writeTagManagerTrigger(ctx context.Context, action string, trigger *tagmanager.Trigger) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"trigger": trigger})
	}
	if outfmt.IsPlain(ctx) {
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "TRIGGER_ID\tNAME\tTYPE")
		if trigger != nil {
			fmt.Fprintf(w, "%s\t%s\t%s\n", sanitizeTab(trigger.TriggerId), sanitizeTab(trigger.Name), sanitizeTab(trigger.Type))
		}
		return nil
	}
	if trigger == nil {
		ui.FromContext(ctx).Out().Printf("%s trigger: no published version", action)
		return nil
	}
	ui.FromContext(ctx).Out().Printf("%s trigger: %s", action, trigger.Path)
	return nil
}
