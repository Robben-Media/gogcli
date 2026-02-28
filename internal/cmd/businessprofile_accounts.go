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

// BusinessProfileAccountsCreateCmd creates a new account.
type BusinessProfileAccountsCreateCmd struct {
	AccountName  string `name:"account-name" required:"" help:"Display name for the account"`
	Type         string `name:"type" help:"Account type (LOCATION_GROUP, USER_GROUP)" default:"LOCATION_GROUP"`
	PrimaryOwner string `name:"primary-owner" help:"Primary owner account resource name (accounts/...)"`
}

func (c *BusinessProfileAccountsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	accountName := strings.TrimSpace(c.AccountName)
	if accountName == "" {
		return usage("required: --account-name")
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	acct := &mybusinessaccountmanagement.Account{
		AccountName: accountName,
		Type:        c.Type,
	}

	primaryOwner := strings.TrimSpace(c.PrimaryOwner)
	if primaryOwner != "" {
		if !strings.HasPrefix(primaryOwner, "accounts/") {
			primaryOwner = "accounts/" + primaryOwner
		}
		acct.PrimaryOwner = primaryOwner
	}

	resp, err := svc.Accounts.Create(acct).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"account": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.AccountName != "" {
		u.Out().Printf("accountName\t%s", resp.AccountName)
	}
	if resp.Type != "" {
		u.Out().Printf("type\t%s", resp.Type)
	}
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	return nil
}

// BusinessProfileAccountsGetCmd gets an account by name.
type BusinessProfileAccountsGetCmd struct {
	Name string `arg:"" name:"name" help:"Account resource name (accounts/...)"`
}

func (c *BusinessProfileAccountsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}
	if !strings.HasPrefix(name, "accounts/") {
		name = "accounts/" + name
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Accounts.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"account": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.AccountName != "" {
		u.Out().Printf("accountName\t%s", resp.AccountName)
	}
	if resp.Type != "" {
		u.Out().Printf("type\t%s", resp.Type)
	}
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	if resp.PermissionLevel != "" {
		u.Out().Printf("permissionLevel\t%s", resp.PermissionLevel)
	}
	return nil
}

// BusinessProfileAccountsPatchCmd patches an account (partial update).
type BusinessProfileAccountsPatchCmd struct {
	Name         string `arg:"" name:"name" help:"Account resource name (accounts/...)"`
	AccountName  string `name:"account-name" help:"New display name"`
	PrimaryOwner string `name:"primary-owner" help:"New primary owner account resource name"`
}

func (c *BusinessProfileAccountsPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: name")
	}
	if !strings.HasPrefix(name, "accounts/") {
		name = "accounts/" + name
	}

	var fields []string
	acct := &mybusinessaccountmanagement.Account{}

	if flagProvided(kctx, "account-name") {
		fields = append(fields, "accountName")
		acct.AccountName = c.AccountName
	}
	if flagProvided(kctx, "primary-owner") {
		fields = append(fields, "primaryOwner")
		primaryOwner := strings.TrimSpace(c.PrimaryOwner)
		if primaryOwner != "" && !strings.HasPrefix(primaryOwner, "accounts/") {
			primaryOwner = "accounts/" + primaryOwner
		}
		acct.PrimaryOwner = primaryOwner
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided to update")
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	mask := strings.Join(fields, ",")
	resp, err := svc.Accounts.Patch(name, acct).UpdateMask(mask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"account": resp})
	}

	if resp.Name != "" {
		u.Out().Printf("name\t%s", resp.Name)
	}
	if resp.AccountName != "" {
		u.Out().Printf("accountName\t%s", resp.AccountName)
	}
	if resp.Type != "" {
		u.Out().Printf("type\t%s", resp.Type)
	}
	if resp.Role != "" {
		u.Out().Printf("role\t%s", resp.Role)
	}
	return nil
}

// BusinessProfileAccountsListCmd lists all accounts (restructured from direct Run on parent).
type BusinessProfileAccountsListCmd struct{}

func (c *BusinessProfileAccountsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Accounts.List().Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"accounts":      resp.Accounts,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Accounts) == 0 {
		u.Err().Println("No accounts")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tACCOUNT_NAME\tTYPE\tROLE")
	for _, acct := range resp.Accounts {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", acct.Name, acct.AccountName, acct.Type, acct.Role)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}
