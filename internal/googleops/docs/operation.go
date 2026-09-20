// Package docs provides typed, bounded Google Docs MCP operations.
package docs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	defaultMaxBytes = 262144
	hardMaxBytes    = 1024 * 1024
	// Field masks cannot express arbitrary recursion through tabs, tables, and
	// tables of contents. Table and TOC branches therefore retain nested content,
	// while paragraphs project only text runs. This cap bounds that retained
	// nested structure.
	maxUpstreamBytes = 8 * hardMaxBytes
)

const documentFields = "documentId,title,revisionId," +
	"tabs.tabProperties.tabId,tabs.tabProperties.title,tabs.tabProperties.parentTabId," +
	"tabs.tabProperties.index,tabs.tabProperties.nestingLevel," +
	"tabs.documentTab.body.content.paragraph.elements.textRun.content," +
	"tabs.documentTab.body.content.table,tabs.documentTab.body.content.tableOfContents," +
	"tabs.childTabs"

var errResponseLimit = errors.New("response exceeds bounded read limit")

type GetTextRequest struct {
	mcpcontract.Selection
	DocumentID string `json:"document_id" jsonschema:"Google Docs document ID; required"`
	TabID      string `json:"tab_id,omitempty" jsonschema:"Optional document tab ID"`
	MaxBytes   int    `json:"max_bytes,omitempty" jsonschema:"Maximum extracted UTF-8 bytes; default 262144, maximum 1048576"`
}

type DocumentTab struct {
	ID           string `json:"id"`
	Title        string `json:"title,omitempty"`
	ParentID     string `json:"parent_id,omitempty"`
	Index        int64  `json:"index"`
	NestingLevel int64  `json:"nesting_level,omitempty"`
}

type GetTextData struct {
	DocumentID     string        `json:"document_id"`
	Title          string        `json:"title,omitempty"`
	RevisionID     string        `json:"revision_id,omitempty"`
	TabID          string        `json:"tab_id,omitempty"`
	URL            string        `json:"url,omitempty"`
	Text           string        `json:"text"`
	MaxBytes       int           `json:"max_bytes"`
	ParagraphCount int           `json:"paragraph_count"`
	TableCount     int           `json:"table_count"`
	Tabs           []DocumentTab `json:"tabs,omitempty"`
	truncated      bool
}

func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("docs_get_text", validateGetText, func(ctx context.Context, id mcpcontract.Identity, in GetTextRequest) (mcpcontract.Result[GetTextData], error) {
			data, err := getText(ctx, provider, id, in)
			if err != nil {
				return mcpcontract.Result[GetTextData]{}, err
			}
			out := mcpcontract.NewResult(id, data)
			out.Truncated = data.truncated

			return out, nil
		}),
	}
}

func validateGetText(in GetTextRequest) error {
	if strings.TrimSpace(in.DocumentID) == "" {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "document_id is required"}
	}

	if in.MaxBytes == 0 {
		return nil
	}

	if in.MaxBytes < 0 || in.MaxBytes > hardMaxBytes {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: fmt.Sprintf("max_bytes must be between 1 and %d", hardMaxBytes)}
	}

	return nil
}

func getText(ctx context.Context, provider mcpcontract.ClientProvider, identity mcpcontract.Identity, in GetTextRequest) (GetTextData, error) {
	maxBytes := in.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxBytes
	}
	documentID := strings.TrimSpace(in.DocumentID)
	tabID := strings.TrimSpace(in.TabID)

	client, err := provider.HTTPClient(ctx, identity, mcpcontract.CallOptions{Operation: "docs_get_text", Retry: mcpcontract.SafeRead})
	if err != nil {
		return GetTextData{}, nativegoogleapi.NativePublicError(err)
	}
	client = boundedClient(client, maxUpstreamBytes)

	svc, err := docs.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return GetTextData{}, nativegoogleapi.NativePublicError(err)
	}

	doc, err := svc.Documents.Get(documentID).
		IncludeTabsContent(true).
		Fields(googleapi.Field(documentFields)).
		Context(ctx).
		Do()
	if err != nil {
		return GetTextData{}, mapGoogleError("docs_get_text", err)
	}

	if doc == nil {
		return GetTextData{}, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "docs_get_text returned no document"}
	}

	var body *docs.Body

	if tabID != "" {
		tab := findTab(doc.Tabs, tabID)
		if tab == nil || tab.DocumentTab == nil {
			return GetTextData{}, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "tab_id was not found in the document"}
		}
		body = tab.DocumentTab.Body
	} else {
		if tab := firstDocumentTab(doc.Tabs); tab != nil {
			tabID = tab.TabProperties.TabId
			body = tab.DocumentTab.Body
		}
	}

	extracted := extractBody(body, maxBytes)
	data := GetTextData{
		DocumentID:     doc.DocumentId,
		Title:          doc.Title,
		RevisionID:     doc.RevisionId,
		TabID:          tabID,
		URL:            documentURL(doc.DocumentId, tabID),
		Text:           extracted.text,
		MaxBytes:       maxBytes,
		ParagraphCount: extracted.paragraphs,
		TableCount:     extracted.tables,
		Tabs:           projectTabs(doc.Tabs),
		truncated:      extracted.truncated,
	}

	return data, nil
}

type limitedTransport struct {
	base http.RoundTripper
	max  int64
}

func (t limitedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}

	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	response.Body = &limitedBody{ReadCloser: response.Body, remaining: t.max}

	return response, nil
}

type limitedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		probe := make([]byte, 1)

		n, err := b.ReadCloser.Read(probe)
		if n > 0 {
			return 0, errResponseLimit
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, io.EOF
			}

			return 0, fmt.Errorf("%w", err)
		}

		return 0, nil
	}

	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)

	if err != nil {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}

		return n, fmt.Errorf("%w", err)
	}

	return n, nil
}

func boundedClient(client *http.Client, maxBytes int64) *http.Client {
	out := *client
	out.Transport = limitedTransport{base: client.Transport, max: maxBytes}

	return &out
}

type extraction struct {
	text       string
	truncated  bool
	paragraphs int
	tables     int
}

type textWriter struct {
	builder   strings.Builder
	remaining int
	truncated bool
}

func (w *textWriter) add(text string) bool {
	if w.remaining <= 0 {
		if text != "" {
			w.truncated = true
		}

		return false
	}

	if len(text) <= w.remaining {
		w.builder.WriteString(text)
		w.remaining -= len(text)

		return true
	}

	cut := w.remaining
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}

	w.builder.WriteString(text[:cut])
	w.remaining = 0
	w.truncated = true

	return false
}

func extractBody(body *docs.Body, maxBytes int) extraction {
	var writer textWriter
	writer.remaining = maxBytes

	result := extraction{}
	if body == nil {
		return result
	}

	extractElements(body.Content, &writer, &result)
	result.text = writer.builder.String()
	result.truncated = writer.truncated

	return result
}

func extractElements(elements []*docs.StructuralElement, writer *textWriter, result *extraction) bool {
	for _, element := range elements {
		if element == nil {
			continue
		}

		switch {
		case element.Paragraph != nil:
			result.paragraphs++

			for _, item := range element.Paragraph.Elements {
				if item != nil && item.TextRun != nil {
					if !writer.add(item.TextRun.Content) {
						return false
					}
				}
			}
		case element.Table != nil:
			result.tables++
			if !extractTable(element.Table, writer, result) {
				return false
			}
		case element.TableOfContents != nil:
			if !extractElements(element.TableOfContents.Content, writer, result) {
				return false
			}
		}
	}

	return true
}

func extractTable(table *docs.Table, writer *textWriter, result *extraction) bool {
	for _, row := range table.TableRows {
		if row == nil {
			continue
		}

		for _, cell := range row.TableCells {
			if cell == nil {
				continue
			}

			if !extractElements(cell.Content, writer, result) {
				return false
			}
		}
	}

	return true
}

func firstDocumentTab(tabs []*docs.Tab) *docs.Tab {
	for _, tab := range tabs {
		if tab != nil && tab.DocumentTab != nil && tab.TabProperties != nil {
			return tab
		}
	}

	return nil
}

func projectTabs(tabs []*docs.Tab) []DocumentTab {
	var out []DocumentTab
	var walk func(items []*docs.Tab)
	walk = func(items []*docs.Tab) {
		for _, tab := range items {
			if tab == nil || tab.TabProperties == nil {
				continue
			}
			props := tab.TabProperties
			out = append(out, DocumentTab{
				ID:           props.TabId,
				Title:        props.Title,
				ParentID:     props.ParentTabId,
				Index:        props.Index,
				NestingLevel: props.NestingLevel,
			})

			walk(tab.ChildTabs)
		}
	}
	walk(tabs)

	return out
}

func findTab(tabs []*docs.Tab, id string) *docs.Tab {
	for _, tab := range tabs {
		if tab == nil {
			continue
		}

		if tab.TabProperties != nil && tab.TabProperties.TabId == id {
			return tab
		}

		if found := findTab(tab.ChildTabs, id); found != nil {
			return found
		}
	}

	return nil
}

func documentURL(id, tabID string) string {
	if id == "" {
		return ""
	}

	link := "https://docs.google.com/document/d/" + url.PathEscape(id) + "/edit"
	if tabID != "" {
		link += "?tab=" + url.QueryEscape(tabID)
	}

	return link
}

func mapGoogleError(operation string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, errResponseLimit) {
		return &mcpcontract.Error{
			Category: mcpcontract.UpstreamFailure,
			Message:  operation + " response exceeded the bounded read limit",
		}
	}

	return nativegoogleapi.NativePublicError(err)
}
