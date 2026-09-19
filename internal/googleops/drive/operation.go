// Package drive provides typed, read-only Google Drive MCP operations.
package drive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	defaultMaxResults = 25
	maxMaxResults     = 100
)

type SearchRequest struct {
	mcpcontract.Selection
	Text                string `json:"text" jsonschema:"Full-text search query; required"`
	MaxResults          int    `json:"max_results,omitempty" jsonschema:"Maximum results to return; default 25, maximum 100"`
	PageToken           string `json:"page_token,omitempty" jsonschema:"Opaque token returned by a previous drive_search result"`
	DriveID             string `json:"drive_id,omitempty" jsonschema:"Optional shared-drive ID"`
	IncludeSharedDrives *bool  `json:"include_shared_drives,omitempty" jsonschema:"Include shared-drive items; default true"`
}

type FileMetadata struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	MimeType     string   `json:"mime_type,omitempty"`
	Size         *int64   `json:"size,omitempty"`
	CreatedTime  string   `json:"created_time,omitempty"`
	ModifiedTime string   `json:"modified_time,omitempty"`
	Parents      []string `json:"parents,omitempty"`
	WebViewLink  string   `json:"web_view_link,omitempty"`
	Description  string   `json:"description,omitempty"`
	Starred      bool     `json:"starred,omitempty"`
}

type SearchData struct {
	Query        string         `json:"query"`
	Files        []FileMetadata `json:"files"`
	DriveID      string         `json:"drive_id,omitempty"`
	SharedDrives bool           `json:"shared_drives"`
}

type GetFileRequest struct {
	mcpcontract.Selection
	FileID string `json:"file_id" jsonschema:"Google Drive file or folder ID; required"`
}

type GetFileData struct {
	File FileMetadata `json:"file"`
}

func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("drive_search", validateSearch, func(ctx context.Context, id mcpcontract.Identity, in SearchRequest) (mcpcontract.Result[SearchData], error) {
			data, nextPageToken, err := search(ctx, provider, id, in)
			if err != nil {
				return mcpcontract.Result[SearchData]{}, err
			}
			out := mcpcontract.NewResult(id, data)
			out.NextPageToken = nextPageToken

			return out, nil
		}),
		mcpcontract.NewOperation("drive_get_file", validateGetFile, func(ctx context.Context, id mcpcontract.Identity, in GetFileRequest) (mcpcontract.Result[GetFileData], error) {
			data, err := getFile(ctx, provider, id, in)
			if err != nil {
				return mcpcontract.Result[GetFileData]{}, err
			}

			return mcpcontract.NewResult(id, data), nil
		}),
	}
}

func validateSearch(in SearchRequest) error {
	if strings.TrimSpace(in.Text) == "" {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "text is required"}
	}

	if in.MaxResults == 0 {
		return nil
	}

	if in.MaxResults < 0 || in.MaxResults > maxMaxResults {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: fmt.Sprintf("max_results must be between 1 and %d", maxMaxResults)}
	}

	return nil
}

func search(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in SearchRequest) (SearchData, string, error) {
	maxResults := in.MaxResults
	if maxResults == 0 {
		maxResults = defaultMaxResults
	}
	includeSharedDrives := in.IncludeSharedDrives == nil || *in.IncludeSharedDrives

	client, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "drive_search", Retry: mcpcontract.SafeRead})
	if err != nil {
		return SearchData{}, "", setupError("drive_search", err)
	}

	svc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return SearchData{}, "", setupError("drive_search", err)
	}

	call := svc.Files.List().
		Q(searchQuery(strings.TrimSpace(in.Text))).
		PageSize(int64(maxResults)).
		PageToken(strings.TrimSpace(in.PageToken)).
		OrderBy("modifiedTime desc").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(includeSharedDrives).
		Fields("nextPageToken, files(id, name, mimeType, size, createdTime, modifiedTime, parents, webViewLink)").
		Context(ctx)
	if includeSharedDrives {
		call = call.Corpora("allDrives")
		if in.DriveID != "" {
			call = call.DriveId(strings.TrimSpace(in.DriveID))
		}
	}

	resp, err := call.Do()
	if err != nil {
		return SearchData{}, "", mapGoogleError("drive_search", err)
	}

	data := SearchData{
		Query:        strings.TrimSpace(in.Text),
		Files:        make([]FileMetadata, 0, len(resp.Files)),
		DriveID:      strings.TrimSpace(in.DriveID),
		SharedDrives: includeSharedDrives,
	}
	for _, file := range resp.Files {
		if file != nil {
			data.Files = append(data.Files, projectFile(file))
		}
	}

	return data, resp.NextPageToken, nil
}

func validateGetFile(in GetFileRequest) error {
	if strings.TrimSpace(in.FileID) == "" {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "file_id is required"}
	}

	return nil
}

func getFile(ctx context.Context, provider mcpcontract.ClientProvider, id mcpcontract.Identity, in GetFileRequest) (GetFileData, error) {
	fileID := strings.TrimSpace(in.FileID)

	client, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "drive_get_file", Retry: mcpcontract.SafeRead})
	if err != nil {
		return GetFileData{}, setupError("drive_get_file", err)
	}

	svc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return GetFileData{}, setupError("drive_get_file", err)
	}

	file, err := svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, size, createdTime, modifiedTime, parents, webViewLink, description, starred").
		Context(ctx).
		Do()
	if err != nil {
		return GetFileData{}, mapGoogleError("drive_get_file", err)
	}

	if file == nil {
		return GetFileData{}, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "drive_get_file returned no file"}
	}

	return GetFileData{File: projectFile(file)}, nil
}

func projectFile(file *drive.File) FileMetadata {
	out := FileMetadata{
		ID:           file.Id,
		Name:         file.Name,
		MimeType:     file.MimeType,
		CreatedTime:  file.CreatedTime,
		ModifiedTime: file.ModifiedTime,
		Parents:      append([]string(nil), file.Parents...),
		WebViewLink:  file.WebViewLink,
		Description:  file.Description,
		Starred:      file.Starred,
	}
	if file.Size != 0 {
		size := file.Size
		out.Size = &size
	}

	if out.WebViewLink == "" && out.ID != "" {
		out.WebViewLink = "https://drive.google.com/file/d/" + out.ID + "/view"
	}

	return out
}

func searchQuery(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "fullText contains '" + replacer.Replace(text) + "' and trashed = false"
}

func mapGoogleError(operation string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		out := &mcpcontract.Error{Message: fmt.Sprintf("%s failed with Google status %d", operation, apiErr.Code), Retryable: apiErr.Code == 429 || apiErr.Code >= 500}
		switch apiErr.Code {
		case http.StatusBadRequest:
			out.Category = mcpcontract.InvalidInput
		case http.StatusUnauthorized:
			out.Category = mcpcontract.AuthRequired
		case http.StatusForbidden:
			out.Category = mcpcontract.Forbidden
		case http.StatusNotFound:
			out.Category = mcpcontract.NotFound
		case http.StatusTooManyRequests:
			out.Category = mcpcontract.QuotaExhausted
		default:
			out.Category = mcpcontract.UpstreamFailure
		}

		return out
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: operation + " exceeded its deadline"}
	}

	return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: operation + " failed"}
}

func setupError(operation string, err error) error {
	var public *mcpcontract.Error
	if errors.As(err, &public) {
		return public
	}

	return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: operation + " client setup failed"}
}
