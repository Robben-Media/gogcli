package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	keepapi "google.golang.org/api/keep/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// KeepNotesCreateCmd creates a new Google Keep note.
// Supports text notes (--body or --body-from-file) or list notes (--list-items).
type KeepNotesCreateCmd struct {
	Title        string   `name:"title" help:"Note title (optional)"`
	Body         string   `name:"body" help:"Text body content (mutually exclusive with --body-from-file and --list-items)"`
	BodyFromFile string   `name:"body-from-file" help:"Read body from file path (mutually exclusive with --body and --list-items)"`
	ListItems    []string `name:"list-items" help:"Create a list note with these items (mutually exclusive with --body and --body-from-file)"`
	Checked      []int    `name:"checked" help:"Indices of list items to mark as checked (0-based, use with --list-items)"`
}

func (c *KeepNotesCreateCmd) Run(ctx context.Context, flags *RootFlags, keep *KeepCmd) error {
	u := ui.FromContext(ctx)

	// Validate mutual exclusivity
	contentFlags := 0
	if c.Body != "" {
		contentFlags++
	}
	if c.BodyFromFile != "" {
		contentFlags++
	}
	if len(c.ListItems) > 0 {
		contentFlags++
	}
	if contentFlags == 0 {
		return usage("at least one of --body, --body-from-file, or --list-items is required")
	}
	if contentFlags > 1 {
		return usage("--body, --body-from-file, and --list-items are mutually exclusive")
	}

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	note := &keepapi.Note{
		Title: strings.TrimSpace(c.Title),
	}

	// Build content based on which flag was provided
	if len(c.ListItems) > 0 {
		// Create list note
		checkedSet := make(map[int]bool)
		for _, idx := range c.Checked {
			checkedSet[idx] = true
		}

		listItems := make([]*keepapi.ListItem, len(c.ListItems))
		for i, item := range c.ListItems {
			listItems[i] = &keepapi.ListItem{
				Text:    &keepapi.TextContent{Text: item},
				Checked: checkedSet[i],
			}
		}
		note.Body = &keepapi.Section{
			List: &keepapi.ListContent{ListItems: listItems},
		}
	} else {
		// Create text note
		var bodyText string
		if c.BodyFromFile != "" {
			data, readErr := os.ReadFile(c.BodyFromFile)
			if readErr != nil {
				return fmt.Errorf("read body file: %w", readErr)
			}
			bodyText = string(data)
		} else {
			bodyText = c.Body
		}

		note.Body = &keepapi.Section{
			Text: &keepapi.TextContent{Text: bodyText},
		}
	}

	created, err := svc.Notes.Create(note).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create note: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"note": created})
	}

	// Text output
	noteType := "text"
	if created.Body != nil && created.Body.List != nil {
		noteType = "list"
	}

	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("title\t%s", created.Title)
	u.Out().Printf("type\t%s", noteType)
	u.Out().Printf("created\t%s", created.CreateTime)
	return nil
}

// KeepNotesDeleteCmd deletes a Google Keep note.
type KeepNotesDeleteCmd struct {
	NoteID string `arg:"" name:"noteId" help:"Note ID or name (e.g. notes/abc123)"`
}

func (c *KeepNotesDeleteCmd) Run(ctx context.Context, flags *RootFlags, keep *KeepCmd) error {
	u := ui.FromContext(ctx)

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	name := c.NoteID
	if !strings.HasPrefix(name, "notes/") {
		name = "notes/" + name
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete note %s (moves to trash, permanently deleted after ~30 days)", name)); err != nil {
		return err
	}

	if _, err := svc.Notes.Delete(name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete note: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"name":    name,
		})
	}

	u.Err().Printf("Deleted note %s", name)
	return nil
}
