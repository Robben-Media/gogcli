// Package mailworkflow implements bounded Gmail composition workflows. The
// sending identity, signature, and reply headers always come from the selected
// account or its verified Gmail sendAs configuration.
package mailworkflow

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"mime"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	native "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mailcompose"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	maxGmailIDBytes = 256
	maxSubjectBytes = 255
	digestHexLength = 64
)

var messageIDPattern = regexp.MustCompile(`^<[^<>\r\n\t ]+@[^<>\r\n\t ]+>$`)

var now = func() time.Time {
	return time.Now().UTC()
}

type Content struct {
	Format mailcompose.ContentFormat `json:"format" jsonschema:"plain, html, plain_with_html_alternative, or plain_and_html_alternative; Markdown is unsupported"`
	Plain  string                    `json:"plain,omitempty"`
	HTML   string                    `json:"html,omitempty" jsonschema:"Bounded HTML subset with bounded inline CSS; includes text structure, links, tables, HTTPS or matching cid images, numeric image dimensions up to 9999, color, borders, spacing, dimensions, fonts, and text alignment. Unsafe CSS fails; unsupported safe declarations are removed with a warning"`
}

type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"bytes" jsonschema:"Decoded attachment bytes; the JSON transport encodes them as base64"`
	Disposition string `json:"disposition,omitempty" jsonschema:"attachment or inline; at most 10 inline attachments; inline requires HTML, PNG/JPEG/GIF/WebP, and content_id"`
	ContentID   string `json:"content_id,omitempty" jsonschema:"RFC Content-ID without angle brackets, at most 255 ASCII bytes, unique case-insensitively; required for inline images and referenced as cid:ID"`
}

type ReplyContext struct {
	SourceMessageID string `json:"source_message_id"`
	ThreadID        string `json:"thread_id"`
	ExpectedSubject string `json:"expected_subject"`
}

// EmailRequest contains only task data. Signature content, Date, MIME, and
// resource limits are deliberately not caller-configurable.
type EmailRequest struct {
	mcpcontract.Selection
	From             string        `json:"from,omitempty" jsonschema:"Optional verified sendAs address; defaults to the selected account address"`
	To               []string      `json:"to" jsonschema:"RFC 5322 addresses; at most 100"`
	Cc               []string      `json:"cc,omitempty" jsonschema:"RFC 5322 addresses; at most 100"`
	Bcc              []string      `json:"bcc,omitempty" jsonschema:"RFC 5322 addresses; at most 100"`
	Subject          string        `json:"subject"`
	Content          Content       `json:"content"`
	IncludeSignature bool          `json:"include_signature,omitempty"`
	Reply            *ReplyContext `json:"reply,omitempty"`
	Attachments      []Attachment  `json:"attachments,omitempty" jsonschema:"At most 25 decoded attachments totaling at most 20 MiB; also subject to server request limit, 8MiB by default including JSON/base64"`
}

type PrepareInput = EmailRequest

type DraftInput struct {
	EmailRequest
	ExpectedContentDigest string `json:"expected_content_digest,omitempty" jsonschema:"Optional stale-preview guard; checked before the write, never proof of approval or prepare"`
}

type SendInput struct {
	EmailRequest
	ExpectedContentDigest string `json:"expected_content_digest,omitempty" jsonschema:"Optional stale-preview guard; checked before the write, never proof of approval or prepare"`
}

type SenderSummary struct {
	Email              string `json:"email"`
	DisplayName        string `json:"display_name,omitempty"`
	IsPrimary          bool   `json:"is_primary"`
	VerificationStatus string `json:"verification_status,omitempty"`
	SignatureIncluded  bool   `json:"signature_included"`
}

type PreparedEmail struct {
	ContentDigest     string                          `json:"content_digest"`
	Sender            SenderSummary                   `json:"sender"`
	To                []mailcompose.Address           `json:"to"`
	Cc                []mailcompose.Address           `json:"cc,omitempty"`
	Bcc               []mailcompose.Address           `json:"bcc,omitempty"`
	Subject           string                          `json:"subject"`
	Format            mailcompose.ContentFormat       `json:"format"`
	SignatureIncluded bool                            `json:"signature_included"`
	Reply             *mailcompose.ReplySummary       `json:"reply,omitempty"`
	Attachments       []mailcompose.AttachmentSummary `json:"attachments,omitempty"`
	Preview           mailcompose.ContentPreview      `json:"preview"`
	Styles            []mailcompose.StyleDeclaration  `json:"styles,omitempty" jsonschema:"First 128 unique declarations; styles_truncated signals additional rendered styles"`
	StylesTruncated   bool                            `json:"styles_truncated,omitempty"`
	RawSize           int                             `json:"raw_size"`
	Warnings          []string                        `json:"warnings,omitempty"`
}

type DraftData struct {
	DraftID   string        `json:"draft_id"`
	MessageID string        `json:"message_id"`
	ThreadID  string        `json:"thread_id,omitempty"`
	Prepared  PreparedEmail `json:"prepared"`
}

type SentData struct {
	MessageID string        `json:"message_id"`
	ThreadID  string        `json:"thread_id,omitempty"`
	Prepared  PreparedEmail `json:"prepared"`
}

// Operations returns the approved bounded mail workflows.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("mail_compose_prepare", validatePrepare, func(ctx context.Context, id mcpcontract.Identity, in PrepareInput) (mcpcontract.Result[PreparedEmail], error) {
			return prepare(ctx, provider, id, in)
		}),
		mcpcontract.NewOperation("mail_compose_draft", validateDraft, func(ctx context.Context, id mcpcontract.Identity, in DraftInput) (mcpcontract.Result[DraftData], error) {
			return draft(ctx, provider, id, in)
		}),
		mcpcontract.NewOperation("mail_compose_send", validateSend, func(ctx context.Context, id mcpcontract.Identity, in SendInput) (mcpcontract.Result[SentData], error) {
			return send(ctx, provider, id, in)
		}),
	}
}

type resolvedEmail struct {
	reply    *mailcompose.ReplySummary
	result   mailcompose.Result
	prepared PreparedEmail
}

func prepare(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in PrepareInput) (mcpcontract.Result[PreparedEmail], error) {
	if err := validatePrepare(in); err != nil {
		return mcpcontract.Result[PreparedEmail]{}, err
	}

	resolved, err := resolvePreflight(ctx, provider, id, in, "", requiredCalls("mail_compose_prepare", in.Reply != nil))
	if err != nil {
		return mcpcontract.Result[PreparedEmail]{}, err
	}

	return mcpcontract.NewResult(id, resolved.prepared), nil
}

func draft(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in DraftInput) (mcpcontract.Result[DraftData], error) {
	if err := validateDraft(in); err != nil {
		return mcpcontract.Result[DraftData]{}, err
	}

	resolved, err := resolvePreflight(ctx, provider, id, in.EmailRequest, in.ExpectedContentDigest, requiredCalls("mail_compose_draft", in.Reply != nil))
	if err != nil {
		return mcpcontract.Result[DraftData]{}, err
	}

	api, err := service(ctx, provider, id, "mail_compose_draft")
	if err != nil {
		return mcpcontract.Result[DraftData]{}, err
	}

	created, err := api.Users.Drafts.Create("me", &gmailapi.Draft{
		Message: &gmailapi.Message{
			Raw:      resolved.result.Base64URL,
			ThreadId: replyThreadID(resolved.reply),
		},
	}).Context(ctx).Do()
	if err != nil {
		return mcpcontract.Result[DraftData]{}, native.NativeWritePublicError(err)
	}

	if created == nil || created.Message == nil || blank(created.Id) || blank(created.Message.Id) {
		return mcpcontract.Result[DraftData]{}, outcomeUnknown("Google returned no draft or message ID; reconcile before repeating")
	}

	return mcpcontract.NewResult(id, DraftData{
		DraftID:   created.Id,
		MessageID: created.Message.Id,
		ThreadID:  created.Message.ThreadId,
		Prepared:  resolved.prepared,
	}), nil
}

func send(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in SendInput) (mcpcontract.Result[SentData], error) {
	if err := validateSend(in); err != nil {
		return mcpcontract.Result[SentData]{}, err
	}

	resolved, err := resolvePreflight(ctx, provider, id, in.EmailRequest, in.ExpectedContentDigest, requiredCalls("mail_compose_send", in.Reply != nil))
	if err != nil {
		return mcpcontract.Result[SentData]{}, err
	}

	api, err := service(ctx, provider, id, "mail_compose_send")
	if err != nil {
		return mcpcontract.Result[SentData]{}, err
	}

	sent, err := api.Users.Messages.Send("me", &gmailapi.Message{
		Raw:      resolved.result.Base64URL,
		ThreadId: replyThreadID(resolved.reply),
	}).Context(ctx).Do()
	if err != nil {
		return mcpcontract.Result[SentData]{}, native.NativeWritePublicError(err)
	}

	if sent == nil || blank(sent.Id) {
		return mcpcontract.Result[SentData]{}, outcomeUnknown("Google returned no message ID; reconcile before repeating")
	}

	return mcpcontract.NewResult(id, SentData{
		MessageID: sent.Id,
		ThreadID:  sent.ThreadId,
		Prepared:  resolved.prepared,
	}), nil
}

func resolvePreflight(
	ctx context.Context,
	provider mcpcontract.ClientProvider,
	id mcpcontract.Identity,
	in EmailRequest,
	expectedDigest string,
	maxCalls int64,
) (resolvedEmail, error) {
	if err := requireBudget(ctx, maxCalls); err != nil {
		return resolvedEmail{}, err
	}

	api, err := service(ctx, provider, id, "mail_compose_prepare")
	if err != nil {
		return resolvedEmail{}, err
	}

	alias, err := resolveSender(ctx, api, id, in.From)
	if err != nil {
		return resolvedEmail{}, err
	}

	signature, err := resolveSignature(alias, in.IncludeSignature)
	if err != nil {
		return resolvedEmail{}, err
	}

	var reply *mailcompose.ReplySummary
	if in.Reply != nil {
		reply, err = resolveReply(ctx, api, in.Reply, in.Subject)
		if err != nil {
			return resolvedEmail{}, err
		}
	}

	result, err := compose(in, alias, signature, reply)
	if err != nil {
		return resolvedEmail{}, err
	}

	inlineIDs := inlineContentIDs(in.Attachments)

	plain, htmlText, styles, _, err := mailcompose.Render(mailcompose.Content{
		Format: in.Content.Format,
		Plain:  in.Content.Plain,
		HTML:   in.Content.HTML,
	}, signature, in.IncludeSignature, inlineIDs)
	if err != nil {
		return resolvedEmail{}, composeError(fmt.Errorf("render email body: %w", err))
	}

	resolved := resolvedEmail{
		reply:    reply,
		result:   result,
		prepared: newPrepared(alias, signature, reply, result, in.Attachments, plain, htmlText, styles),
	}

	if expectedDigest != "" && subtle.ConstantTimeCompare([]byte(strings.ToLower(expectedDigest)), []byte(resolved.prepared.ContentDigest)) != 1 {
		return resolvedEmail{}, invalid("content changed after prepare; the stale-preview guard rejected the write")
	}

	return resolved, nil
}

func requiredCalls(operation string, hasReply bool) int64 {
	if operation == "mail_compose_prepare" {
		if hasReply {
			return 2
		}

		return 1
	}

	if hasReply {
		return 3
	}

	return 2
}

func service(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, operation string) (*gmailapi.Service, error) {
	if provider == nil {
		return nil, invalid("client provider is required")
	}

	definition, ok := mcpcontract.Lookup(operation)
	if !ok {
		return nil, invalid("unknown mail workflow operation")
	}

	client, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: operation, Retry: definition.Retry})
	if err != nil {
		return nil, native.NativePublicError(err)
	}

	if client == nil {
		return nil, invalid("client provider returned no HTTP client")
	}

	api, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, native.NativePublicError(err)
	}

	return api, nil
}

func resolveSender(ctx context.Context, api *gmailapi.Service, id mcpcontract.Identity, requested string) (*gmailapi.SendAs, error) {
	want := strings.ToLower(strings.TrimSpace(id.Email))
	if want == "" {
		return nil, invalid("selected account has no address")
	}

	if requested != "" {
		parsed, err := mail.ParseAddress(strings.TrimSpace(requested))
		if err != nil || blank(parsed.Address) {
			return nil, invalid("invalid from address")
		}

		want = strings.ToLower(parsed.Address)
	}

	response, err := api.Users.Settings.SendAs.List("me").Context(ctx).Do()
	if err != nil {
		return nil, native.NativePublicError(err)
	}

	for _, alias := range response.SendAs {
		if alias == nil || !strings.EqualFold(strings.TrimSpace(alias.SendAsEmail), want) {
			continue
		}

		if !alias.IsPrimary && !strings.EqualFold(alias.VerificationStatus, "accepted") {
			return nil, forbidden("sending identity is not verified")
		}

		return alias, nil
	}

	return nil, forbidden("sending identity is not configured for the selected account")
}

func resolveSignature(alias *gmailapi.SendAs, include bool) (mailcompose.Signature, error) {
	if !include {
		return mailcompose.Signature{}, nil
	}

	htmlSignature := alias.Signature
	if blank(htmlSignature) {
		return mailcompose.Signature{}, invalid("verified sending identity has no HTML signature")
	}

	return mailcompose.Signature{
		Plain: mailcompose.PlainFromHTML(htmlSignature),
		HTML:  htmlSignature,
	}, nil
}

func compose(in EmailRequest, alias *gmailapi.SendAs, signature mailcompose.Signature, reply *mailcompose.ReplySummary) (mailcompose.Result, error) {
	input := mailcompose.Input{
		From:    (&mail.Address{Name: alias.DisplayName, Address: alias.SendAsEmail}).String(),
		To:      in.To,
		Cc:      in.Cc,
		Bcc:     in.Bcc,
		Subject: in.Subject,
		Content: mailcompose.Content{
			Format: in.Content.Format,
			Plain:  in.Content.Plain,
			HTML:   in.Content.HTML,
		},
		Signature:        signature,
		IncludeSignature: in.IncludeSignature,
		Attachments:      attachments(in.Attachments),
		Date:             now(),
	}

	if reply != nil {
		input.Reply = mailcompose.Reply{
			InReplyTo:  reply.InReplyTo,
			References: reply.References,
			ThreadID:   reply.ThreadID,
		}
	}

	result, err := mailcompose.Compose(input)
	if err != nil {
		return mailcompose.Result{}, composeError(fmt.Errorf("compose email: %w", err))
	}

	return result, nil
}

func attachments(in []Attachment) []mailcompose.Attachment {
	out := make([]mailcompose.Attachment, 0, len(in))
	for _, attachment := range in {
		out = append(out, mailcompose.Attachment{
			Filename:    attachment.Filename,
			ContentType: attachment.ContentType,
			Data:        attachment.Data,
			Disposition: attachment.Disposition,
			ContentID:   attachment.ContentID,
		})
	}

	return out
}

func resolveReply(ctx context.Context, api *gmailapi.Service, reply *ReplyContext, outboundSubject string) (*mailcompose.ReplySummary, error) {
	source, err := api.Users.Messages.Get("me", strings.TrimSpace(reply.SourceMessageID)).
		Format("metadata").
		MetadataHeaders("Message-ID", "References", "Subject").
		Fields("id,threadId,payload(headers)").
		Context(ctx).
		Do()
	if err != nil {
		return nil, native.NativePublicError(err)
	}

	if source == nil || source.Id != reply.SourceMessageID || source.ThreadId != reply.ThreadID {
		return nil, invalid("reply source message or thread does not match")
	}

	expected := strings.TrimSpace(reply.ExpectedSubject)
	if decodeHeader(header(source, "Subject")) != expected || strings.TrimSpace(outboundSubject) != expected {
		return nil, invalid("reply subject must match the source and outbound message")
	}

	sourceID := strings.TrimSpace(header(source, "Message-ID"))
	if !messageIDPattern.MatchString(sourceID) {
		return nil, invalid("reply source has no valid RFC 5322 Message-ID")
	}

	references, err := parseReferences(header(source, "References"))
	if err != nil {
		return nil, err
	}

	hasSource := false

	for _, reference := range references {
		if reference == sourceID {
			hasSource = true
			break
		}
	}

	if !hasSource {
		references = append(references, sourceID)
	}

	return &mailcompose.ReplySummary{
		InReplyTo:  sourceID,
		References: references,
		ThreadID:   source.ThreadId,
	}, nil
}

func parseReferences(value string) ([]string, error) {
	fields := strings.Fields(value)
	references := make([]string, 0, len(fields))

	for _, field := range fields {
		if !messageIDPattern.MatchString(field) {
			return nil, invalid("reply source contains invalid References")
		}

		references = append(references, field)
	}

	return references, nil
}

func newPrepared(alias *gmailapi.SendAs, signature mailcompose.Signature, reply *mailcompose.ReplySummary, result mailcompose.Result, inputAttachments []Attachment, plain, htmlText string, styles []mailcompose.StyleDeclaration) PreparedEmail {
	summary := result.Summary

	return PreparedEmail{
		ContentDigest: contentDigest(alias, signature, summary, reply, inputAttachments, plain, htmlText, styles),
		Sender: SenderSummary{
			Email:              alias.SendAsEmail,
			DisplayName:        alias.DisplayName,
			IsPrimary:          alias.IsPrimary,
			VerificationStatus: alias.VerificationStatus,
			SignatureIncluded:  summary.SignatureIncluded,
		},
		To:                summary.To,
		Cc:                summary.Cc,
		Bcc:               summary.Bcc,
		Subject:           summary.Subject,
		Format:            summary.Format,
		SignatureIncluded: summary.SignatureIncluded,
		Reply:             reply,
		Attachments:       summary.Attachments,
		Preview:           summary.Preview,
		Styles:            summary.Styles,
		StylesTruncated:   summary.StylesTruncated,
		RawSize:           summary.RawSize,
		Warnings:          summary.Warnings,
	}
}

func inlineContentIDs(in []Attachment) map[string]string {
	ids := make(map[string]string)

	for _, attachment := range in {
		if strings.EqualFold(strings.TrimSpace(attachment.Disposition), "inline") {
			contentID := strings.TrimSpace(attachment.ContentID)
			ids[strings.ToLower(contentID)] = contentID
		}
	}

	return ids
}

func contentDigest(alias *gmailapi.SendAs, signature mailcompose.Signature, summary mailcompose.Summary, reply *mailcompose.ReplySummary, inputAttachments []Attachment, plain, htmlText string, styles []mailcompose.StyleDeclaration) string {
	hash := sha256.New()
	writeHash(hash, "sender", alias.SendAsEmail, alias.DisplayName, fmt.Sprintf("%t", alias.IsPrimary), alias.VerificationStatus)
	writeHash(hash, "signature", fmt.Sprintf("%t", summary.SignatureIncluded), signature.Plain, signature.HTML)
	writeHash(hash, "subject", summary.Subject, string(summary.Format), plain, htmlText)

	for _, style := range styles {
		writeHash(hash, "style", style.Property, style.Value)
	}

	writeAddresses(hash, "to", summary.To)
	writeAddresses(hash, "cc", summary.Cc)
	writeAddresses(hash, "bcc", summary.Bcc)

	if reply != nil {
		writeHash(hash, "in-reply-to", reply.InReplyTo)
		writeHash(hash, "thread-id", reply.ThreadID)

		for _, reference := range reply.References {
			writeHash(hash, "reference", reference)
		}
	}

	for _, attachment := range summary.Attachments {
		writeHash(hash, "attachment", attachment.Filename, attachment.ContentType, fmt.Sprintf("%d", attachment.Size), attachment.Disposition, attachment.ContentID)
	}

	for _, attachment := range inputAttachments {
		digest := sha256.Sum256(attachment.Data)
		writeHash(hash, "attachment-bytes", attachment.ContentID, hex.EncodeToString(digest[:]))
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func writeHash(hash interface{ Write([]byte) (int, error) }, fields ...string) {
	for _, field := range fields {
		_, _ = fmt.Fprintf(hash, "%d:%s\n", len(field), field)
	}
}

func writeAddresses(hash interface{ Write([]byte) (int, error) }, field string, addresses []mailcompose.Address) {
	for _, address := range addresses {
		writeHash(hash, field, address.Name, address.Email)
	}
}

func replyThreadID(reply *mailcompose.ReplySummary) string {
	if reply == nil {
		return ""
	}

	return reply.ThreadID
}

func header(message *gmailapi.Message, name string) string {
	if message == nil || message.Payload == nil {
		return ""
	}

	for _, candidate := range message.Payload.Headers {
		if strings.EqualFold(candidate.Name, name) {
			return candidate.Value
		}
	}

	return ""
}

func decodeHeader(value string) string {
	decoder := mime.WordDecoder{CharsetReader: charset.NewReaderLabel}

	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return strings.TrimSpace(value)
	}

	return strings.TrimSpace(decoded)
}

func requireBudget(ctx context.Context, calls int64) error {
	if remaining := native.UpstreamBudgetRemaining(ctx); remaining >= 0 && remaining < calls {
		return &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "insufficient remaining API budget for bounded mail workflow", Retryable: false}
	}

	return nil
}

func blank(value string) bool {
	return strings.TrimSpace(value) == ""
}
