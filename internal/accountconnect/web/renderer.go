package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/steipete/gogcli/internal/accountconnect"
)

//go:embed templates/accounts.tmpl
var templateFS embed.FS

// Renderer implements accountconnect.Renderer without browser scripts or token storage.
type Renderer struct {
	template *template.Template
}

// NewRenderer compiles the account page template.
func NewRenderer() (*Renderer, error) {
	tmpl, err := template.New("accounts.tmpl").ParseFS(templateFS, "templates/accounts.tmpl")
	if err != nil {
		return nil, fmt.Errorf("accountconnect web: parse account template: %w", err)
	}

	return &Renderer{template: tmpl}, nil
}

// RenderAccounts writes the account list and connect form as HTML.
func (r *Renderer) RenderAccounts(w http.ResponseWriter, _ *http.Request, page accountconnect.PageModel) error {
	if page.CSRFToken == "" {
		return errMissingCSRF
	}

	accounts := make([]accountItem, 0, len(page.Accounts))
	for index, account := range page.Accounts {
		item := newAccountItem(account, page.ScopeChoices)
		item.CSRFToken = page.CSRFToken
		item.FormID = "reconnect-" + fmt.Sprintf("%d", index)
		accounts = append(accounts, item)
	}

	data := accountsPage{
		CSRFToken:         page.CSRFToken,
		Accounts:          accounts,
		CapabilityChoices: newCapabilityChoices(page.ScopeChoices, page.RequestedCapabilities),
		ConnectHeading:    "Connect Google account",
		ConnectButton:     "Connect Google account",
	}
	if len(accounts) > 0 {
		data.ConnectHeading = "Connect another account"
		data.ConnectButton = "Connect another account"
	}

	return r.execute(w, "accounts", data)
}

// RenderStatus writes a focused success, empty, or friendly failure state.
func (r *Renderer) RenderStatus(w http.ResponseWriter, _ *http.Request, page accountconnect.PageModel) error {
	status := page.Status
	if status == nil {
		status = &accountconnect.StatusResponse{}
	}

	data := statusPage{OK: status.OK}
	if status.Account != nil {
		account := newAccountItem(*status.Account, nil)
		data.Account = &account
	}

	if status.OK {
		data.Heading, data.Message = successCopy(status, data.Account)
	} else {
		data.Heading, data.Message = failureCopy(status.Error)
	}

	return r.execute(w, "status", data)
}

func (r *Renderer) execute(w http.ResponseWriter, templateName string, data any) error {
	var body bytes.Buffer
	if err := r.template.ExecuteTemplate(&body, templateName, data); err != nil {
		return fmt.Errorf("accountconnect web: execute %s template: %w", templateName, err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self' https://accounts.google.com; base-uri 'none'; frame-ancestors 'none'")
	// Preserve Origin on same-origin form POSTs; omit referrers to Google.
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(body.Bytes()); err != nil {
		return fmt.Errorf("accountconnect web: write response: %w", err)
	}

	return nil
}
