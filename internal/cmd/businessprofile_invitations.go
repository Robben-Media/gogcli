package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	mybusinessaccountmanagement "google.golang.org/api/mybusinessaccountmanagement/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// BusinessProfileInvitationsCmd is a parent for account invitation subcommands.
type BusinessProfileInvitationsCmd struct {
	List    BusinessProfileInvitationsListCmd    `cmd:"" name:"list" help:"List pending invitations for an account"`
	Accept  BusinessProfileInvitationsAcceptCmd  `cmd:"" name:"accept" help:"Accept an invitation"`
	Decline BusinessProfileInvitationsDeclineCmd `cmd:"" name:"decline" help:"Decline an invitation"`
}

// BusinessProfileInvitationsListCmd lists invitations for an account.
type BusinessProfileInvitationsListCmd struct {
	Parent string `arg:"" name:"parent" help:"Account resource name (e.g. '123' or 'accounts/123')"`
	Filter string `name:"filter" help:"Filter by target type (e.g. 'target_type=ACCEPT_INVITATION')"`
}

func (c *BusinessProfileInvitationsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	call := svc.Accounts.Invitations.List(parent)
	if filter := strings.TrimSpace(c.Filter); filter != "" {
		call = call.Filter(filter)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"invitations": resp.Invitations,
		})
	}

	if len(resp.Invitations) == 0 {
		u.Err().Println("No invitations")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tROLE\tTARGET_TYPE")
	for _, inv := range resp.Invitations {
		fmt.Fprintf(w, "%s\t%s\t%s\n", inv.Name, inv.Role, inv.TargetType)
	}
	return nil
}

// BusinessProfileInvitationsAcceptCmd accepts an invitation.
type BusinessProfileInvitationsAcceptCmd struct {
	Name string `arg:"" name:"name" help:"Invitation resource name (e.g. 'accounts/123/invitations/456')"`
}

func (c *BusinessProfileInvitationsAcceptCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: invitation name")
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	req := &mybusinessaccountmanagement.AcceptInvitationRequest{}
	if _, err := svc.Accounts.Invitations.Accept(name, req).Do(); err != nil {
		return err
	}

	u.Err().Println("Invitation accepted")
	return nil
}

// BusinessProfileInvitationsDeclineCmd declines an invitation.
type BusinessProfileInvitationsDeclineCmd struct {
	Name string `arg:"" name:"name" help:"Invitation resource name (e.g. 'accounts/123/invitations/456')"`
}

func (c *BusinessProfileInvitationsDeclineCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: invitation name")
	}

	svc, err := newBusinessProfileAccountsService(ctx, account)
	if err != nil {
		return err
	}

	req := &mybusinessaccountmanagement.DeclineInvitationRequest{}
	if _, err := svc.Accounts.Invitations.Decline(name, req).Do(); err != nil {
		return err
	}

	u.Err().Println("Invitation declined")
	return nil
}
