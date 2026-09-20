package access

import "strings"

const serviceGmail = "gmail"

// serviceAliases maps CLI/Kong names onto canonical policy service IDs.
var serviceAliases = map[string]string{
	"bq":               "bigquery",
	"business":         "businessprofile",
	"business-profile": "businessprofile",
	"email":            serviceGmail,
	"ga":               "analytics",
	"ga4":              "analytics",
	"gbp":              "businessprofile",
	"gsc":              "searchconsole",
	"gtm":              "tagmanager",
	"mail":             serviceGmail,
	"sc":               "searchconsole",
	"search-console":   "searchconsole",
	"tag-manager":      "tagmanager",
	"yt":               "youtube",
}

// CanonicalService maps Kong dashed names and CLI aliases to catalog service IDs.
func CanonicalService(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if canonical, ok := serviceAliases[raw]; ok {
		return canonical
	}

	return raw
}
