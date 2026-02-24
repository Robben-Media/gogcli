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

// ChatSpacesCompleteImportCmd completes the import of a space.
type ChatSpacesCompleteImportCmd struct {
	Name string `arg:"" name:"name" help:"Space resource name (spaces/...)"`
}

func (c *ChatSpacesCompleteImportCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	if !strings.HasPrefix(name, "spaces/") {
		name = "spaces/" + name
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.CompleteImport(name, &chat.CompleteImportSpaceRequest{}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": resp.Space})
	}

	printSpaceDetails(u, resp.Space)
	return nil
}

// ChatSpacesCreateDirectCmd creates a space via the spaces.create API (named
// space without members). This differs from the existing "create" command which
// uses spaces.setup (creates a space with initial members).
type ChatSpacesCreateDirectCmd struct {
	DisplayName     string `name:"display-name" help:"Space display name" required:""`
	Type            string `name:"type" help:"Space type (SPACE or GROUP_CHAT)" default:"SPACE" enum:"SPACE,GROUP_CHAT"`
	Description     string `name:"description" help:"Space description"`
	ExternalAllowed bool   `name:"external-allowed" help:"Allow external users"`
	Threading       string `name:"threading" help:"Threading state (THREADED_MESSAGES or UNTHREADED_MESSAGES)"`
}

func (c *ChatSpacesCreateDirectCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	displayName := strings.TrimSpace(c.DisplayName)
	if displayName == "" {
		return usage("required: --display-name")
	}

	if c.Threading != "" && c.Threading != "THREADED_MESSAGES" && c.Threading != "UNTHREADED_MESSAGES" {
		return usage("--threading must be THREADED_MESSAGES or UNTHREADED_MESSAGES")
	}

	space := &chat.Space{
		SpaceType:           c.Type,
		DisplayName:         displayName,
		ExternalUserAllowed: c.ExternalAllowed,
		SpaceThreadingState: c.Threading,
	}
	if c.Description != "" {
		space.SpaceDetails = &chat.SpaceDetails{
			Description: c.Description,
		}
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Create(space).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": resp})
	}

	printSpaceDetails(u, resp)
	return nil
}

// ChatSpacesDeleteCmd deletes a space.
type ChatSpacesDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Space resource name (spaces/...)"`
}

func (c *ChatSpacesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	if !strings.HasPrefix(name, "spaces/") {
		name = "spaces/" + name
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete space %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.Spaces.Delete(name).Do()
	if err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, fmt.Sprintf("space %s", name))
}

// ChatSpacesFindDmCmd finds a direct message space with a user.
type ChatSpacesFindDmCmd struct {
	User string `name:"user" help:"User resource name or email" required:""`
}

func (c *ChatSpacesFindDmCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	user := strings.TrimSpace(c.User)
	if user == "" {
		return usage("required: --user")
	}
	user = normalizeUser(user)

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.FindDirectMessage().Name(user).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": resp})
	}

	printSpaceDetails(u, resp)
	return nil
}

// ChatSpacesGetCmd gets details about a space.
type ChatSpacesGetCmd struct {
	Name string `arg:"" name:"name" help:"Space resource name (spaces/...)"`
}

func (c *ChatSpacesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	if !strings.HasPrefix(name, "spaces/") {
		name = "spaces/" + name
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": resp})
	}

	printSpaceDetails(u, resp)
	return nil
}

// ChatSpacesPatchCmd patches a space (partial update).
type ChatSpacesPatchCmd struct {
	Name            string `arg:"" name:"name" help:"Space resource name (spaces/...)"`
	DisplayName     string `name:"display-name" help:"New display name"`
	Description     string `name:"description" help:"New description"`
	ExternalAllowed *bool  `name:"external-allowed" help:"Allow external users"`
}

func (c *ChatSpacesPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
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
	if !strings.HasPrefix(name, "spaces/") {
		name = "spaces/" + name
	}

	// Build update mask from provided flags
	var fields []string
	space := &chat.Space{}

	if flagProvided(kctx, "display-name") {
		fields = append(fields, "displayName")
		space.DisplayName = c.DisplayName
	}
	if flagProvided(kctx, "description") {
		fields = append(fields, "spaceDetails.description")
		if space.SpaceDetails == nil {
			space.SpaceDetails = &chat.SpaceDetails{}
		}
		space.SpaceDetails.Description = c.Description
	}
	if flagProvided(kctx, "external-allowed") {
		fields = append(fields, "externalUserAllowed")
		if c.ExternalAllowed != nil {
			space.ExternalUserAllowed = *c.ExternalAllowed
		}
	}

	if len(fields) == 0 {
		return usage("at least one field must be provided to update")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Spaces.Patch(name, space).UpdateMask(strings.Join(fields, ","))
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": resp})
	}

	printSpaceDetails(u, resp)
	return nil
}

// ChatSpacesSearchCmd searches for spaces.
type ChatSpacesSearchCmd struct {
	Query string `name:"query" help:"Search query (e.g., 'spaceType = \"SPACE\"')" required:""`
	Max   int64  `name:"max" aliases:"limit" help:"Max results (default: 100)" default:"100"`
	Page  string `name:"page" help:"Page token"`
}

func (c *ChatSpacesSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	query := strings.TrimSpace(c.Query)
	if query == "" {
		return usage("required: --query")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Spaces.Search().Query(query).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource    string `json:"resource"`
			Name        string `json:"name,omitempty"`
			SpaceType   string `json:"type,omitempty"`
			SpaceURI    string `json:"uri,omitempty"`
			ThreadState string `json:"threading,omitempty"`
		}
		items := make([]item, 0, len(resp.Spaces))
		for _, space := range resp.Spaces {
			if space == nil {
				continue
			}
			items = append(items, item{
				Resource:    space.Name,
				Name:        space.DisplayName,
				SpaceType:   chatSpaceType(space),
				SpaceURI:    space.SpaceUri,
				ThreadState: space.SpaceThreadingState,
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spaces":        items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Spaces) == 0 {
		u.Err().Println("No spaces found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tTYPE\tTHREADING")
	for _, space := range resp.Spaces {
		if space == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			space.Name,
			sanitizeTab(space.DisplayName),
			sanitizeTab(chatSpaceType(space)),
			sanitizeTab(space.SpaceThreadingState),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// printSpaceDetails prints space details in text mode.
func printSpaceDetails(u *ui.UI, space *chat.Space) {
	if space == nil {
		return
	}
	if space.Name != "" {
		u.Out().Printf("resource\t%s", space.Name)
	}
	if space.DisplayName != "" {
		u.Out().Printf("name\t%s", space.DisplayName)
	}
	st := chatSpaceType(space)
	if st != "" {
		u.Out().Printf("type\t%s", st)
	}
	if space.SpaceThreadingState != "" {
		u.Out().Printf("threading\t%s", space.SpaceThreadingState)
	}
	if space.SpaceUri != "" {
		u.Out().Printf("uri\t%s", space.SpaceUri)
	}
	if space.SpaceDetails != nil && space.SpaceDetails.Description != "" {
		u.Out().Printf("description\t%s", space.SpaceDetails.Description)
	}
}
