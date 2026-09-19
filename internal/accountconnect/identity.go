package accountconnect

import (
	"strings"

	"google.golang.org/api/idtoken"
)

const (
	googleIssuerHTTPS = "https://accounts.google.com"
	googleIssuerShort = "accounts.google.com"
)

func identityFromPayload(payload *idtoken.Payload) (string, string, error) {
	if payload == nil {
		return "", "", ErrUnverifiedIdentity
	}

	if payload.Issuer != googleIssuerHTTPS && payload.Issuer != googleIssuerShort {
		return "", "", ErrUnverifiedIdentity
	}

	if strings.TrimSpace(payload.Subject) == "" {
		return "", "", ErrUnverifiedIdentity
	}

	email, _ := payload.Claims["email"].(string)

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !emailVerified(payload.Claims["email_verified"]) {
		return "", "", ErrUnverifiedIdentity
	}

	return payload.Subject, email, nil
}

func emailVerified(raw any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}
