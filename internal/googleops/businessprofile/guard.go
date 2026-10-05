package businessprofile

import (
	"regexp"
	"strings"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

// humanOnlyFieldPrefixes are the Location fields no automated operation may
// change under any grant: business name, address and phone numbers. Prefixes
// are lowercase with underscores removed, so camelCase, snake_case and every
// sub-field path match.
var humanOnlyFieldPrefixes = []string{"title", "storefrontaddress", "phonenumbers"}

// fieldPathPattern is the ASCII grammar every update-mask path must match
// after surrounding whitespace is trimmed. It rejects wildcards, empty
// segments, internal whitespace and Unicode lookalikes before the deny check.
var fieldPathPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)*$`)

// ForbiddenUpdateMask rejects an update mask that is empty, has a path outside
// the field-path grammar, or touches the business name, address or phone
// numbers, with or without a leading "location." prefix. Every Business
// Profile write must call it before sending a patch.
func ForbiddenUpdateMask(mask string) error {
	if strings.TrimSpace(mask) == "" {
		return forbidden("update mask must name explicit fields")
	}

	for path := range strings.SplitSeq(mask, ",") {
		path = strings.TrimSpace(path)
		if !fieldPathPattern.MatchString(path) {
			return forbidden("update mask must list explicit ASCII field paths without wildcards or spaces")
		}

		normalized := strings.TrimPrefix(strings.ToLower(path), "location.")
		normalized = strings.ReplaceAll(normalized, "_", "")

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
