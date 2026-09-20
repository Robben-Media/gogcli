package web

import (
	"errors"

	"github.com/steipete/gogcli/internal/accountconnect"
)

var errMissingCSRF = errors.New("accountconnect web: missing CSRF token")

var capabilityNames = map[string]string{
	"gmail.read":         "Gmail read",
	"drive.read":         "Drive read",
	"docs.read":          "Docs read",
	"calendar.read":      "Calendar read",
	"analytics.read":     "Analytics read",
	"searchconsole.read": "Search Console read",
	"sheets.read":        "Sheets read",
}

type capabilityChoice struct {
	Capability string
	Name       string
	Checked    bool
}

type accountItem struct {
	CSRFToken         string
	Email             string
	Label             string
	AccountID         string
	FormID            string
	IsActive          bool
	IsPending         bool
	IsDisconnecting   bool
	CleanupPending    bool
	StateNote         string
	Capabilities      []capabilityStatus
	AdditionalChoices []capabilityChoice
}

type capabilityStatus struct {
	Name   string
	Status string
}

type accountsPage struct {
	CSRFToken         string
	Accounts          []accountItem
	CapabilityChoices []capabilityChoice
	ConnectHeading    string
	ConnectButton     string
}

func newCapabilityChoices(choices []accountconnect.ScopeChoice, requested []string) []capabilityChoice {
	requestedSet := make(map[string]bool, len(requested))
	for _, capability := range requested {
		requestedSet[capability] = true
	}

	out := make([]capabilityChoice, 0, len(choices))
	for _, choice := range choices {
		out = append(out, capabilityChoice{
			Capability: choice.Capability,
			Name:       capabilityDisplayName(choice.Capability),
			Checked:    requestedSet[choice.Capability],
		})
	}

	return out
}

type statusPage struct {
	OK      bool
	Heading string
	Message string
	Account *accountItem
}

func newAccountItem(account accountconnect.AccountView, choices []accountconnect.ScopeChoice) accountItem {
	item := accountItem{
		Email:           account.Email,
		Label:           account.Label,
		AccountID:       account.AccountID,
		IsActive:        account.State == "" || account.State == accountconnect.RecordStateActive,
		IsPending:       account.State == accountconnect.RecordStatePending,
		IsDisconnecting: account.State == accountconnect.RecordStateDisconnecting,
		CleanupPending:  account.CleanupPending,
	}

	switch {
	case item.IsPending:
		item.StateNote = "Connection interrupted. Reconnect to finish Google sign-in."
	case item.IsDisconnecting:
		item.StateNote = "Disconnect is pending. Retry disconnect to finish removing this account."
	case item.CleanupPending:
		item.StateNote = "Connected. Previous credential cleanup is pending."
	}

	granted := make(map[string]bool, len(account.Capabilities))
	for _, capability := range account.Capabilities {
		granted[capability] = true
	}

	if item.IsActive {
		for _, capability := range account.Capabilities {
			item.Capabilities = append(item.Capabilities, capabilityStatus{
				Name:   capabilityDisplayName(capability),
				Status: "Granted",
			})
		}
	}

	if item.IsActive {
		for _, choice := range choices {
			if granted[choice.Capability] {
				continue
			}

			item.AdditionalChoices = append(item.AdditionalChoices, capabilityChoice{
				Capability: choice.Capability,
				Name:       capabilityDisplayName(choice.Capability),
			})
		}
	}

	return item
}

func capabilityDisplayName(capability string) string {
	if name := capabilityNames[capability]; name != "" {
		return name
	}

	return capability
}

func successCopy(status *accountconnect.StatusResponse, account *accountItem) (heading, message string) {
	if status.Action == "disconnected" || status.Disconnected {
		if status.Detail != "" {
			return "Google account disconnected", status.Detail
		}

		return "Google account disconnected", "The connection was removed. Other Google accounts are unchanged."
	}

	if account == nil {
		return "Google account updated", "The account request completed. Return to Google accounts to see the current list."
	}

	if account.IsPending {
		return "Connection pending", "Google sign-in is not complete. Return to Google accounts and finish connecting."
	}

	if account.IsDisconnecting {
		return "Disconnect pending", "Retry disconnect from Google accounts to finish removing this account."
	}

	if account.CleanupPending {
		return "Google account connected", "Connected " + account.Email + ". Previous credential cleanup is pending."
	}

	return "Google account connected", "Connected " + account.Email + ". Reconnect or disconnect it from Google accounts."
}

func failureCopy(code string) (heading, message string) {
	switch code {
	case "signin_rejected":
		return "Sign-in rejected", "Google did not complete this sign-in. Start again from the Google accounts page."
	case "missing_refresh_token":
		return "Offline access missing", "Google did not grant the offline access this connection needs. Reconnect and grant access again."
	case "account_mismatch":
		return "Wrong Google account", "The Google account you selected does not match this connection. Start again and choose the original account."
	case "unverified_identity":
		return "Identity not verified", "Google did not return a verified account identity. Try connecting again."
	case "access_denied":
		return "Access denied", "Google access was denied. You can reconnect later if you want this account available."
	case "unknown_account":
		return "Connection unavailable", "That connection is no longer present. Open Google accounts to connect it again."
	case "forbidden":
		return "Connection unavailable", "That connection is not available for this account-management page."
	default:
		return "Sign-in failed", "Google sign-in did not complete. Try again from the Google accounts page."
	}
}
