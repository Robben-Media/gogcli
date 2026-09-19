package googleapi

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

// NativePublicError maps Google SDK, OAuth, and context errors to a safe public error.
// Already-safe mcpcontract.Error values are preserved. Raw bodies, URLs, and emails are omitted.
func NativePublicError(err error) *mcpcontract.Error {
	if err == nil {
		return nil
	}

	var safe *mcpcontract.Error
	if errors.As(err, &safe) {
		return safe
	}

	if mapped := nativeContextError(err); mapped != nil {
		return mapped
	}

	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		return nativeRetrieveError(retrieve)
	}

	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		return nativeAPIError(apiErr)
	}

	if mapped := nativeNetworkError(err); mapped != nil {
		return mapped
	}

	return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: nativeFailMessage}
}

func nativeContextError(err error) *mcpcontract.Error {
	if errors.Is(err, context.Canceled) {
		return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: nativeCanceledMessage, Retryable: false}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: nativeDeadlineMessage, Retryable: true}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() || errors.Is(urlErr.Err, context.DeadlineExceeded) {
			return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: nativeDeadlineMessage, Retryable: true}
		}

		if errors.Is(urlErr.Err, context.Canceled) {
			return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: nativeCanceledMessage, Retryable: false}
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: nativeDeadlineMessage, Retryable: true}
	}

	return nil
}

func nativeNetworkError(err error) *mcpcontract.Error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: nativeFailMessage, Retryable: true}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: nativeFailMessage, Retryable: true}
	}

	return nil
}

func nativeRetrieveError(err *oauth2.RetrieveError) *mcpcontract.Error {
	code := ""
	status := 0

	if err != nil {
		code = strings.ToLower(strings.TrimSpace(err.ErrorCode))
		if err.Response != nil {
			status = err.Response.StatusCode
		}
	}

	if code == "invalid_grant" || code == "invalid_client" || code == "unauthorized_client" || status == 401 {
		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	if status == 400 {
		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	if status == 403 {
		return nativeForbiddenError(nil, code)
	}

	if status == 429 || code == "slow_down" {
		return &mcpcontract.Error{Category: mcpcontract.QuotaExhausted, Message: nativeQuotaMessage, Retryable: true}
	}

	if status >= 500 {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: nativeFailMessage, Retryable: true}
	}

	return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
}

func nativeAPIError(apiErr *gapi.Error) *mcpcontract.Error {
	if apiErr == nil {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: nativeFailMessage}
	}

	switch apiErr.Code {
	case 400:
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: nativeInvalidGoogleMessage}
	case 401:
		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	case 403:
		return nativeForbiddenError(apiErr, "")
	case 404:
		return &mcpcontract.Error{Category: mcpcontract.NotFound, Message: nativeNotFoundMessage}
	case 408:
		return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: nativeDeadlineMessage, Retryable: true}
	case 429:
		return &mcpcontract.Error{Category: mcpcontract.QuotaExhausted, Message: nativeQuotaMessage, Retryable: true}
	default:
		if apiErr.Code >= 500 {
			return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: nativeFailMessage, Retryable: true}
		}

		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: nativeFailMessage}
	}
}

func nativeForbiddenError(apiErr *gapi.Error, extra string) *mcpcontract.Error {
	reasons := []string{extra}

	if apiErr != nil {
		for _, item := range apiErr.Errors {
			reasons = append(reasons, item.Reason, item.Message)
		}

		reasons = append(reasons, apiErr.Message, apiErr.Body)
	}

	joined := strings.ToLower(strings.Join(reasons, " "))
	switch {
	case strings.Contains(joined, "dailylimitexceeded"):
		return &mcpcontract.Error{Category: mcpcontract.QuotaExhausted, Message: nativeQuotaMessage, Retryable: false}
	case strings.Contains(joined, "ratelimitexceeded"), strings.Contains(joined, "userratelimitexceeded"), strings.Contains(joined, "quotaexceeded"):
		return &mcpcontract.Error{Category: mcpcontract.QuotaExhausted, Message: nativeQuotaMessage, Retryable: true}
	case strings.Contains(joined, "insufficient authentication scopes"), strings.Contains(joined, "insufficientauthenticationscopes"), strings.Contains(joined, "insufficientpermissions"), strings.Contains(joined, "access_token_scope_insufficient"), strings.Contains(joined, "insufficient scope"):
		return &mcpcontract.Error{Category: mcpcontract.InsufficientScope, Message: nativeScopeMessage}
	default:
		return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: nativeForbiddenMessage}
	}
}
