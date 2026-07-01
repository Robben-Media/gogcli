package cmd

// AnalyticsAdminCmd groups all GA4 Admin API operations.
type AnalyticsAdminCmd struct {
	DataStreams      AADataStreamsCmd      `cmd:"" name:"data-streams" help:"Data stream operations"`
	MpSecrets        AAMpSecretsCmd        `cmd:"" name:"mp-secrets" help:"Measurement Protocol secret operations"`
	Accounts         AAAccountsCmd         `cmd:"" name:"accounts" help:"Account operations"`
	Properties       AAPropertiesCmd       `cmd:"" name:"properties" help:"Property operations"`
	ConversionEvents AAConversionEventsCmd `cmd:"" name:"conversion-events" help:"Conversion event operations"`
	CustomDimensions AACustomDimensionsCmd `cmd:"" name:"custom-dimensions" help:"Custom dimension operations"`
	CustomMetrics    AACustomMetricsCmd    `cmd:"" name:"custom-metrics" help:"Custom metric operations"`
	FirebaseLinks    AAFirebaseLinksCmd    `cmd:"" name:"firebase-links" help:"Firebase link operations"`
	GoogleAdsLinks   AAGoogleAdsLinksCmd   `cmd:"" name:"googleads-links" help:"Google Ads link operations"`
	KeyEvents        AAKeyEventsCmd        `cmd:"" name:"key-events" help:"Key event operations"`
}

// Stubs for resources not yet implemented (PR 2).

type (
	AAAccountsCmd         struct{}
	AAPropertiesCmd       struct{}
	AAConversionEventsCmd struct{}
	AACustomDimensionsCmd struct{}
	AACustomMetricsCmd    struct{}
	AAFirebaseLinksCmd    struct{}
	AAGoogleAdsLinksCmd   struct{}
	AAKeyEventsCmd        struct{}
)
