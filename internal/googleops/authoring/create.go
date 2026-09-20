package authoring

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf16"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	native "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func client(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, name string) (*http.Client, error) {
	if provider == nil {
		return nil, invalid("client provider is required")
	}

	c, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: name, Retry: mcpcontract.NonReplayableWrite})
	if err != nil {
		return nil, native.NativePublicError(err)
	}

	if c == nil {
		return nil, invalid("client provider returned no client")
	}

	return c, nil
}

func createDocument(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in DocumentInput) (mcpcontract.Result[Created], error) {
	if err := requireBudget(ctx, 2); err != nil {
		return mcpcontract.Result[Created]{}, err
	}

	c, err := client(ctx, provider, id, "docs_create_document")
	if err != nil {
		return mcpcontract.Result[Created]{}, err
	}

	api, err := docs.NewService(ctx, option.WithHTTPClient(c))
	if err != nil {
		return mcpcontract.Result[Created]{}, native.NativePublicError(err)
	}

	doc, err := api.Documents.Create(&docs.Document{Title: in.Title}).Fields("documentId,title").Context(ctx).Do()
	if err != nil {
		return mcpcontract.Result[Created]{}, native.NativeWritePublicError(err)
	}

	if doc.DocumentId == "" {
		return mcpcontract.Result[Created]{}, missingCreatedID()
	}
	result := Created{ID: doc.DocumentId, URL: "https://docs.google.com/document/d/" + url.PathEscape(doc.DocumentId) + "/edit", Title: doc.Title}

	var text strings.Builder
	for _, paragraph := range in.Paragraphs {
		text.WriteString(paragraph.Text)
		text.WriteByte('\n')
	}
	requests := []*docs.Request{{InsertText: &docs.InsertTextRequest{Location: &docs.Location{Index: 1}, Text: text.String()}}}

	index := int64(1)
	for _, paragraph := range in.Paragraphs {
		end := index + int64(len(utf16.Encode([]rune(paragraph.Text+"\n"))))

		style := paragraph.Style
		if style == "" {
			style = "NORMAL_TEXT"
		}
		requests = append(requests, &docs.Request{UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{Range: &docs.Range{StartIndex: index, EndIndex: end}, ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: style}, Fields: "namedStyleType"}})
		index = end
	}

	_, err = api.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{Requests: requests}).Context(ctx).Do()
	if err != nil {
		return partial(id, result, err), nil
	}
	result.Populated = true

	return mcpcontract.NewResult(id, result), nil
}

func createPresentation(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in PresentationInput) (mcpcontract.Result[Created], error) {
	if err := requireBudget(ctx, 2); err != nil {
		return mcpcontract.Result[Created]{}, err
	}

	c, err := client(ctx, provider, id, "slides_create_presentation")
	if err != nil {
		return mcpcontract.Result[Created]{}, err
	}

	api, err := slides.NewService(ctx, option.WithHTTPClient(c))
	if err != nil {
		return mcpcontract.Result[Created]{}, native.NativePublicError(err)
	}

	presentation, err := api.Presentations.Create(&slides.Presentation{Title: in.Title}).Fields("presentationId,title,slides.objectId").Context(ctx).Do()
	if err != nil {
		return mcpcontract.Result[Created]{}, native.NativeWritePublicError(err)
	}

	if presentation.PresentationId == "" {
		return mcpcontract.Result[Created]{}, missingCreatedID()
	}
	result := Created{ID: presentation.PresentationId, URL: "https://docs.google.com/presentation/d/" + url.PathEscape(presentation.PresentationId) + "/edit", Title: presentation.Title}
	var requests []*slides.Request
	// Remove any provider-created starter slides so the result contains exactly
	// the requested slides. An empty initial presentation needs no deletion.
	for _, initial := range presentation.Slides {
		if initial == nil || initial.ObjectId == "" {
			return partial(id, result, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "created presentation returned a slide without an ID; reconcile before populating"}), nil
		}
		requests = append(requests, &slides.Request{DeleteObject: &slides.DeleteObjectRequest{ObjectId: initial.ObjectId}})
	}

	for index, slide := range in.Slides {
		slideID := fmt.Sprintf("slide_%03d", index+1)
		requests = append(requests, &slides.Request{CreateSlide: &slides.CreateSlideRequest{ObjectId: slideID, SlideLayoutReference: &slides.LayoutReference{PredefinedLayout: "BLANK"}}})

		requests = append(requests, textBox(slideID, fmt.Sprintf("title_%03d", index+1), slide.Title, 32, 30, 70, 30, true)...)
		if len(slide.Body) > 0 {
			requests = append(requests, textBox(slideID, fmt.Sprintf("body_%03d", index+1), strings.Join(slide.Body, "\n"), 32, 110, 250, 18, false)...)
		}
	}

	_, err = api.Presentations.BatchUpdate(presentation.PresentationId, &slides.BatchUpdatePresentationRequest{Requests: requests}).Context(ctx).Do()
	if err != nil {
		return partial(id, result, err), nil
	}
	result.Populated = true
	result.NextAction = "Review the slide layout before sharing; rendered text fit has not been checked."

	return mcpcontract.NewResult(id, result), nil
}

func textBox(slideID, objectID, text string, x, y, height, fontSize float64, bold bool) []*slides.Request {
	return []*slides.Request{
		{CreateShape: &slides.CreateShapeRequest{ObjectId: objectID, ShapeType: "TEXT_BOX", ElementProperties: &slides.PageElementProperties{PageObjectId: slideID, Size: &slides.Size{Width: &slides.Dimension{Magnitude: 650, Unit: "PT"}, Height: &slides.Dimension{Magnitude: height, Unit: "PT"}}, Transform: &slides.AffineTransform{ScaleX: 1, ScaleY: 1, TranslateX: x, TranslateY: y, Unit: "PT"}}}},
		{InsertText: &slides.InsertTextRequest{ObjectId: objectID, Text: text}},
		{UpdateTextStyle: &slides.UpdateTextStyleRequest{ObjectId: objectID, TextRange: &slides.Range{Type: "ALL"}, Style: &slides.TextStyle{FontFamily: "Arial", FontSize: &slides.Dimension{Magnitude: fontSize, Unit: "PT"}, Bold: bold, ForceSendFields: []string{"Bold"}}, Fields: "fontFamily,fontSize,bold"}},
	}
}

func createSpreadsheet(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in SpreadsheetInput) (mcpcontract.Result[Created], error) {
	if err := requireBudget(ctx, 1); err != nil {
		return mcpcontract.Result[Created]{}, err
	}

	c, err := client(ctx, provider, id, "sheets_create_spreadsheet")
	if err != nil {
		return mcpcontract.Result[Created]{}, err
	}

	api, err := sheets.NewService(ctx, option.WithHTTPClient(c))
	if err != nil {
		return mcpcontract.Result[Created]{}, native.NativePublicError(err)
	}
	var rows []*sheets.RowData

	columns := 1
	for rowIndex, row := range in.Rows {
		columns = max(columns, len(row))
		data := &sheets.RowData{}

		for _, cell := range row {
			value := &sheets.CellData{}

			switch {
			case cell.Text != nil:
				value.UserEnteredValue = &sheets.ExtendedValue{StringValue: cell.Text}
			case cell.Number != nil:
				value.UserEnteredValue = &sheets.ExtendedValue{NumberValue: cell.Number}
			case cell.Boolean != nil:
				value.UserEnteredValue = &sheets.ExtendedValue{BoolValue: cell.Boolean}
			case cell.Formula != nil:
				value.UserEnteredValue = &sheets.ExtendedValue{FormulaValue: cell.Formula}
			}

			if in.Header && rowIndex == 0 {
				value.UserEnteredFormat = &sheets.CellFormat{TextFormat: &sheets.TextFormat{Bold: true}}
			}

			data.Values = append(data.Values, value)
		}

		rows = append(rows, data)
	}

	grid := &sheets.GridProperties{RowCount: int64(len(rows)), ColumnCount: int64(columns)}
	if in.Header {
		grid.FrozenRowCount = 1
		grid.RowCount = max(grid.RowCount, 2)
	}

	created, err := api.Spreadsheets.Create(&sheets.Spreadsheet{Properties: &sheets.SpreadsheetProperties{Title: in.Title}, Sheets: []*sheets.Sheet{{Properties: &sheets.SheetProperties{Title: in.SheetTitle, GridProperties: grid}, Data: []*sheets.GridData{{RowData: rows}}}}}).Fields("spreadsheetId,spreadsheetUrl,properties.title").Context(ctx).Do()
	if err != nil {
		return mcpcontract.Result[Created]{}, native.NativeWritePublicError(err)
	}

	if created.SpreadsheetId == "" {
		return mcpcontract.Result[Created]{}, missingCreatedID()
	}

	return mcpcontract.NewResult(id, Created{ID: created.SpreadsheetId, URL: "https://docs.google.com/spreadsheets/d/" + url.PathEscape(created.SpreadsheetId) + "/edit", Title: in.Title, Populated: true}), nil
}

func missingCreatedID() error {
	return &mcpcontract.Error{Category: mcpcontract.OutcomeUnknown, Message: "Google returned no created resource ID; reconcile before repeating creation", Retryable: false}
}

func requireBudget(ctx context.Context, calls int64) error {
	if remaining := native.UpstreamBudgetRemaining(ctx); remaining >= 0 && remaining < calls {
		return &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "insufficient remaining API budget to complete creation; no resource was created", Retryable: false}
	}

	return nil
}
