// Package gmail implements the typed read-only Gmail MCP operations.
package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
	googleapi "google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	defaultSearchMaxResults = 25
	maxSearchMaxResults     = 100
	defaultBodyBytes        = 32 * 1024
	maxBodyBytes            = 256 * 1024
	defaultGetBodyBytes     = 64 * 1024
	defaultMaxMessages      = 100
	maxMaxMessages          = 100
	maxTokenLength          = 2048
	maxIdLength             = 2048
	maxQueryLength          = 8192
	messageFetchConcurrency = 10
	maxAttachments          = 25
	maxHeaderBytes          = 16 * 1024
)

var (
	errNoHTTPClient = errors.New("client provider returned no HTTP client")
	errEmptyMessage = errors.New("gmail returned an empty message")
)

type SearchInput struct {
	mcpcontract.Selection
	Query        string `json:"query" jsonschema:"Gmail search query syntax"`
	MaxResults   int    `json:"max_results,omitempty" jsonschema:"Maximum messages returned; default 25, maximum 100"`
	PageToken    string `json:"page_token,omitempty" jsonschema:"Opaque page token returned by a previous response"`
	IncludeBody  bool   `json:"include_body,omitempty" jsonschema:"Decode and return bounded message body text; default false"`
	MaxBodyBytes int    `json:"max_body_bytes,omitempty" jsonschema:"Maximum body bytes per message when include_body is true; default 32768, maximum 262144"`
}

type GetMessageInput struct {
	mcpcontract.Selection
	MessageID    string `json:"message_id"`
	IncludeBody  *bool  `json:"include_body,omitempty" jsonschema:"Return bounded MIME body text; default true"`
	MaxBodyBytes int    `json:"max_body_bytes,omitempty" jsonschema:"Maximum body bytes; default 65536, maximum 262144"`
}

type GetThreadInput struct {
	mcpcontract.Selection
	ThreadID     string `json:"thread_id"`
	MaxMessages  int    `json:"max_messages,omitempty" jsonschema:"Maximum messages returned; default 100, maximum 100"`
	MaxBodyBytes int    `json:"max_body_bytes,omitempty" jsonschema:"Maximum body bytes per message; default 32768, maximum 262144"`
}

type AttachmentView struct {
	AttachmentID string `json:"attachment_id"`
	Filename     string `json:"filename,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
}

type MessageView struct {
	ID                   string           `json:"id"`
	ThreadID             string           `json:"thread_id,omitempty"`
	InternalDate         string           `json:"internal_date,omitempty"`
	From                 string           `json:"from,omitempty"`
	To                   string           `json:"to,omitempty"`
	Cc                   string           `json:"cc,omitempty"`
	Bcc                  string           `json:"bcc,omitempty"`
	Subject              string           `json:"subject,omitempty"`
	Date                 string           `json:"date,omitempty"`
	ListUnsubscribe      string           `json:"list_unsubscribe,omitempty"`
	Labels               []string         `json:"labels"`
	Snippet              string           `json:"snippet,omitempty"`
	SizeEstimate         int64            `json:"size_estimate,omitempty"`
	Body                 string           `json:"body,omitempty"`
	BodyTruncated        bool             `json:"body_truncated,omitempty"`
	MetadataTruncated    bool             `json:"metadata_truncated,omitempty"`
	Attachments          []AttachmentView `json:"attachments,omitempty"`
	AttachmentsTruncated bool             `json:"attachments_truncated,omitempty"`
}

type SearchData struct {
	Messages           []MessageView `json:"messages"`
	ResultSizeEstimate int64         `json:"result_size_estimate,omitempty"`
}

type ThreadView struct {
	ID                string        `json:"id"`
	Snippet           string        `json:"snippet,omitempty"`
	SnippetTruncated  bool          `json:"snippet_truncated,omitempty"`
	Messages          []MessageView `json:"messages"`
	MessagesTruncated bool          `json:"messages_truncated,omitempty"`
}

type service struct {
	provider mcpcontract.ClientProvider
}

// Operations returns the three Gmail operations in public catalog order.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	s := &service{provider: provider}

	return []mcpcontract.Operation{
		s.withInputConstraints(mcpcontract.NewOperation[SearchInput, mcpcontract.Result[SearchData]](
			"gmail_search",
			validateSearch,
			s.search,
		)),
		s.withInputConstraints(mcpcontract.NewOperation[GetMessageInput, mcpcontract.Result[MessageView]](
			"gmail_get_message",
			validateGetMessage,
			s.getMessage,
		)),
		s.withInputConstraints(mcpcontract.NewOperation[GetThreadInput, mcpcontract.Result[ThreadView]](
			"gmail_get_thread",
			validateGetThread,
			s.getThread,
		)),
	}
}

func (s *service) search(ctx context.Context, id mcpcontract.Identity, in SearchInput) (mcpcontract.Result[SearchData], error) {
	maxResults := in.MaxResults
	if maxResults == 0 {
		maxResults = defaultSearchMaxResults
	}

	bodyLimit := in.MaxBodyBytes
	if bodyLimit == 0 {
		bodyLimit = defaultBodyBytes
	}

	client, err := s.httpClient(ctx, id, "gmail_search")
	if err != nil {
		return mcpcontract.Result[SearchData]{}, err
	}

	api, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return mcpcontract.Result[SearchData]{}, upstreamError("gmail", "", err)
	}

	list := api.Users.Messages.List("me").
		Q(strings.TrimSpace(in.Query)).
		MaxResults(int64(maxResults)).
		Fields("messages(id,threadId),nextPageToken,resultSizeEstimate").
		Context(ctx)
	if token := strings.TrimSpace(in.PageToken); token != "" {
		list = list.PageToken(token)
	}

	resp, err := list.Do()
	if err != nil {
		return mcpcontract.Result[SearchData]{}, mapGoogleError(err, "message list", "")
	}

	labelNames, err := fetchLabelNames(ctx, api)
	if err != nil {
		return mcpcontract.Result[SearchData]{}, err
	}

	messages, truncated, err := fetchMessageViews(ctx, api, resp.Messages, labelNames, in.IncludeBody, bodyLimit)
	if err != nil {
		return mcpcontract.Result[SearchData]{}, err
	}
	result := mcpcontract.NewResult(id, SearchData{Messages: messages, ResultSizeEstimate: resp.ResultSizeEstimate})
	result.NextPageToken = resp.NextPageToken
	result.Truncated = truncated || resp.NextPageToken != ""

	return result, nil
}

func (s *service) getMessage(ctx context.Context, id mcpcontract.Identity, in GetMessageInput) (mcpcontract.Result[MessageView], error) {
	includeBody := true
	if in.IncludeBody != nil {
		includeBody = *in.IncludeBody
	}

	bodyLimit := in.MaxBodyBytes
	if bodyLimit == 0 {
		bodyLimit = defaultGetBodyBytes
	}

	client, err := s.httpClient(ctx, id, "gmail_get_message")
	if err != nil {
		return mcpcontract.Result[MessageView]{}, err
	}

	api, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return mcpcontract.Result[MessageView]{}, upstreamError("gmail", "", err)
	}

	call := api.Users.Messages.Get("me", strings.TrimSpace(in.MessageID)).Context(ctx)
	var fieldMask string

	if includeBody {
		call = call.Format("full")
		fieldMask = "id,threadId,internalDate,labelIds,sizeEstimate,snippet,payload"
	} else {
		call = call.Format("metadata").MetadataHeaders("From", "To", "Cc", "Bcc", "Subject", "Date", "List-Unsubscribe")
		fieldMask = "id,threadId,internalDate,labelIds,sizeEstimate,payload(headers)"
	}

	msg, err := call.Fields(googleapi.Field(fieldMask)).Do()
	if err != nil {
		return mcpcontract.Result[MessageView]{}, mapGoogleError(err, "message", in.MessageID)
	}

	view, err := newMessageView(msg, map[string]string{}, includeBody, bodyLimit)
	if err != nil {
		return mcpcontract.Result[MessageView]{}, err
	}
	result := mcpcontract.NewResult(id, view)
	result.Truncated = view.BodyTruncated || view.MetadataTruncated || view.AttachmentsTruncated

	return result, nil
}

func (s *service) getThread(ctx context.Context, id mcpcontract.Identity, in GetThreadInput) (mcpcontract.Result[ThreadView], error) {
	maxMessages := in.MaxMessages
	if maxMessages == 0 {
		maxMessages = defaultMaxMessages
	}

	bodyLimit := in.MaxBodyBytes
	if bodyLimit == 0 {
		bodyLimit = defaultBodyBytes
	}

	client, err := s.httpClient(ctx, id, "gmail_get_thread")
	if err != nil {
		return mcpcontract.Result[ThreadView]{}, err
	}

	api, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return mcpcontract.Result[ThreadView]{}, upstreamError("gmail", "", err)
	}

	thread, err := api.Users.Threads.Get("me", strings.TrimSpace(in.ThreadID)).
		Format("full").
		Fields("id,snippet,messages(id,threadId,internalDate,labelIds,sizeEstimate,payload)").
		Context(ctx).
		Do()
	if err != nil {
		return mcpcontract.Result[ThreadView]{}, mapGoogleError(err, "thread", in.ThreadID)
	}

	labelNames, err := fetchLabelNames(ctx, api)
	if err != nil {
		return mcpcontract.Result[ThreadView]{}, err
	}

	messages := make([]MessageView, 0, len(thread.Messages))
	truncated := len(thread.Messages) > maxMessages

	messageLimit := len(thread.Messages)
	if truncated {
		messageLimit = maxMessages
	}

	for _, msg := range thread.Messages[:messageLimit] {
		view, err := newMessageView(msg, labelNames, true, bodyLimit)
		if err != nil {
			return mcpcontract.Result[ThreadView]{}, err
		}

		messages = append(messages, view)
		truncated = truncated || view.BodyTruncated || view.MetadataTruncated || view.AttachmentsTruncated
	}
	threadSnippet, threadSnippetTruncated := boundedBytes(thread.Snippet, maxHeaderBytes)
	result := mcpcontract.NewResult(id, ThreadView{
		ID:                thread.Id,
		Snippet:           threadSnippet,
		SnippetTruncated:  threadSnippetTruncated,
		Messages:          messages,
		MessagesTruncated: len(thread.Messages) > maxMessages,
	})
	result.Truncated = truncated || threadSnippetTruncated

	return result, nil
}

func (s *service) httpClient(ctx context.Context, id mcpcontract.Identity, operation string) (*http.Client, error) {
	if s == nil || s.provider == nil {
		return nil, invalidInput("client provider is required")
	}

	def, ok := mcpcontract.Lookup(operation)
	if !ok {
		return nil, invalidInput("unknown Gmail operation " + operation)
	}

	client, err := s.provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: def.Name, Retry: def.Retry})
	if err != nil {
		return nil, upstreamError("gmail", "", err)
	}

	if client == nil {
		return nil, upstreamError("gmail", "", errNoHTTPClient)
	}

	return client, nil
}

func validateSearch(in SearchInput) error {
	if strings.TrimSpace(in.Query) == "" {
		return invalidInput("query is required")
	}

	if len([]rune(strings.TrimSpace(in.Query))) > maxQueryLength {
		return invalidInput(fmt.Sprintf("query must be at most %d characters", maxQueryLength))
	}

	if err := validateCount("max_results", in.MaxResults, maxSearchMaxResults); err != nil {
		return err
	}

	if err := validateToken(in.PageToken); err != nil {
		return err
	}

	if err := validateBodyBytes(in.MaxBodyBytes); err != nil {
		return err
	}

	return nil
}

func validateGetMessage(in GetMessageInput) error {
	if err := validateID("message_id", in.MessageID); err != nil {
		return err
	}

	return validateBodyBytes(in.MaxBodyBytes)
}

func validateGetThread(in GetThreadInput) error {
	if err := validateID("thread_id", in.ThreadID); err != nil {
		return err
	}

	if err := validateCount("max_messages", in.MaxMessages, maxMaxMessages); err != nil {
		return err
	}

	return validateBodyBytes(in.MaxBodyBytes)
}

func validateID(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return invalidInput(name + " is required")
	}

	if len(value) > maxIdLength {
		return invalidInput(fmt.Sprintf("%s must be at most %d characters", name, maxIdLength))
	}

	return nil
}

func validateToken(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > maxTokenLength {
		return invalidInput(fmt.Sprintf("page_token must be at most %d characters", maxTokenLength))
	}

	return nil
}

func validateBodyBytes(value int) error {
	if value < 0 || value > maxBodyBytes {
		return invalidInput(fmt.Sprintf("max_body_bytes must be between 1 and %d, or omitted for the default", maxBodyBytes))
	}

	return nil
}

func validateCount(name string, value, maximum int) error {
	if value < 0 || value > maximum {
		return invalidInput(fmt.Sprintf("%s must be between 1 and %d, or omitted for the default", name, maximum))
	}

	return nil
}

func fetchLabelNames(ctx context.Context, api *gmailapi.Service) (map[string]string, error) {
	resp, err := api.Users.Labels.List("me").Fields("labels(id,name,type)").Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err, "label list", "")
	}

	names := make(map[string]string, len(resp.Labels))
	for _, label := range resp.Labels {
		if label == nil || label.Id == "" {
			continue
		}

		name := strings.TrimSpace(label.Name)
		if name == "" {
			name = label.Id
		}
		names[label.Id] = name
	}

	return names, nil
}

type messageFetchResult struct {
	index     int
	messageID string
	view      MessageView
	err       error
}

func fetchMessageViews(ctx context.Context, api *gmailapi.Service, messages []*gmailapi.Message, labelNames map[string]string, includeBody bool, bodyLimit int) ([]MessageView, bool, error) {
	if len(messages) == 0 {
		return []MessageView{}, false, nil
	}

	results := make(chan messageFetchResult, len(messages))
	var wg sync.WaitGroup
	sem := make(chan struct{}, messageFetchConcurrency)
	truncated := false

	for index, listed := range messages {
		if listed == nil || listed.Id == "" {
			continue
		}

		wg.Add(1)
		go func(index int, messageID string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- messageFetchResult{index: index, messageID: messageID, err: contextError(ctx.Err(), "message", messageID)}
				return
			}

			call := api.Users.Messages.Get("me", messageID).Context(ctx)
			var fieldMask string

			if includeBody {
				call = call.Format("full")
				fieldMask = "id,threadId,internalDate,labelIds,sizeEstimate,payload"
			} else {
				call = call.Format("metadata").
					MetadataHeaders("From", "To", "Cc", "Bcc", "Subject", "Date", "List-Unsubscribe")
				fieldMask = "id,threadId,internalDate,labelIds,sizeEstimate,payload(headers)"
			}

			msg, err := call.Fields(googleapi.Field(fieldMask)).Do()
			if err != nil {
				results <- messageFetchResult{index: index, messageID: messageID, err: mapGoogleError(err, "message", messageID)}
				return
			}

			view, err := newMessageView(msg, labelNames, includeBody, bodyLimit)
			if err != nil {
				results <- messageFetchResult{index: index, messageID: messageID, err: err}
				return
			}

			results <- messageFetchResult{index: index, messageID: messageID, view: view}
		}(index, listed.Id)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]MessageView, len(messages))

	for result := range results {
		if result.err != nil {
			return nil, false, result.err
		}

		ordered[result.index] = result.view
		if result.view.BodyTruncated || result.view.MetadataTruncated || result.view.AttachmentsTruncated {
			truncated = true
		}
	}

	items := make([]MessageView, 0, len(ordered))
	for _, view := range ordered {
		if view.ID != "" {
			items = append(items, view)
		}
	}

	return items, truncated, nil
}

func newMessageView(msg *gmailapi.Message, labelNames map[string]string, includeBody bool, bodyLimit int) (MessageView, error) {
	if msg == nil {
		return MessageView{}, upstreamError("gmail", "message", errEmptyMessage)
	}

	view := MessageView{
		ID:       msg.Id,
		ThreadID: msg.ThreadId,
		Labels:   []string{},
	}
	if msg.InternalDate != 0 {
		view.InternalDate = time.UnixMilli(msg.InternalDate).UTC().Format(time.RFC3339)
	}
	view.SizeEstimate = msg.SizeEstimate
	view.MetadataTruncated = setHeaders(&view, msg.Payload)
	view.Labels = labelViews(msg.LabelIds, labelNames)

	var attachments []AttachmentView
	var attachmentsTruncated bool
	attachments, attachmentsTruncated = attachmentViews(msg.Payload)
	view.Attachments = attachments
	view.AttachmentsTruncated = attachmentsTruncated

	if includeBody {
		body, bodyTruncated, err := boundedBody(msg.Payload, bodyLimit)
		if err != nil {
			return MessageView{}, err
		}
		view.Body = body
		view.BodyTruncated = bodyTruncated
	}

	if msg.Snippet != "" && includeBody {
		snippet := boundedString(msg.Snippet, maxHeaderBytes)

		view.Snippet = snippet
		if len(msg.Snippet) > maxHeaderBytes {
			view.MetadataTruncated = true
		}
	}

	return view, nil
}

func invalidInput(message string) error {
	return mcpcontract.Invalid(message) //nolint:wrapcheck // This is the intended public typed error.
}

func setHeaders(view *MessageView, payload *gmailapi.MessagePart) bool {
	from, fromCut := boundedHeaderValue(payload, "From")
	to, toCut := boundedHeaderValue(payload, "To")
	cc, ccCut := boundedHeaderValue(payload, "Cc")
	bcc, bccCut := boundedHeaderValue(payload, "Bcc")
	subject, subjectCut := boundedHeaderValue(payload, "Subject")
	date, dateCut := boundedHeaderValue(payload, "Date")
	unsubscribe, unsubscribeCut := boundedHeaderValue(payload, "List-Unsubscribe")
	view.From = from
	view.To = to
	view.Cc = cc
	view.Bcc = bcc
	view.Subject = subject
	view.Date = date
	view.ListUnsubscribe = unsubscribe

	return fromCut || toCut || ccCut || bccCut || subjectCut || dateCut || unsubscribeCut
}

func labelViews(ids []string, names map[string]string) []string {
	labels := make([]string, 0, len(ids))

	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}

		if _, duplicate := seen[id]; duplicate {
			continue
		}

		seen[id] = struct{}{}
		if name, ok := names[id]; ok && name != "" {
			labels = append(labels, name)
		} else {
			labels = append(labels, id)
		}
	}

	return labels
}

func attachmentViews(payload *gmailapi.MessagePart) ([]AttachmentView, bool) {
	attachments := collectAttachments(payload)
	limit := len(attachments)

	truncated := limit > maxAttachments
	if truncated {
		limit = maxAttachments
	}

	out := make([]AttachmentView, 0, limit)
	for _, attachment := range attachments[:limit] {
		out = append(out, AttachmentView{
			AttachmentID: attachment.ID,
			Filename:     boundedString(attachment.Filename, maxHeaderBytes),
			MimeType:     boundedString(attachment.MimeType, 256),
			SizeBytes:    attachment.Size,
		})
	}

	return out, truncated
}

type attachmentInfo struct {
	ID       string
	Filename string
	MimeType string
	Size     int64
}

func collectAttachments(payload *gmailapi.MessagePart) []attachmentInfo {
	if payload == nil {
		return nil
	}

	var attachments []attachmentInfo
	if payload.Body != nil && payload.Body.AttachmentId != "" {
		attachments = append(attachments, attachmentInfo{
			ID:       payload.Body.AttachmentId,
			Filename: payload.Filename,
			MimeType: payload.MimeType,
			Size:     payload.Body.Size,
		})
	}

	for _, part := range payload.Parts {
		attachments = append(attachments, collectAttachments(part)...)
	}

	return attachments
}

func (s *service) withInputConstraints(operation mcpcontract.Operation) mcpcontract.Operation {
	properties := operation.InputSchema.Properties
	setIntegerConstraint(properties, "max_results", 1, maxSearchMaxResults, defaultSearchMaxResults)
	setIntegerConstraint(properties, "max_body_bytes", 1, maxBodyBytes, defaultBodyBytes)
	setIntegerConstraint(properties, "max_messages", 1, maxMaxMessages, defaultMaxMessages)

	if property, ok := operation.InputSchema.Properties["max_body_bytes"]; ok && operation.Definition.Name == "gmail_get_message" {
		property.Default = json.RawMessage(fmt.Sprintf("%d", defaultGetBodyBytes))
	}

	if property, ok := operation.InputSchema.Properties["include_body"]; ok {
		property.Types = nil

		property.Type = "boolean"
		if operation.Definition.Name == "gmail_get_message" {
			property.Default = json.RawMessage("true")
		} else {
			property.Default = json.RawMessage("false")
		}
	}

	return operation
}

func setIntegerConstraint(properties map[string]*jsonschema.Schema, name string, minimum, maximum, defaultValue int) {
	property, ok := properties[name]
	if !ok {
		return
	}
	minimumValue := float64(minimum)
	maximumValue := float64(maximum)
	property.Minimum = &minimumValue
	property.Maximum = &maximumValue
	property.Default = json.RawMessage(fmt.Sprintf("%d", defaultValue))
}
