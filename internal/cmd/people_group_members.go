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

// ContactGroupMembersCmd contains subcommands for contact group members.
type ContactGroupMembersCmd struct {
	Modify ContactGroupMembersModifyCmd `cmd:"" name:"modify" help:"Add or remove members from a contact group"`
}

// ContactGroupMembersModifyCmd modifies the members of a contact group.
type ContactGroupMembersModifyCmd struct {
	GroupName string   `arg:"" name:"resourceName" help:"Contact group resource name (contactGroups/...)"`
	Add       []string `name:"add" help:"Contact resource names to add to the group (e.g., people/c1)"`
	Remove    []string `name:"remove" help:"Contact resource names to remove from the group"`
}

func (c *ContactGroupMembersModifyCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	// Validate that at least one add or remove is provided
	if len(c.Add) == 0 && len(c.Remove) == 0 {
		return usage("at least one of --add or --remove is required")
	}

	groupName := strings.TrimSpace(c.GroupName)
	if groupName == "" {
		return usage("empty resourceName")
	}

	// Allow short form (just the ID)
	if !strings.HasPrefix(groupName, "contactGroups/") {
		groupName = "contactGroups/" + groupName
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	// Normalize member resource names (prepend "people/" if needed)
	addMembers := normalizeMemberNames(c.Add)
	removeMembers := normalizeMemberNames(c.Remove)

	// Get the group for confirmation and to show what will change
	existing, err := svc.ContactGroups.Get(groupName).
		GroupFields("name,memberCount,memberResourceNames").
		MaxMembers(1000).
		Do()
	if err != nil {
		return err
	}

	// Confirm if adding or removing a significant number of members
	if !flags.Force {
		// Check if confirmation is needed
		totalChanges := len(addMembers) + len(removeMembers)
		if totalChanges > 0 {
			msg := fmt.Sprintf("modify group %q: add %d member(s), remove %d member(s)",
				existing.Name, len(addMembers), len(removeMembers))
			if confirmErr := confirmDestructive(ctx, flags, msg); confirmErr != nil {
				return confirmErr
			}
		}
	}

	// Build and execute the modify request
	req := &people.ModifyContactGroupMembersRequest{
		ResourceNamesToAdd:    addMembers,
		ResourceNamesToRemove: removeMembers,
	}

	resp, err := svc.ContactGroups.Members.Modify(groupName, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"modifiedGroup":    existing,
			"notFoundCount":    len(resp.NotFoundResourceNames),
			"notFound":         resp.NotFoundResourceNames,
			"cannotRemoveLast": resp.CanNotRemoveLastContactGroupResourceNames,
		})
	}

	// Human-friendly output
	u.Out().Printf("resource\t%s", groupName)
	u.Out().Printf("name\t%s", existing.Name)
	if len(addMembers) > 0 {
		u.Out().Printf("members_added\t%d", len(addMembers))
	}
	if len(removeMembers) > 0 {
		u.Out().Printf("members_removed\t%d", len(removeMembers))
	}
	if len(resp.NotFoundResourceNames) > 0 {
		u.Err().Printf("Warning: %d member(s) not found", len(resp.NotFoundResourceNames))
	}
	return nil
}

// normalizeMemberNames ensures all member names have the "people/" prefix.
func normalizeMemberNames(members []string) []string {
	if len(members) == 0 {
		return nil
	}
	result := make([]string, 0, len(members))
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if !strings.HasPrefix(m, "people/") {
			m = "people/" + m
		}
		result = append(result, m)
	}
	return result
}
