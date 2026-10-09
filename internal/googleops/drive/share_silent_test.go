package drive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type shareRecorder struct {
	calls  int
	method string
	path   string
	query  url.Values
	body   map[string]any
	status int
	resp   string
}

type shareProvider struct {
	target  *url.URL
	options []mcpcontract.CallOptions
}

func (p *shareProvider) HTTPClient(_ context.Context, _ mcpcontract.Identity, opts mcpcontract.CallOptions) (*http.Client, error) {
	p.options = append(p.options, opts)

	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme, req.URL.Host = p.target.Scheme, p.target.Host

		return http.DefaultTransport.RoundTrip(req)
	})}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func shareFixture(t *testing.T, status int, resp string) (mcpcontract.Operation, *shareProvider, *shareRecorder) {
	t.Helper()
	rec := &shareRecorder{status: status, resp: resp}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.calls++
		rec.method, rec.path, rec.query = r.Method, r.URL.EscapedPath(), r.URL.Query()
		raw, _ := io.ReadAll(r.Body)
		rec.body = map[string]any{}
		_ = json.Unmarshal(raw, &rec.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rec.status)
		_, _ = io.WriteString(w, rec.resp)
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	provider := &shareProvider{target: target}

	return ShareOperations(provider)[0], provider, rec
}

func shareIdentity() mcpcontract.Identity {
	return mcpcontract.Identity{AccountID: "acct", PrincipalID: "p", ClientName: "native-mcp", Label: "acct", Scopes: []string{mcpcontract.DriveFullScope}}
}

func runShare(t *testing.T, op mcpcontract.Operation, raw string) (any, error) {
	t.Helper()
	call, err := op.Decode(json.RawMessage(raw))
	if err != nil {
		return nil, err
	}

	return call.Run(context.Background(), shareIdentity())
}

func TestShareUserSilentFixesTypeAndSuppressesNotification(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"reader", "commenter", "writer"} {
		op, provider, rec := shareFixture(t, http.StatusOK, `{"id":"perm-1","type":"user","role":"`+role+`","emailAddress":"a@example.test"}`)
		result, err := runShare(t, op, `{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"`+role+`"}`)
		if err != nil {
			t.Fatal(err)
		}

		out := result.(mcpcontract.Result[ShareUserSilentData])
		if out.Data.PermissionID != "perm-1" || out.Data.Role != role || out.Data.Notification {
			t.Fatalf("unexpected result: %#v", out.Data)
		}

		if rec.calls != 1 || rec.method != http.MethodPost || rec.path != "/drive/v3/files/1AbC_def-123456/permissions" {
			t.Fatalf("calls=%d %s %s", rec.calls, rec.method, rec.path)
		}

		if rec.query.Get("sendNotificationEmail") != "false" {
			t.Fatalf("notification not suppressed: %v", rec.query)
		}

		for _, forbidden := range []string{"transferOwnership", "moveToNewOwnersRoot", "useDomainAdminAccess", "emailMessage", "enforceSingleParent", "enforceExpansiveAccess"} {
			if _, ok := rec.query[forbidden]; ok {
				t.Fatalf("forbidden parameter %s sent: %v", forbidden, rec.query)
			}
		}

		if rec.body["type"] != "user" || rec.body["role"] != role || rec.body["emailAddress"] != "a@example.test" || len(rec.body) != 3 {
			t.Fatalf("unexpected permission body: %#v", rec.body)
		}

		if provider.options[0].Retry != mcpcontract.NonReplayableWrite {
			t.Fatalf("share must be non-replayable: %#v", provider.options[0])
		}
	}
}

func TestShareUserSilentRejectsWideningInputsBeforeUpstream(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"owner"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"organizer"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"fileOrganizer"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"Reader"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"reader","type":"anyone"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"reader","type":"domain"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"reader","send_notification_email":true}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"reader","transfer_ownership":true}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"reader","email_message":"hi"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","domain":"example.test","role":"reader"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"","role":"reader"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"A@Example.test","role":"reader"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"Pat <a@example.test>","role":"reader"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test,b@example.test","role":"reader"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"*@example.test","role":"reader"}`,
		`{"account_id":"acct","file_id":"../x/permissions","email_address":"a@example.test","role":"reader"}`,
		`{"account_id":"acct","file_id":"1AbC_def-123456?x=1","email_address":"a@example.test","role":"reader"}`,
	} {
		op, _, rec := shareFixture(t, http.StatusOK, `{"id":"x"}`)
		if _, err := runShare(t, op, raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}

		if rec.calls != 0 {
			t.Fatalf("%s reached upstream", raw)
		}
	}
}

func TestShareUserSilentAmbiguousFailureIsOutcomeUnknown(t *testing.T) {
	t.Parallel()
	op, _, _ := shareFixture(t, http.StatusInternalServerError, `{"error":{"code":500,"message":"boom"}}`)
	_, err := runShare(t, op, `{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"reader"}`)

	var safe *mcpcontract.Error
	if err == nil || !asContract(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("unexpected error: %#v", err)
	}

	op, _, _ = shareFixture(t, http.StatusForbidden, `{"error":{"code":403,"message":"secret-detail"}}`)
	_, err = runShare(t, op, `{"account_id":"acct","file_id":"1AbC_def-123456","email_address":"a@example.test","role":"reader"}`)
	if err == nil || !asContract(err, &safe) || safe.Category != mcpcontract.Forbidden || strings.Contains(safe.Message, "secret-detail") {
		t.Fatalf("unexpected 403 mapping: %#v", err)
	}
}

func asContract(err error, target **mcpcontract.Error) bool {
	e, ok := err.(*mcpcontract.Error) //nolint:errorlint // contract errors are returned unwrapped
	if ok {
		*target = e
	}

	return ok
}

func TestShareUserSilentDefinitionIsGatedWrite(t *testing.T) {
	t.Parallel()
	def, ok := mcpcontract.Lookup(ShareUserSilentOperation)
	if !ok || def.Retry != mcpcontract.NonReplayableWrite || len(def.Actions) != 1 || def.Actions[0] != "drive:share.user.silent" || len(def.Scopes) != 1 || def.Scopes[0] != mcpcontract.DriveFullScope {
		t.Fatalf("unexpected definition: %#v", def)
	}
}
