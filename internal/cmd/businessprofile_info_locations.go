package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// BusinessProfileInfoLocationsCmd is a parent for location info subcommands.
type BusinessProfileInfoLocationsCmd struct {
	Create           BusinessProfileInfoLocationsCreateCmd           `cmd:"" name:"create" help:"Create a location under an account"`
	Delete           BusinessProfileInfoLocationsDeleteCmd           `cmd:"" name:"delete" help:"Delete a location"`
	GetGoogleUpdated BusinessProfileInfoLocationsGetGoogleUpdatedCmd `cmd:"" name:"get-google-updated" help:"Get Google-updated version of a location"`
	Patch            BusinessProfileInfoLocationsPatchCmd            `cmd:"" name:"patch" help:"Patch a location (partial update)"`
}

// BusinessProfileInfoGoogleLocationsCmd searches for Google locations.
type BusinessProfileInfoGoogleLocationsCmd struct {
	Query string `name:"query" required:"" help:"Search query (business name + address)"`
	Max   int64  `name:"max" help:"Max results" default:"10"`
}

func (c *BusinessProfileInfoGoogleLocationsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	req := &mybusinessbusinessinformation.SearchGoogleLocationsRequest{
		Query:    c.Query,
		PageSize: c.Max,
	}

	resp, err := svc.GoogleLocations.Search(req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"googleLocations": resp.GoogleLocations,
		})
	}

	if len(resp.GoogleLocations) == 0 {
		u.Err().Println("No Google locations found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tTITLE\tADDRESS")
	for _, gl := range resp.GoogleLocations {
		title := ""
		addr := ""
		if gl.Location != nil {
			title = gl.Location.Title
			addr = formatAddress(gl.Location.StorefrontAddress)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", gl.Name, title, addr)
	}
	return nil
}

// BusinessProfileInfoLocationsCreateCmd creates a location under an account.
type BusinessProfileInfoLocationsCreateCmd struct {
	Parent       string  `arg:"" name:"parent" help:"Account resource name (e.g. '123' or 'accounts/123')"`
	Title        string  `name:"title" required:"" help:"Business name"`
	CategoryID   string  `name:"category-id" required:"" help:"Primary category ID (e.g. 'gcid:restaurant')"`
	StoreCode    string  `name:"store-code" help:"External store code"`
	Phone        string  `name:"phone" help:"Primary phone number"`
	Website      string  `name:"website" help:"Website URL"`
	AddressLines string  `name:"address-lines" help:"Comma-separated address lines"`
	Locality     string  `name:"locality" help:"City"`
	Region       string  `name:"region" help:"State/province"`
	PostalCode   string  `name:"postal-code" help:"Postal/zip code"`
	Country      string  `name:"country" help:"ISO 3166-1 alpha-2 country code"`
	Latitude     float64 `name:"latitude" help:"Latitude"`
	Longitude    float64 `name:"longitude" help:"Longitude"`
}

func (c *BusinessProfileInfoLocationsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	loc := &mybusinessbusinessinformation.Location{
		Title: c.Title,
		Categories: &mybusinessbusinessinformation.Categories{
			PrimaryCategory: &mybusinessbusinessinformation.Category{
				Name: c.CategoryID,
			},
		},
	}

	if c.StoreCode != "" {
		loc.StoreCode = c.StoreCode
	}

	if c.Phone != "" {
		loc.PhoneNumbers = &mybusinessbusinessinformation.PhoneNumbers{
			PrimaryPhone: c.Phone,
		}
	}

	if c.Website != "" {
		loc.WebsiteUri = c.Website
	}

	addr := buildPostalAddress(c.AddressLines, c.Locality, c.Region, c.PostalCode, c.Country)
	if addr != nil {
		loc.StorefrontAddress = addr
	}

	if c.Latitude != 0 || c.Longitude != 0 {
		loc.Latlng = &mybusinessbusinessinformation.LatLng{
			Latitude:  c.Latitude,
			Longitude: c.Longitude,
		}
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Accounts.Locations.Create(parent, loc).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"location": resp})
	}

	u.Out().Printf("name\t%s", resp.Name)
	u.Out().Printf("title\t%s", resp.Title)
	if resp.StorefrontAddress != nil {
		u.Out().Printf("address\t%s", formatAddress(resp.StorefrontAddress))
	}
	return nil
}

// BusinessProfileInfoLocationsDeleteCmd deletes a location.
type BusinessProfileInfoLocationsDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Location resource name (e.g. '123' or 'locations/123')"`
}

func (c *BusinessProfileInfoLocationsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("permanently delete location %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	if _, delErr := svc.Locations.Delete(name).Do(); delErr != nil {
		return delErr
	}

	u.Err().Println("Deleted")
	return nil
}

// BusinessProfileInfoLocationsGetGoogleUpdatedCmd gets the Google-updated version.
type BusinessProfileInfoLocationsGetGoogleUpdatedCmd struct {
	Name string `arg:"" name:"name" help:"Location resource name (e.g. '123' or 'locations/123')"`
}

func (c *BusinessProfileInfoLocationsGetGoogleUpdatedCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Locations.GetGoogleUpdated(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"location":    resp.Location,
			"diffMask":    resp.DiffMask,
			"pendingMask": resp.PendingMask,
		})
	}

	if resp.Location != nil {
		u.Out().Printf("name\t%s", resp.Location.Name)
		u.Out().Printf("title\t%s", resp.Location.Title)
		if resp.Location.StorefrontAddress != nil {
			u.Out().Printf("address\t%s", formatAddress(resp.Location.StorefrontAddress))
		}
	}
	if resp.DiffMask != "" {
		u.Out().Printf("diffMask\t%s", resp.DiffMask)
	}
	if resp.PendingMask != "" {
		u.Out().Printf("pendingMask\t%s", resp.PendingMask)
	}
	return nil
}

// BusinessProfileInfoLocationsPatchCmd patches a location.
type BusinessProfileInfoLocationsPatchCmd struct {
	Name         string `arg:"" name:"name" help:"Location resource name (e.g. '123' or 'locations/123')"`
	Title        string `name:"title" help:"Business name"`
	Phone        string `name:"phone" help:"Primary phone number"`
	Website      string `name:"website" help:"Website URL"`
	AddressLines string `name:"address-lines" help:"Comma-separated address lines"`
	Locality     string `name:"locality" help:"City"`
	Region       string `name:"region" help:"State/province"`
	PostalCode   string `name:"postal-code" help:"Postal/zip code"`
	Country      string `name:"country" help:"ISO 3166-1 alpha-2 country code"`
	CategoryID   string `name:"category-id" help:"Primary category ID"`
}

func (c *BusinessProfileInfoLocationsPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
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

	var fields []string
	loc := &mybusinessbusinessinformation.Location{}

	if flagProvided(kctx, "title") {
		fields = append(fields, "title")
		loc.Title = c.Title
	}
	if flagProvided(kctx, "phone") {
		fields = append(fields, "phoneNumbers.primaryPhone")
		loc.PhoneNumbers = &mybusinessbusinessinformation.PhoneNumbers{
			PrimaryPhone: c.Phone,
		}
	}
	if flagProvided(kctx, "website") {
		fields = append(fields, "websiteUri")
		loc.WebsiteUri = c.Website
	}
	if flagProvided(kctx, "category-id") {
		fields = append(fields, "categories.primaryCategory")
		loc.Categories = &mybusinessbusinessinformation.Categories{
			PrimaryCategory: &mybusinessbusinessinformation.Category{
				Name: c.CategoryID,
			},
		}
	}

	// Address fields — if any address flag is provided, update the whole address
	addrProvided := flagProvided(kctx, "address-lines") || flagProvided(kctx, "locality") ||
		flagProvided(kctx, "region") || flagProvided(kctx, "postal-code") || flagProvided(kctx, "country")
	if addrProvided {
		fields = append(fields, "storefrontAddress")
		loc.StorefrontAddress = buildPostalAddress(c.AddressLines, c.Locality, c.Region, c.PostalCode, c.Country)
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided to update")
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	mask := strings.Join(fields, ",")
	resp, err := svc.Locations.Patch(name, loc).UpdateMask(mask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"location": resp})
	}

	u.Out().Printf("name\t%s", resp.Name)
	u.Out().Printf("title\t%s", resp.Title)
	if resp.StorefrontAddress != nil {
		u.Out().Printf("address\t%s", formatAddress(resp.StorefrontAddress))
	}
	return nil
}

// buildPostalAddress constructs a PostalAddress from individual fields.
func buildPostalAddress(addressLines, locality, region, postalCode, country string) *mybusinessbusinessinformation.PostalAddress {
	addr := &mybusinessbusinessinformation.PostalAddress{}
	hasAny := false

	if al := strings.TrimSpace(addressLines); al != "" {
		for _, line := range strings.Split(al, ",") {
			if s := strings.TrimSpace(line); s != "" {
				addr.AddressLines = append(addr.AddressLines, s)
			}
		}
		hasAny = true
	}
	if s := strings.TrimSpace(locality); s != "" {
		addr.Locality = s
		hasAny = true
	}
	if s := strings.TrimSpace(region); s != "" {
		addr.AdministrativeArea = s
		hasAny = true
	}
	if s := strings.TrimSpace(postalCode); s != "" {
		addr.PostalCode = s
		hasAny = true
	}
	if s := strings.TrimSpace(country); s != "" {
		addr.RegionCode = s
		hasAny = true
	}

	if !hasAny {
		return nil
	}
	return addr
}
