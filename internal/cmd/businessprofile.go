package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var (
	newBusinessProfileAccountsService = googleapi.NewBusinessProfileAccounts
	newBusinessProfileInfoService     = googleapi.NewBusinessProfileInfo
)

type BusinessProfileCmd struct {
	Accounts        BusinessProfileAccountsCmd            `cmd:"" name:"accounts" help:"Account management"`
	Admins          BusinessProfileAccountAdminsCmd       `cmd:"" name:"account-admins" help:"Account admin management"`
	Invitations     BusinessProfileInvitationsCmd         `cmd:"" name:"account-invitations" help:"Account invitation management"`
	LocationAdmins  BusinessProfileLocationAdminsCmd      `cmd:"" name:"location-admins" help:"Location admin management"`
	Transfer        BusinessProfileLocationTransferCmd    `cmd:"" name:"locations-transfer" help:"Transfer location ownership"`
	Categories      BusinessProfileInfoCategoriesCmd      `cmd:"" name:"categories" help:"Business category reference data"`
	Chains          BusinessProfileInfoChainsCmd          `cmd:"" name:"chains" help:"Chain reference data"`
	GoogleLocations BusinessProfileInfoGoogleLocationsCmd `cmd:"" name:"google-locations" help:"Search for Google locations"`
	InfoLocations   BusinessProfileInfoLocationsCmd       `cmd:"" name:"info-locations" help:"Location creation, deletion, and patching"`
	Locations       BusinessProfileLocationsCmd           `cmd:"" name:"locations" group:"Read" help:"List locations for an account"`
	Get             BusinessProfileGetCmd                 `cmd:"" name:"get" group:"Read" help:"Get location details"`
}

// BusinessProfileAccountsCmd is a parent struct with account subcommands.
type BusinessProfileAccountsCmd struct {
	List   BusinessProfileAccountsListCmd   `cmd:"" name:"list" help:"List accounts"`
	Create BusinessProfileAccountsCreateCmd `cmd:"" name:"create" help:"Create an account"`
	Get    BusinessProfileAccountsGetCmd    `cmd:"" name:"get" help:"Get an account"`
	Patch  BusinessProfileAccountsPatchCmd  `cmd:"" name:"patch" help:"Patch an account (partial update)"`
}

// --- locations ---

type BusinessProfileLocationsCmd struct {
	Parent   string `arg:"" name:"account" help:"Account resource name (e.g. '123456789' or 'accounts/123456789')"`
	PageSize int64  `name:"page-size" help:"Max results per page" default:"100"`
	Page     string `name:"page" help:"Page token"`
	ReadMask string `name:"read-mask" help:"Comma-separated fields to return" default:"name,title,storefrontAddress"`
}

func (c *BusinessProfileLocationsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parentAccount := strings.TrimSpace(c.Parent)
	if parentAccount == "" {
		return usage("account argument required")
	}
	if !strings.HasPrefix(parentAccount, "accounts/") {
		parentAccount = "accounts/" + parentAccount
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.Locations.List(parentAccount).
		PageSize(c.PageSize).
		PageToken(c.Page).
		ReadMask(c.ReadMask)

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"locations":     resp.Locations,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Locations) == 0 {
		u.Err().Println("No locations")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTITLE\tADDRESS")
	for _, loc := range resp.Locations {
		addr := formatAddress(loc.StorefrontAddress)
		fmt.Fprintf(w, "%s\t%s\t%s\n", loc.Name, loc.Title, addr)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- get ---

type BusinessProfileGetCmd struct {
	LocationName string `arg:"" name:"locationName" help:"Location resource name (e.g. '123456789' or 'locations/123456789')"`
	ReadMask     string `name:"read-mask" help:"Comma-separated fields to return" default:"name,title,storefrontAddress,phoneNumbers,websiteUri"`
}

func (c *BusinessProfileGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	locationName := strings.TrimSpace(c.LocationName)
	if locationName == "" {
		return usage("locationName required")
	}
	if !strings.HasPrefix(locationName, "locations/") {
		locationName = "locations/" + locationName
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	loc, err := svc.Locations.Get(locationName).ReadMask(c.ReadMask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"location": loc})
	}

	u.Out().Printf("name\t%s", loc.Name)
	u.Out().Printf("title\t%s", loc.Title)
	if loc.StorefrontAddress != nil {
		u.Out().Printf("address\t%s", formatAddress(loc.StorefrontAddress))
	}
	if loc.PhoneNumbers != nil {
		u.Out().Printf("phone\t%s", loc.PhoneNumbers.PrimaryPhone)
		if len(loc.PhoneNumbers.AdditionalPhones) > 0 {
			u.Out().Printf("additionalPhones\t%s", strings.Join(loc.PhoneNumbers.AdditionalPhones, ", "))
		}
	}
	if loc.WebsiteUri != "" {
		u.Out().Printf("website\t%s", loc.WebsiteUri)
	}
	return nil
}

// formatAddress renders a PostalAddress as a single-line string.
func formatAddress(addr *mybusinessbusinessinformation.PostalAddress) string {
	if addr == nil {
		return ""
	}
	var parts []string
	for _, line := range addr.AddressLines {
		if s := strings.TrimSpace(line); s != "" {
			parts = append(parts, s)
		}
	}
	if s := strings.TrimSpace(addr.Locality); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(addr.AdministrativeArea); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(addr.PostalCode); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(addr.RegionCode); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// Ensure service types are used to avoid import cycle lint errors.
var _ *mybusinessbusinessinformation.Service
