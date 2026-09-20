package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

var (
	errMustNotFetchClient = errors.New("must not fetch client")
	errWriteTimeout       = errors.New("write timeout after send")
)

type recordingProvider struct {
	mu         sync.Mutex
	client     *http.Client
	options    []mcpcontract.CallOptions
	identities []mcpcontract.Identity
	err        error
}

func (p *recordingProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.options = append(p.options, options)

	p.identities = append(p.identities, id)
	if p.err != nil {
		return nil, p.err
	}

	return p.client, nil
}

type rewriteTransport struct {
	url      string
	saw      []*http.Request
	err      error
	response *http.Response
}

func (t *rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())

	t.saw = append(t.saw, cloned)
	if t.err != nil {
		return nil, t.err
	}

	if t.response != nil {
		return t.response, nil
	}

	rewritten := request.Clone(request.Context())
	rewritten.URL.Scheme = "http"
	rewritten.URL.Host = strings.TrimPrefix(t.url, "http://")
	rewritten.Host = ""

	response, err := http.DefaultTransport.RoundTrip(rewritten)
	if err != nil {
		return nil, err //nolint:wrapcheck // Transparent test transport.
	}

	return response, nil
}

type memoryArtifacts struct {
	puts []artifactPut
}

type artifactPut struct {
	accountID string
	operation string
	name      string
	mimeType  string
	data      []byte
}

func (m *memoryArtifacts) Put(_ context.Context, id mcpcontract.Identity, operation, name, mimeType string, data []byte) (mcpcontract.MediaReference, error) {
	m.puts = append(m.puts, artifactPut{accountID: id.AccountID, operation: operation, name: name, mimeType: mimeType, data: append([]byte(nil), data...)})
	return mcpcontract.MediaReference{URI: "artifact://" + operation + "/" + id.AccountID, Name: name, MIMEType: mimeType, SizeBytes: len(data), SHA256: "abc"}, nil
}

func testIdentity(account string) mcpcontract.Identity {
	return mcpcontract.Identity{AccountID: account, Label: account}
}

func requirePinnedHost(t *testing.T, req *http.Request, host string) {
	t.Helper()

	if req == nil || req.URL == nil || req.URL.Scheme != "https" || req.URL.Host != host {
		t.Fatalf("pinned request %#v", req)
	}
}

func TestOperationsUseCatalogActions(t *testing.T) {
	ops := Operations(nil)
	if len(ops) != 5 {
		t.Fatalf("ops = %d", len(ops))
	}
	want := []string{"gmail:attachment", "drive:download", "drive:download", "drive:upload", "drive:upload"}

	for i, op := range ops {
		def, ok := mcpcontract.Lookup(op.Definition.Name)
		if !ok || def.Name != op.Definition.Name {
			t.Fatalf("lookup %s", op.Definition.Name)
		}

		if strings.Join(op.Definition.Actions, ",") != want[i] {
			t.Fatalf("%s actions %v", op.Definition.Name, op.Definition.Actions)
		}

		if strings.Join(def.Actions, ",") != want[i] {
			t.Fatalf("lookup actions %s %v", def.Name, def.Actions)
		}
	}

	call, err := ops[0].Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1","attachment_id":"a1","delivery":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(call.Actions, ",") != "gmail:attachment" {
		t.Fatalf("decode actions %v", call.Actions)
	}
}

func TestGmailAttachmentInlineOneGet(t *testing.T) {
	raw := []byte("x")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/messages/m1/attachments/a1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"size":1,"data":"` + base64.RawURLEncoding.EncodeToString(raw) + `"}`))
	}))
	defer server.Close()
	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1","attachment_id":"a1","max_bytes":1,"delivery":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity("acct"))
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[AttachmentData])
	if result.Data.Data != base64.StdEncoding.EncodeToString(raw) || result.Data.Encoding != "base64" || result.Data.Artifact != nil {
		t.Fatalf("%#v", result.Data)
	}

	if result.Data.MessageID != "m1" || strings.Contains(jsonDump(t, result.Data), "filename") {
		t.Fatalf("unexpected fields %#v", result.Data)
	}

	if len(transport.saw) != 1 {
		t.Fatalf("requests %#v", transport.saw)
	}

	requirePinnedHost(t, transport.saw[0], "gmail.googleapis.com")

	if provider.options[0] != (mcpcontract.CallOptions{Operation: "gmail_get_attachment", Retry: mcpcontract.SafeRead}) {
		t.Fatalf("options %#v", provider.options)
	}
}

func TestGmailEmptyAttachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"size":0,"data":""}`))
	}))
	defer server.Close()
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1","attachment_id":"a1","delivery":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity("acct"))
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[AttachmentData])
	if result.Data.SizeBytes != 0 || result.Data.Data != "" {
		t.Fatalf("%#v", result.Data)
	}
}

func TestGmailArtifactOmitsName(t *testing.T) {
	store := &memoryArtifacts{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"size":1,"data":"YQ"}`))
	}))
	defer server.Close()
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := OperationsWithArtifacts(provider, store)[0].Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1","attachment_id":"a1"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity("acct"))
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[AttachmentData])
	if result.Data.Data != "" || result.Data.Artifact == nil || result.Data.Artifact.Name != "" {
		t.Fatalf("%#v puts %#v", result.Data, store.puts)
	}

	if len(store.puts) != 1 || store.puts[0].name != "" || store.puts[0].operation != "gmail_get_attachment" {
		t.Fatalf("puts %#v", store.puts)
	}
}

func TestInvalidIDsDoNotCallProvider(t *testing.T) {
	provider := &recordingProvider{err: errMustNotFetchClient}

	op := Operations(provider)[0]
	for _, raw := range []string{
		`{"account_id":"acct","message_id":"../x","attachment_id":"a"}`,
		`{"account_id":"acct","message_id":" m ","attachment_id":"a"}`,
		`{"account_id":"acct","message_id":"m","attachment_id":" a "}`,
		`{"account_id":"acct","message_id":"m","attachment_id":"a/b"}`,
		`{"account_id":"acct","attachment_id":"a"}`,
		`{"account_id":"acct","message_id":"m","attachment_id":"a","delivery":"disk"}`,
		`{"account_id":"acct","message_id":"m","attachment_id":"a","delivery":"artifact"}`,
	} {
		if _, err := op.Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}

	create := Operations(provider)[3]
	if _, err := create.Decode(json.RawMessage(`{"account_id":"acct","name":"a.txt","parent_id":"   ","data":"x"}`)); err == nil {
		t.Fatal("accepted blank parent ID")
	}

	if _, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"acct","file_id":" file-1 "}`)); err == nil {
		t.Fatal("accepted padded file ID")
	}

	if _, err := create.Decode(json.RawMessage("{\"account_id\":\"acct\",\"name\":\"a.txt\",\"mime_type\":\"text/plain\\r\\nX: 1\",\"data\":\"x\"}")); err == nil {
		t.Fatal("accepted CR/LF mime type")
	}

	if len(provider.options) != 0 {
		t.Fatalf("provider called: %#v", provider.options)
	}
}

func TestDriveDownloadOneMediaGet(t *testing.T) {
	store := &memoryArtifacts{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drive/v3/files/file-1" || r.URL.Query().Get("alt") != "media" {
			t.Errorf("url = %s", r.URL.String())
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()
	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"work","file_id":"file-1","delivery":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity("work"))
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[FileContentData])
	if result.Data.Data != base64.StdEncoding.EncodeToString([]byte("hello")) || result.Data.Name != "" {
		t.Fatalf("%#v", result.Data)
	}

	if len(transport.saw) != 1 {
		t.Fatalf("calls %d", len(transport.saw))
	}

	requirePinnedHost(t, transport.saw[0], "www.googleapis.com")

	if provider.options[0].Operation != "drive_download_file" || provider.options[0].Retry != mcpcontract.SafeRead {
		t.Fatalf("%#v", provider.options)
	}

	transport.saw = nil
	provider.options = nil
	provider.identities = nil
	provider.client = &http.Client{Transport: transport}

	call, err = OperationsWithArtifacts(provider, store)[1].Decode(json.RawMessage(`{"account_id":"work","file_id":"file-1"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err = call.Run(context.Background(), testIdentity("work"))
	if err != nil {
		t.Fatal(err)
	}

	result = resultAny.(mcpcontract.Result[FileContentData])
	if result.Data.Data != "" || result.Data.Artifact == nil || result.Data.Artifact.Name != "" {
		t.Fatalf("%#v", result.Data)
	}

	if len(store.puts) != 1 || store.puts[0].name != "" {
		t.Fatalf("puts %#v", store.puts)
	}
}

func TestDriveDownloadMetadataRefusesOversizeBeforeMedia(t *testing.T) {
	var paths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RawQuery)
		if r.URL.Query().Get("alt") == "media" {
			t.Error("media get after oversize metadata")
		}
		_, _ = w.Write([]byte(`{"id":"file-1","name":"big.bin","mimeType":"application/octet-stream","size":"9999999"}`))
	}))
	defer server.Close()
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"acct","file_id":"file-1","include_metadata":true,"max_bytes":10,"delivery":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity("acct"))

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.BudgetExhausted {
		t.Fatalf("err = %#v", err)
	}

	if len(paths) != 1 || strings.Contains(paths[0], "alt=media") {
		t.Fatalf("paths %#v", paths)
	}
}

func TestIncludeMetadataRequiresTwoBudgetUnits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer server.Close()
	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"acct","file_id":"file-1","include_metadata":true,"delivery":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, remaining := range []int64{0, 1} {
		_, err = call.Run(nativegoogleapi.WithUpstreamBudget(context.Background(), remaining), testIdentity("acct"))

		var safe *mcpcontract.Error
		if !errors.As(err, &safe) || safe.Category != mcpcontract.BudgetExhausted {
			t.Fatalf("remaining %d err %#v", remaining, err)
		}
	}

	if len(transport.saw) != 0 {
		t.Fatalf("metadata GET issued: %#v", transport.saw)
	}
}

func TestDriveExportAndCreate(t *testing.T) {
	var createBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/export"):
			_, _ = w.Write([]byte("exported"))
		default:
			raw, _ := io.ReadAll(r.Body)
			createBody = string(raw)
			_, _ = w.Write([]byte(`{"id":"created-1","name":"note.txt","mimeType":"text/plain","size":"4"}`))
		}
	}))
	defer server.Close()
	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}
	ops := Operations(provider)

	if _, err := ops[2].Decode(json.RawMessage(`{"account_id":"acct","file_id":"doc-1","mime_type":"application/zip"}`)); err == nil {
		t.Fatal("accepted zip export")
	}

	call, err := ops[2].Decode(json.RawMessage(`{"account_id":"acct","file_id":"doc-1","mime_type":"text/plain","delivery":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = call.Run(context.Background(), testIdentity("acct")); err != nil {
		t.Fatal(err)
	}

	if len(transport.saw) != 1 {
		t.Fatalf("export requests %#v", transport.saw)
	}
	export := transport.saw[0]
	requirePinnedHost(t, export, "www.googleapis.com")

	if export.URL.Path != "/drive/v3/files/doc-1/export" || export.URL.Query().Get("mimeType") != "text/plain" {
		t.Fatalf("export %s", export.URL)
	}

	if export.URL.Query().Has("supportsAllDrives") {
		t.Fatalf("export must not send supportsAllDrives: %s", export.URL.RawQuery)
	}

	call, err = ops[3].Decode(json.RawMessage(`{"account_id":"acct","name":"  note.txt  ","encoding":"utf8","data":"hi!"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity("acct"))
	if err != nil {
		t.Fatal(err)
	}

	created := resultAny.(mcpcontract.Result[DriveWriteData])
	if created.Data.FileID != "created-1" {
		t.Fatalf("%#v", created.Data)
	}

	if len(transport.saw) != 2 {
		t.Fatalf("requests %#v", transport.saw)
	}
	upload := transport.saw[1]
	requirePinnedHost(t, upload, "www.googleapis.com")

	if upload.Method != http.MethodPost || !strings.HasPrefix(upload.URL.Path, "/upload/drive/v3/files") {
		t.Fatalf("upload %s %s", upload.Method, upload.URL)
	}

	if upload.URL.Query().Get("uploadType") != "multipart" {
		t.Fatalf("upload query %s", upload.URL.RawQuery)
	}

	if !strings.Contains(upload.Header.Get("Content-Type"), "multipart/related") {
		t.Fatalf("content-type %s", upload.Header.Get("Content-Type"))
	}

	if provider.options[1].Retry != mcpcontract.NonReplayableWrite || provider.options[1].Operation != "drive_create_file" {
		t.Fatalf("%#v", provider.options)
	}

	if !strings.Contains(createBody, `"name":"  note.txt  "`) {
		t.Fatalf("create body mutated name: %q", createBody)
	}
}

func TestWriteTimeoutUnknown(t *testing.T) {
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{err: errWriteTimeout}}}

	call, err := Operations(provider)[3].Decode(json.RawMessage(`{"account_id":"acct","name":"a.txt","data":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity("acct"))

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("err %#v", err)
	}
}

func TestWriteOversizeResponseUnknown(t *testing.T) {
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{
		response: &http.Response{StatusCode: http.StatusOK, ContentLength: int64(maxDecodedBytes) + 10, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)},
	}}}

	call, err := Operations(provider)[3].Decode(json.RawMessage(`{"account_id":"acct","name":"a.txt","data":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity("acct"))

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown {
		t.Fatalf("err %#v", err)
	}
}

func TestWriteMissingIDUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"note.txt"}`))
	}))
	defer server.Close()
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := Operations(provider)[3].Decode(json.RawMessage(`{"account_id":"acct","name":"a.txt","data":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity("acct"))

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown {
		t.Fatalf("err %#v", err)
	}
}

func TestWriteInvalidJSONUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := Operations(provider)[3].Decode(json.RawMessage(`{"account_id":"acct","name":"a.txt","data":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity("acct"))

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown {
		t.Fatalf("err %#v", err)
	}
}

func TestTwoAccountIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	op := Operations(provider)[1]
	for _, account := range []string{"a", "b"} {
		call, err := op.Decode(json.RawMessage(`{"account_id":"` + account + `","file_id":"f1","delivery":"inline"}`))
		if err != nil {
			t.Fatal(err)
		}

		if _, err := call.Run(context.Background(), testIdentity(account)); err != nil {
			t.Fatal(err)
		}
	}

	if len(provider.identities) != 2 || provider.identities[0].AccountID != "a" || provider.identities[1].AccountID != "b" {
		t.Fatalf("%#v", provider.identities)
	}
}

func jsonDump(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func TestDriveUpdateMultipart(t *testing.T) {
	var body string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		body = string(raw)
		_, _ = w.Write([]byte(`{"id":"file-1","name":"new.txt","mimeType":"text/plain","size":"11"}`))
	}))
	defer server.Close()
	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}

	call, err := Operations(provider)[4].Decode(json.RawMessage(`{"account_id":"acct","file_id":"file-1","name":"new.txt","mime_type":"text/plain","data":"replacement"}`))
	if err != nil {
		t.Fatal(err)
	}

	output, err := call.Run(t.Context(), testIdentity("acct"))
	if err != nil {
		t.Fatal(err)
	}

	result := output.(mcpcontract.Result[DriveWriteData])
	if result.Data.FileID != "file-1" || result.Data.Name != "new.txt" || result.Data.MimeType != "text/plain" || result.Data.SizeBytes != 11 {
		t.Fatalf("result: %#v", result)
	}

	if len(transport.saw) != 1 {
		t.Fatalf("requests: %d", len(transport.saw))
	}
	request := transport.saw[0]
	requirePinnedHost(t, request, "www.googleapis.com")

	if request.Method != http.MethodPatch || request.URL.Path != "/upload/drive/v3/files/file-1" || request.URL.Query().Get("uploadType") != "multipart" || request.URL.Query().Get("supportsAllDrives") != "true" {
		t.Fatalf("request: %s %s", request.Method, request.URL)
	}

	if request.URL.RawQuery != "uploadType=multipart&supportsAllDrives=true" {
		t.Fatalf("query: %q", request.URL.RawQuery)
	}

	mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/related" {
		t.Fatalf("multipart type: %q %v", mediaType, err)
	}
	reader := multipart.NewReader(strings.NewReader(body), params["boundary"])

	metadata, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}

	if metadata.Header.Get("Content-Type") != "application/json; charset=UTF-8" {
		t.Fatalf("metadata content type: %q", metadata.Header.Get("Content-Type"))
	}

	var fields map[string]string
	if err = json.NewDecoder(metadata).Decode(&fields); err != nil {
		t.Fatal(err)
	}

	if len(fields) != 2 || fields["name"] != "new.txt" || fields["mimeType"] != "text/plain" {
		t.Fatalf("metadata: %#v", fields)
	}

	content, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}

	if content.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("media content type: %q", content.Header.Get("Content-Type"))
	}

	data, err := io.ReadAll(content)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("media bytes: %q %v", data, err)
	}

	if _, err := reader.NextPart(); !errors.Is(err, io.EOF) {
		t.Fatalf("extra part: %v", err)
	}

	if len(provider.options) != 1 || provider.options[0].Operation != "drive_update_file" || provider.options[0].Retry != mcpcontract.NonReplayableWrite {
		t.Fatalf("options: %#v", provider.options)
	}
}
