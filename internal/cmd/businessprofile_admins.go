package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	mybusinessaccountmanagement "google.golang.org/api/mybusinessaccountmanagement/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// BusinessProfileAccountAdminsCmd is a parent for account admin subcommands.
type BusinessProfileAccountAdminsCmd struct {
	List   BusinessProfileAccountAdminsListCmd   `cmd:"" name:"list" help:"List admins for an account"`
	Create BusinessProfileAccountAdminsCreateCmd `cmd:"" name:"create" help:"Add an admin to an account"`
	Delete BusinessProfileAccountAdminsDeleteCmd `cmd:"" name:"delete" help:"Remove an admin from an account"`
	Patch  BusinessProfileAccountAdminsPatchCmd  `cmd:"" name:"patch" help:"Update an admin's role"`
}

// BusinessProfileAccountAdminsListCmd lists admins for an account.
type BusinessProfileAccountAdminsListCmd struct {
	Parent string `arg:"" name:"parent" help:"Account resource name (e.g. '123' or 'accounts/123')"`
}

func (c *BusinessProfileAccountAdminsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("required: parent account")
	}
	if !strings.HasPrefix(parent, "accounts/") {
		parent = "accounts/" + parent
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Accounts.Admins.List(parent).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"admins": resp.AccountAdmins,
		})
	}

	if len(resp.AccountAdmins) == 0 {
		u.Err().Println("No admins")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tADMIN\tROLE\tPENDING")
	for _, a := range resp.AccountAdmins {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", a.Name, a.Admin, a.Role, a.PendingInvitation)
	}
	return nil
}

// BusinessProfileAccountAdminsCreateCmd creates an admin for an account.
type BusinessProfileAccountAdminsCreateCmd struct {
	Parent string `arg:"" name:"parent" help:"Account resource name (e.g. '123' or 'accounts/123')"`
	Admin  string `name:"admin" required:"" help:"Admin email address"`
	Role   string `name:"role" help:"Admin role (OWNER, MANAGER, SITE_MANAGER)" default:"MANAGER"`
}

func (c *BusinessProfileAccountAdminsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("required: parent account")
	}
	if !strings.HasPrefix(parent, "accounts/") {
		parent = "accounts/" + parent
	}

	adminEmail := strings.TrimSpace(c.Admin)
	if adminEmail == "" {
		return usage("required: --admin")
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	admin := &mybusinessaccountmanagement.Admin{
		Admin: adminEmail,
		Role:  c.Role,
	}

	resp, err := svc.Accounts.Admins.Create(parent, admin).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"admin": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.Admin != "" {
		u.Out().Printf("admin\t%s", resp.Admin)
	}
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	return nil
}

// BusinessProfileAccountAdminsDeleteCmd deletes an admin from an account.
type BusinessProfileAccountAdminsDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Admin resource name (e.g. 'accounts/123/admins/456')"`
}

func (c *BusinessProfileAccountAdminsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: admin name")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete admin %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	if _, delErr := svc.Accounts.Admins.Delete(name).Do(); delErr != nil {
		return delErr
	}

	u.Err().Println("Deleted")
	return nil
}

// BusinessProfileAccountAdminsPatchCmd patches an admin's role.
type BusinessProfileAccountAdminsPatchCmd struct {
	Name string `arg:"" name:"name" help:"Admin resource name (e.g. 'accounts/123/admins/456')"`
	Role string `name:"role" help:"New role (OWNER, MANAGER, SITE_MANAGER)"`
}

func (c *BusinessProfileAccountAdminsPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: admin name")
	}

	var fields []string
	admin := &mybusinessaccountmanagement.Admin{}

	if flagProvided(kctx, "role") {
		fields = append(fields, "role")
		admin.Role = c.Role
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided to update")
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	mask := strings.Join(fields, ",")
	resp, err := svc.Accounts.Admins.Patch(name, admin).UpdateMask(mask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"admin": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.Admin != "" {
		u.Out().Printf("admin\t%s", resp.Admin)
	}
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	return nil
}
