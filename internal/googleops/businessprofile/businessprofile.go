// Package businessprofile implements native Google Business Profile read tools.
//
// Google's discovery documents omit scopes for the mybusiness services, so the
// generated google_ catalog gates those methods off (missing scopes). These
// operations are explicit, read-only, and bind the documented scope through the
// curated definitions instead. Method and scope references verified 2026-09-20:
//
//   - accounts.list (mybusinessaccountmanagement v1) requires
//     https://www.googleapis.com/auth/business.manage:
//     https://developers.google.com/my-business/reference/accountmanagement/rest/v1/accounts/list
//   - accounts.locations.list (mybusinessbusinessinformation v1) requires
//     https://www.googleapis.com/auth/business.manage:
//     https://developers.google.com/my-business/reference/businessinformation/rest/v1/accounts.locations/list
package businessprofile

import (
	"context"
	"strings"
	"unicode"

	mybusinessaccountmanagement "google.golang.org/api/mybusinessaccountmanagement/v1"
	mybusinessbusinessinformation "google.golang.org/api/mybusinessbusinessinformation/v1"
	"google.golang.org/api/option"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	// accountsMaxPageSize is the documented default and maximum page size for
	// accounts.list; the read stays on one bounded page of at most 20 accounts.
	accountsMaxPageSize = 20
	// locationsMaxPageSize is the documented maximum page size for
	// accounts.locations.list; the read stays on one bounded page.
	locationsMaxPageSize = 100
	// locationsReadMask is the narrow documented field mask requested for
	// accounts.locations.list; readMask is a required parameter.
	locationsReadMask = "name,title,storeCode,websiteUri"
)

type listAccountsInput struct {
	mcpcontract.Selection
	PageToken string `json:"page_token,omitempty" jsonschema:"Opaque next_page_token from a previous businessprofile_list_accounts result; each call returns at most 20 accounts"`
}

type listLocationsInput struct {
	mcpcontract.Selection
	Parent    string `json:"parent" jsonschema:"Exact opaque parent account resource name returned by businessprofile_list_accounts, in accounts/{id} form; never guessed or auto-discovered"`
	PageSize  int64  `json:"page_size,omitempty" jsonschema:"Maximum locations in the single returned page; default and maximum 100"`
	PageToken string `json:"page_token,omitempty" jsonschema:"Opaque next_page_token from a previous businessprofile_list_locations result"`
}

type Account struct {
	Name        string `json:"name"`
	AccountName string `json:"account_name,omitempty"`
	Type        string `json:"type,omitempty"`
	Role        string `json:"role,omitempty"`
}

type AccountsData struct {
	Accounts []Account `json:"accounts"`
}

type Location struct {
	Name       string `json:"name"`
	Title      string `json:"title,omitempty"`
	StoreCode  string `json:"store_code,omitempty"`
	WebsiteURI string `json:"website_uri,omitempty"`
}

type LocationsData struct {
	Parent    string     `json:"parent"`
	Locations []Location `json:"locations"`
}

// Operations returns the explicit Business Profile read tool implementations.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("businessprofile_list_accounts", nil, func(ctx context.Context, id mcpcontract.Identity, in listAccountsInput) (mcpcontract.Result[AccountsData], error) {
			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "businessprofile_list_accounts", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[AccountsData]{}, nativegoogleapi.NativePublicError(err)
			}

			svc, err := mybusinessaccountmanagement.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[AccountsData]{}, nativegoogleapi.NativePublicError(err)
			}

			resp, err := svc.Accounts.List().PageSize(accountsMaxPageSize).PageToken(in.PageToken).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[AccountsData]{}, nativegoogleapi.NativePublicError(err)
			}

			out := mcpcontract.NewResult(id, AccountsData{Accounts: []Account{}})
			for _, account := range resp.Accounts {
				if account == nil {
					continue
				}

				out.Data.Accounts = append(out.Data.Accounts, Account{
					Name: account.Name, AccountName: account.AccountName, Type: account.Type, Role: account.Role,
				})
			}
			out.NextPageToken = resp.NextPageToken

			return out, nil
		}),
		mcpcontract.NewOperation("businessprofile_list_locations", validateListLocations, func(ctx context.Context, id mcpcontract.Identity, in listLocationsInput) (mcpcontract.Result[LocationsData], error) {
			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "businessprofile_list_locations", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[LocationsData]{}, nativegoogleapi.NativePublicError(err)
			}

			svc, err := mybusinessbusinessinformation.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[LocationsData]{}, nativegoogleapi.NativePublicError(err)
			}

			pageSize := in.PageSize
			if pageSize == 0 {
				pageSize = locationsMaxPageSize
			}

			resp, err := svc.Accounts.Locations.List(in.Parent).PageSize(pageSize).PageToken(in.PageToken).ReadMask(locationsReadMask).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[LocationsData]{}, nativegoogleapi.NativePublicError(err)
			}

			out := mcpcontract.NewResult(id, LocationsData{Parent: in.Parent, Locations: []Location{}})
			for _, location := range resp.Locations {
				if location == nil {
					continue
				}

				out.Data.Locations = append(out.Data.Locations, Location{
					Name: location.Name, Title: location.Title, StoreCode: location.StoreCode, WebsiteURI: location.WebsiteUri,
				})
			}
			out.NextPageToken = resp.NextPageToken

			return out, nil
		}),
	}
}

// validateListLocations checks structure only; account IDs stay opaque.
func validateListLocations(in listLocationsInput) error {
	if in.Parent != strings.TrimSpace(in.Parent) {
		return invalid("parent must not contain surrounding whitespace")
	}

	if rest, ok := strings.CutPrefix(in.Parent, "accounts/"); !ok || rest == "" || rest == "." || rest == ".." || strings.ContainsAny(rest, "/\\?#%") || strings.ContainsFunc(rest, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return invalid("parent must be the exact accounts/{id} resource name returned by businessprofile_list_accounts")
	}

	if in.PageSize < 0 || in.PageSize > locationsMaxPageSize {
		return invalid("page_size must be between 1 and 100 when supplied")
	}

	return nil
}

func invalid(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: message}
}
