package mailcompose

// ValidateSemantic checks reusable message semantics without a date, sender, or
// signature. It is suitable for callers that resolve those values separately
// and for rejecting malformed input before provider access.
func ValidateSemantic(in Input) error {
	limits, err := effectiveLimits(in.Limits)
	if err != nil {
		return err
	}

	to, cc, bcc, parseErr := parseRecipientGroups(in, limits.MaxRecipientsPerField)
	if parseErr != nil {
		return parseErr
	}

	_, _, _ = to, cc, bcc

	if _, subjectErr := validateSubject(in.Subject); subjectErr != nil {
		return subjectErr
	}

	content, contentErr := normalizeContent(in.Content)
	if contentErr != nil {
		return contentErr
	}

	sourceSize := len(content.Plain) + len(content.HTML)
	if sourceSize > limits.MaxBodyBytes {
		return validationError("body is %d bytes, limit is %d", sourceSize, limits.MaxBodyBytes)
	}

	plain, htmlText, _, renderErr := renderContent(content, Signature{}, false)
	if renderErr != nil {
		return renderErr
	}

	if bodySize := len(plain) + len(htmlText); bodySize > limits.MaxBodyBytes {
		return validationError("body is %d bytes, limit is %d", bodySize, limits.MaxBodyBytes)
	}

	_, attachmentErr := validateAttachments(in.Attachments, limits)

	return attachmentErr
}
