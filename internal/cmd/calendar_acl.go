package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ACL scope type constants
const (
	aclScopeTypeDefault = "default"
	aclScopeTypeUser    = "user"
	aclScopeTypeGroup   = "group"
	aclScopeTypeDomain  = "domain"
)

// Calendar ACL commands - manage calendar sharing permissions.
// ACL rules control who can access a calendar and what level of access they have.

// CalendarAclCmd is the parent command for ACL operations.
type CalendarAclCmd struct {
	List   CalendarAclListCmd   `cmd:"" name:"list" help:"List ACL rules for a calendar"`
	Get    CalendarAclGetCmd    `cmd:"" name:"get" help:"Get an ACL rule by ID"`
	Insert CalendarAclInsertCmd `cmd:"" name:"insert" help:"Insert an ACL rule (share calendar)"`
	Delete CalendarAclDeleteCmd `cmd:"" name:"delete" help:"Delete an ACL rule (remove sharing)"`
	Patch  CalendarAclPatchCmd  `cmd:"" name:"patch" help:"Patch an ACL rule (change role)"`
	Watch  CalendarAclWatchCmd  `cmd:"" name:"watch" help:"Watch for ACL changes"`
}

// CalendarAclListCmd lists all ACL rules for a calendar.
type CalendarAclListCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page       string `name:"page" help:"Page token"`
}

func (c *CalendarAclListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Acl.List(calendarID).MaxResults(c.Max).PageToken(c.Page).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"rules":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No ACL rules")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SCOPE_TYPE\tSCOPE_VALUE\tROLE\tID")
	for _, rule := range resp.Items {
		scopeType := ""
		scopeValue := ""
		if rule.Scope != nil {
			scopeType = rule.Scope.Type
			scopeValue = rule.Scope.Value
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", scopeType, scopeValue, rule.Role, rule.Id)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// CalendarAclGetCmd retrieves a single ACL rule.
type CalendarAclGetCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	RuleID     string `arg:"" name:"ruleId" help:"ACL rule ID"`
}

func (c *CalendarAclGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}
	ruleID := strings.TrimSpace(c.RuleID)
	if ruleID == "" {
		return usage("ruleId required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	rule, err := svc.Acl.Get(calendarID, ruleID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"rule": rule})
	}

	u.Out().Printf("id\t%s", rule.Id)
	u.Out().Printf("role\t%s", rule.Role)
	if rule.Scope != nil {
		u.Out().Printf("scope_type\t%s", rule.Scope.Type)
		if rule.Scope.Value != "" {
			u.Out().Printf("scope_value\t%s", rule.Scope.Value)
		}
	}
	return nil
}

// CalendarAclInsertCmd creates a new ACL rule to share a calendar.
type CalendarAclInsertCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	ScopeType  string `name:"scope-type" required:"" help:"Scope type (user, group, domain, default)"`
	ScopeValue string `name:"scope-value" help:"Email address or domain (required for user, group, domain)"`
	Role       string `name:"role" required:"" help:"Access role (none, freeBusyReader, reader, writer, owner)"`
	SendEmail  bool   `name:"send-email" help:"Send notification email to grantee"`
}

func (c *CalendarAclInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	// Validate scope type
	validScopeTypes := map[string]bool{aclScopeTypeUser: true, aclScopeTypeGroup: true, aclScopeTypeDomain: true, aclScopeTypeDefault: true}
	if !validScopeTypes[c.ScopeType] {
		return usage("invalid scope-type: must be user, group, domain, or default")
	}

	// Scope value is required for user, group, domain
	if c.ScopeType != aclScopeTypeDefault && strings.TrimSpace(c.ScopeValue) == "" {
		return usage("--scope-value is required for scope-type user, group, or domain")
	}

	// Validate role
	validRoles := map[string]bool{"none": true, "freeBusyReader": true, "reader": true, "writer": true, "owner": true}
	if !validRoles[c.Role] {
		return usage("invalid role: must be none, freeBusyReader, reader, writer, or owner")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	rule := &calendar.AclRule{
		Role: c.Role,
		Scope: &calendar.AclRuleScope{
			Type:  c.ScopeType,
			Value: c.ScopeValue,
		},
	}

	call := svc.Acl.Insert(calendarID, rule)
	if c.SendEmail {
		call.SendNotifications(true)
	}

	created, err := call.Do()
	if err != nil {
		return fmt.Errorf("insert ACL rule: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"rule": created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("role\t%s", created.Role)
	if created.Scope != nil {
		u.Out().Printf("scope_type\t%s", created.Scope.Type)
		if created.Scope.Value != "" {
			u.Out().Printf("scope_value\t%s", created.Scope.Value)
		}
	}
	return nil
}

// CalendarAclDeleteCmd deletes an ACL rule to remove calendar sharing.
type CalendarAclDeleteCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	RuleID     string `arg:"" name:"ruleId" help:"ACL rule ID to delete"`
}

func (c *CalendarAclDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}
	ruleID := strings.TrimSpace(c.RuleID)
	if ruleID == "" {
		return usage("ruleId required")
	}

	// Confirm destructive action
	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete ACL rule %s from calendar %s", ruleID, calendarID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Acl.Delete(calendarID, ruleID).Do(); err != nil {
		return fmt.Errorf("delete ACL rule: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":      ruleID,
			"deleted": true,
		})
	}

	u.Err().Printf("ACL rule %q deleted from calendar %q", ruleID, calendarID)
	return nil
}

// CalendarAclPatchCmd updates an ACL rule's role (partial update).
type CalendarAclPatchCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	RuleID     string `arg:"" name:"ruleId" help:"ACL rule ID to patch"`
	Role       string `name:"role" help:"New access role (none, freeBusyReader, reader, writer, owner)"`
}

func (c *CalendarAclPatchCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}
	ruleID := strings.TrimSpace(c.RuleID)
	if ruleID == "" {
		return usage("ruleId required")
	}

	// Check if any updates provided
	if !flagProvided(kctx, "role") {
		return usage("no updates provided; use --role")
	}

	// Validate role
	validRoles := map[string]bool{"none": true, "freeBusyReader": true, "reader": true, "writer": true, "owner": true}
	if !validRoles[c.Role] {
		return usage("invalid role: must be none, freeBusyReader, reader, writer, or owner")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	rule := &calendar.AclRule{
		Role: c.Role,
	}

	updated, err := svc.Acl.Patch(calendarID, ruleID, rule).Do()
	if err != nil {
		return fmt.Errorf("patch ACL rule: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"rule": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("role\t%s", updated.Role)
	if updated.Scope != nil {
		u.Out().Printf("scope_type\t%s", updated.Scope.Type)
		if updated.Scope.Value != "" {
			u.Out().Printf("scope_value\t%s", updated.Scope.Value)
		}
	}
	return nil
}

// CalendarAclWatchCmd sets up a webhook to watch for ACL changes.
type CalendarAclWatchCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	WebhookURL string `name:"webhook-url" required:"" help:"Webhook URL to receive notifications"`
	ChannelID  string `name:"channel-id" help:"Unique channel ID (auto-generated if not provided)"`
	AuthToken  string `name:"auth-token" help:"Token sent with each notification"`
	TTL        string `name:"ttl" help:"Watch expiration (Go duration, e.g. 24h)"`
}

func (c *CalendarAclWatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}
	if strings.TrimSpace(c.WebhookURL) == "" {
		return usage("--webhook-url is required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	// Generate channel ID if not provided
	channelID := c.ChannelID
	if channelID == "" {
		channelID = uuid.New().String()
	}

	channel := &calendar.Channel{
		Id:      channelID,
		Type:    "web_hook",
		Address: c.WebhookURL,
	}

	if c.AuthToken != "" {
		channel.Token = c.AuthToken
	}

	call := svc.Acl.Watch(calendarID, channel)

	resp, err := call.Do()
	if err != nil {
		return fmt.Errorf("watch ACL: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"channel": resp})
	}

	u.Out().Printf("channel_id\t%s", resp.Id)
	u.Out().Printf("resource_id\t%s", resp.ResourceId)
	u.Out().Printf("resource_uri\t%s", resp.ResourceUri)
	if resp.Expiration > 0 {
		u.Out().Printf("expiration\t%s", formatUnixMillis(resp.Expiration))
	}
	return nil
}
