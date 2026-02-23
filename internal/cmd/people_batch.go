package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ContactsBatchCmd contains subcommands for batch contact operations.
type ContactsBatchCmd struct {
	Create ContactsBatchCreateCmd `cmd:"" name:"create" help:"Create multiple contacts at once"`
	Delete ContactsBatchDeleteCmd `cmd:"" name:"delete" help:"Delete multiple contacts at once"`
	Update ContactsBatchUpdateCmd `cmd:"" name:"update" help:"Update multiple contacts at once"`
	Get    ContactsBatchGetCmd    `cmd:"" name:"get" help:"Get multiple contacts at once"`
}

// ContactsBatchCreateCmd creates multiple contacts in a single request.
// Accepts JSON input via --contacts-json or --contacts-file.
type ContactsBatchCreateCmd struct {
	ContactsJSON string `name:"contacts-json" help:"JSON array of contacts to create"`
	ContactsFile string `name:"contacts-file" help:"File containing JSON contacts (@path or - for stdin)"`
	ReadMask     string `name:"read-mask" help:"Fields to return (comma-separated)" default:"names,emailAddresses,phoneNumbers"`
}

func (c *ContactsBatchCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	// Read JSON input
	raw, err := readContactsJSON(c.ContactsJSON, c.ContactsFile)
	if err != nil {
		return err
	}

	// Parse the contacts - accept either an array of ContactToCreate or an array of Person
	var contactsToCreate []*people.ContactToCreate

	// First try as array of ContactToCreate (with contactPerson wrapper)
	var wrappedContacts []struct {
		ContactPerson *people.Person `json:"contactPerson"`
	}
	if parseErr := json.Unmarshal([]byte(raw), &wrappedContacts); parseErr == nil && len(wrappedContacts) > 0 && wrappedContacts[0].ContactPerson != nil {
		for _, wc := range wrappedContacts {
			contactsToCreate = append(contactsToCreate, &people.ContactToCreate{
				ContactPerson: wc.ContactPerson,
			})
		}
	} else {
		// Try as array of Person objects directly
		var persons []*people.Person
		if parseErr := json.Unmarshal([]byte(raw), &persons); parseErr != nil {
			return fmt.Errorf("invalid contacts JSON: expected array of Person or ContactToCreate objects: %w", parseErr)
		}
		for _, p := range persons {
			contactsToCreate = append(contactsToCreate, &people.ContactToCreate{
				ContactPerson: p,
			})
		}
	}

	if len(contactsToCreate) == 0 {
		return usage("no contacts provided")
	}

	if len(contactsToCreate) > 200 {
		return usage("maximum 200 contacts allowed per batch")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	req := &people.BatchCreateContactsRequest{
		Contacts: contactsToCreate,
		ReadMask: c.ReadMask,
	}

	resp, err := svc.People.BatchCreateContacts(req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"createdContacts": resp.CreatedPeople,
			"count":           len(resp.CreatedPeople),
		})
	}

	if len(resp.CreatedPeople) == 0 {
		u.Err().Println("No contacts created")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL")
	for _, r := range resp.CreatedPeople {
		if r == nil || r.Person == nil {
			continue
		}
		p := r.Person
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
		)
	}
	u.Out().Printf("Created %d contact(s)", len(resp.CreatedPeople))
	return nil
}

// ContactsBatchDeleteCmd deletes multiple contacts in a single request.
type ContactsBatchDeleteCmd struct {
	ResourceNames []string `arg:"" name:"resourceNames" help:"Resource names to delete (people/...)"`
}

func (c *ContactsBatchDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if len(c.ResourceNames) == 0 {
		return usage("at least one resource name is required")
	}

	if len(c.ResourceNames) > 500 {
		return usage("maximum 500 contacts allowed per batch delete")
	}

	// Normalize resource names
	resourceNames := make([]string, 0, len(c.ResourceNames))
	for _, rn := range c.ResourceNames {
		rn = strings.TrimSpace(rn)
		if rn == "" {
			continue
		}
		if !strings.HasPrefix(rn, "people/") {
			rn = "people/" + rn
		}
		resourceNames = append(resourceNames, rn)
	}

	if len(resourceNames) == 0 {
		return usage("no valid resource names provided")
	}

	// Confirm destructive operation
	msg := fmt.Sprintf("permanently delete %d contact(s)", len(resourceNames))
	if confirmErr := confirmDestructive(ctx, flags, msg); confirmErr != nil {
		return confirmErr
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	req := &people.BatchDeleteContactsRequest{
		ResourceNames: resourceNames,
	}

	if _, err := svc.People.BatchDeleteContacts(req).Do(); err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, fmt.Sprintf("%d contact(s)", len(resourceNames)))
}

// ContactsBatchUpdateCmd updates multiple contacts in a single request.
// Accepts JSON input via --contacts-json or --contacts-file.
// The JSON should be a map of resource name to Person object.
type ContactsBatchUpdateCmd struct {
	ContactsJSON string `name:"contacts-json" help:"JSON map of resource names to contact updates"`
	ContactsFile string `name:"contacts-file" help:"File containing JSON contacts (@path or - for stdin)"`
	ReadMask     string `name:"read-mask" help:"Fields to return (comma-separated)" default:"names,emailAddresses,phoneNumbers"`
	UpdateMask   string `name:"update-mask" help:"Fields to update (comma-separated)" default:"names,emailAddresses,phoneNumbers"`
}

func (c *ContactsBatchUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	// Read JSON input
	raw, err := readContactsJSON(c.ContactsJSON, c.ContactsFile)
	if err != nil {
		return err
	}

	// Parse as map of resource name to Person
	var contactsMap map[string]people.Person
	if parseErr := json.Unmarshal([]byte(raw), &contactsMap); parseErr != nil {
		return fmt.Errorf("invalid contacts JSON: expected map of resource name to Person: %w", parseErr)
	}

	if len(contactsMap) == 0 {
		return usage("no contacts provided")
	}

	if len(contactsMap) > 200 {
		return usage("maximum 200 contacts allowed per batch")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	req := &people.BatchUpdateContactsRequest{
		Contacts:   contactsMap,
		ReadMask:   c.ReadMask,
		UpdateMask: c.UpdateMask,
	}

	resp, err := svc.People.BatchUpdateContacts(req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"updatedContacts": resp.UpdateResult,
			"count":           len(contactsMap),
		})
	}

	u.Out().Printf("Updated %d contact(s)", len(contactsMap))
	return nil
}

// ContactsBatchGetCmd retrieves multiple contacts in a single request.
type ContactsBatchGetCmd struct {
	ResourceNames []string `name:"resource-names" help:"Resource names to retrieve (up to 200)" required:""`
	ReadMask      string   `name:"read-mask" help:"Fields to return (comma-separated)" default:"names,emailAddresses,phoneNumbers"`
}

func (c *ContactsBatchGetCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	resourceNames := make([]string, 0, len(c.ResourceNames))
	for _, rn := range c.ResourceNames {
		rn = strings.TrimSpace(rn)
		if rn == "" {
			continue
		}
		if !strings.HasPrefix(rn, "people/") {
			rn = "people/" + rn
		}
		resourceNames = append(resourceNames, rn)
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.People.GetBatchGet().
		ResourceNames(resourceNames...).
		PersonFields(c.ReadMask).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"responses": resp.Responses,
		})
	}

	if len(resp.Responses) == 0 {
		u.Err().Println("No contacts found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL\tPHONE\tSTATUS")
	for _, r := range resp.Responses {
		if r == nil {
			continue
		}
		if r.Person == nil {
			fmt.Fprintf(w, "\t\t\t\tnot found\n")
			continue
		}
		p := r.Person
		status := "OK"
		if r.Status != nil && r.Status.Code != 0 {
			status = r.Status.Message
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
			sanitizeTab(primaryPhone(p)),
			status,
		)
	}
	return nil
}

// readContactsJSON reads JSON input from either raw string or file path.
func readContactsJSON(raw, path string) (string, error) {
	raw = strings.TrimSpace(raw)
	path = strings.TrimSpace(path)
	if raw == "" && path == "" {
		return "", usagef("provide contacts via --contacts-json or --contacts-file")
	}
	if raw != "" && path != "" {
		return "", usagef("use only one of --contacts-json or --contacts-file")
	}
	if path == "" {
		return raw, nil
	}
	var (
		b   []byte
		err error
	)
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		path, err = config.ExpandPath(path)
		if err != nil {
			return "", err
		}
		b, err = os.ReadFile(path) //nolint:gosec // user-provided path
	}
	if err != nil {
		return "", fmt.Errorf("reading contacts file: %w", err)
	}
	return string(b), nil
}
