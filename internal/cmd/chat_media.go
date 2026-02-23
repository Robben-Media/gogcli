package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/chat/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ChatMediaCmd contains subcommands for media management.
type ChatMediaCmd struct {
	Upload   ChatMediaUploadCmd   `cmd:"" name:"upload" help:"Upload a file attachment"`
	Download ChatMediaDownloadCmd `cmd:"" name:"download" help:"Download a media attachment"`
}

// ChatMediaUploadCmd uploads a media file to a Chat space.
type ChatMediaUploadCmd struct {
	Space string `arg:"" name:"space" help:"Space name (spaces/...)"`
	File  string `name:"file" short:"f" help:"Path to file to upload (use - for stdin)"`
	Name  string `name:"name" help:"Filename for the attachment (default: derived from --file)"`
}

func (c *ChatMediaUploadCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	space, err := normalizeSpace(c.Space)
	if err != nil {
		return usage("required: space")
	}

	filePath := strings.TrimSpace(c.File)
	if filePath == "" {
		return usage("required: --file")
	}

	// Determine filename
	filename := strings.TrimSpace(c.Name)
	if filename == "" {
		if filePath == "-" {
			return usage("required: --name when reading from stdin")
		}
		filename = filepath.Base(filePath)
	}

	// Read the file
	var fileData io.Reader
	var size int64
	if filePath == "-" {
		// For stdin, we'll need to buffer to get size
		stdinData, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return fmt.Errorf("reading stdin: %w", readErr)
		}
		fileData = strings.NewReader(string(stdinData))
		size = int64(len(stdinData))
	} else {
		filePath, err = config.ExpandPath(filePath)
		if err != nil {
			return fmt.Errorf("expanding file path: %w", err)
		}
		f, openErr := os.Open(filePath) //nolint:gosec // user-provided path
		if openErr != nil {
			return fmt.Errorf("opening file: %w", openErr)
		}
		defer f.Close()
		stat, statErr := f.Stat()
		if statErr != nil {
			return fmt.Errorf("getting file info: %w", statErr)
		}
		fileData = f
		size = stat.Size()
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	// Build the upload request
	req := &chat.UploadAttachmentRequest{
		Filename: filename,
	}

	// Determine content type
	mimeType := guessMimeType(filename)

	// Upload the file
	resp, err := svc.Media.Upload(space, req).
		Media(fileData, gapi.ContentType(mimeType), gapi.ChunkSize(1024*1024)).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"attachmentDataRef": resp.AttachmentDataRef,
			"filename":          filename,
		})
	}

	if resp.AttachmentDataRef != nil {
		if resp.AttachmentDataRef.ResourceName != "" {
			u.Out().Printf("resource\t%s", resp.AttachmentDataRef.ResourceName)
		}
		if resp.AttachmentDataRef.AttachmentUploadToken != "" {
			u.Out().Printf("uploadToken\t%s", resp.AttachmentDataRef.AttachmentUploadToken)
		}
	}
	u.Out().Printf("filename\t%s", filename)
	u.Out().Printf("size\t%s", formatDriveSize(size))
	return nil
}

// ChatMediaDownloadCmd downloads a media attachment.
type ChatMediaDownloadCmd struct {
	Resource string         `arg:"" name:"resource" help:"Media resource name (media/...)"`
	Output   OutputPathFlag `embed:""`
}

func (c *ChatMediaDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	resource := strings.TrimSpace(c.Resource)
	if resource == "" {
		return usage("required: resource")
	}

	// Normalize resource name if needed
	if !strings.HasPrefix(resource, "media/") {
		resource = "media/" + resource
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	// Download the media
	resp, err := svc.Media.Download(resource).Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	// Determine output path
	destPath := strings.TrimSpace(c.Output.Path)
	if destPath == "" {
		// Extract filename from resource name or use default
		parts := strings.Split(resource, "/")
		if len(parts) > 1 {
			destPath = parts[len(parts)-1]
		} else {
			destPath = "download"
		}
	}

	destPath, err = config.ExpandPath(destPath)
	if err != nil {
		return fmt.Errorf("expanding output path: %w", err)
	}

	// Create the output file
	f, err := os.Create(destPath) //nolint:gosec // user-provided path
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	// Copy the response body to the file
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"resource": resource,
			"path":     destPath,
			"size":     n,
		})
	}

	u.Out().Printf("Downloaded %s to %s (%s)", resource, destPath, formatDriveSize(n))
	return nil
}
