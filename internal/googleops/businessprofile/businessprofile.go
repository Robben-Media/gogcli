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
//   - locations.get (mybusinessbusinessinformation v1) requires
//     https://www.googleapis.com/auth/business.manage:
//     https://developers.google.com/my-business/reference/businessinformation/rest/v1/locations/get
//   - accounts.locations.reviews.list (mybusiness v4, read-only) is in reviews.go.
//
// The package provides no write operation. Any future Business Profile write
// must pass its update mask through ForbiddenUpdateMask first.
package businessprofile

import (
	"context"
	"fmt"
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
	// locationReadMask is the fixed field mask for locations.get. It covers the
	// current state an operator must see before any profile change.
	locationReadMask = "name,title,storefrontAddress,phoneNumbers,categories,serviceItems,profile,regularHours,websiteUri,openInfo,metadata"
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

type getLocationInput struct {
	mcpcontract.Selection
	Name string `json:"name" jsonschema:"Exact opaque location resource name returned by businessprofile_list_locations, in locations/{id} form; never guessed"`
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

type Address struct {
	RegionCode         string   `json:"region_code,omitempty"`
	LanguageCode       string   `json:"language_code,omitempty"`
	PostalCode         string   `json:"postal_code,omitempty"`
	SortingCode        string   `json:"sorting_code,omitempty"`
	AdministrativeArea string   `json:"administrative_area,omitempty"`
	Locality           string   `json:"locality,omitempty"`
	Sublocality        string   `json:"sublocality,omitempty"`
	AddressLines       []string `json:"address_lines,omitempty"`
}

type PhoneNumbers struct {
	PrimaryPhone     string   `json:"primary_phone,omitempty"`
	AdditionalPhones []string `json:"additional_phones,omitempty"`
}

type Category struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

type Categories struct {
	Primary    *Category  `json:"primary,omitempty"`
	Additional []Category `json:"additional,omitempty"`
}

type Price struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Units        int64  `json:"units,omitempty"`
	Nanos        int64  `json:"nanos,omitempty"`
}

// ServiceItem is either structured (service_type_id) or free-form (category
// plus label).
type ServiceItem struct {
	ServiceTypeID string `json:"service_type_id,omitempty"`
	Category      string `json:"category,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	Description   string `json:"description,omitempty"`
	LanguageCode  string `json:"language_code,omitempty"`
	Price         *Price `json:"price,omitempty"`
}

type HoursPeriod struct {
	OpenDay   string `json:"open_day"`
	OpenTime  string `json:"open_time"`
	CloseDay  string `json:"close_day"`
	CloseTime string `json:"close_time"`
}

type OpenInfo struct {
	Status      string `json:"status,omitempty"`
	CanReopen   bool   `json:"can_reopen,omitempty"`
	OpeningDate string `json:"opening_date,omitempty"`
}

type LocationMetadata struct {
	PlaceID              string `json:"place_id,omitempty"`
	MapsURI              string `json:"maps_uri,omitempty"`
	NewReviewURI         string `json:"new_review_uri,omitempty"`
	DuplicateLocation    string `json:"duplicate_location,omitempty"`
	HasGoogleUpdated     bool   `json:"has_google_updated,omitempty"`
	HasPendingEdits      bool   `json:"has_pending_edits,omitempty"`
	HasVoiceOfMerchant   bool   `json:"has_voice_of_merchant,omitempty"`
	CanModifyServiceList bool   `json:"can_modify_service_list,omitempty"`
}

// LocationDetail is the projection of one locations.get response under
// locationReadMask.
type LocationDetail struct {
	Name               string            `json:"name"`
	Title              string            `json:"title,omitempty"`
	StorefrontAddress  *Address          `json:"storefront_address,omitempty"`
	PhoneNumbers       *PhoneNumbers     `json:"phone_numbers,omitempty"`
	Categories         *Categories       `json:"categories,omitempty"`
	ServiceItems       []ServiceItem     `json:"service_items,omitempty"`
	ProfileDescription string            `json:"profile_description,omitempty"`
	RegularHours       []HoursPeriod     `json:"regular_hours,omitempty"`
	WebsiteURI         string            `json:"website_uri,omitempty"`
	OpenInfo           *OpenInfo         `json:"open_info,omitempty"`
	Metadata           *LocationMetadata `json:"metadata,omitempty"`
}

type LocationData struct {
	Location LocationDetail `json:"location"`
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
		mcpcontract.NewOperation("businessprofile_get_location", validateGetLocation, func(ctx context.Context, id mcpcontract.Identity, in getLocationInput) (mcpcontract.Result[LocationData], error) {
			httpClient, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "businessprofile_get_location", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[LocationData]{}, nativegoogleapi.NativePublicError(err)
			}

			svc, err := mybusinessbusinessinformation.NewService(ctx, option.WithHTTPClient(httpClient))
			if err != nil {
				return mcpcontract.Result[LocationData]{}, nativegoogleapi.NativePublicError(err)
			}

			location, err := svc.Locations.Get(in.Name).ReadMask(locationReadMask).Context(ctx).Do()
			if err != nil {
				return mcpcontract.Result[LocationData]{}, nativegoogleapi.NativePublicError(err)
			}

			return mcpcontract.NewResult(id, LocationData{Location: projectLocation(location)}), nil
		}),
		reviewsOperation(provider),
	}
}

func projectLocation(location *mybusinessbusinessinformation.Location) LocationDetail {
	out := LocationDetail{Name: location.Name, Title: location.Title, WebsiteURI: location.WebsiteUri}
	if address := location.StorefrontAddress; address != nil {
		out.StorefrontAddress = &Address{
			RegionCode: address.RegionCode, LanguageCode: address.LanguageCode, PostalCode: address.PostalCode, SortingCode: address.SortingCode,
			AdministrativeArea: address.AdministrativeArea, Locality: address.Locality, Sublocality: address.Sublocality, AddressLines: address.AddressLines,
		}
	}

	if phones := location.PhoneNumbers; phones != nil {
		out.PhoneNumbers = &PhoneNumbers{PrimaryPhone: phones.PrimaryPhone, AdditionalPhones: phones.AdditionalPhones}
	}

	if categories := location.Categories; categories != nil {
		out.Categories = &Categories{}
		if primary := categories.PrimaryCategory; primary != nil {
			out.Categories.Primary = &Category{Name: primary.Name, DisplayName: primary.DisplayName}
		}

		for _, category := range categories.AdditionalCategories {
			if category != nil {
				out.Categories.Additional = append(out.Categories.Additional, Category{Name: category.Name, DisplayName: category.DisplayName})
			}
		}
	}

	for _, item := range location.ServiceItems {
		if item != nil {
			out.ServiceItems = append(out.ServiceItems, projectServiceItem(item))
		}
	}

	if location.Profile != nil {
		out.ProfileDescription = location.Profile.Description
	}

	if hours := location.RegularHours; hours != nil {
		for _, period := range hours.Periods {
			if period != nil {
				out.RegularHours = append(out.RegularHours, HoursPeriod{OpenDay: period.OpenDay, OpenTime: formatTime(period.OpenTime), CloseDay: period.CloseDay, CloseTime: formatTime(period.CloseTime)})
			}
		}
	}

	if info := location.OpenInfo; info != nil {
		out.OpenInfo = &OpenInfo{Status: info.Status, CanReopen: info.CanReopen}
		if date := info.OpeningDate; date != nil {
			out.OpenInfo.OpeningDate = fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day)
		}
	}

	if metadata := location.Metadata; metadata != nil {
		out.Metadata = &LocationMetadata{
			PlaceID: metadata.PlaceId, MapsURI: metadata.MapsUri, NewReviewURI: metadata.NewReviewUri, DuplicateLocation: metadata.DuplicateLocation,
			HasGoogleUpdated: metadata.HasGoogleUpdated, HasPendingEdits: metadata.HasPendingEdits, HasVoiceOfMerchant: metadata.HasVoiceOfMerchant,
			CanModifyServiceList: metadata.CanModifyServiceList,
		}
	}

	return out
}

func projectServiceItem(item *mybusinessbusinessinformation.ServiceItem) ServiceItem {
	var out ServiceItem
	if structured := item.StructuredServiceItem; structured != nil {
		out.ServiceTypeID = structured.ServiceTypeId
		out.Description = structured.Description
	}

	if freeForm := item.FreeFormServiceItem; freeForm != nil {
		out.Category = freeForm.Category
		if label := freeForm.Label; label != nil {
			out.DisplayName = label.DisplayName
			out.Description = label.Description
			out.LanguageCode = label.LanguageCode
		}
	}

	if price := item.Price; price != nil {
		out.Price = &Price{CurrencyCode: price.CurrencyCode, Units: price.Units, Nanos: price.Nanos}
	}

	return out
}

func formatTime(value *mybusinessbusinessinformation.TimeOfDay) string {
	if value == nil {
		return ""
	}

	return fmt.Sprintf("%02d:%02d", value.Hours, value.Minutes)
}

// exactResourceName reports whether value is prefix followed by one opaque ID
// segment with no path, query, escape, whitespace, or control characters.
func exactResourceName(value, prefix string) bool {
	rest, ok := strings.CutPrefix(value, prefix)

	return ok && rest != "" && rest != "." && rest != ".." && !strings.ContainsAny(rest, "/\\?#%") && !strings.ContainsFunc(rest, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

// validateGetLocation checks structure only; location IDs stay opaque.
func validateGetLocation(in getLocationInput) error {
	if !exactResourceName(in.Name, "locations/") {
		return invalid("name must be the exact locations/{id} resource name returned by businessprofile_list_locations")
	}

	return nil
}

// validateListLocations checks structure only; account IDs stay opaque.
func validateListLocations(in listLocationsInput) error {
	if in.Parent != strings.TrimSpace(in.Parent) {
		return invalid("parent must not contain surrounding whitespace")
	}

	if !exactResourceName(in.Parent, "accounts/") {
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
