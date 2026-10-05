package businessprofile

import (
	"strings"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

// humanOnlyFieldPrefixes are the Location fields no automated operation may
// change under any grant: business name, address and phone numbers. Prefixes
// are lowercase with underscores removed, so camelCase, snake_case and every
// sub-field path match.
var humanOnlyFieldPrefixes = []string{"title", "storefrontaddress", "phonenumbers"}

// ForbiddenUpdateMask rejects an update mask that is empty, contains a
// wildcard or empty path, or touches the business name, address or phone
// numbers. Every Business Profile write must call it before sending a patch.
func ForbiddenUpdateMask(mask string) error {
	if strings.TrimSpace(mask) == "" {
		return forbidden("update mask must name explicit fields")
	}

	for path := range strings.SplitSeq(mask, ",") {
		normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(path)), "_", "")
		if normalized == "" || strings.Contains(normalized, "*") {
			return forbidden("update mask must list explicit field paths without wildcards")
		}

		for _, prefix := range humanOnlyFieldPrefixes {
			if strings.HasPrefix(normalized, prefix) {
				return forbidden("business name, address and phone numbers are human-only and cannot be changed by automation")
			}
		}
	}

	return nil
}

func forbidden(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: message}
}
