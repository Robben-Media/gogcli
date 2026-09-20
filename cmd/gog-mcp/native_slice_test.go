package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/googleapi"
	nativegmail "github.com/steipete/gogcli/internal/googleops/gmail"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

var (
	errNativeSliceRequestRequired = errors.New("native slice request is required")
	errNativeSliceEndpoint        = errors.New("native slice API endpoint is invalid")
	errNativeSliceMailbox         = errors.New("native slice returned the wrong mailbox")
	errNativeSliceToolResult      = errors.New("native slice tool returned an error result")
	errNativeSliceProvider        = errors.New("native slice provider failed")
	errNativeSliceTransport       = errors.New("native slice Gmail transport failed")
)

type nativeSliceCounters struct {
	mu           sync.Mutex
	refreshHits  map[string]int
	apiHits      map[string]int
	unauthorized int
}

func (c *nativeSliceCounters) refresh(token string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.refreshHits[token]
}

func (c *nativeSliceCounters) api(token string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.apiHits[token]
}

func (c *nativeSliceCounters) snapshot() (refresh map[string]int, api map[string]int, unauthorized int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return mapsClone(c.refreshHits), mapsClone(c.apiHits), c.unauthorized
}

func mapsClone(values map[string]int) map[string]int {
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}

	return out
}

type nativeSliceFixture struct {
	registry  *accountconnect.MemoryRegistry
	tokens    *accountconnect.MemoryTokenStore
	lifecycle *accountconnect.Lifecycle
	native    *googleapi.NativeProvider
	provider  mcpcontract.ClientProvider
	api       *httptest.Server
	token     *httptest.Server
	counters  *nativeSliceCounters
	personal  mcpcontract.Identity
	work      mcpcontract.Identity
	denied    mcpcontract.Identity
}

func newNativeSliceFixture(t *testing.T) *nativeSliceFixture {
	t.Helper()

	fixture := &nativeSliceFixture{
		registry:  accountconnect.NewMemoryRegistry(),
		tokens:    accountconnect.NewMemoryTokenStore(),
		lifecycle: accountconnect.NewLifecycle(),
		counters: &nativeSliceCounters{
			refreshHits: map[string]int{}, apiHits: map[string]int{},
		},
	}
	fixture.api = httptest.NewServer(http.HandlerFunc(fixture.serveGmail))
	t.Cleanup(fixture.api.Close)

	fixture.token = httptest.NewServer(http.HandlerFunc(fixture.serveToken))
	t.Cleanup(fixture.token.Close)

	provider, err := googleapi.NewNativeProvider(googleapi.NativeOptions{
		Registry: fixture.registry,
		Tokens:   fixture.tokens,
		Credentials: func(string) (string, string, error) {
			return "fixture-client-id", "fixture-client-secret", nil
		},
		Endpoint:         oauth2.Endpoint{TokenURL: fixture.token.URL + "/token", AuthURL: fixture.token.URL + "/auth"},
		RefreshHTTP:      &http.Client{Timeout: 2 * time.Second},
		Lifecycle:        fixture.lifecycle,
		RequestTimeout:   2 * time.Second,
		MaxConcurrency:   8,
		MaxResponseBytes: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("create native provider: %v", err)
	}
	fixture.native = provider

	endpoint, err := url.Parse(fixture.api.URL)
	if err != nil {
		t.Fatalf("parse API endpoint: %v", err)
	}
	fixture.provider = &nativeSliceAPIProvider{native: provider, endpoint: endpoint}

	scopes := []string{mcpcontract.GmailReadScope}
	fixture.personal = fixture.putAccount(t, "personal", "personal@example.test", "rt-personal", scopes)
	fixture.work = fixture.putAccount(t, "work", "work@example.test", "rt-work", scopes)
	fixture.denied = fixture.putAccount(t, "denied", "denied@example.test", "rt-denied", scopes)

	return fixture
}

func (fixture *nativeSliceFixture) putAccount(t *testing.T, accountID, email, refreshToken string, scopes []string) mcpcontract.Identity {
	t.Helper()

	record := accountconnect.Record{
		AccountID: accountID, Subject: "subject-" + accountID, Email: email, Label: email,
		PrincipalID: "native-principal", ClientName: "fixture-client", AuthMode: accountconnect.AuthModeOAuth,
		Scopes: scopes, Generation: 1, UpdatedAt: time.Now().UTC(),
	}
	if err := fixture.registry.Upsert(context.Background(), record); err != nil {
		t.Fatalf("put %s registry record: %v", accountID, err)
	}
	if err := fixture.tokens.Put(context.Background(), record.ClientName, record.Email, refreshToken, scopes); err != nil {
		t.Fatalf("put %s refresh token: %v", accountID, err)
	}

	return record.Identity()
}

func (fixture *nativeSliceFixture) serveToken(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/token" || request.Method != http.MethodPost {
		http.Error(w, "unexpected OAuth request", http.StatusBadRequest)

		return
	}
	if err := request.ParseForm(); err != nil {
		http.Error(w, "invalid OAuth form", http.StatusBadRequest)

		return
	}

	refreshToken := request.PostFormValue("refresh_token")
	fixture.counters.mu.Lock()
	fixture.counters.refreshHits[refreshToken]++
	fixture.counters.mu.Unlock()

	accessToken := map[string]string{
		"rt-personal": "at-personal", "rt-work": "at-work", "rt-denied": "at-denied",
	}[refreshToken]
	if accessToken == "" {
		http.Error(w, "unknown refresh token", http.StatusUnauthorized)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": accessToken, "token_type": "Bearer", "expires_in": 3600,
	})
}

func (fixture *nativeSliceFixture) serveGmail(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/gmail/v1/users/me/messages/message-native" || request.Method != http.MethodGet {
		http.Error(w, "unexpected Gmail request", http.StatusBadRequest)

		return
	}

	authorization := request.Header.Get("Authorization")
	accessToken, hasBearer := strings.CutPrefix(authorization, "Bearer ")
	mailbox := map[string]string{
		"at-personal": "personal@example.test", "at-work": "work@example.test", "at-denied": "denied@example.test",
	}[accessToken]
	if !hasBearer || mailbox == "" {
		fixture.counters.mu.Lock()
		fixture.counters.unauthorized++
		fixture.counters.mu.Unlock()
		http.Error(w, "missing or unknown bearer token", http.StatusUnauthorized)

		return
	}

	fixture.counters.mu.Lock()
	fixture.counters.apiHits[accessToken]++
	fixture.counters.mu.Unlock()

	body := base64.StdEncoding.EncodeToString([]byte(mailbox + " message body"))
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "message-native", "threadId": "thread-native", "internalDate": "1765900800000",
		"labelIds": []string{"INBOX"}, "sizeEstimate": len(body), "snippet": mailbox + " snippet",
		"payload": map[string]any{
			"headers": []map[string]string{
				{"name": "From", "value": "sender@example.test"},
				{"name": "To", "value": mailbox},
				{"name": "Subject", "value": mailbox + " native slice"},
			},
			"mimeType": "text/plain", "body": map[string]string{"data": body},
		},
	})
}

func (fixture *nativeSliceFixture) setRecordState(t *testing.T, accountID string, state accountconnect.RecordState) {
	t.Helper()

	record, ok, err := fixture.registry.Get(context.Background(), accountID)
	if err != nil || !ok {
		t.Fatalf("get %s registry record: %v %v", accountID, ok, err)
	}
	record.State = state
	if err := fixture.registry.Upsert(context.Background(), record); err != nil {
		t.Fatalf("set %s state %q: %v", accountID, state, err)
	}
}

type nativeSliceAPIProvider struct {
	native   *googleapi.NativeProvider
	endpoint *url.URL
}

func (provider *nativeSliceAPIProvider) HTTPClient(ctx context.Context, identity mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	client, err := provider.native.HTTPClient(ctx, identity, options)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNativeSliceProvider, err)
	}

	rewritten := *client
	rewritten.Transport = &nativeSliceTransport{endpoint: provider.endpoint, base: client.Transport}

	return &rewritten, nil
}

type nativeSliceTransport struct {
	endpoint *url.URL
	base     http.RoundTripper
}

func (transport *nativeSliceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, errNativeSliceRequestRequired
	}
	if transport.endpoint == nil || transport.base == nil {
		return nil, errNativeSliceEndpoint
	}

	rewritten := request.Clone(request.Context())
	rewritten.URL.Scheme = transport.endpoint.Scheme
	rewritten.URL.Host = transport.endpoint.Host
	rewritten.Host = transport.endpoint.Host

	response, err := transport.base.RoundTrip(rewritten)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNativeSliceTransport, err)
	}

	return response, nil
}

type nativeSliceEnvelope struct {
	AccountID    string `json:"account_id"`
	AccountLabel string `json:"account_label"`
	Data         struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	} `json:"data"`
}

func TestNativeSliceTwoAccountsThroughMCP(t *testing.T) {
	t.Parallel()

	fixture := newNativeSliceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runtime, err := mcpserver.New(mcpserver.Config{
		Principal: mcpcontract.Principal{ID: "native-principal"},
		Grants: []mcpcontract.Grant{{
			PrincipalID: "native-principal", AccountIDs: []string{"personal", "work"},
			ClientNames: []string{"fixture-client"}, Operations: []string{"gmail_get_message"},
		}},
		AllowOperations: []string{"accounts_list", "gmail_get_message"},
		Operations:      nativegmail.Operations(fixture.provider),
		Accounts:        registryAccounts{registry: fixture.registry},
		Logger:          slogDiscard(),
		RequestTimeout:  2 * time.Second,
		MaxConcurrency:  8,
	})
	if err != nil {
		t.Fatal(err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := runtime.Server().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "native-slice-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	catalog, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "accounts_list", Arguments: map[string]any{},
	})
	if err != nil || catalog.IsError {
		t.Fatalf("accounts_list: %#v %v", catalog, err)
	}
	var catalogOutput struct {
		Accounts []struct {
			AccountID string `json:"account_id"`
		} `json:"accounts"`
	}
	decodeNativeSlice(t, catalog.StructuredContent, &catalogOutput)
	catalogIDs := map[string]bool{}
	for _, account := range catalogOutput.Accounts {
		catalogIDs[account.AccountID] = true
	}
	if len(catalogOutput.Accounts) != 2 || !catalogIDs["personal"] || !catalogIDs["work"] || catalogIDs["denied"] {
		t.Fatalf("accounts_list exposed unexpected accounts: %#v", catalogOutput.Accounts)
	}
	if refresh, api, unauthorized := fixture.counters.snapshot(); len(refresh) != 0 || len(api) != 0 || unauthorized != 0 {
		t.Fatalf("accounts_list reached upstream: refresh=%v api=%v unauthorized=%d", refresh, api, unauthorized)
	}

	const callsPerAccount = 12
	var workers sync.WaitGroup
	failures := make(chan error, callsPerAccount*2)

	for range callsPerAccount {
		workers.Add(2)
		go func() {
			defer workers.Done()
			if err := callAndCheckNativeMailbox(ctx, session, "personal", fixture.personal); err != nil {
				failures <- err
			}
		}()
		go func() {
			defer workers.Done()
			if err := callAndCheckNativeMailbox(ctx, session, "work", fixture.work); err != nil {
				failures <- err
			}
		}()
	}
	workers.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	if t.Failed() {
		t.FailNow()
	}

	wantRefresh := map[string]int{"rt-personal": 1, "rt-work": 1}
	wantAPI := map[string]int{"at-personal": callsPerAccount, "at-work": callsPerAccount}
	assertNativeSliceTraffic(t, fixture, wantRefresh, wantAPI)

	beforeRefresh, beforeAPI, beforeUnauthorized := fixture.counters.snapshot()
	for _, selector := range []string{"denied", "work@example.test", "missing"} {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "gmail_get_message",
			Arguments: map[string]any{"account_id": selector, "message_id": "message-native"},
		})
		if callErr == nil && !result.IsError {
			t.Fatalf("unauthorized selector %q succeeded: %#v", selector, result)
		}
		var denial mcpcontract.Error
		decodeNativeSlice(t, result.StructuredContent, &denial)
		if denial.Category != mcpcontract.Forbidden {
			t.Fatalf("unauthorized selector %q category = %q", selector, denial.Category)
		}
	}
	afterRefresh, afterAPI, afterUnauthorized := fixture.counters.snapshot()
	if !reflect.DeepEqual(beforeRefresh, afterRefresh) || !reflect.DeepEqual(beforeAPI, afterAPI) || beforeUnauthorized != afterUnauthorized {
		t.Fatalf("unauthorized selectors reached upstream: before=%v/%v/%d after=%v/%v/%d",
			beforeRefresh, beforeAPI, beforeUnauthorized, afterRefresh, afterAPI, afterUnauthorized)
	}

	options := mcpcontract.CallOptions{Operation: "gmail_get_message", Retry: mcpcontract.SafeRead}
	fixture.setRecordState(t, "personal", accountconnect.RecordStatePending)
	if _, err := fixture.provider.HTTPClient(ctx, fixture.personal, options); !nativeSliceAuthRequired(err) {
		t.Fatalf("pending record accepted warm cached token: %v", err)
	}

	fixture.native.InvalidateAccount("personal")
	if _, err := fixture.provider.HTTPClient(ctx, fixture.personal, options); !nativeSliceAuthRequired(err) {
		t.Fatalf("pending invalidated warm cached token remained usable: %v", err)
	}

	fixture.setRecordState(t, "personal", accountconnect.RecordStateDisconnecting)
	fixture.native.InvalidateAccount("personal")
	if _, err := fixture.provider.HTTPClient(ctx, fixture.personal, options); !nativeSliceAuthRequired(err) {
		t.Fatalf("disconnecting record accepted warm cached token: %v", err)
	}

	if err := callAndCheckNativeMailbox(ctx, session, "work", fixture.work); err != nil {
		t.Fatalf("work mailbox failed after personal invalidation: %v", err)
	}

	wantAPI["at-work"]++
	assertNativeSliceTraffic(t, fixture, wantRefresh, wantAPI)
}

func callAndCheckNativeMailbox(ctx context.Context, session *mcp.ClientSession, accountID string, identity mcpcontract.Identity) error {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gmail_get_message",
		Arguments: map[string]any{"account_id": accountID, "message_id": "message-native"},
	})
	if err != nil {
		return fmt.Errorf("%s MCP call: %w", accountID, err)
	}
	if result.IsError {
		return fmt.Errorf("%w: %s", errNativeSliceToolResult, accountID)
	}

	var envelope nativeSliceEnvelope
	if err := json.Unmarshal(mustMarshalNativeSlice(result.StructuredContent), &envelope); err != nil {
		return fmt.Errorf("%s decode: %w", accountID, err)
	}
	if envelope.AccountID != identity.AccountID || envelope.AccountLabel != identity.Label ||
		envelope.Data.ID != "message-native" || envelope.Data.Subject != identity.Email+" native slice" ||
		envelope.Data.Body != identity.Email+" message body" {
		return fmt.Errorf("%w: %#v", errNativeSliceMailbox, envelope)
	}

	return nil
}

func assertNativeSliceTraffic(t *testing.T, fixture *nativeSliceFixture, wantRefresh, wantAPI map[string]int) {
	t.Helper()

	gotRefresh, gotAPI, unauthorized := fixture.counters.snapshot()
	if !reflect.DeepEqual(gotRefresh, wantRefresh) {
		t.Fatalf("OAuth refresh traffic = %v, want %v", gotRefresh, wantRefresh)
	}
	if !reflect.DeepEqual(gotAPI, wantAPI) {
		t.Fatalf("Gmail API traffic = %v, want %v", gotAPI, wantAPI)
	}
	if fixture.counters.refresh("rt-denied") != 0 || fixture.counters.api("at-denied") != 0 {
		t.Fatal("denied account reached OAuth or Gmail")
	}
	if unauthorized != 0 {
		t.Fatalf("unauthorized Gmail calls = %d", unauthorized)
	}
}

func nativeSliceAuthRequired(err error) bool {
	var public *mcpcontract.Error

	return errors.As(err, &public) && public.Category == mcpcontract.AuthRequired
}

func decodeNativeSlice(t *testing.T, value any, target any) {
	t.Helper()

	if err := json.Unmarshal(mustMarshalNativeSlice(value), target); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
}

func mustMarshalNativeSlice(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic("marshal native slice structured content: " + err.Error())
	}

	return encoded
}

func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
