package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ChatMembersCmd contains subcommands for space members management.
type ChatMembersCmd struct {
	List   ChatMembersListCmd   `cmd:"" name:"list" help:"List space members"`
	Get    ChatMembersGetCmd    `cmd:"" name:"get" help:"Get a member"`
	Create ChatMembersCreateCmd `cmd:"" name:"create" help:"Add a member to a space"`
	Delete ChatMembersDeleteCmd `cmd:"" name:"delete" help:"Remove a member from a space"`
	Patch  ChatMembersPatchCmd  `cmd:"" name:"patch" help:"Update a member's role"`
}

// memberDisplayName returns a display name for a membership's user.
func memberDisplayName(m *chat.Membership) string {
	if m == nil || m.Member == nil {
		return ""
	}
	if m.Member.DisplayName != "" {
		return m.Member.DisplayName
	}
	return m.Member.Name
}

// ChatMembersListCmd lists members in a space.
type ChatMembersListCmd struct {
	Parent string `arg:"" name:"parent" help:"Space resource name (spaces/...)"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results (default: 100)" default:"100"`
	Page   string `name:"page" help:"Page token"`
	Filter string `name:"filter" help:"Filter expression"`
}

func (c *ChatMembersListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	call := svc.Spaces.Members.List(parent).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	if c.Filter != "" {
		call = call.Filter(c.Filter)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			User     string `json:"user,omitempty"`
			Role     string `json:"role,omitempty"`
			State    string `json:"state,omitempty"`
		}
		items := make([]item, 0, len(resp.Memberships))
		for _, member := range resp.Memberships {
			if member == nil {
				continue
			}
			items = append(items, item{
				Resource: member.Name,
				User:     memberDisplayName(member),
				Role:     member.Role,
				State:    member.State,
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"memberships":   items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Memberships) == 0 {
		u.Err().Println("No members found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tUSER\tROLE\tSTATE")
	for _, member := range resp.Memberships {
		if member == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			member.Name,
			sanitizeTab(memberDisplayName(member)),
			sanitizeTab(member.Role),
			sanitizeTab(member.State),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// ChatMembersGetCmd gets a specific membership.
type ChatMembersGetCmd struct {
	Name string `arg:"" name:"name" help:"Membership resource name (spaces/.../members/...)"`
}

func (c *ChatMembersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resp, err := svc.Spaces.Members.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"membership": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	u.Out().Printf("user\t%s", memberDisplayName(resp))
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	if resp.State != "" {
		u.Out().Printf("state\t%s", resp.State)
	}
	return nil
}

// ChatMembersCreateCmd adds a member to a space.
type ChatMembersCreateCmd struct {
	Parent string `arg:"" name:"parent" help:"Space resource name (spaces/...)"`
	User   string `name:"user" required:"" help:"User resource name (users/...)"`
	Role   string `name:"role" help:"Member role (default: ROLE_MEMBER)" default:"ROLE_MEMBER"`
}

func (c *ChatMembersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	user := strings.TrimSpace(c.User)
	if user == "" {
		return usage("required: --user")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	membership := &chat.Membership{
		Member: &chat.User{
			Name: normalizeUser(user),
			Type: "HUMAN",
		},
		Role: c.Role,
	}

	resp, err := svc.Spaces.Members.Create(parent, membership).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"membership": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	u.Out().Printf("user\t%s", memberDisplayName(resp))
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	if resp.State != "" {
		u.Out().Printf("state\t%s", resp.State)
	}
	return nil
}

// ChatMembersDeleteCmd removes a member from a space.
type ChatMembersDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Membership resource name (spaces/.../members/...)"`
}

func (c *ChatMembersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove member %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.Spaces.Members.Delete(name).Do()
	if err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, fmt.Sprintf("member %s", name))
}

// ChatMembersPatchCmd updates a member's role.
type ChatMembersPatchCmd struct {
	Name string `arg:"" name:"name" help:"Membership resource name (spaces/.../members/...)"`
	Role string `name:"role" help:"New role for the member"`
}

func (c *ChatMembersPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
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

	// Build update mask from provided flags
	var fields []string
	membership := &chat.Membership{}

	if flagProvided(kctx, "role") {
		fields = append(fields, "role")
		membership.Role = c.Role
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided to update")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	mask := strings.Join(fields, ",")
	resp, err := svc.Spaces.Members.Patch(name, membership).UpdateMask(mask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"membership": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	u.Out().Printf("user\t%s", memberDisplayName(resp))
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	if resp.State != "" {
		u.Out().Printf("state\t%s", resp.State)
	}
	return nil
}
