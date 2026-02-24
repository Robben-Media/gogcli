package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// BusinessProfileInfoAttributesCmd is a parent for attribute subcommands.
type BusinessProfileInfoAttributesCmd struct {
	List BusinessProfileInfoAttributesListCmd `cmd:"" name:"list" help:"List available attribute metadata"`
}

// BusinessProfileInfoLocationAttrsCmd is a parent for location-attribute subcommands.
type BusinessProfileInfoLocationAttrsCmd struct {
	Get              BusinessProfileInfoLocationAttrsGetCmd              `cmd:"" name:"get" help:"Get attributes for a location"`
	GetGoogleUpdated BusinessProfileInfoLocationAttrsGetGoogleUpdatedCmd `cmd:"" name:"get-google-updated" help:"Get Google-updated attributes"`
	Update           BusinessProfileInfoLocationAttrsUpdateCmd           `cmd:"" name:"update" help:"Update attributes for a location"`
}

// BusinessProfileInfoAttributesListCmd lists available attribute metadata.
type BusinessProfileInfoAttributesListCmd struct {
	Parent       string `name:"parent" help:"Location resource name (if set, other filters are ignored)"`
	CategoryName string `name:"category-name" help:"Category filter (e.g. 'categories/gcid:restaurant')"`
	LanguageCode string `name:"language-code" help:"BCP 47 language code" default:"en"`
	RegionCode   string `name:"region-code" help:"ISO 3166-1 alpha-2 region code"`
	Max          int64  `name:"max" help:"Max results per page" default:"200"`
	Page         string `name:"page" help:"Page token"`
}

func (c *BusinessProfileInfoAttributesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Attributes.List().
		LanguageCode(c.LanguageCode).
		PageSize(c.Max)

	if parent := strings.TrimSpace(c.Parent); parent != "" {
		if !strings.HasPrefix(parent, "locations/") {
			parent = "locations/" + parent
		}
		call = call.Parent(parent)
	}
	if cn := strings.TrimSpace(c.CategoryName); cn != "" {
		call = call.CategoryName(cn)
	}
	if rc := strings.TrimSpace(c.RegionCode); rc != "" {
		call = call.RegionCode(rc)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"attributeMetadata": resp.AttributeMetadata,
			"nextPageToken":     resp.NextPageToken,
		})
	}

	if len(resp.AttributeMetadata) == 0 {
		u.Err().Println("No attributes")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PARENT\tDISPLAY_NAME\tVALUE_TYPE")
	for _, am := range resp.AttributeMetadata {
		fmt.Fprintf(w, "%s\t%s\t%s\n", am.Parent, am.DisplayName, am.ValueType)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// BusinessProfileInfoLocationAttrsGetCmd gets attributes for a location.
type BusinessProfileInfoLocationAttrsGetCmd struct {
	Name string `arg:"" name:"name" help:"Location attributes resource name (e.g. 'locations/123/attributes')"`
}

func (c *BusinessProfileInfoLocationAttrsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: location attributes name")
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Locations.GetAttributes(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"name":       resp.Name,
			"attributes": resp.Attributes,
		})
	}

	u.Out().Printf("name\t%s", resp.Name)
	if len(resp.Attributes) == 0 {
		u.Err().Println("No attributes set")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ATTRIBUTE_ID\tVALUE_TYPE\tVALUES")
	for _, a := range resp.Attributes {
		vals := fmt.Sprintf("%v", a.Values)
		fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, a.ValueType, vals)
	}
	return nil
}

// BusinessProfileInfoLocationAttrsGetGoogleUpdatedCmd gets Google-updated attributes.
type BusinessProfileInfoLocationAttrsGetGoogleUpdatedCmd struct {
	Name string `arg:"" name:"name" help:"Location attributes resource name (e.g. 'locations/123/attributes')"`
}

func (c *BusinessProfileInfoLocationAttrsGetGoogleUpdatedCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: location attributes name")
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Locations.Attributes.GetGoogleUpdated(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"name":       resp.Name,
			"attributes": resp.Attributes,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("name\t%s", resp.Name)
	if len(resp.Attributes) == 0 {
		u.Err().Println("No Google-updated attributes")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ATTRIBUTE_ID\tVALUE_TYPE\tVALUES")
	for _, a := range resp.Attributes {
		vals := fmt.Sprintf("%v", a.Values)
		fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, a.ValueType, vals)
	}
	return nil
}

// BusinessProfileInfoLocationAttrsUpdateCmd updates attributes for a location.
type BusinessProfileInfoLocationAttrsUpdateCmd struct {
	Name           string `arg:"" name:"name" help:"Location attributes resource name (e.g. 'locations/123/attributes')"`
	AttributesJSON string `name:"attributes-json" required:"" help:"JSON array of attributes (or @filepath to read from file)"`
	AttributeMask  string `name:"attribute-mask" help:"Comma-separated attribute names to update"`
}

func (c *BusinessProfileInfoLocationAttrsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("required: location attributes name")
	}

	// Read JSON input — support @filepath
	jsonData := c.AttributesJSON
	if strings.HasPrefix(jsonData, "@") {
		filePath := strings.TrimPrefix(jsonData, "@")
		data, readErr := os.ReadFile(filePath) // #nosec G304
		if readErr != nil {
			return fmt.Errorf("reading attributes file: %w", readErr)
		}
		jsonData = string(data)
	}

	var attrs []*mybusinessbusinessinformation.Attribute
	if unmarshalErr := json.Unmarshal([]byte(jsonData), &attrs); unmarshalErr != nil {
		return fmt.Errorf("parsing attributes JSON: %w", unmarshalErr)
	}

	svc, err := newBusinessProfileInfoService(ctx, account)
	if err != nil {
		return err
	}

	attrsObj := &mybusinessbusinessinformation.Attributes{
		Name:       name,
		Attributes: attrs,
	}

	call := svc.Locations.UpdateAttributes(name, attrsObj)
	if mask := strings.TrimSpace(c.AttributeMask); mask != "" {
		call = call.AttributeMask(mask)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"name":       resp.Name,
			"attributes": resp.Attributes,
		})
	}

	u.Out().Printf("name\t%s", resp.Name)
	u.Err().Printf("Updated %d attributes", len(resp.Attributes))
	return nil
}
