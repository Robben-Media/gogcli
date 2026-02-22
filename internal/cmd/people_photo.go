package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ContactsPhotoCmd contains subcommands for contact photo management.
type ContactsPhotoCmd struct {
	Delete ContactsPhotoDeleteCmd `cmd:"" name:"delete" help:"Delete a contact's photo"`
	Update ContactsPhotoUpdateCmd `cmd:"" name:"update" help:"Update a contact's photo"`
}

// ContactsPhotoDeleteCmd deletes a contact's photo.
type ContactsPhotoDeleteCmd struct {
	ResourceName string `arg:"" name:"resourceName" help:"Resource name of the contact (people/...)"`
}

func (c *ContactsPhotoDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceName := strings.TrimSpace(c.ResourceName)
	if resourceName == "" {
		return usage("resource name is required")
	}

	// Normalize resource name if needed
	if !strings.HasPrefix(resourceName, "people/") {
		resourceName = "people/" + resourceName
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete photo for contact %s", resourceName)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.People.DeleteContactPhoto(resourceName).Do()
	if err != nil {
		return err
	}

	return writeDeleteResult(ctx, u, fmt.Sprintf("photo for contact %s", resourceName))
}

// ContactsPhotoUpdateCmd updates a contact's photo from an image file.
type ContactsPhotoUpdateCmd struct {
	ResourceName string `arg:"" name:"resourceName" help:"Resource name of the contact (people/...)"`
	File         string `name:"file" short:"f" help:"Path to image file (use - for stdin)"`
}

func (c *ContactsPhotoUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceName := strings.TrimSpace(c.ResourceName)
	if resourceName == "" {
		return usage("resource name is required")
	}

	// Normalize resource name if needed
	if !strings.HasPrefix(resourceName, "people/") {
		resourceName = "people/" + resourceName
	}

	filePath := strings.TrimSpace(c.File)
	if filePath == "" {
		return usage("--file is required")
	}

	// Read the image file
	var imageData []byte
	if filePath == "-" {
		imageData, err = io.ReadAll(os.Stdin)
	} else {
		filePath, err = config.ExpandPath(filePath)
		if err != nil {
			return fmt.Errorf("expanding file path: %w", err)
		}
		imageData, err = os.ReadFile(filePath) //nolint:gosec // user-provided path
	}
	if err != nil {
		return fmt.Errorf("reading image file: %w", err)
	}

	if len(imageData) == 0 {
		return usage("image file is empty")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	// Build the update request with base64-encoded photo bytes
	req := &people.UpdateContactPhotoRequest{
		PhotoBytes: base64.StdEncoding.EncodeToString(imageData),
	}

	resp, err := svc.People.UpdateContactPhoto(resourceName, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"person":       resp.Person,
			"resourceName": resourceName,
		})
	}

	u.Out().Printf("Updated photo for %s", resourceName)
	return nil
}
