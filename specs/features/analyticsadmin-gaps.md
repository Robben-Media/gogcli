# Analytics Admin v1beta -- Gap Coverage Spec

## Overview

**API**: Google Analytics Admin API v1beta
**Go package**: `google.golang.org/api/analyticsadmin/v1beta`
**Service factory**: `newAnalyticsAdminService` (existing in `analytics.go`)
**Currently implemented**: `accountSummaries.list`, `accounts.list`
**Missing methods**: 53

## Why

The Analytics Admin API controls GA4 account structure, property configuration, data streams, and integrations. Without these methods, users cannot manage properties, configure data retention, set up conversion events, or link Firebase/Google Ads from the CLI. This blocks any GA4 administration workflow.

---

## Resource: Accounts (7 methods)

### accounts.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin accounts get <accountId>` |
| API | `Accounts.Get("accounts/{accountId}")` |
| Args | `accountId` (positional, required) |
| Flags | none |
| Output JSON | `{"account": {...}}` |
| Output TSV | `name`, `displayName`, `regionCode`, `createTime`, `updateTime` |
| Notes | Normalize input: prepend `accounts/` if missing |

### accounts.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin accounts delete <accountId>` |
| API | `Accounts.Delete("accounts/{accountId}")` |
| Args | `accountId` (positional, required) |
| Flags | `--force` (skip confirmation) |
| Guard | `confirmDestructive(ctx, flags, "delete analytics account {accountId}")` |
| Output JSON | `{"deleted": true, "account": "accounts/{accountId}"}` |
| Output TSV | `Deleted account: accounts/{accountId}` |

### accounts.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin accounts patch <accountId>` |
| API | `Accounts.Patch("accounts/{accountId}", &Account{...})` |
| Args | `accountId` (positional, required) |
| Flags | `--display-name`, `--region-code` |
| Patch logic | Use `flagProvided()` to build `updateMask` field mask; only send changed fields |
| Output JSON | `{"account": {...}}` |
| Output TSV | key-value pairs of updated account |

### accounts.getDataSharingSettings

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin accounts data-sharing <accountId>` |
| API | `Accounts.GetDataSharingSettings("accounts/{accountId}/dataSharingSettings")` |
| Args | `accountId` (positional, required) |
| Output JSON | `{"dataSharingSettings": {...}}` |
| Output TSV | key-value pairs: `sharingWithGoogleAnySalesEnabled`, `sharingWithGoogleAssignedSalesEnabled`, etc. |

### accounts.provisionAccountTicket

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin accounts provision-ticket` |
| API | `Accounts.ProvisionAccountTicket(&ProvisionAccountTicketRequest{...})` |
| Flags | `--redirect-uri` (required) |
| Output JSON | `{"accountTicketId": "..."}` |
| Notes | Returns a ticket ID for the account provisioning flow |

### accounts.runAccessReport

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin accounts access-report <accountId>` |
| API | `Accounts.RunAccessReport("accounts/{accountId}", &RunAccessReportRequest{...})` |
| Args | `accountId` (positional, required) |
| Flags | `--dimensions`, `--metrics`, `--date-ranges`, `--limit` |
| Output JSON | `{"dimensionHeaders": [...], "metricHeaders": [...], "rows": [...]}` |
| Output TSV | Dynamic columns from dimension/metric headers |

### accounts.searchChangeHistoryEvents

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin accounts change-history <accountId>` |
| API | `Accounts.SearchChangeHistoryEvents("accounts/{accountId}", &SearchChangeHistoryEventsRequest{...})` |
| Args | `accountId` (positional, required) |
| Flags | `--property` (optional filter), `--resource-type` (optional), `--action` (optional: CREATED/UPDATED/DELETED), `--earliest-change-time`, `--latest-change-time`, `--page-size`, `--page-token` |
| Output JSON | `{"changeHistoryEvents": [...], "nextPageToken": "..."}` |
| Output TSV | `changeTime`, `userActorEmail`, `changesCount` |

---

## Resource: Properties (9 methods)

### properties.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties create` |
| API | `Properties.Create(&Property{...})` |
| Flags | `--parent` (required, account resource name), `--display-name` (required), `--industry-category`, `--time-zone` (required), `--currency-code` |
| Output JSON | `{"property": {...}}` |
| Output TSV | `name`, `displayName`, `timeZone`, `currencyCode` |

### properties.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties delete <propertyId>` |
| API | `Properties.Delete("properties/{propertyId}")` |
| Args | `propertyId` (positional, required) |
| Flags | `--force` |
| Guard | `confirmDestructive()` |
| Output JSON | `{"property": {...}}` (returns soft-deleted property) |

### properties.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties get <propertyId>` |
| API | `Properties.Get("properties/{propertyId}")` |
| Args | `propertyId` (positional, required) |
| Output JSON | `{"property": {...}}` |
| Output TSV | `name`, `displayName`, `parent`, `timeZone`, `currencyCode`, `industryCategory`, `serviceLevel` |

### properties.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties list` |
| API | `Properties.List()` with `.Filter("parent:accounts/{accountId}")` |
| Flags | `--filter` (required, e.g. `parent:accounts/123`), `--page-size`, `--page-token` |
| Output JSON | `{"properties": [...], "nextPageToken": "..."}` |
| Output TSV | `NAME`, `DISPLAY_NAME`, `TIME_ZONE`, `CURRENCY` |

### properties.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties patch <propertyId>` |
| API | `Properties.Patch("properties/{propertyId}", &Property{...})` |
| Args | `propertyId` (positional, required) |
| Flags | `--display-name`, `--industry-category`, `--time-zone`, `--currency-code` |
| Patch logic | `flagProvided()` to build `updateMask` |
| Output JSON | `{"property": {...}}` |

### properties.acknowledgeUserDataCollection

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties acknowledge-data <propertyId>` |
| API | `Properties.AcknowledgeUserDataCollection("properties/{propertyId}", &AcknowledgeUserDataCollectionRequest{...})` |
| Args | `propertyId` (positional, required) |
| Flags | `--acknowledgement` (required, the acknowledgement string) |
| Output JSON | `{"acknowledged": true}` |

### properties.runAccessReport

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties access-report <propertyId>` |
| API | `Properties.RunAccessReport("properties/{propertyId}", &RunAccessReportRequest{...})` |
| Args | `propertyId` (positional, required) |
| Flags | Same as accounts.runAccessReport |

### properties.getDataRetentionSettings

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties data-retention get <propertyId>` |
| API | `Properties.GetDataRetentionSettings("properties/{propertyId}/dataRetentionSettings")` |
| Args | `propertyId` (positional, required) |
| Output JSON | `{"dataRetentionSettings": {...}}` |
| Output TSV | `eventDataRetention`, `resetUserDataOnNewActivity` |

### properties.updateDataRetentionSettings

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin properties data-retention update <propertyId>` |
| API | `Properties.UpdateDataRetentionSettings("properties/{propertyId}/dataRetentionSettings", &DataRetentionSettings{...})` |
| Args | `propertyId` (positional, required) |
| Flags | `--event-data-retention` (enum: TWO_MONTHS, FOURTEEN_MONTHS, TWENTY_SIX_MONTHS, THIRTY_EIGHT_MONTHS, FIFTY_MONTHS), `--reset-user-data-on-new-activity` (bool) |
| Patch logic | `flagProvided()` to build `updateMask` |

---

## Resource: Property Conversion Events (5 methods)

Path pattern: `properties/{propertyId}/conversionEvents/{conversionEventId}`

### conversionEvents.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin conversion-events create --property <id>` |
| API | `Properties.ConversionEvents.Create("properties/{propertyId}", &ConversionEvent{...})` |
| Flags | `--property` (required), `--event-name` (required), `--counting-method` (optional: ONCE_PER_EVENT, ONCE_PER_SESSION) |
| Output JSON | `{"conversionEvent": {...}}` |

### conversionEvents.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin conversion-events delete --property <id> <conversionEventId>` |
| API | `Properties.ConversionEvents.Delete("properties/{propertyId}/conversionEvents/{id}")` |
| Guard | `confirmDestructive()` with `--force` |

### conversionEvents.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin conversion-events get --property <id> <conversionEventId>` |
| API | `Properties.ConversionEvents.Get(name)` |
| Output JSON | `{"conversionEvent": {...}}` |
| Output TSV | `name`, `eventName`, `createTime`, `countingMethod`, `deletable`, `custom` |

### conversionEvents.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin conversion-events list --property <id>` |
| API | `Properties.ConversionEvents.List("properties/{propertyId}")` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"conversionEvents": [...], "nextPageToken": "..."}` |
| Output TSV | `NAME`, `EVENT_NAME`, `COUNTING_METHOD`, `CUSTOM` |

### conversionEvents.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin conversion-events patch --property <id> <conversionEventId>` |
| Flags | `--counting-method` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Property Custom Dimensions (5 methods)

Path pattern: `properties/{propertyId}/customDimensions/{customDimensionId}`

### customDimensions.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-dimensions create --property <id>` |
| Flags | `--property` (required), `--parameter-name` (required), `--display-name` (required), `--scope` (required: EVENT, USER, ITEM), `--description` |
| Output JSON | `{"customDimension": {...}}` |

### customDimensions.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-dimensions get --property <id> <customDimensionId>` |
| Output JSON | `{"customDimension": {...}}` |
| Output TSV | `name`, `parameterName`, `displayName`, `scope`, `description` |

### customDimensions.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-dimensions list --property <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"customDimensions": [...], "nextPageToken": "..."}` |
| Output TSV | `NAME`, `PARAMETER_NAME`, `DISPLAY_NAME`, `SCOPE` |

### customDimensions.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-dimensions patch --property <id> <customDimensionId>` |
| Flags | `--display-name`, `--description` |
| Patch logic | `flagProvided()` for `updateMask` |

### customDimensions.archive

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-dimensions archive --property <id> <customDimensionId>` |
| API | `Properties.CustomDimensions.Archive(name, &ArchiveCustomDimensionRequest{})` |
| Guard | `confirmDestructive()` with `--force` (archive is irreversible) |
| Output JSON | `{"archived": true, "name": "..."}` |

---

## Resource: Property Custom Metrics (5 methods)

Path pattern: `properties/{propertyId}/customMetrics/{customMetricId}`

Identical CRUD+archive pattern to Custom Dimensions.

### customMetrics.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-metrics create --property <id>` |
| Flags | `--property` (required), `--parameter-name` (required), `--display-name` (required), `--scope` (required: EVENT), `--measurement-unit` (required: STANDARD, CURRENCY, FEET, METERS, KILOMETERS, MILES, MILLISECONDS, SECONDS, MINUTES, HOURS), `--description` |

### customMetrics.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-metrics get --property <id> <customMetricId>` |
| Output TSV | `name`, `parameterName`, `displayName`, `scope`, `measurementUnit` |

### customMetrics.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-metrics list --property <id>` |
| Flags | `--page-size`, `--page-token` |

### customMetrics.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-metrics patch --property <id> <customMetricId>` |
| Flags | `--display-name`, `--measurement-unit`, `--description` |
| Patch logic | `flagProvided()` for `updateMask` |

### customMetrics.archive

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin custom-metrics archive --property <id> <customMetricId>` |
| Guard | `confirmDestructive()` with `--force` |

---

## Resource: Property Data Streams (5 methods)

Path pattern: `properties/{propertyId}/dataStreams/{dataStreamId}`

### dataStreams.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin data-streams create --property <id>` |
| Flags | `--property` (required), `--type` (required: WEB_DATA_STREAM, ANDROID_APP_DATA_STREAM, IOS_APP_DATA_STREAM), `--display-name` (required), `--web-stream-data.default-uri` (for web), `--android-app-data-stream.package-name` (for android), `--ios-app-data-stream.bundle-id` (for ios) |
| Output JSON | `{"dataStream": {...}}` |

### dataStreams.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin data-streams delete --property <id> <dataStreamId>` |
| Guard | `confirmDestructive()` with `--force` |

### dataStreams.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin data-streams get --property <id> <dataStreamId>` |
| Output JSON | `{"dataStream": {...}}` |
| Output TSV | `name`, `type`, `displayName`, `createTime`, `updateTime` |

### dataStreams.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin data-streams list --property <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"dataStreams": [...], "nextPageToken": "..."}` |
| Output TSV | `NAME`, `TYPE`, `DISPLAY_NAME` |

### dataStreams.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin data-streams patch --property <id> <dataStreamId>` |
| Flags | `--display-name` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Data Stream Measurement Protocol Secrets (5 methods)

Path pattern: `properties/{propertyId}/dataStreams/{dataStreamId}/measurementProtocolSecrets/{secretId}`

### measurementProtocolSecrets.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin mp-secrets create --property <id> --stream <id>` |
| Flags | `--property` (required), `--stream` (required), `--display-name` (required) |
| Output JSON | `{"measurementProtocolSecret": {...}}` |
| Notes | The `secretValue` is auto-generated and returned in response |

### measurementProtocolSecrets.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin mp-secrets delete --property <id> --stream <id> <secretId>` |
| Guard | `confirmDestructive()` with `--force` |

### measurementProtocolSecrets.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin mp-secrets get --property <id> --stream <id> <secretId>` |
| Output JSON | `{"measurementProtocolSecret": {...}}` |
| Output TSV | `name`, `displayName`, `secretValue` |

### measurementProtocolSecrets.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin mp-secrets list --property <id> --stream <id>` |
| Flags | `--page-size`, `--page-token` |

### measurementProtocolSecrets.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin mp-secrets patch --property <id> --stream <id> <secretId>` |
| Flags | `--display-name` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Property Firebase Links (3 methods)

Path pattern: `properties/{propertyId}/firebaseLinks/{firebaseLinkId}`

### firebaseLinks.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin firebase-links create --property <id>` |
| Flags | `--property` (required), `--project` (required, Firebase project resource name) |
| Output JSON | `{"firebaseLink": {...}}` |

### firebaseLinks.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin firebase-links delete --property <id> <firebaseLinkId>` |
| Guard | `confirmDestructive()` with `--force` |

### firebaseLinks.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin firebase-links list --property <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"firebaseLinks": [...], "nextPageToken": "..."}` |
| Output TSV | `NAME`, `PROJECT`, `CREATE_TIME` |

---

## Resource: Property Google Ads Links (4 methods)

Path pattern: `properties/{propertyId}/googleAdsLinks/{googleAdsLinkId}`

### googleAdsLinks.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin googleads-links create --property <id>` |
| Flags | `--property` (required), `--customer-id` (required, Google Ads customer ID) |
| Output JSON | `{"googleAdsLink": {...}}` |

### googleAdsLinks.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin googleads-links delete --property <id> <linkId>` |
| Guard | `confirmDestructive()` with `--force` |

### googleAdsLinks.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin googleads-links list --property <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"googleAdsLinks": [...], "nextPageToken": "..."}` |
| Output TSV | `NAME`, `CUSTOMER_ID`, `ADS_PERSONALIZATION_ENABLED`, `CREATE_TIME` |

### googleAdsLinks.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin googleads-links patch --property <id> <linkId>` |
| Flags | `--ads-personalization-enabled` (bool) |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Property Key Events (5 methods)

Path pattern: `properties/{propertyId}/keyEvents/{keyEventId}`

### keyEvents.create

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin key-events create --property <id>` |
| Flags | `--property` (required), `--event-name` (required), `--counting-method` (ONCE_PER_EVENT, ONCE_PER_SESSION) |
| Output JSON | `{"keyEvent": {...}}` |

### keyEvents.delete

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin key-events delete --property <id> <keyEventId>` |
| Guard | `confirmDestructive()` with `--force` |

### keyEvents.get

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin key-events get --property <id> <keyEventId>` |
| Output JSON | `{"keyEvent": {...}}` |
| Output TSV | `name`, `eventName`, `createTime`, `countingMethod`, `custom`, `deletable` |

### keyEvents.list

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin key-events list --property <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"keyEvents": [...], "nextPageToken": "..."}` |
| Output TSV | `NAME`, `EVENT_NAME`, `COUNTING_METHOD`, `CUSTOM` |

### keyEvents.patch

| Field | Value |
|-------|-------|
| CLI | `gog analytics admin key-events patch --property <id> <keyEventId>` |
| Flags | `--counting-method` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Kong Struct Layout

```go
type AnalyticsAdminCmd struct {
    Accounts         AAAccountsCmd         `cmd:"" name:"accounts" help:"Account operations"`
    Properties       AAPropertiesCmd       `cmd:"" name:"properties" help:"Property operations"`
    ConversionEvents AAConversionEventsCmd `cmd:"" name:"conversion-events" help:"Conversion event operations"`
    CustomDimensions AACustomDimensionsCmd `cmd:"" name:"custom-dimensions" help:"Custom dimension operations"`
    CustomMetrics    AACustomMetricsCmd    `cmd:"" name:"custom-metrics" help:"Custom metric operations"`
    DataStreams       AADataStreamsCmd       `cmd:"" name:"data-streams" help:"Data stream operations"`
    MpSecrets        AAMpSecretsCmd        `cmd:"" name:"mp-secrets" help:"Measurement Protocol secret operations"`
    FirebaseLinks    AAFirebaseLinksCmd    `cmd:"" name:"firebase-links" help:"Firebase link operations"`
    GoogleAdsLinks   AAGoogleAdsLinksCmd   `cmd:"" name:"googleads-links" help:"Google Ads link operations"`
    KeyEvents        AAKeyEventsCmd        `cmd:"" name:"key-events" help:"Key event operations"`
}
```

Integration: Add `Admin AnalyticsAdminCmd` to `AnalyticsCmd` struct in `analytics.go`.

---

## Test Requirements

Each command requires at least one test using `httptest.NewServer` to mock API responses.

### Test patterns

1. **List commands**: Mock paginated response, assert JSON array output, verify TSV table headers
2. **Get commands**: Mock single resource response, assert JSON object fields
3. **Create commands**: Mock 200 response with created resource, verify request body contains required fields
4. **Delete commands**: Verify `confirmDestructive()` is called (test without `--force` returns error), mock 204/empty response
5. **Patch commands**: Verify `updateMask` query parameter matches provided flags, verify request body only contains changed fields
6. **Archive commands**: Same as delete pattern (irreversible action)

### Factory injection

Use existing pattern: `var newAnalyticsAdminService = googleapi.NewAnalyticsAdmin` with test override.

### Example test structure

```go
func TestAAConversionEventsList(t *testing.T) {
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/v1beta/properties/123/conversionEvents" {
            t.Errorf("unexpected path: %s", r.URL.Path)
        }
        json.NewEncoder(w).Encode(map[string]any{
            "conversionEvents": []map[string]any{
                {"name": "properties/123/conversionEvents/456", "eventName": "purchase"},
            },
        })
    }))
    defer ts.Close()

    // Override service factory, run command, assert JSON output
}
```

### Test file organization

- `analytics_admin_test.go` -- accounts, properties tests
- `analytics_admin_resources_test.go` -- conversion events, custom dimensions, custom metrics
- `analytics_admin_streams_test.go` -- data streams, MP secrets
- `analytics_admin_links_test.go` -- Firebase links, Google Ads links, key events
