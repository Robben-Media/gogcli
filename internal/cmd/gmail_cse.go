package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailCseCmd handles CSE (Client-Side Encryption) operations.
// CSE is an enterprise-only feature for Gmail.
type GmailCseCmd struct {
	Identities GmailCseIdentitiesCmd `cmd:"" name:"identities" help:"CSE identity operations"`
	Keypairs   GmailCseKeypairsCmd   `cmd:"" name:"keypairs" help:"CSE key pair operations"`
}

// CSE Identities Commands

type GmailCseIdentitiesCmd struct {
	List   GmailCseIdentitiesListCmd   `cmd:"" name:"list" help:"List all CSE identities"`
	Get    GmailCseIdentitiesGetCmd    `cmd:"" name:"get" help:"Get a specific CSE identity"`
	Create GmailCseIdentitiesCreateCmd `cmd:"" name:"create" help:"Create a CSE identity"`
	Delete GmailCseIdentitiesDeleteCmd `cmd:"" name:"delete" help:"Delete a CSE identity"`
	Patch  GmailCseIdentitiesPatchCmd  `cmd:"" name:"patch" help:"Patch a CSE identity"`
}

type GmailCseIdentitiesListCmd struct{}

func (c *GmailCseIdentitiesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Cse.Identities.List("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseIdentities": resp.CseIdentities})
	}

	if len(resp.CseIdentities) == 0 {
		u.Err().Println("No CSE identities")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "EMAIL\tPRIMARY KEYPAIR ID")
	for _, id := range resp.CseIdentities {
		fmt.Fprintf(tw, "%s\t%s\n",
			id.EmailAddress,
			id.PrimaryKeyPairId)
	}
	_ = tw.Flush()
	return nil
}

type GmailCseIdentitiesGetCmd struct {
	Email string `arg:"" name:"email" help:"Email address of the CSE identity"`
}

func (c *GmailCseIdentitiesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return errors.New("email is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	identity, err := svc.Users.Settings.Cse.Identities.Get("me", email).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseIdentity": identity})
	}

	u.Out().Printf("email_address\t%s", identity.EmailAddress)
	u.Out().Printf("primary_keypair_id\t%s", identity.PrimaryKeyPairId)
	if identity.SignAndEncryptKeyPairs != nil {
		u.Out().Printf("signing_keypair_id\t%s", identity.SignAndEncryptKeyPairs.SigningKeyPairId)
		u.Out().Printf("encryption_keypair_id\t%s", identity.SignAndEncryptKeyPairs.EncryptionKeyPairId)
	}
	return nil
}

type GmailCseIdentitiesCreateCmd struct {
	Email               string `arg:"" name:"email" help:"Email address for the CSE identity"`
	PrimaryKeyPairID    string `name:"primary-keypair-id" help:"ID of the primary key pair to associate"`
	SigningKeyPairID    string `name:"signing-keypair-id" help:"ID of the signing key pair (for sign-and-encrypt mode)"`
	EncryptionKeyPairID string `name:"encryption-keypair-id" help:"ID of the encryption key pair (for sign-and-encrypt mode)"`
}

func (c *GmailCseIdentitiesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return errors.New("email is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	identity := &gmail.CseIdentity{
		EmailAddress:     email,
		PrimaryKeyPairId: c.PrimaryKeyPairID,
	}

	// Handle sign-and-encrypt key pairs if specified
	if c.SigningKeyPairID != "" || c.EncryptionKeyPairID != "" {
		if c.SigningKeyPairID == "" || c.EncryptionKeyPairID == "" {
			return errors.New("both --signing-keypair-id and --encryption-keypair-id must be specified together for sign-and-encrypt mode")
		}
		identity.SignAndEncryptKeyPairs = &gmail.SignAndEncryptKeyPairs{
			SigningKeyPairId:    c.SigningKeyPairID,
			EncryptionKeyPairId: c.EncryptionKeyPairID,
		}
	}

	created, err := svc.Users.Settings.Cse.Identities.Create("me", identity).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseIdentity": created})
	}

	u.Out().Println("CSE identity created successfully")
	u.Out().Printf("email_address\t%s", created.EmailAddress)
	u.Out().Printf("primary_keypair_id\t%s", created.PrimaryKeyPairId)
	return nil
}

type GmailCseIdentitiesDeleteCmd struct {
	Email string `arg:"" name:"email" help:"Email address of the CSE identity to delete"`
}

func (c *GmailCseIdentitiesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return errors.New("email is required")
	}

	if !flags.Force {
		if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("Delete CSE identity for %s?", email)); confirmErr != nil {
			return confirmErr
		}
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Settings.Cse.Identities.Delete("me", email).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"success": true,
			"email":   email,
		})
	}

	u.Out().Printf("CSE identity deleted: %s", email)
	return nil
}

type GmailCseIdentitiesPatchCmd struct {
	Email               string `arg:"" name:"email" help:"Email address of the CSE identity"`
	PrimaryKeyPairID    string `name:"primary-keypair-id" help:"ID of the primary key pair to associate"`
	SigningKeyPairID    string `name:"signing-keypair-id" help:"ID of the signing key pair (for sign-and-encrypt mode)"`
	EncryptionKeyPairID string `name:"encryption-keypair-id" help:"ID of the encryption key pair (for sign-and-encrypt mode)"`
}

func (c *GmailCseIdentitiesPatchCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return errors.New("email is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Get current identity first
	current, err := svc.Users.Settings.Cse.Identities.Get("me", email).Do()
	if err != nil {
		return err
	}

	// Build update request
	identity := &gmail.CseIdentity{
		EmailAddress:     email,
		PrimaryKeyPairId: current.PrimaryKeyPairId,
	}

	// Apply patches
	if flagProvided(kctx, "primary-keypair-id") {
		identity.PrimaryKeyPairId = c.PrimaryKeyPairID
	}

	// Handle sign-and-encrypt key pairs
	if flagProvided(kctx, "signing-keypair-id") || flagProvided(kctx, "encryption-keypair-id") {
		if c.SigningKeyPairID == "" || c.EncryptionKeyPairID == "" {
			return errors.New("both --signing-keypair-id and --encryption-keypair-id must be specified together for sign-and-encrypt mode")
		}
		identity.SignAndEncryptKeyPairs = &gmail.SignAndEncryptKeyPairs{
			SigningKeyPairId:    c.SigningKeyPairID,
			EncryptionKeyPairId: c.EncryptionKeyPairID,
		}
	} else if current.SignAndEncryptKeyPairs != nil {
		identity.SignAndEncryptKeyPairs = current.SignAndEncryptKeyPairs
	}

	updated, err := svc.Users.Settings.Cse.Identities.Patch("me", email, identity).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseIdentity": updated})
	}

	u.Out().Println("CSE identity patched successfully")
	u.Out().Printf("email_address\t%s", updated.EmailAddress)
	u.Out().Printf("primary_keypair_id\t%s", updated.PrimaryKeyPairId)
	return nil
}

// CSE Key Pairs Commands

type GmailCseKeypairsCmd struct {
	List       GmailCseKeypairsListCmd       `cmd:"" name:"list" help:"List all CSE key pairs"`
	Get        GmailCseKeypairsGetCmd        `cmd:"" name:"get" help:"Get a specific CSE key pair"`
	Create     GmailCseKeypairsCreateCmd     `cmd:"" name:"create" help:"Create a CSE key pair"`
	Enable     GmailCseKeypairsEnableCmd     `cmd:"" name:"enable" help:"Enable a CSE key pair"`
	Disable    GmailCseKeypairsDisableCmd    `cmd:"" name:"disable" help:"Disable a CSE key pair"`
	Obliterate GmailCseKeypairsObliterateCmd `cmd:"" name:"obliterate" help:"Permanently delete a CSE key pair"`
}

type GmailCseKeypairsListCmd struct{}

func (c *GmailCseKeypairsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Cse.Keypairs.List("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseKeyPairs": resp.CseKeyPairs})
	}

	if len(resp.CseKeyPairs) == 0 {
		u.Err().Println("No CSE key pairs")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY PAIR ID\tSTATE\tSUBJECT EMAILS")
	for _, kp := range resp.CseKeyPairs {
		emails := strings.Join(kp.SubjectEmailAddresses, ",")
		if len(emails) > 40 {
			emails = emails[:37] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			kp.KeyPairId,
			kp.EnablementState,
			emails)
	}
	_ = tw.Flush()
	return nil
}

type GmailCseKeypairsGetCmd struct {
	KeyPairID string `arg:"" name:"keyPairId" help:"ID of the CSE key pair"`
}

func (c *GmailCseKeypairsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	keyPairID := strings.TrimSpace(c.KeyPairID)
	if keyPairID == "" {
		return errors.New("keyPairId is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	kp, err := svc.Users.Settings.Cse.Keypairs.Get("me", keyPairID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseKeyPair": kp})
	}

	u.Out().Printf("key_pair_id\t%s", kp.KeyPairId)
	u.Out().Printf("enablement_state\t%s", kp.EnablementState)
	if kp.DisableTime != "" {
		u.Out().Printf("disable_time\t%s", kp.DisableTime)
	}
	u.Out().Printf("subject_emails\t%s", strings.Join(kp.SubjectEmailAddresses, ","))
	if kp.Pem != "" {
		// Truncate PEM for text output
		pemPreview := kp.Pem
		if len(pemPreview) > 100 {
			pemPreview = pemPreview[:97] + "..."
		}
		u.Out().Printf("pem_preview\t%s", pemPreview)
	}
	return nil
}

type GmailCseKeypairsCreateCmd struct {
	Pkcs7 string `name:"pkcs7" help:"PKCS#7 formatted public key and certificate chain (PEM encoded)"`
	File  string `name:"file" short:"f" help:"File containing PKCS#7 data (use - for stdin)"`
}

func (c *GmailCseKeypairsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	pkcs7 := c.Pkcs7
	if c.File != "" {
		var data []byte
		if c.File == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(c.File)
		}
		if err != nil {
			return fmt.Errorf("reading pkcs7 file: %w", err)
		}
		pkcs7 = string(data)
	}

	pkcs7 = strings.TrimSpace(pkcs7)
	if pkcs7 == "" {
		return errors.New("PKCS#7 data is required (use --pkcs7 or --file)")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	keypair := &gmail.CseKeyPair{
		Pkcs7: pkcs7,
	}

	created, err := svc.Users.Settings.Cse.Keypairs.Create("me", keypair).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseKeyPair": created})
	}

	u.Out().Println("CSE key pair created successfully")
	u.Out().Printf("key_pair_id\t%s", created.KeyPairId)
	u.Out().Printf("enablement_state\t%s", created.EnablementState)
	u.Out().Printf("subject_emails\t%s", strings.Join(created.SubjectEmailAddresses, ","))
	return nil
}

type GmailCseKeypairsEnableCmd struct {
	KeyPairID string `arg:"" name:"keyPairId" help:"ID of the CSE key pair to enable"`
}

func (c *GmailCseKeypairsEnableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	keyPairID := strings.TrimSpace(c.KeyPairID)
	if keyPairID == "" {
		return errors.New("keyPairId is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	req := &gmail.EnableCseKeyPairRequest{}
	enabled, err := svc.Users.Settings.Cse.Keypairs.Enable("me", keyPairID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseKeyPair": enabled})
	}

	u.Out().Printf("CSE key pair enabled: %s", keyPairID)
	u.Out().Printf("enablement_state\t%s", enabled.EnablementState)
	return nil
}

type GmailCseKeypairsDisableCmd struct {
	KeyPairID string `arg:"" name:"keyPairId" help:"ID of the CSE key pair to disable"`
}

func (c *GmailCseKeypairsDisableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	keyPairID := strings.TrimSpace(c.KeyPairID)
	if keyPairID == "" {
		return errors.New("keyPairId is required")
	}

	if !flags.Force {
		if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("Disable CSE key pair %s?", keyPairID)); confirmErr != nil {
			return confirmErr
		}
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	req := &gmail.DisableCseKeyPairRequest{}
	disabled, err := svc.Users.Settings.Cse.Keypairs.Disable("me", keyPairID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"cseKeyPair": disabled})
	}

	u.Out().Printf("CSE key pair disabled: %s", keyPairID)
	u.Out().Printf("enablement_state\t%s", disabled.EnablementState)
	if disabled.DisableTime != "" {
		u.Out().Printf("disable_time\t%s", disabled.DisableTime)
	}
	return nil
}

type GmailCseKeypairsObliterateCmd struct {
	KeyPairID string `arg:"" name:"keyPairId" help:"ID of the CSE key pair to obliterate"`
}

func (c *GmailCseKeypairsObliterateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	keyPairID := strings.TrimSpace(c.KeyPairID)
	if keyPairID == "" {
		return errors.New("keyPairId is required")
	}

	if !flags.Force {
		if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("OBLITERATE CSE key pair %s? This is IRREVERSIBLE and will permanently delete the key pair and make all encrypted emails unreadable!", keyPairID)); confirmErr != nil {
			return confirmErr
		}
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	req := &gmail.ObliterateCseKeyPairRequest{}
	err = svc.Users.Settings.Cse.Keypairs.Obliterate("me", keyPairID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"success":     true,
			"keyPairId":   keyPairID,
			"obliterated": true,
		})
	}

	u.Out().Printf("CSE key pair OBLITERATED: %s (this action is irreversible)", keyPairID)
	return nil
}
