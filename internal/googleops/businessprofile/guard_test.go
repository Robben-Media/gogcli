package businessprofile

import (
	"errors"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestForbiddenUpdateMaskRejectsHumanOnlyFieldsAndWildcards(t *testing.T) {
	t.Parallel()

	for _, mask := range []string{
		"",
		"   ",
		"*",
		"profile,*",
		"categories.*",
		"profile,,serviceItems",
		"title",
		"TITLE",
		"profile.description, title",
		"storefrontAddress",
		"storefrontAddress.addressLines",
		"storefront_address.postal_code",
		"phoneNumbers",
		"phoneNumbers.primaryPhone",
		"PhoneNumbers.additionalPhones",
		"phone_numbers",
		"serviceItems,phoneNumbers.additionalPhones",
		"location.title",
		"title ",
		"ti tle",
		"titl\u0435", // Cyrillic ie lookalike for Latin e
		"phone_numbers.primary_phone",
		"Location.storefrontAddress.postalCode",
		"location.*",
		"profile..description",
	} {
		var safe *mcpcontract.Error
		if err := ForbiddenUpdateMask(mask); !errors.As(err, &safe) || safe.Category != mcpcontract.Forbidden {
			t.Fatalf("mask %q: err=%v, want forbidden_operation", mask, err)
		}
	}
}

func TestForbiddenUpdateMaskAllowsOtherFields(t *testing.T) {
	t.Parallel()

	for _, mask := range []string{
		"profile.description",
		"serviceItems",
		"categories",
		"regularHours, websiteUri",
		"profile,serviceItems,openInfo.status",
		"location.profile.description",
		"service_items",
	} {
		if err := ForbiddenUpdateMask(mask); err != nil {
			t.Fatalf("mask %q: unexpected error %v", mask, err)
		}
	}
}
