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

// KeepNotesCreateCmd creates a new Google Keep note (text or list).
type KeepNotesCreateCmd struct {
	Title        string   `name:"title" help:"Note title (optional)"`
	Body         string   `name:"body" help:"Text body content for a text note (mutually exclusive with --list-items)"`
	BodyFromFile string   `name:"body-from-file" help:"Read body from file path (mutually exclusive with --body and --list-items)"`
	ListItems    []string `name:"list-items" help:"Create a list note with these items (mutually exclusive with --body)"`
	Checked      []int    `name:"checked" help:"Indices of list items to mark as checked (0-based)"`
}

func (c *KeepNotesCreateCmd) Run(ctx context.Context, flags *RootFlags, keep *KeepCmd) error {
	u := ui.FromContext(ctx)

	// Determine content source - body, body-from-file, or list-items
	body := strings.TrimSpace(c.Body)
	bodyFromFile := strings.TrimSpace(c.BodyFromFile)
	hasListItems := len(c.ListItems) > 0

	contentCount := 0
	if body != "" {
		contentCount++
	}
	if bodyFromFile != "" {
		contentCount++
	}
	if hasListItems {
		contentCount++
	}

	if contentCount == 0 {
		return usage("must provide --body, --body-from-file, or --list-items")
	}
	if contentCount > 1 {
		return usage("--body, --body-from-file, and --list-items are mutually exclusive")
	}

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	note := &keepapi.Note{
		Title: strings.TrimSpace(c.Title),
	}

	if hasListItems {
		// Create list note
		listContent := &keepapi.ListContent{}
		for i, item := range c.ListItems {
			checked := false
			for _, ci := range c.Checked {
				if ci == i {
					checked = true
					break
				}
			}
			listContent.ListItems = append(listContent.ListItems, &keepapi.ListItem{
				Text:    &keepapi.TextContent{Text: item},
				Checked: checked,
			})
		}
		note.Body = &keepapi.Section{List: listContent}
	} else {
		// Create text note
		textBody := body
		if bodyFromFile != "" {
			data, readErr := os.ReadFile(bodyFromFile) //nolint:gosec // user-provided file path
			if readErr != nil {
				return fmt.Errorf("read body file: %w", readErr)
			}
			textBody = string(data)
		}
		note.Body = &keepapi.Section{Text: &keepapi.TextContent{Text: textBody}}
	}

	created, err := svc.Notes.Create(note).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"note": created})
	}

	noteType := "text"
	if created.Body != nil && created.Body.List != nil {
		noteType = "list"
	}
	u.Out().Printf("name\t%s", created.Name)
	if created.Title != "" {
		u.Out().Printf("title\t%s", created.Title)
	}
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

	name := strings.TrimSpace(c.NoteID)
	if name == "" {
		return usage("empty noteId")
	}
	if !strings.HasPrefix(name, "notes/") {
		name = "notes/" + name
	}

	if confErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete note %s (moves to trash, permanently deleted after ~30 days)", name)); confErr != nil {
		return confErr
	}

	svc, err := getKeepService(ctx, flags, keep)
	if err != nil {
		return err
	}

	if _, delErr := svc.Notes.Delete(name).Context(ctx).Do(); delErr != nil {
		return delErr
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
