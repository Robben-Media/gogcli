package mailworkflow

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/steipete/gogcli/internal/mailcompose"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func validatePrepare(in PrepareInput) error {
	return validateEmailRequest(in)
}

func validateDraft(in DraftInput) error {
	if err := validateEmailRequest(in.EmailRequest); err != nil {
		return err
	}

	return validateExpectedDigest(in.ExpectedContentDigest)
}

func validateSend(in SendInput) error {
	if err := validateEmailRequest(in.EmailRequest); err != nil {
		return err
	}

	return validateExpectedDigest(in.ExpectedContentDigest)
}

func validateEmailRequest(in EmailRequest) error {
	if err := mailcompose.ValidateSemantic(mailcompose.Input{
		To:          in.To,
		Cc:          in.Cc,
		Bcc:         in.Bcc,
		Subject:     in.Subject,
		Content:     mailcompose.Content(in.Content),
		Attachments: attachments(in.Attachments),
	}); err != nil {
		return invalid(err.Error())
	}

	return validateReply(in.Reply)
}

func validateReply(reply *ReplyContext) error {
	if reply == nil {
		return nil
	}

	if err := validateID("source_message_id", reply.SourceMessageID); err != nil {
		return err
	}

	if err := validateID("thread_id", reply.ThreadID); err != nil {
		return err
	}

	return validateSubject(reply.ExpectedSubject)
}

func validateSubject(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > maxSubjectBytes || strings.ContainsAny(value, "\r\n\x00") {
		return invalid("subject is required and must contain at most 255 bytes")
	}

	return nil
}

func validateID(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > maxGmailIDBytes || strings.ContainsAny(trimmed, "\r\n\x00") {
		return invalid(field + " is required and must contain at most 256 bytes")
	}

	return nil
}

func validateExpectedDigest(value string) error {
	if value == "" {
		return nil
	}

	if len(value) != digestHexLength {
		return invalid("expected_content_digest must contain 64 hexadecimal characters")
	}

	if _, err := hex.DecodeString(value); err != nil {
		return invalid("expected_content_digest must contain hexadecimal characters")
	}

	return nil
}

func composeError(err error) error {
	var validation mailcompose.ValidationError
	if errors.As(err, &validation) {
		return invalid(validation.Message)
	}

	return err
}

func invalid(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: message}
}

func forbidden(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: message}
}

func outcomeUnknown(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.OutcomeUnknown, Message: message, Retryable: false}
}
