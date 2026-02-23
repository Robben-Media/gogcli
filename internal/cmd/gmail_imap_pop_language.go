package cmd

import (
	"context"
	"errors"
	"os"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailImapCmd handles IMAP settings operations.
type GmailImapCmd struct {
	Get    GmailImapGetCmd    `cmd:"" name:"get" help:"Get current IMAP settings"`
	Update GmailImapUpdateCmd `cmd:"" name:"update" help:"Update IMAP settings"`
}

// GmailPopCmd handles POP settings operations.
type GmailPopCmd struct {
	Get    GmailPopGetCmd    `cmd:"" name:"get" help:"Get current POP settings"`
	Update GmailPopUpdateCmd `cmd:"" name:"update" help:"Update POP settings"`
}

// GmailLanguageCmd handles language settings operations.
type GmailLanguageCmd struct {
	Get    GmailLanguageGetCmd    `cmd:"" name:"get" help:"Get current display language"`
	Update GmailLanguageUpdateCmd `cmd:"" name:"update" help:"Update display language"`
}

// IMAP Commands

type GmailImapGetCmd struct{}

func (c *GmailImapGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	imap, err := svc.Users.Settings.GetImap("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"imap": imap})
	}

	u.Out().Printf("enabled\t%t", imap.Enabled)
	u.Out().Printf("auto_expunge\t%t", imap.AutoExpunge)
	if imap.ExpungeBehavior != "" {
		u.Out().Printf("expunge_behavior\t%s", imap.ExpungeBehavior)
	}
	if imap.MaxFolderSize > 0 {
		u.Out().Printf("max_folder_size\t%d", imap.MaxFolderSize)
	}
	return nil
}

type GmailImapUpdateCmd struct {
	Enable          bool   `name:"enable" help:"Enable IMAP access"`
	Disable         bool   `name:"disable" help:"Disable IMAP access"`
	AutoExpunge     bool   `name:"auto-expunge" help:"Immediately expunge messages when marked as deleted"`
	NoAutoExpunge   bool   `name:"no-auto-expunge" help:"Wait for client update before expunging deleted messages"`
	ExpungeBehavior string `name:"expunge-behavior" help:"Action for expunged messages: archive, trash, deleteForever"`
	MaxFolderSize   int64  `name:"max-folder-size" help:"Max messages per IMAP folder (0, 1000, 2000, 5000, or 10000)"`
}

func (c *GmailImapUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if c.Enable && c.Disable {
		return errors.New("cannot specify both --enable and --disable")
	}

	if c.AutoExpunge && c.NoAutoExpunge {
		return errors.New("cannot specify both --auto-expunge and --no-auto-expunge")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Get current settings first
	current, err := svc.Users.Settings.GetImap("me").Do()
	if err != nil {
		return err
	}

	// Build update request, preserving existing values if not specified
	imap := &gmail.ImapSettings{
		Enabled:         current.Enabled,
		AutoExpunge:     current.AutoExpunge,
		ExpungeBehavior: current.ExpungeBehavior,
		MaxFolderSize:   current.MaxFolderSize,
	}

	// Apply flags
	if c.Enable {
		imap.Enabled = true
	}
	if c.Disable {
		imap.Enabled = false
	}
	if c.AutoExpunge {
		imap.AutoExpunge = true
	}
	if c.NoAutoExpunge {
		imap.AutoExpunge = false
	}
	if flagProvided(kctx, "expunge-behavior") {
		validExpunge := map[string]bool{
			"archive":       true,
			"trash":         true,
			"deleteForever": true,
		}
		if !validExpunge[c.ExpungeBehavior] {
			return errors.New("invalid expunge-behavior value; must be one of: archive, trash, deleteForever")
		}
		imap.ExpungeBehavior = c.ExpungeBehavior
	}
	if flagProvided(kctx, "max-folder-size") {
		validSizes := map[int64]bool{0: true, 1000: true, 2000: true, 5000: true, 10000: true}
		if !validSizes[c.MaxFolderSize] {
			return errors.New("invalid max-folder-size value; must be one of: 0, 1000, 2000, 5000, 10000")
		}
		imap.MaxFolderSize = c.MaxFolderSize
	}

	updated, err := svc.Users.Settings.UpdateImap("me", imap).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"imap": updated})
	}

	u.Out().Println("IMAP settings updated successfully")
	u.Out().Printf("enabled\t%t", updated.Enabled)
	u.Out().Printf("auto_expunge\t%t", updated.AutoExpunge)
	if updated.ExpungeBehavior != "" {
		u.Out().Printf("expunge_behavior\t%s", updated.ExpungeBehavior)
	}
	if updated.MaxFolderSize > 0 {
		u.Out().Printf("max_folder_size\t%d", updated.MaxFolderSize)
	}
	return nil
}

// POP Commands

type GmailPopGetCmd struct{}

func (c *GmailPopGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	pop, err := svc.Users.Settings.GetPop("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"pop": pop})
	}

	u.Out().Printf("access_window\t%s", pop.AccessWindow)
	if pop.Disposition != "" {
		u.Out().Printf("disposition\t%s", pop.Disposition)
	}
	return nil
}

type GmailPopUpdateCmd struct {
	AccessWindow string `name:"access-window" help:"Access window: disabled, allMail, fromNowOn"`
	Disposition  string `name:"disposition" help:"Disposition for retrieved messages: leaveInInbox, archive, trash, markRead"`
}

func (c *GmailPopUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Get current settings first
	current, err := svc.Users.Settings.GetPop("me").Do()
	if err != nil {
		return err
	}

	// Build update request, preserving existing values if not specified
	pop := &gmail.PopSettings{
		AccessWindow: current.AccessWindow,
		Disposition:  current.Disposition,
	}

	// Apply flags
	if flagProvided(kctx, "access-window") {
		validAccessWindow := map[string]bool{
			"disabled":  true,
			"allMail":   true,
			"fromNowOn": true,
		}
		if !validAccessWindow[c.AccessWindow] {
			return errors.New("invalid access-window value; must be one of: disabled, allMail, fromNowOn")
		}
		pop.AccessWindow = c.AccessWindow
	}
	if flagProvided(kctx, "disposition") {
		validDisposition := map[string]bool{
			"leaveInInbox": true,
			"archive":      true,
			"trash":        true,
			"markRead":     true,
		}
		if !validDisposition[c.Disposition] {
			return errors.New("invalid disposition value; must be one of: leaveInInbox, archive, trash, markRead")
		}
		pop.Disposition = c.Disposition
	}

	updated, err := svc.Users.Settings.UpdatePop("me", pop).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"pop": updated})
	}

	u.Out().Println("POP settings updated successfully")
	u.Out().Printf("access_window\t%s", updated.AccessWindow)
	if updated.Disposition != "" {
		u.Out().Printf("disposition\t%s", updated.Disposition)
	}
	return nil
}

// Language Commands

type GmailLanguageGetCmd struct{}

func (c *GmailLanguageGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	lang, err := svc.Users.Settings.GetLanguage("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"language": lang})
	}

	u.Out().Printf("display_language\t%s", lang.DisplayLanguage)
	return nil
}

type GmailLanguageUpdateCmd struct {
	DisplayLanguage string `name:"display-language" help:"Display language code (e.g., en, es, fr, de, ja)"`
}

func (c *GmailLanguageUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if c.DisplayLanguage == "" {
		return errors.New("--display-language is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	lang := &gmail.LanguageSettings{
		DisplayLanguage: c.DisplayLanguage,
	}

	updated, err := svc.Users.Settings.UpdateLanguage("me", lang).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"language": updated})
	}

	u.Out().Println("Language settings updated successfully")
	u.Out().Printf("display_language\t%s", updated.DisplayLanguage)
	return nil
}
