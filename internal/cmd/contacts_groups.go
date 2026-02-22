package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ContactGroupsCmd contains subcommands for contact groups.
type ContactGroupsCmd struct {
	List     ContactGroupsListCmd     `cmd:"" default:"withargs" help:"List contact groups"`
	Get      ContactGroupsGetCmd      `cmd:"" name:"get" help:"Get a contact group"`
	BatchGet ContactGroupsBatchGetCmd `cmd:"" name:"batch-get" help:"Get multiple contact groups"`
	Create   ContactGroupsCreateCmd   `cmd:"" name:"create" help:"Create a contact group" aliases:"add,new"`
	Update   ContactGroupsUpdateCmd   `cmd:"" name:"update" help:"Update a contact group"`
	Delete   ContactGroupsDeleteCmd   `cmd:"" name:"delete" help:"Delete a contact group" aliases:"rm,del"`
}

const groupsReadMask = "metadata,groupType,memberCount,name"

// ContactGroupsListCmd lists all contact groups owned by the authenticated user.
type ContactGroupsListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results (default: 10)" default:"10"`
	Page      string `name:"page" help:"Page token"`
	SyncToken string `name:"sync-token" help:"Sync token for incremental sync"`
}

func (c *ContactGroupsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.ContactGroups.List().PageSize(c.Max).GroupFields(groupsReadMask)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	if c.SyncToken != "" {
		call = call.SyncToken(c.SyncToken)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"contactGroups": resp.ContactGroups,
			"nextPageToken": resp.NextPageToken,
			"nextSyncToken": resp.NextSyncToken,
			"totalItems":    resp.TotalItems,
		})
	}

	if len(resp.ContactGroups) == 0 {
		u.Err().Println("No contact groups")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tMEMBER_COUNT\tTYPE")
	for _, g := range resp.ContactGroups {
		if g == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			g.ResourceName,
			g.Name,
			g.MemberCount,
			g.GroupType,
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// ContactGroupsGetCmd retrieves a specific contact group.
type ContactGroupsGetCmd struct {
	ResourceName string `arg:"" name:"resourceName" help:"Resource name (contactGroups/...)"`
	MaxMembers   int64  `name:"max-members" help:"Max members to return (default: 0)"`
}

func (c *ContactGroupsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceName := strings.TrimSpace(c.ResourceName)
	if resourceName == "" {
		return usage("empty resourceName")
	}

	// Allow short form (just the ID)
	if !strings.HasPrefix(resourceName, "contactGroups/") {
		resourceName = "contactGroups/" + resourceName
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.ContactGroups.Get(resourceName).GroupFields(groupsReadMask + ",memberResourceNames")
	if c.MaxMembers > 0 {
		call = call.MaxMembers(c.MaxMembers)
	}

	g, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"contactGroup": g})
	}

	u.Out().Printf("resource\t%s", g.ResourceName)
	u.Out().Printf("name\t%s", g.Name)
	u.Out().Printf("formattedName\t%s", g.FormattedName)
	u.Out().Printf("type\t%s", g.GroupType)
	u.Out().Printf("memberCount\t%d", g.MemberCount)
	if len(g.MemberResourceNames) > 0 {
		u.Out().Printf("members\t%d (use --max-members to list)", len(g.MemberResourceNames))
	}
	return nil
}

// ContactGroupsBatchGetCmd retrieves multiple contact groups in a single request.
type ContactGroupsBatchGetCmd struct {
	ResourceNames []string `name:"resource-names" help:"Resource names to retrieve (up to 200)" required:""`
	MaxMembers    int64    `name:"max-members" help:"Max members to return per group (default: 0)"`
}

func (c *ContactGroupsBatchGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if len(c.ResourceNames) == 0 {
		return usage("at least one --resource-names is required")
	}

	if len(c.ResourceNames) > 200 {
		return usage("maximum 200 resource names allowed")
	}

	// Normalize resource names
	resourceNames := make([]string, len(c.ResourceNames))
	for i, rn := range c.ResourceNames {
		rn = strings.TrimSpace(rn)
		if !strings.HasPrefix(rn, "contactGroups/") {
			rn = "contactGroups/" + rn
		}
		resourceNames[i] = rn
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.ContactGroups.BatchGet().ResourceNames(resourceNames...).GroupFields(groupsReadMask)
	if c.MaxMembers > 0 {
		call = call.MaxMembers(c.MaxMembers)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"responses": resp.Responses,
		})
	}

	if len(resp.Responses) == 0 {
		u.Err().Println("No contact groups found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tMEMBER_COUNT\tSTATUS")
	for _, r := range resp.Responses {
		if r == nil || r.ContactGroup == nil {
			continue
		}
		g := r.ContactGroup
		status := "OK"
		if r.Status != nil && r.Status.Code != 0 {
			status = r.Status.Message
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			g.ResourceName,
			g.Name,
			g.MemberCount,
			status,
		)
	}
	return nil
}

// ContactGroupsCreateCmd creates a new contact group.
type ContactGroupsCreateCmd struct {
	Name string `arg:"" name:"name" help:"Contact group name"`
}

func (c *ContactGroupsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("empty name")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	created, err := svc.ContactGroups.Create(&people.CreateContactGroupRequest{
		ContactGroup: &people.ContactGroup{
			Name: name,
		},
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"contactGroup": created})
	}

	u.Out().Printf("resource\t%s", created.ResourceName)
	u.Out().Printf("name\t%s", created.Name)
	return nil
}

// ContactGroupsUpdateCmd updates an existing contact group.
type ContactGroupsUpdateCmd struct {
	ResourceName string `arg:"" name:"resourceName" help:"Resource name (contactGroups/...)"`
	Name         string `name:"name" help:"New name for the group"`
}

func (c *ContactGroupsUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceName := strings.TrimSpace(c.ResourceName)
	if resourceName == "" {
		return usage("empty resourceName")
	}

	// Allow short form (just the ID)
	if !strings.HasPrefix(resourceName, "contactGroups/") {
		resourceName = "contactGroups/" + resourceName
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	// Get existing group to retrieve etag
	existing, err := svc.ContactGroups.Get(resourceName).GroupFields(groupsReadMask).Do()
	if err != nil {
		return err
	}

	// Build update request
	updateFields := make([]string, 0, 1)

	if flagProvided(kctx, "name") {
		existing.Name = strings.TrimSpace(c.Name)
		updateFields = append(updateFields, "name")
	}

	if len(updateFields) == 0 {
		return usage("no updates provided; use --name")
	}

	req := &people.UpdateContactGroupRequest{
		ContactGroup:      existing,
		UpdateGroupFields: strings.Join(updateFields, ","),
	}
	updated, err := svc.ContactGroups.Update(resourceName, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"contactGroup": updated})
	}

	u.Out().Printf("resource\t%s", updated.ResourceName)
	u.Out().Printf("name\t%s", updated.Name)
	return nil
}

// ContactGroupsDeleteCmd deletes a contact group.
type ContactGroupsDeleteCmd struct {
	ResourceName   string `arg:"" name:"resourceName" help:"Resource name (contactGroups/...)"`
	DeleteContacts bool   `name:"delete-contacts" help:"Also delete contacts in this group"`
}

func (c *ContactGroupsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceName := strings.TrimSpace(c.ResourceName)
	if resourceName == "" {
		return usage("empty resourceName")
	}

	// Allow short form (just the ID)
	if !strings.HasPrefix(resourceName, "contactGroups/") {
		resourceName = "contactGroups/" + resourceName
	}

	// Warn about contacts if the group has members
	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	existing, err := svc.ContactGroups.Get(resourceName).GroupFields("name,memberCount").Do()
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("delete contact group %q (%s)", existing.Name, resourceName)
	if existing.MemberCount > 0 && !c.DeleteContacts {
		msg = fmt.Sprintf("delete contact group %q (%s) with %d contacts (contacts will be removed from group but not deleted)", existing.Name, resourceName, existing.MemberCount)
	}
	if c.DeleteContacts {
		msg = fmt.Sprintf("delete contact group %q (%s) and its %d contacts permanently", existing.Name, resourceName, existing.MemberCount)
	}

	if confirmErr := confirmDestructive(ctx, flags, msg); confirmErr != nil {
		return confirmErr
	}

	call := svc.ContactGroups.Delete(resourceName)
	if c.DeleteContacts {
		call = call.DeleteContacts(c.DeleteContacts)
	}

	if _, err := call.Do(); err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, resourceName)
}
