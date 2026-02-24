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

// BusinessProfileLocationAdminsCmd is a parent for location admin subcommands.
type BusinessProfileLocationAdminsCmd struct {
	List   BusinessProfileLocationAdminsListCmd   `cmd:"" name:"list" help:"List admins for a location"`
	Create BusinessProfileLocationAdminsCreateCmd `cmd:"" name:"create" help:"Add an admin to a location"`
	Delete BusinessProfileLocationAdminsDeleteCmd `cmd:"" name:"delete" help:"Remove an admin from a location"`
	Patch  BusinessProfileLocationAdminsPatchCmd  `cmd:"" name:"patch" help:"Update a location admin's role"`
}

// BusinessProfileLocationAdminsListCmd lists admins for a location.
type BusinessProfileLocationAdminsListCmd struct {
	Parent string `arg:"" name:"parent" help:"Location resource name (e.g. '123' or 'locations/123')"`
}

func (c *BusinessProfileLocationAdminsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("required: parent location")
	}
	if !strings.HasPrefix(parent, "locations/") {
		parent = "locations/" + parent
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Locations.Admins.List(parent).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"admins": resp.Admins,
		})
	}

	if len(resp.Admins) == 0 {
		u.Err().Println("No admins")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tADMIN\tROLE\tPENDING")
	for _, a := range resp.Admins {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", a.Name, a.Admin, a.Role, a.PendingInvitation)
	}
	return nil
}

// BusinessProfileLocationAdminsCreateCmd creates an admin for a location.
type BusinessProfileLocationAdminsCreateCmd struct {
	Parent string `arg:"" name:"parent" help:"Location resource name (e.g. '123' or 'locations/123')"`
	Admin  string `name:"admin" required:"" help:"Admin email address"`
	Role   string `name:"role" help:"Admin role (MANAGER, SITE_MANAGER)" default:"MANAGER"`
}

func (c *BusinessProfileLocationAdminsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("required: parent location")
	}
	if !strings.HasPrefix(parent, "locations/") {
		parent = "locations/" + parent
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

	resp, err := svc.Locations.Admins.Create(parent, admin).Do()
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

// BusinessProfileLocationAdminsDeleteCmd deletes an admin from a location.
type BusinessProfileLocationAdminsDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Admin resource name (e.g. 'locations/123/admins/456')"`
}

func (c *BusinessProfileLocationAdminsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: admin name")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete location admin %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	if _, delErr := svc.Locations.Admins.Delete(name).Do(); delErr != nil {
		return delErr
	}

	u.Err().Println("Deleted")
	return nil
}

// BusinessProfileLocationAdminsPatchCmd patches a location admin's role.
type BusinessProfileLocationAdminsPatchCmd struct {
	Name string `arg:"" name:"name" help:"Admin resource name (e.g. 'locations/123/admins/456')"`
	Role string `name:"role" help:"New role (MANAGER, SITE_MANAGER)"`
}

func (c *BusinessProfileLocationAdminsPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
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
	resp, err := svc.Locations.Admins.Patch(name, admin).UpdateMask(mask).Do()
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

// BusinessProfileLocationTransferCmd transfers a location to another account.
type BusinessProfileLocationTransferCmd struct {
	Name               string `arg:"" name:"name" help:"Location resource name (e.g. '123' or 'locations/123')"`
	DestinationAccount string `name:"destination-account" required:"" help:"Destination account resource name (e.g. 'accounts/456')"`
}

func (c *BusinessProfileLocationTransferCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: location name")
	}
	if !strings.HasPrefix(name, "locations/") {
		name = "locations/" + name
	}

	dest := strings.TrimSpace(c.DestinationAccount)
	if dest == "" {
		return usage("required: --destination-account")
	}
	if !strings.HasPrefix(dest, "accounts/") {
		dest = "accounts/" + dest
	}

	if confirmErr := confirmDestructive(ctx, flags,
		fmt.Sprintf("transfer location %s to %s (this permanently changes ownership)", name, dest),
	); confirmErr != nil {
		return confirmErr
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	req := &mybusinessaccountmanagement.TransferLocationRequest{
		DestinationAccount: dest,
	}

	if _, transferErr := svc.Locations.Transfer(name, req).Do(); transferErr != nil {
		return transferErr
	}

	u.Err().Println("Location transferred")
	return nil
}
