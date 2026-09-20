package access

import "github.com/steipete/gogcli/internal/mcpcontract"

func invalid(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: message}
}

func forbidden(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: message, Retryable: false}
}

func authRequired(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: message, Retryable: false}
}

func insufficientScope(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InsufficientScope, Message: message, Retryable: false}
}
