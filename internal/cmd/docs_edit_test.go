package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newDocsEditTestServer(t *testing.T, batchRequests *[][]*docs.Request) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			*batchRequests = append(*batchRequests, req.Requests)
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/documents/"), ":batchUpdate")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"replies":    []any{},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"title":      "Test Doc",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func setupDocsEditTest(t *testing.T, batchRequests *[][]*docs.Request) (context.Context, *RootFlags) {
	t.Helper()

	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := newDocsEditTestServer(t, batchRequests)
	t.Cleanup(srv.Close)

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	return ctx, flags
}

func TestExecute_DocsDeleteRange_JSON(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	out := captureStdout(t, func() {
		err := runKong(t, &DocsDeleteRangeCmd{}, []string{"doc1", "--start", "5", "--end", "10"}, ctx, flags)
		if err != nil {
			t.Fatalf("delete-range: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].DeleteContentRange == nil {
		t.Fatal("expected DeleteContentRange request")
	}
	r := reqs[0].DeleteContentRange.Range
	if r.StartIndex != 5 || r.EndIndex != 10 {
		t.Fatalf("expected range 5-10, got %d-%d", r.StartIndex, r.EndIndex)
	}

	if !strings.Contains(out, "doc1") {
		t.Fatalf("expected documentId in output, got: %q", out)
	}
}

func TestExecute_DocsDeleteRange_InvalidRange(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	// start == end
	err := runKong(t, &DocsDeleteRangeCmd{}, []string{"doc1", "--start", "5", "--end", "5"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for start == end")
	}
	if !strings.Contains(err.Error(), "start must be less than end") {
		t.Fatalf("expected 'start must be less than end' error, got: %v", err)
	}

	// start > end
	err = runKong(t, &DocsDeleteRangeCmd{}, []string{"doc1", "--start", "10", "--end", "5"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for start > end")
	}
	if !strings.Contains(err.Error(), "start must be less than end") {
		t.Fatalf("expected 'start must be less than end' error, got: %v", err)
	}

	// No batch requests should have been made
	if len(batchRequests) != 0 {
		t.Fatalf("expected 0 batch requests, got %d", len(batchRequests))
	}
}

func TestExecute_DocsFormat_JSON(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	// Test bold + italic
	out := captureStdout(t, func() {
		err := runKong(t, &DocsFormatCmd{}, []string{
			"doc1", "--start", "1", "--end", "10", "--bold", "--italic",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("format bold+italic: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].UpdateTextStyle == nil {
		t.Fatal("expected UpdateTextStyle request")
	}
	uts := reqs[0].UpdateTextStyle
	if !uts.TextStyle.Bold {
		t.Fatal("expected bold to be true")
	}
	if !uts.TextStyle.Italic {
		t.Fatal("expected italic to be true")
	}
	if !strings.Contains(uts.Fields, "bold") || !strings.Contains(uts.Fields, "italic") {
		t.Fatalf("expected fields to contain bold and italic, got: %q", uts.Fields)
	}
	if !strings.Contains(out, "doc1") {
		t.Fatalf("expected documentId in output, got: %q", out)
	}
}

func TestExecute_DocsFormat_Heading(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	_ = captureStdout(t, func() {
		err := runKong(t, &DocsFormatCmd{}, []string{
			"doc1", "--start", "1", "--end", "10", "--heading", "2",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("format heading: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request (heading only), got %d", len(reqs))
	}
	if reqs[0].UpdateParagraphStyle == nil {
		t.Fatal("expected UpdateParagraphStyle request")
	}
	ups := reqs[0].UpdateParagraphStyle
	if ups.ParagraphStyle.NamedStyleType != "HEADING_2" {
		t.Fatalf("expected HEADING_2, got %q", ups.ParagraphStyle.NamedStyleType)
	}
}

func TestExecute_DocsFormat_BoldAndHeading(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	_ = captureStdout(t, func() {
		err := runKong(t, &DocsFormatCmd{}, []string{
			"doc1", "--start", "1", "--end", "10", "--bold", "--heading", "3",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("format bold+heading: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (text style + paragraph style), got %d", len(reqs))
	}
	if reqs[0].UpdateTextStyle == nil {
		t.Fatal("expected first request to be UpdateTextStyle")
	}
	if reqs[1].UpdateParagraphStyle == nil {
		t.Fatal("expected second request to be UpdateParagraphStyle")
	}
	if reqs[1].UpdateParagraphStyle.ParagraphStyle.NamedStyleType != "HEADING_3" {
		t.Fatalf("expected HEADING_3, got %q", reqs[1].UpdateParagraphStyle.ParagraphStyle.NamedStyleType)
	}
}

func TestExecute_DocsFormat_NoOptions(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	err := runKong(t, &DocsFormatCmd{}, []string{
		"doc1", "--start", "1", "--end", "10",
	}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for no formatting options")
	}
	if !strings.Contains(err.Error(), "no formatting options") {
		t.Fatalf("expected 'no formatting options' error, got: %v", err)
	}
}

func TestExecute_DocsInsertTable_JSON(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	out := captureStdout(t, func() {
		err := runKong(t, &DocsInsertTableCmd{}, []string{
			"doc1", "--index", "1", "--rows", "3", "--cols", "4",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("insert-table: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].InsertTable == nil {
		t.Fatal("expected InsertTable request")
	}
	it := reqs[0].InsertTable
	if it.Rows != 3 || it.Columns != 4 {
		t.Fatalf("expected 3x4 table, got %dx%d", it.Rows, it.Columns)
	}
	if it.Location == nil || it.Location.Index != 1 {
		t.Fatalf("expected insert at index 1, got %v", it.Location)
	}
	if !strings.Contains(out, "doc1") {
		t.Fatalf("expected documentId in output, got: %q", out)
	}
}

func TestExecute_DocsInsertTable_InvalidRows(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	err := runKong(t, &DocsInsertTableCmd{}, []string{
		"doc1", "--index", "1", "--rows", "0", "--cols", "4",
	}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for rows=0")
	}
	if !strings.Contains(err.Error(), "rows must be positive") {
		t.Fatalf("expected 'rows must be positive' error, got: %v", err)
	}
}

func TestExecute_DocsInsertImage_JSON(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	out := captureStdout(t, func() {
		err := runKong(t, &DocsInsertImageCmd{}, []string{
			"doc1", "--index", "5", "--uri", "https://example.com/image.png",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("insert-image: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].InsertInlineImage == nil {
		t.Fatal("expected InsertInlineImage request")
	}
	img := reqs[0].InsertInlineImage
	if img.Uri != "https://example.com/image.png" {
		t.Fatalf("expected URI 'https://example.com/image.png', got %q", img.Uri)
	}
	if img.Location == nil || img.Location.Index != 5 {
		t.Fatalf("expected insert at index 5, got %v", img.Location)
	}
	if !strings.Contains(out, "doc1") {
		t.Fatalf("expected documentId in output, got: %q", out)
	}
}

func TestExecute_DocsBullets_JSON(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	out := captureStdout(t, func() {
		err := runKong(t, &DocsBulletsCmd{}, []string{
			"doc1", "--start", "1", "--end", "20",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("bullets: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].CreateParagraphBullets == nil {
		t.Fatal("expected CreateParagraphBullets request")
	}
	cpb := reqs[0].CreateParagraphBullets
	if cpb.Range.StartIndex != 1 || cpb.Range.EndIndex != 20 {
		t.Fatalf("expected range 1-20, got %d-%d", cpb.Range.StartIndex, cpb.Range.EndIndex)
	}
	if cpb.BulletPreset != "BULLET_DISC_CIRCLE_SQUARE" {
		t.Fatalf("expected default preset BULLET_DISC_CIRCLE_SQUARE, got %q", cpb.BulletPreset)
	}
	if !strings.Contains(out, "doc1") {
		t.Fatalf("expected documentId in output, got: %q", out)
	}
}

func TestExecute_DocsBullets_CustomPreset(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	_ = captureStdout(t, func() {
		err := runKong(t, &DocsBulletsCmd{}, []string{
			"doc1", "--start", "1", "--end", "20", "--preset", "NUMBERED_DECIMAL_ALPHA_ROMAN",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("bullets custom preset: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	cpb := batchRequests[0][0].CreateParagraphBullets
	if cpb.BulletPreset != "NUMBERED_DECIMAL_ALPHA_ROMAN" {
		t.Fatalf("expected preset NUMBERED_DECIMAL_ALPHA_ROMAN, got %q", cpb.BulletPreset)
	}
}

func TestExecute_DocsBullets_InvalidRange(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	err := runKong(t, &DocsBulletsCmd{}, []string{
		"doc1", "--start", "20", "--end", "10",
	}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for start > end")
	}
	if !strings.Contains(err.Error(), "start must be less than end") {
		t.Fatalf("expected 'start must be less than end' error, got: %v", err)
	}
}

func TestExecute_DocsFormat_FontSize(t *testing.T) {
	var batchRequests [][]*docs.Request
	ctx, flags := setupDocsEditTest(t, &batchRequests)

	_ = captureStdout(t, func() {
		err := runKong(t, &DocsFormatCmd{}, []string{
			"doc1", "--start", "1", "--end", "10", "--font-size", "14",
		}, ctx, flags)
		if err != nil {
			t.Fatalf("format font-size: %v", err)
		}
	})

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	reqs := batchRequests[0]
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	uts := reqs[0].UpdateTextStyle
	if uts == nil {
		t.Fatal("expected UpdateTextStyle request")
	}
	if uts.TextStyle.FontSize == nil || uts.TextStyle.FontSize.Magnitude != 14 || uts.TextStyle.FontSize.Unit != "PT" {
		t.Fatalf("expected fontSize 14 PT, got %v", uts.TextStyle.FontSize)
	}
	if !strings.Contains(uts.Fields, "fontSize") {
		t.Fatalf("expected fields to contain fontSize, got: %q", uts.Fields)
	}
}
