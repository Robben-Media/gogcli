package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ---------------------------------------------------------------------------
// Data Streams
// ---------------------------------------------------------------------------

type AADataStreamsCmd struct {
	Create AADataStreamsCreateCmd `cmd:"" name:"create" help:"Create a data stream"`
	Delete AADataStreamsDeleteCmd `cmd:"" name:"delete" help:"Delete a data stream"`
	Get    AADataStreamsGetCmd    `cmd:"" name:"get" help:"Get a data stream"`
	List   AADataStreamsListCmd   `cmd:"" name:"list" help:"List data streams for a property"`
	Patch  AADataStreamsPatchCmd  `cmd:"" name:"patch" help:"Update a data stream"`
}

// --- create ---

type AADataStreamsCreateCmd struct {
	Property    string `name:"property" required:"" help:"GA4 property ID (e.g. 123456 or properties/123456)"`
	Type        string `name:"type" required:"" help:"Stream type: WEB_DATA_STREAM, ANDROID_APP_DATA_STREAM, IOS_APP_DATA_STREAM"`
	DisplayName string `name:"display-name" required:"" help:"Human-readable display name"`
	WebURI      string `name:"web-default-uri" help:"Default URI for web streams (e.g. https://example.com)"`
	PackageName string `name:"package-name" help:"Android app package name"`
	BundleID    string `name:"bundle-id" help:"iOS app bundle ID"`
}

func (c *AADataStreamsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)

	ds := &analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{
		DisplayName: c.DisplayName,
		Type:        c.Type,
	}

	switch c.Type {
	case "WEB_DATA_STREAM":
		if c.WebURI == "" {
			return usage("--web-default-uri required for WEB_DATA_STREAM")
		}
		ds.WebStreamData = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamWebStreamData{
			DefaultUri: c.WebURI,
		}
	case "ANDROID_APP_DATA_STREAM":
		if c.PackageName == "" {
			return usage("--package-name required for ANDROID_APP_DATA_STREAM")
		}
		ds.AndroidAppStreamData = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamAndroidAppStreamData{
			PackageName: c.PackageName,
		}
	case "IOS_APP_DATA_STREAM":
		if c.BundleID == "" {
			return usage("--bundle-id required for IOS_APP_DATA_STREAM")
		}
		ds.IosAppStreamData = &analyticsadmin.GoogleAnalyticsAdminV1betaDataStreamIosAppStreamData{
			BundleId: c.BundleID,
		}
	default:
		return usagef("unsupported stream type %q (expected WEB_DATA_STREAM, ANDROID_APP_DATA_STREAM, IOS_APP_DATA_STREAM)", c.Type)
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.DataStreams.Create(propID, ds).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"dataStream": resp})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Created data stream: %s", resp.Name)
	if resp.WebStreamData != nil && resp.WebStreamData.MeasurementId != "" {
		u.Out().Printf("Measurement ID: %s", resp.WebStreamData.MeasurementId)
	}
	return nil
}

// --- delete ---

type AADataStreamsDeleteCmd struct {
	Property string `name:"property" required:"" help:"GA4 property ID"`
	Stream   string `arg:"" name:"stream" help:"Data stream ID (numeric or full resource name)"`
}

func (c *AADataStreamsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := normalizeStreamName(c.Property, c.Stream)
	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete data stream %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.Properties.DataStreams.Delete(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": true, "name": name})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Deleted data stream: %s", name)
	return nil
}

// --- get ---

type AADataStreamsGetCmd struct {
	Property string `name:"property" required:"" help:"GA4 property ID"`
	Stream   string `arg:"" name:"stream" help:"Data stream ID"`
}

func (c *AADataStreamsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := normalizeStreamName(c.Property, c.Stream)

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.DataStreams.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"dataStream": resp})
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTYPE\tDISPLAY_NAME\tCREATED\tUPDATED")
	measurementID := ""
	if resp.WebStreamData != nil {
		measurementID = resp.WebStreamData.MeasurementId
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", resp.Name, resp.Type, resp.DisplayName, resp.CreateTime, resp.UpdateTime)
	if measurementID != "" {
		u := ui.FromContext(ctx)
		u.Out().Printf("Measurement ID: %s", measurementID)
	}
	return nil
}

// --- list ---

type AADataStreamsListCmd struct {
	Property  string `name:"property" required:"" help:"GA4 property ID"`
	PageSize  int64  `name:"page-size" help:"Max results per page" default:"50"`
	PageToken string `name:"page-token" help:"Page token for pagination"`
}

func (c *AADataStreamsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propID := normalizePropertyID(c.Property)

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Properties.DataStreams.List(propID)
	if c.PageSize > 0 {
		call = call.PageSize(c.PageSize)
	}
	if c.PageToken != "" {
		call = call.PageToken(c.PageToken)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"dataStreams":   resp.DataStreams,
			"nextPageToken": resp.NextPageToken,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.DataStreams) == 0 {
		u.Err().Println("No data streams")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTYPE\tDISPLAY_NAME\tMEASUREMENT_ID")
	for _, ds := range resp.DataStreams {
		mid := ""
		if ds.WebStreamData != nil {
			mid = ds.WebStreamData.MeasurementId
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ds.Name, ds.Type, ds.DisplayName, mid)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- patch ---

type AADataStreamsPatchCmd struct {
	Property    string `name:"property" required:"" help:"GA4 property ID"`
	Stream      string `arg:"" name:"stream" help:"Data stream ID"`
	DisplayName string `name:"display-name" help:"New display name"`
}

func (c *AADataStreamsPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := normalizeStreamName(c.Property, c.Stream)

	var fields []string
	ds := &analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{}

	if flagProvided(kctx, "display-name") {
		fields = append(fields, "displayName")
		ds.DisplayName = c.DisplayName
	}

	if len(fields) == 0 {
		return usage("at least one field must be specified for patch")
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.DataStreams.Patch(name, ds).UpdateMask(strings.Join(fields, ",")).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"dataStream": resp})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Updated data stream: %s", resp.Name)
	return nil
}

// ---------------------------------------------------------------------------
// Measurement Protocol Secrets
// ---------------------------------------------------------------------------

type AAMpSecretsCmd struct {
	Create AAMpSecretsCreateCmd `cmd:"" name:"create" help:"Create a Measurement Protocol secret"`
	Delete AAMpSecretsDeleteCmd `cmd:"" name:"delete" help:"Delete a Measurement Protocol secret"`
	Get    AAMpSecretsGetCmd    `cmd:"" name:"get" help:"Get a Measurement Protocol secret"`
	List   AAMpSecretsListCmd   `cmd:"" name:"list" help:"List Measurement Protocol secrets"`
	Patch  AAMpSecretsPatchCmd  `cmd:"" name:"patch" help:"Update a Measurement Protocol secret"`
}

// --- mp-secrets create ---

type AAMpSecretsCreateCmd struct {
	Property    string `name:"property" required:"" help:"GA4 property ID"`
	Stream      string `name:"stream" required:"" help:"Data stream ID"`
	DisplayName string `name:"display-name" required:"" help:"Human-readable display name for the secret"`
}

func (c *AAMpSecretsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := normalizeStreamName(c.Property, c.Stream)

	secret := &analyticsadmin.GoogleAnalyticsAdminV1betaMeasurementProtocolSecret{
		DisplayName: c.DisplayName,
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.DataStreams.MeasurementProtocolSecrets.Create(parent, secret).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"measurementProtocolSecret": resp})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Created secret: %s", resp.Name)
	u.Out().Printf("Secret value: %s", resp.SecretValue)
	return nil
}

// --- mp-secrets delete ---

type AAMpSecretsDeleteCmd struct {
	Property string `name:"property" required:"" help:"GA4 property ID"`
	Stream   string `name:"stream" required:"" help:"Data stream ID"`
	Secret   string `arg:"" name:"secret" help:"Measurement Protocol secret ID"`
}

func (c *AAMpSecretsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := normalizeMpSecretName(c.Property, c.Stream, c.Secret)
	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete measurement protocol secret %s", name)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.Properties.DataStreams.MeasurementProtocolSecrets.Delete(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": true, "name": name})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Deleted secret: %s", name)
	return nil
}

// --- mp-secrets get ---

type AAMpSecretsGetCmd struct {
	Property string `name:"property" required:"" help:"GA4 property ID"`
	Stream   string `name:"stream" required:"" help:"Data stream ID"`
	Secret   string `arg:"" name:"secret" help:"Measurement Protocol secret ID"`
}

func (c *AAMpSecretsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := normalizeMpSecretName(c.Property, c.Stream, c.Secret)

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.DataStreams.MeasurementProtocolSecrets.Get(name).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"measurementProtocolSecret": resp})
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY_NAME\tSECRET_VALUE")
	fmt.Fprintf(w, "%s\t%s\t%s\n", resp.Name, resp.DisplayName, resp.SecretValue)
	return nil
}

// --- mp-secrets list ---

type AAMpSecretsListCmd struct {
	Property  string `name:"property" required:"" help:"GA4 property ID"`
	Stream    string `name:"stream" required:"" help:"Data stream ID"`
	PageSize  int64  `name:"page-size" help:"Max results per page" default:"50"`
	PageToken string `name:"page-token" help:"Page token for pagination"`
}

func (c *AAMpSecretsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := normalizeStreamName(c.Property, c.Stream)

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Properties.DataStreams.MeasurementProtocolSecrets.List(parent)
	if c.PageSize > 0 {
		call = call.PageSize(c.PageSize)
	}
	if c.PageToken != "" {
		call = call.PageToken(c.PageToken)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"measurementProtocolSecrets": resp.MeasurementProtocolSecrets,
			"nextPageToken":              resp.NextPageToken,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.MeasurementProtocolSecrets) == 0 {
		u.Err().Println("No measurement protocol secrets")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY_NAME\tSECRET_VALUE")
	for _, s := range resp.MeasurementProtocolSecrets {
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.DisplayName, s.SecretValue)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- mp-secrets patch ---

type AAMpSecretsPatchCmd struct {
	Property    string `name:"property" required:"" help:"GA4 property ID"`
	Stream      string `name:"stream" required:"" help:"Data stream ID"`
	Secret      string `arg:"" name:"secret" help:"Measurement Protocol secret ID"`
	DisplayName string `name:"display-name" help:"New display name"`
}

func (c *AAMpSecretsPatchCmd) Run(ctx context.Context, flags *RootFlags, kctx *kong.Context) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := normalizeMpSecretName(c.Property, c.Stream, c.Secret)

	var fields []string
	secret := &analyticsadmin.GoogleAnalyticsAdminV1betaMeasurementProtocolSecret{}

	if flagProvided(kctx, "display-name") {
		fields = append(fields, "displayName")
		secret.DisplayName = c.DisplayName
	}

	if len(fields) == 0 {
		return usage("at least one field must be specified for patch")
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.DataStreams.MeasurementProtocolSecrets.Patch(name, secret).UpdateMask(strings.Join(fields, ",")).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"measurementProtocolSecret": resp})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Updated secret: %s", resp.Name)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// normalizeStreamName builds a full data stream resource name.
func normalizeStreamName(property, stream string) string {
	propID := normalizePropertyID(property)
	stream = strings.TrimSpace(stream)
	if strings.HasPrefix(stream, "properties/") {
		return stream
	}
	return propID + "/dataStreams/" + stream
}

// normalizeMpSecretName builds a full measurement protocol secret resource name.
func normalizeMpSecretName(property, stream, secret string) string {
	streamName := normalizeStreamName(property, stream)
	secret = strings.TrimSpace(secret)
	if strings.HasPrefix(secret, "properties/") {
		return secret
	}
	return streamName + "/measurementProtocolSecrets/" + secret
}
