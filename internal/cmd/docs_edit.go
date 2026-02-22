package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DocsDeleteRangeCmd deletes content in a range within a Google Doc.
type DocsDeleteRangeCmd struct {
	DocID string `arg:"" name:"docId" help:"Document ID"`
	Start int64  `name:"start" required:"" help:"Start index"`
	End   int64  `name:"end" required:"" help:"End index"`
}

func (c *DocsDeleteRangeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if c.Start >= c.End {
		return usage("start must be less than end")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{
					StartIndex: c.Start,
					EndIndex:   c.End,
				},
			},
		}},
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"replies":    resp.Replies,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("deleted\t%d-%d", c.Start, c.End)
	return nil
}

// DocsFormatCmd formats text in a range within a Google Doc.
type DocsFormatCmd struct {
	DocID     string `arg:"" name:"docId" help:"Document ID"`
	Start     int64  `name:"start" required:"" help:"Start index"`
	End       int64  `name:"end" required:"" help:"End index"`
	Bold      bool   `name:"bold" help:"Set bold"`
	Italic    bool   `name:"italic" help:"Set italic"`
	Underline bool   `name:"underline" help:"Set underline"`
	FontSize  int64  `name:"font-size" help:"Font size in points"`
	Heading   int64  `name:"heading" help:"Heading level (1-6, 0 for normal)"`
}

func (c *DocsFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if c.Start >= c.End {
		return usage("start must be less than end")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	var reqs []*docs.Request

	// Build text style request if any text style flags are set.
	var fields []string
	style := &docs.TextStyle{}

	if c.Bold {
		style.Bold = true
		fields = append(fields, "bold")
	}
	if c.Italic {
		style.Italic = true
		fields = append(fields, "italic")
	}
	if c.Underline {
		style.Underline = true
		fields = append(fields, "underline")
	}
	if c.FontSize > 0 {
		style.FontSize = &docs.Dimension{
			Magnitude: float64(c.FontSize),
			Unit:      "PT",
		}
		fields = append(fields, "fontSize")
	}

	if len(fields) > 0 {
		reqs = append(reqs, &docs.Request{
			UpdateTextStyle: &docs.UpdateTextStyleRequest{
				Range: &docs.Range{
					StartIndex: c.Start,
					EndIndex:   c.End,
				},
				TextStyle: style,
				Fields:    strings.Join(fields, ","),
			},
		})
	}

	// Build paragraph style request if heading is set.
	if c.Heading >= 0 && c.Heading <= 6 {
		headingStyle := "NORMAL_TEXT"
		if c.Heading >= 1 && c.Heading <= 6 {
			headingStyle = fmt.Sprintf("HEADING_%d", c.Heading)
		}
		// Only add the heading request if --heading was explicitly provided.
		// Since default is 0 and 0 maps to NORMAL_TEXT, we check if heading
		// is non-zero OR if there are no text style fields (meaning heading
		// was the only thing set).
		if c.Heading > 0 {
			reqs = append(reqs, &docs.Request{
				UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
					Range: &docs.Range{
						StartIndex: c.Start,
						EndIndex:   c.End,
					},
					ParagraphStyle: &docs.ParagraphStyle{
						NamedStyleType: headingStyle,
					},
					Fields: "namedStyleType",
				},
			})
		}
	}

	if len(reqs) == 0 {
		return usage("no formatting options specified (use --bold, --italic, --underline, --font-size, or --heading)")
	}

	resp, err := svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
		Requests: reqs,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"replies":    resp.Replies,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("formatted\t%d-%d", c.Start, c.End)
	u.Out().Printf("requests\t%d", len(reqs))
	return nil
}

// DocsInsertTableCmd inserts a table into a Google Doc.
type DocsInsertTableCmd struct {
	DocID string `arg:"" name:"docId" help:"Document ID"`
	Index int64  `name:"index" required:"" help:"Insert index"`
	Rows  int64  `name:"rows" required:"" help:"Number of rows"`
	Cols  int64  `name:"cols" required:"" help:"Number of columns"`
}

func (c *DocsInsertTableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if c.Rows <= 0 {
		return usage("rows must be positive")
	}
	if c.Cols <= 0 {
		return usage("cols must be positive")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			InsertTable: &docs.InsertTableRequest{
				Rows:    c.Rows,
				Columns: c.Cols,
				Location: &docs.Location{
					Index: c.Index,
				},
			},
		}},
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"replies":    resp.Replies,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("table\t%dx%d", c.Rows, c.Cols)
	u.Out().Printf("index\t%d", c.Index)
	return nil
}

// DocsInsertImageCmd inserts an inline image into a Google Doc.
type DocsInsertImageCmd struct {
	DocID string `arg:"" name:"docId" help:"Document ID"`
	Index int64  `name:"index" required:"" help:"Insert index"`
	URI   string `name:"uri" required:"" help:"Image URI"`
}

func (c *DocsInsertImageCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	uri := strings.TrimSpace(c.URI)
	if uri == "" {
		return usage("empty uri")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			InsertInlineImage: &docs.InsertInlineImageRequest{
				Uri: uri,
				Location: &docs.Location{
					Index: c.Index,
				},
			},
		}},
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"replies":    resp.Replies,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("image\t%s", uri)
	u.Out().Printf("index\t%d", c.Index)
	return nil
}

// DocsBulletsCmd creates paragraph bullets in a Google Doc.
type DocsBulletsCmd struct {
	DocID  string `arg:"" name:"docId" help:"Document ID"`
	Start  int64  `name:"start" required:"" help:"Start index"`
	End    int64  `name:"end" required:"" help:"End index"`
	Preset string `name:"preset" help:"Bullet preset: BULLET_DISC_CIRCLE_SQUARE, NUMBERED_DECIMAL_ALPHA_ROMAN, BULLET_CHECKBOX" default:"BULLET_DISC_CIRCLE_SQUARE"`
}

func (c *DocsBulletsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if c.Start >= c.End {
		return usage("start must be less than end")
	}

	preset := strings.TrimSpace(c.Preset)
	if preset == "" {
		preset = "BULLET_DISC_CIRCLE_SQUARE"
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
				Range: &docs.Range{
					StartIndex: c.Start,
					EndIndex:   c.End,
				},
				BulletPreset: preset,
			},
		}},
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"replies":    resp.Replies,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("bullets\t%d-%d", c.Start, c.End)
	u.Out().Printf("preset\t%s", preset)
	return nil
}
