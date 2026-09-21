package accountconnect

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestConsentDoesNotMergeProjectGrants(t *testing.T) {
	t.Parallel()

	for _, scopes := range [][]string{DefaultConnectScopes(), {scopeOpenID, scopeEmail, "https://www.googleapis.com/auth/gmail.send", "https://www.googleapis.com/auth/drive.file"}} {
		auth, err := NewGoogleProvider().AuthCodeURL(AuthCodeParams{ClientID: "fixture", RedirectURL: "http://localhost:8787/oauth/callback", State: "state", Verifier: "verifier", Scopes: scopes, ForceConsent: true, SelectAccount: true})
		if err != nil {
			t.Fatal(err)
		}

		parsed, err := url.Parse(auth)
		if err != nil {
			t.Fatal(err)
		}

		query := parsed.Query()
		if query.Get("include_granted_scopes") != "false" {
			t.Fatal("unrelated project grants would be merged")
		}

		if !slices.Equal(strings.Fields(query.Get("scope")), scopes) {
			t.Fatalf("selected scopes changed: %q", query.Get("scope"))
		}

		if query.Get("access_type") != "offline" || query.Get("code_challenge_method") != "S256" || query.Get("state") != "state" {
			t.Fatal("offline, PKCE or state protection missing")
		}
	}
}

func TestReconnectExplicitlyRetainsConnectionScopes(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	existing := []string{scopeOpenID, scopeEmail, "https://www.googleapis.com/auth/gmail.readonly"}
	requested := []string{"https://www.googleapis.com/auth/calendar.readonly"}

	scopes := ctrl.reconnectScopes(existing, requested)
	for _, scope := range append(existing, requested...) {
		if !slices.Contains(scopes, scope) {
			t.Fatalf("lost scope %q", scope)
		}
	}

	auth, err := NewGoogleProvider().AuthCodeURL(AuthCodeParams{ClientID: "fixture", RedirectURL: ctrl.RedirectURL(), State: "state", Verifier: "verifier", Scopes: scopes, ForceConsent: true})
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(auth)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Query().Get("include_granted_scopes") != "false" || !slices.Equal(strings.Fields(parsed.Query().Get("scope")), scopes) {
		t.Fatal("reconnect must request its explicit scope union")
	}
}
