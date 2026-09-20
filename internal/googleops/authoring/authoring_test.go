package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	native "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type fixture struct {
	requests        []map[string]any
	paths           []string
	failPopulate    bool
	starterSlide    bool
	malformedCreate bool
	selections      []mcpcontract.Identity
}

func (f *fixture) HTTPClient(_ context.Context, id mcpcontract.Identity, opts mcpcontract.CallOptions) (*http.Client, error) {
	f.selections = append(f.selections, id)

	if opts.Retry != mcpcontract.NonReplayableWrite {
		return nil, invalid("writes must not replay")
	}

	return &http.Client{Transport: &native.NativeRetryTransport{Base: f, Class: opts.Retry}}, nil
}

func (f *fixture) RoundTrip(req *http.Request) (*http.Response, error) {
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode fixture request: %w", err)
	}
	f.requests = append(f.requests, body)
	f.paths = append(f.paths, req.URL.Path)
	response := `{"documentId":"doc-fixture","presentationId":"deck-fixture","spreadsheetId":"sheet-fixture","title":"Fixture"}`

	if f.malformedCreate {
		response = `{broken-json`
	}

	if f.starterSlide && strings.HasSuffix(req.URL.Path, "/presentations") {
		response = `{"presentationId":"deck-fixture","title":"Fixture","slides":[{"objectId":"initial_slide"}]}`
	}

	if strings.Contains(req.URL.Path, ":batchUpdate") {
		if f.failPopulate {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		}
		response = `{}`
	}

	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
}

func TestDocumentFormatsUnicodeWithTwoCallsAndExplicitAccount(t *testing.T) {
	f := &fixture{}
	in := DocumentInput{Selection: mcpcontract.Selection{AccountID: "work"}, Title: "Brief", Paragraphs: []Paragraph{{Text: "A😀", Style: "TITLE"}, {Text: "Next"}}}

	out, err := createDocument(native.WithUpstreamBudget(t.Context(), 2), f, mcpcontract.Identity{AccountID: "work"}, in)
	if err != nil || !out.Data.Populated || len(f.requests) != 2 {
		t.Fatalf("out=%+v err=%v calls=%d", out, err, len(f.requests))
	}
	b, _ := json.Marshal(f.requests[1])

	encoded := string(b)
	if !strings.Contains(encoded, `"endIndex":5`) || !strings.Contains(encoded, `"startIndex":5`) || !strings.Contains(encoded, `"namedStyleType":"TITLE"`) {
		t.Fatalf("UTF16 styles incorrect: %s", encoded)
	}

	if len(f.selections) != 1 || f.selections[0].AccountID != "work" {
		t.Fatal("account selection not retained")
	}
}

func TestFailedPopulationKeepsCreatedIDAndNeverRepeatsCreate(t *testing.T) {
	f := &fixture{failPopulate: true}

	out, err := createDocument(native.WithUpstreamBudget(t.Context(), 3), f, mcpcontract.Identity{AccountID: "work"}, DocumentInput{Title: "Brief", Paragraphs: []Paragraph{{Text: "Content"}}})
	if err != nil || out.Data.ID != "doc-fixture" || out.Data.Populated || len(out.PartialFailures) != 1 || out.PartialFailures[0].Category != string(mcpcontract.OutcomeUnknown) || len(f.requests) != 2 {
		t.Fatalf("partial=%+v err=%v calls=%d", out, err, len(f.requests))
	}

	if !strings.Contains(out.Data.NextAction, "Do not repeat") {
		t.Fatal("missing reconciliation instruction")
	}
}

func TestKnownInsufficientBudgetDoesNotCreateOrLoadClient(t *testing.T) {
	f := &fixture{}
	_, err := createDocument(native.WithUpstreamBudget(t.Context(), 1), f, mcpcontract.Identity{}, DocumentInput{})

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.BudgetExhausted || len(f.selections) != 0 || len(f.requests) != 0 {
		t.Fatalf("err=%v fixture=%+v", err, f)
	}
}

func TestPresentationBatchesShapesTextAndStyle(t *testing.T) {
	f := &fixture{}

	out, err := createPresentation(native.WithUpstreamBudget(t.Context(), 2), f, mcpcontract.Identity{AccountID: "work"}, PresentationInput{Title: "Deck", Slides: []Slide{{Title: "First", Body: []string{"Details"}}, {Title: "Second"}}})
	if err != nil || !out.Data.Populated || len(f.requests) != 2 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	b, _ := json.Marshal(f.requests[1])

	encoded := string(b)
	if strings.Count(encoded, `"createSlide"`) != 2 || strings.Count(encoded, `"createShape"`) != 3 || !strings.Contains(encoded, `"updateTextStyle"`) {
		t.Fatalf("unformatted deck: %s", encoded)
	}
}

func TestSpreadsheetKeepsTextLiteralAndFormatsHeaderInOneCall(t *testing.T) {
	f := &fixture{}
	text, formula := "=not-a-formula", "=1+2"

	input := SpreadsheetInput{Title: "Sheet", SheetTitle: "Data", Header: true, Rows: [][]Cell{{{Text: &text}, {Formula: &formula}}}}
	if err := validateSpreadsheet(input); err != nil {
		t.Fatal(err)
	}

	out, err := createSpreadsheet(native.WithUpstreamBudget(t.Context(), 1), f, mcpcontract.Identity{AccountID: "work"}, input)
	if err != nil || !out.Data.Populated || len(f.requests) != 1 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	b, _ := json.Marshal(f.requests[0])

	encoded := string(b)
	for _, want := range []string{`"stringValue":"=not-a-formula"`, `"formulaValue":"=1+2"`, `"frozenRowCount":1`, `"bold":true`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("missing %s: %s", want, encoded)
		}
	}
}

func TestInvalidAuthoringArgumentsRejectedBeforeProvider(t *testing.T) {
	f := &fixture{}
	for _, op := range Operations(f) {
		if _, err := op.Decode(json.RawMessage(`{"account_id":"work","title":"invalid","unexpected":true}`)); err == nil {
			t.Fatalf("accepted invalid %s", op.Definition.Name)
		}
	}

	if len(f.selections) > 0 {
		t.Fatal("invalid request reached provider")
	}
}

func TestPresentationReplacesProviderStarterSlide(t *testing.T) {
	f := &fixture{starterSlide: true}

	out, err := createPresentation(native.WithUpstreamBudget(t.Context(), 2), f, mcpcontract.Identity{AccountID: "work"}, PresentationInput{Title: "Deck", Slides: []Slide{{Title: "Only slide"}}})
	if err != nil || !out.Data.Populated || len(f.requests) != 2 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	requests := f.requests[1]["requests"].([]any)

	first := requests[0].(map[string]any)["deleteObject"].(map[string]any)
	if first["objectId"] != "initial_slide" {
		t.Fatalf("starter not removed before create: %#v", requests)
	}

	encoded, _ := json.Marshal(requests)
	if strings.Count(string(encoded), `"createSlide"`) != 1 {
		t.Fatalf("unexpected slide count: %s", encoded)
	}
}

func TestPresentationParagraphBounds(t *testing.T) {
	in := PresentationInput{Title: "Deck", Slides: []Slide{{Title: "Slide", Body: make([]string, 20)}}}
	if err := validatePresentation(in); err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{"one\ntwo", "one\rtwo", strings.Repeat("\n", 20)} {
		in.Slides[0].Body = []string{text}
		if err := validatePresentation(in); err == nil {
			t.Fatalf("accepted embedded paragraph break %q", text)
		}
	}
}

func TestMalformedCreateResponseDoesNotInviteReplay(t *testing.T) {
	f := &fixture{malformedCreate: true}
	_, err := createDocument(native.WithUpstreamBudget(t.Context(), 2), f, mcpcontract.Identity{AccountID: "work"}, DocumentInput{Title: "Doc", Paragraphs: []Paragraph{{Text: "Text"}}})

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable || len(f.requests) != 1 {
		t.Fatalf("err=%v calls=%d", err, len(f.requests))
	}
}
