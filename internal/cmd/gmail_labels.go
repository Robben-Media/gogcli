package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailLabelsCmd struct {
	List   GmailLabelsListCmd   `cmd:"" name:"list" help:"List labels"`
	Get    GmailLabelsGetCmd    `cmd:"" name:"get" help:"Get label details (including counts)"`
	Create GmailLabelsCreateCmd `cmd:"" name:"create" help:"Create a new label"`
	Delete GmailLabelsDeleteCmd `cmd:"" name:"delete" help:"Delete a label"`
	Patch  GmailLabelsPatchCmd  `cmd:"" name:"patch" help:"Patch a label (partial update)"`
	Update GmailLabelsUpdateCmd `cmd:"" name:"update" help:"Update a label (full replace)"`
	Modify GmailLabelsModifyCmd `cmd:"" name:"modify" help:"Modify labels on threads"`
}

type GmailLabelsGetCmd struct {
	Label string `arg:"" name:"labelIdOrName" help:"Label ID or name"`
}

func (c *GmailLabelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(c.Label)
	if raw == "" {
		return usage("empty label")
	}
	id := raw
	if v, ok := idMap[strings.ToLower(raw)]; ok {
		id = v
	}

	l, err := svc.Users.Labels.Get("me", id).Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"label": l})
	}
	u := ui.FromContext(ctx)
	u.Out().Printf("id\t%s", l.Id)
	u.Out().Printf("name\t%s", l.Name)
	u.Out().Printf("type\t%s", l.Type)
	u.Out().Printf("messages_total\t%d", l.MessagesTotal)
	u.Out().Printf("messages_unread\t%d", l.MessagesUnread)
	u.Out().Printf("threads_total\t%d", l.ThreadsTotal)
	u.Out().Printf("threads_unread\t%d", l.ThreadsUnread)
	return nil
}

type GmailLabelsCreateCmd struct {
	Name string `arg:"" help:"Label name"`
}

func (c *GmailLabelsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("label name is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = ensureLabelNameAvailable(svc, name)
	if err != nil {
		return err
	}

	label, err := createLabel(ctx, svc, name)
	if err != nil {
		return mapLabelCreateError(err, name)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"label": label})
	}
	u.Out().Printf("Created label: %s (id: %s)", label.Name, label.Id)
	return nil
}

func createLabel(ctx context.Context, svc *gmail.Service, name string) (*gmail.Label, error) {
	return svc.Users.Labels.Create("me", &gmail.Label{
		Name:                  name,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}).Context(ctx).Do()
}

type GmailLabelsListCmd struct{}

func (c *GmailLabelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"labels": resp.Labels})
	}
	if len(resp.Labels) == 0 {
		u.Err().Println("No labels")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tTYPE")
	for _, l := range resp.Labels {
		fmt.Fprintf(w, "%s\t%s\t%s\n", l.Id, l.Name, l.Type)
	}
	return nil
}

type GmailLabelsModifyCmd struct {
	ThreadIDs []string `arg:"" name:"threadId" help:"Thread IDs"`
	Add       string   `name:"add" help:"Labels to add (comma-separated, name or ID)"`
	Remove    string   `name:"remove" help:"Labels to remove (comma-separated, name or ID)"`
}

func (c *GmailLabelsModifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	threadIDs := c.ThreadIDs
	addLabels := splitCSV(c.Add)
	removeLabels := splitCSV(c.Remove)
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return usage("must specify --add and/or --remove")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}

	addIDs := resolveLabelIDs(addLabels, idMap)
	removeIDs := resolveLabelIDs(removeLabels, idMap)

	type result struct {
		ThreadID string `json:"threadId"`
		Success  bool   `json:"success"`
		Error    string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(threadIDs))

	for _, tid := range threadIDs {
		_, err := svc.Users.Threads.Modify("me", tid, &gmail.ModifyThreadRequest{
			AddLabelIds:    addIDs,
			RemoveLabelIds: removeIDs,
		}).Context(ctx).Do()
		if err != nil {
			results = append(results, result{ThreadID: tid, Success: false, Error: err.Error()})
			if !outfmt.IsJSON(ctx) {
				u.Err().Errorf("%s: %s", tid, err.Error())
			}
			continue
		}
		results = append(results, result{ThreadID: tid, Success: true})
		if !outfmt.IsJSON(ctx) {
			u.Out().Printf("%s\tok", tid)
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"results": results})
	}
	return nil
}

func fetchLabelNameToID(svc *gmail.Service) (map[string]string, error) {
	resp, err := svc.Users.Labels.List("me").Do()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(resp.Labels))
	for _, l := range resp.Labels {
		if l.Id == "" {
			continue
		}
		m[strings.ToLower(l.Id)] = l.Id
		if l.Name != "" {
			m[strings.ToLower(l.Name)] = l.Id
		}
	}
	return m, nil
}

type GmailLabelsDeleteCmd struct {
	Label string `arg:"" name:"labelIdOrName" help:"Label ID or name to delete"`
}

func (c *GmailLabelsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(c.Label)
	if raw == "" {
		return usage("empty label")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Resolve name to ID if needed
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}
	id := raw
	if v, ok := idMap[strings.ToLower(raw)]; ok {
		id = v
	}

	if err := svc.Users.Labels.Delete("me", id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"id":      id,
			"deleted": true,
		})
	}

	u.Err().Printf("Label %q deleted", raw)
	return nil
}

// GmailLabelsPatchCmd performs a partial update of a label.
// Only fields explicitly provided via flags are updated.
type GmailLabelsPatchCmd struct {
	Label                 string `arg:"" name:"labelIdOrName" help:"Label ID or name to patch"`
	Name                  string `name:"name" help:"New label name"`
	LabelListVisibility   string `name:"label-list-visibility" help:"Visibility in label list (labelShow, labelHide, labelShowIfUnread)"`
	MessageListVisibility string `name:"message-list-visibility" help:"Visibility in message list (show, hide)"`
}

func (c *GmailLabelsPatchCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(c.Label)
	if raw == "" {
		return usage("empty label")
	}

	// Build label with only provided fields - check this BEFORE making any API calls
	label := &gmail.Label{}
	hasUpdates := false

	if flagProvided(kctx, "name") {
		label.Name = strings.TrimSpace(c.Name)
		hasUpdates = true
	}
	if flagProvided(kctx, "label-list-visibility") {
		// Validate visibility values
		validLabelVis := map[string]bool{"labelShow": true, "labelHide": true, "labelShowIfUnread": true}
		if !validLabelVis[c.LabelListVisibility] {
			return usage("invalid --label-list-visibility: must be labelShow, labelHide, or labelShowIfUnread")
		}
		label.LabelListVisibility = c.LabelListVisibility
		hasUpdates = true
	}
	if flagProvided(kctx, "message-list-visibility") {
		// Validate visibility values
		validMsgVis := map[string]bool{"show": true, "hide": true}
		if !validMsgVis[c.MessageListVisibility] {
			return usage("invalid --message-list-visibility: must be show or hide")
		}
		label.MessageListVisibility = c.MessageListVisibility
		hasUpdates = true
	}

	if !hasUpdates {
		return usage("no updates provided; use --name, --label-list-visibility, or --message-list-visibility")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Resolve name to ID if needed
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}
	id := raw
	if v, ok := idMap[strings.ToLower(raw)]; ok {
		id = v
	}

	updated, err := svc.Users.Labels.Patch("me", id, label).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("patch label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"label": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	if updated.LabelListVisibility != "" {
		u.Out().Printf("label_list_visibility\t%s", updated.LabelListVisibility)
	}
	if updated.MessageListVisibility != "" {
		u.Out().Printf("message_list_visibility\t%s", updated.MessageListVisibility)
	}
	return nil
}

// GmailLabelsUpdateCmd performs a full replacement of a label.
// All fields are updated to the provided values (or cleared if not provided).
type GmailLabelsUpdateCmd struct {
	Label                 string `arg:"" name:"labelIdOrName" help:"Label ID or name to update"`
	Name                  string `name:"name" required:"" help:"Label name"`
	LabelListVisibility   string `name:"label-list-visibility" help:"Visibility in label list (labelShow, labelHide, labelShowIfUnread)" enum:"labelShow,labelHide,labelShowIfUnread" default:"labelShow"`
	MessageListVisibility string `name:"message-list-visibility" help:"Visibility in message list (show, hide)" enum:"show,hide" default:"show"`
}

func (c *GmailLabelsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(c.Label)
	if raw == "" {
		return usage("empty label")
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("label name is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Resolve name to ID if needed
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}
	id := raw
	if v, ok := idMap[strings.ToLower(raw)]; ok {
		id = v
	}

	label := &gmail.Label{
		Name:                  name,
		LabelListVisibility:   c.LabelListVisibility,
		MessageListVisibility: c.MessageListVisibility,
	}

	updated, err := svc.Users.Labels.Update("me", id, label).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"label": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	if updated.LabelListVisibility != "" {
		u.Out().Printf("label_list_visibility\t%s", updated.LabelListVisibility)
	}
	if updated.MessageListVisibility != "" {
		u.Out().Printf("message_list_visibility\t%s", updated.MessageListVisibility)
	}
	return nil
}

func fetchLabelIDToName(svc *gmail.Service) (map[string]string, error) {
	resp, err := svc.Users.Labels.List("me").Do()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(resp.Labels))
	for _, l := range resp.Labels {
		if l.Id == "" {
			continue
		}
		if l.Name != "" {
			m[l.Id] = l.Name
		} else {
			m[l.Id] = l.Id
		}
	}
	return m, nil
}
