// Package authoring implements bounded structured document creation workflows.
package authoring

import (
	"context"
	"math"
	"strings"

	native "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type Created struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Populated  bool   `json:"populated"`
	NextAction string `json:"next_action,omitempty"`
}

type Paragraph struct {
	Text  string `json:"text"`
	Style string `json:"style,omitempty" jsonschema:"NORMAL_TEXT (default), TITLE, SUBTITLE, or HEADING_1 through HEADING_6"`
}

type DocumentInput struct {
	mcpcontract.Selection
	Title      string      `json:"title"`
	Paragraphs []Paragraph `json:"paragraphs" jsonschema:"1-100 paragraphs, at most 100000 total text bytes; formatting is explicit, not Markdown"`
}

type Slide struct {
	Title string   `json:"title"`
	Body  []string `json:"body" jsonschema:"Up to 20 body paragraphs; no remote images are fetched"`
}

type PresentationInput struct {
	mcpcontract.Selection
	Title  string  `json:"title"`
	Slides []Slide `json:"slides" jsonschema:"1-30 slides with title and body text; one batched layout/text update"`
}

type Cell struct {
	Text    *string  `json:"text,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
	Formula *string  `json:"formula,omitempty" jsonschema:"Explicit formula beginning with =; text cells never become formulas implicitly"`
}

type SpreadsheetInput struct {
	mcpcontract.Selection
	Title      string   `json:"title"`
	SheetTitle string   `json:"sheet_title"`
	Header     bool     `json:"header,omitempty" jsonschema:"Bold and freeze the first row"`
	Rows       [][]Cell `json:"rows" jsonschema:"1-500 rows, at most 50 columns and 10000 cells; empty cell objects mean blank"`
}

func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("docs_create_document", validateDocument, func(ctx context.Context, id mcpcontract.Identity, in DocumentInput) (mcpcontract.Result[Created], error) {
			return createDocument(ctx, provider, id, in)
		}),
		mcpcontract.NewOperation("slides_create_presentation", validatePresentation, func(ctx context.Context, id mcpcontract.Identity, in PresentationInput) (mcpcontract.Result[Created], error) {
			return createPresentation(ctx, provider, id, in)
		}),
		mcpcontract.NewOperation("sheets_create_spreadsheet", validateSpreadsheet, func(ctx context.Context, id mcpcontract.Identity, in SpreadsheetInput) (mcpcontract.Result[Created], error) {
			return createSpreadsheet(ctx, provider, id, in)
		}),
	}
}

func validTitle(title string) bool {
	return strings.TrimSpace(title) != "" && len(title) <= 512 && !strings.ContainsAny(title, "\x00\r\n")
}

func validateDocument(in DocumentInput) error {
	if !validTitle(in.Title) || len(in.Paragraphs) < 1 || len(in.Paragraphs) > 100 {
		return invalid("title and 1-100 paragraphs are required")
	}

	size := 0
	for _, p := range in.Paragraphs {
		size += len(p.Text)
		if strings.ContainsAny(p.Text, "\x00\r\n") {
			return invalid("each paragraph must contain a single text paragraph")
		}

		switch p.Style {
		case "", "NORMAL_TEXT", "TITLE", "SUBTITLE", "HEADING_1", "HEADING_2", "HEADING_3", "HEADING_4", "HEADING_5", "HEADING_6":
		default:
			return invalid("invalid paragraph style")
		}
	}

	if size > 100000 {
		return invalid("document text exceeds 100000 bytes")
	}

	return nil
}

func validatePresentation(in PresentationInput) error {
	if !validTitle(in.Title) || len(in.Slides) < 1 || len(in.Slides) > 30 {
		return invalid("title and 1-30 slides are required")
	}
	size := 0

	for _, slide := range in.Slides {
		if !validTitle(slide.Title) || len(slide.Body) > 20 {
			return invalid("each slide requires a title and at most 20 body paragraphs")
		}

		size += len(slide.Title)
		for _, line := range slide.Body {
			size += len(line)
			if strings.ContainsAny(line, "\x00\r\n") {
				return invalid("each slide body entry must contain a single text paragraph")
			}
		}
	}

	if size > 100000 {
		return invalid("presentation text exceeds 100000 bytes")
	}

	return nil
}

func validateSpreadsheet(in SpreadsheetInput) error {
	if !validTitle(in.Title) || !validTitle(in.SheetTitle) || len(in.SheetTitle) > 100 || strings.ContainsAny(in.SheetTitle, "[]:*?/\\") || len(in.Rows) < 1 || len(in.Rows) > 500 {
		return invalid("valid titles and 1-500 rows are required")
	}
	count, size := 0, 0

	for _, row := range in.Rows {
		if len(row) > 50 {
			return invalid("maximum 50 columns")
		}

		count += len(row)
		for _, cell := range row {
			kinds := 0
			if cell.Text != nil {
				kinds++
				size += len(*cell.Text)
			}

			if cell.Formula != nil {
				kinds++

				size += len(*cell.Formula)
				if !strings.HasPrefix(*cell.Formula, "=") {
					return invalid("explicit formulas must begin with =")
				}
			}

			if cell.Number != nil {
				kinds++

				if math.IsNaN(*cell.Number) || math.IsInf(*cell.Number, 0) {
					return invalid("cell numbers must be finite")
				}
			}

			if cell.Boolean != nil {
				kinds++
			}

			if kinds > 1 {
				return invalid("a cell must specify at most one value type")
			}
		}
	}

	if count > 10000 || size > 100000 {
		return invalid("spreadsheet exceeds 10000 cells or 100000 text bytes")
	}

	return nil
}

func partial(id mcpcontract.Identity, created Created, err error) mcpcontract.Result[Created] {
	result := mcpcontract.NewResult(id, created)
	safe := native.NativeWritePublicError(err)
	result.PartialFailures = []mcpcontract.Failure{{SourceID: created.ID, Category: string(safe.Category), Message: safe.Message}}
	result.Data.NextAction = "Read the created resource by ID and reconcile its content. Do not repeat the create operation."

	return result
}

func invalid(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: message}
}
