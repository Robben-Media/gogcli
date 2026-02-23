package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DriveRevisionsCmd is the parent command for revisions subcommands
type DriveRevisionsCmd struct {
	List   DriveRevisionsListCmd   `cmd:"" name:"list" help:"List revisions of a file"`
	Get    DriveRevisionsGetCmd    `cmd:"" name:"get" help:"Get a specific revision"`
	Delete DriveRevisionsDeleteCmd `cmd:"" name:"delete" help:"Delete a revision"`
	Update DriveRevisionsUpdateCmd `cmd:"" name:"update" help:"Update a revision"`
}

// DriveRevisionsListCmd lists revisions of a file
type DriveRevisionsListCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page   string `name:"page" help:"Page token"`
}

func (c *DriveRevisionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Revisions.List(fileID).
		Fields("nextPageToken, revisions(id, mimeType, modifiedTime, keepForever, published, publishedLink, size)").
		Context(ctx)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if strings.TrimSpace(c.Page) != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"fileId":        fileID,
			"revisions":     resp.Revisions,
			"revisionCount": len(resp.Revisions),
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Revisions) == 0 {
		u.Err().Println("No revisions")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tMODIFIED\tSIZE\tKEPT\tPUBLISHED")
	for _, r := range resp.Revisions {
		kept := "no"
		if r.KeepForever {
			kept = "yes"
		}
		published := "no"
		if r.Published {
			published = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.Id,
			formatDateTime(r.ModifiedTime),
			formatDriveSize(r.Size),
			kept,
			published,
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// DriveRevisionsGetCmd gets a specific revision
type DriveRevisionsGetCmd struct {
	FileID     string `arg:"" name:"fileId" help:"File ID"`
	RevisionID string `arg:"" name:"revisionId" help:"Revision ID"`
	Download   bool   `name:"download" help:"Download the revision content"`
	Output     string `name:"output" short:"o" help:"Output file path (for download)"`
}

func (c *DriveRevisionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	revisionID := strings.TrimSpace(c.RevisionID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if revisionID == "" {
		return usage("empty revisionId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	// Get revision metadata
	rev, err := svc.Revisions.Get(fileID, revisionID).
		Fields("id, mimeType, modifiedTime, keepForever, published, publishedLink, size, exportLinks").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	// If download requested, download the content
	if c.Download {
		return downloadRevision(ctx, svc, fileID, revisionID, rev, c.Output)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"revision": rev})
	}

	u.Out().Printf("id\t%s", rev.Id)
	u.Out().Printf("modified\t%s", rev.ModifiedTime)
	u.Out().Printf("size\t%s", formatDriveSize(rev.Size))
	u.Out().Printf("mime_type\t%s", rev.MimeType)
	u.Out().Printf("keep_forever\t%t", rev.KeepForever)
	u.Out().Printf("published\t%t", rev.Published)
	if rev.PublishedLink != "" {
		u.Out().Printf("published_link\t%s", rev.PublishedLink)
	}
	return nil
}

func downloadRevision(ctx context.Context, svc *drive.Service, fileID, revisionID string, rev *drive.Revision, outputPath string) error {
	u := ui.FromContext(ctx)

	// Download the revision
	resp, err := svc.Revisions.Get(fileID, revisionID).Context(ctx).Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Determine output path
	if outputPath == "" {
		outputPath = fmt.Sprintf("%s-revision-%s", fileID, revisionID)
	}

	// Create output file
	// #nosec G304 -- User-provided output path for downloaded revision
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Copy content
	size, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"path":       outputPath,
			"size":       size,
			"revisionId": revisionID,
			"mimeType":   rev.MimeType,
		})
	}

	u.Out().Printf("path\t%s", outputPath)
	u.Out().Printf("size\t%s", formatDriveSize(size))
	return nil
}

// DriveRevisionsDeleteCmd deletes a revision
type DriveRevisionsDeleteCmd struct {
	FileID     string `arg:"" name:"fileId" help:"File ID"`
	RevisionID string `arg:"" name:"revisionId" help:"Revision ID"`
}

func (c *DriveRevisionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	revisionID := strings.TrimSpace(c.RevisionID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if revisionID == "" {
		return usage("empty revisionId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete revision %s of file %s", revisionID, fileID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Revisions.Delete(fileID, revisionID).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":    true,
			"fileId":     fileID,
			"revisionId": revisionID,
		})
	}

	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("file_id\t%s", fileID)
	u.Out().Printf("revision_id\t%s", revisionID)
	return nil
}

// DriveRevisionsUpdateCmd updates a revision (mainly keepForever and published settings)
type DriveRevisionsUpdateCmd struct {
	FileID      string `arg:"" name:"fileId" help:"File ID"`
	RevisionID  string `arg:"" name:"revisionId" help:"Revision ID"`
	KeepForever *bool  `name:"keep-forever" help:"Keep this revision forever (prevents automatic purge)"`
	Publish     *bool  `name:"publish" help:"Publish this revision"`
}

func (c *DriveRevisionsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	revisionID := strings.TrimSpace(c.RevisionID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if revisionID == "" {
		return usage("empty revisionId")
	}

	// Check if at least one update field is provided
	if c.KeepForever == nil && c.Publish == nil {
		return usage("at least one of --keep-forever or --publish is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	rev := &drive.Revision{}
	if c.KeepForever != nil {
		rev.KeepForever = *c.KeepForever
		rev.ForceSendFields = append(rev.ForceSendFields, "KeepForever")
	}
	if c.Publish != nil {
		rev.Published = *c.Publish
		rev.ForceSendFields = append(rev.ForceSendFields, "Published")
	}

	updated, err := svc.Revisions.Update(fileID, revisionID, rev).
		Fields("id, mimeType, modifiedTime, keepForever, published, publishedLink, size").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"revision": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("modified\t%s", updated.ModifiedTime)
	u.Out().Printf("keep_forever\t%t", updated.KeepForever)
	u.Out().Printf("published\t%t", updated.Published)
	if updated.PublishedLink != "" {
		u.Out().Printf("published_link\t%s", updated.PublishedLink)
	}
	return nil
}
