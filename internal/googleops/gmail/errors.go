package gmail

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func upstreamError(resource, id string, err error) error {
	var contractErr *mcpcontract.Error
	if errors.As(err, &contractErr) {
		return contractErr
	}

	if id == "" {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: resource + " client request failed"}
	}

	return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: fmt.Sprintf("%s %s client request failed", resource, id)}
}

func contextError(err error, resource, id string) error {
	if errors.Is(err, context.Canceled) {
		return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: fmt.Sprintf("%s %s request was canceled", resource, id), Retryable: false}
	}

	return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: fmt.Sprintf("%s %s request deadline was exceeded", resource, id), Retryable: true}
}

func mapGoogleError(err error, resource, id string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return contextError(err, resource, id)
	}

	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		return mapAPIError(apiErr.Code, resource, id, apiErr.Body)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: fmt.Sprintf("%s request transport failed", resource), Retryable: true}
	}

	return upstreamError(resource, id, err)
}

func mapAPIError(code int, resource, id string, body string) error {
	label := resource
	if id != "" {
		label += " " + id
	}

	switch code {
	case 400:
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: label + " request was rejected by Gmail"}
	case 401:
		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: label + " requires Google authentication", Retryable: false}
	case 403:
		normalized := strings.ToLower(body)
		if strings.Contains(normalized, "insufficientpermissions") || strings.Contains(normalized, "insufficient scopes") || strings.Contains(normalized, "insufficient scope") {
			return &mcpcontract.Error{Category: mcpcontract.InsufficientScope, Message: label + " requires additional Gmail scope", Retryable: false}
		}

		return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: label + " is forbidden for this Google account", Retryable: false}
	case 404:
		return &mcpcontract.Error{Category: mcpcontract.NotFound, Message: label + " was not found", Retryable: false}
	case 429:
		return &mcpcontract.Error{Category: mcpcontract.QuotaExhausted, Message: label + " exhausted Gmail quota", Retryable: true}
	default:
		retryable := code >= 500
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: fmt.Sprintf("%s request failed with Gmail HTTP status %d", label, code), Retryable: retryable}
	}
}
