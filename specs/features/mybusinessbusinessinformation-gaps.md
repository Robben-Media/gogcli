# My Business Business Information v1 -- Gap Coverage Spec

**API**: My Business Business Information v1 (`mybusinessbusinessinformation/v1`)
**Current coverage**: 2 methods (accounts.locations.list, locations.get)
**Gap**: 13 missing methods
**Service factory**: `newMyBusinessBusinessInformationService` (existing or to be created)

## Overview

Adding 13 missing methods to the My Business Business Information CLI commands. Covers location creation, attributes, categories, chains, Google location search, and location management to achieve full Discovery API parity.

**Error handling**: All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete operations require `confirmDestructive()`.

---

## Accounts Locations

### `gog mybusiness-info locations create`

- **API method**: `accounts.locations.create`
- **Struct**: `MyBusinessInfoLocationsCreateCmd`
- **Args/Flags**:
  - `parent` (required arg): account resource name (e.g., `accounts/123`)
  - `--store-code` (string): external store code
  - `--title` (required string): business name
  - `--phone` (string): primary phone number
  - `--website` (string): website URL
  - `--address-lines` (string): comma-separated address lines
  - `--locality` (string): city
  - `--region` (string): state/province
  - `--postal-code` (string): postal/zip code
  - `--country` (string): ISO 3166-1 alpha-2 country code
  - `--category-id` (required string): primary category ID (e.g., `gcid:restaurant`)
  - `--latitude` (float64): latitude
  - `--longitude` (float64): longitude
- **Behavior**: Builds a Location object from flags. Address fields compose a PostalAddress. Category ID sets primaryCategory.
- **Output**: JSON object of created location
- **Test**: httptest mock POST `/v1/{parent}/locations`, assert request body structure

---

## Attributes

### `gog mybusiness-info attributes list`

- **API method**: `attributes.list`
- **Struct**: `MyBusinessInfoAttributesListCmd`
- **Args/Flags**:
  - `--parent` (required string): location resource name
  - `--category-name` (string): category filter
  - `--language-code` (string, default "en"): language for attribute display names
  - `--max` (int64, default 100): page size
  - `--page` (string): page token
- **Output**: JSON array of available attributes; text table ATTRIBUTE_ID, DISPLAY_NAME, VALUE_TYPE
- **Test**: httptest mock GET `/v1/attributes`, assert query params

---

## Categories

### `gog mybusiness-info categories batch-get`

- **API method**: `categories.batchGet`
- **Struct**: `MyBusinessInfoCategoriesBatchGetCmd`
- **Args/Flags**:
  - `--names` (required []string): category resource names
  - `--language-code` (string, default "en"): language code
  - `--region-code` (string): ISO 3166-1 alpha-2 region code
  - `--view` (string): BASIC or FULL
- **Output**: JSON object with `categories` array; text table NAME, DISPLAY_NAME, SERVICE_TYPES
- **Test**: httptest mock GET `/v1/categories:batchGet?names=...`

### `gog mybusiness-info categories list`

- **API method**: `categories.list`
- **Struct**: `MyBusinessInfoCategoriesListCmd`
- **Args/Flags**:
  - `--region-code` (required string): ISO 3166-1 alpha-2 region code
  - `--language-code` (string, default "en"): language code
  - `--filter` (string): search filter text
  - `--max` (int64, default 100): page size
  - `--page` (string): page token
  - `--view` (string): BASIC or FULL
- **Output**: JSON array; text table NAME, DISPLAY_NAME
- **Test**: httptest mock GET `/v1/categories`, assert pagination and filter params

---

## Chains

### `gog mybusiness-info chains get`

- **API method**: `chains.get`
- **Struct**: `MyBusinessInfoChainsGetCmd`
- **Args/Flags**:
  - `name` (required arg): chain resource name (e.g., `chains/123`)
- **Output**: JSON object with chain details; text shows NAME, CHAIN_NAME, WEBSITES
- **Test**: httptest mock GET `/v1/chains/{id}`

### `gog mybusiness-info chains search`

- **API method**: `chains.search`
- **Struct**: `MyBusinessInfoChainsSearchCmd`
- **Args/Flags**:
  - `--chain-name` (required string): chain name to search for
  - `--max` (int64, default 10): max results
- **Output**: JSON array of matching chains; text table NAME, CHAIN_NAME, LOCATION_COUNT
- **Test**: httptest mock GET `/v1/chains:search?chainName=...`

---

## Google Locations

### `gog mybusiness-info google-locations search`

- **API method**: `googleLocations.search`
- **Struct**: `MyBusinessInfoGoogleLocationsSearchCmd`
- **Args/Flags**:
  - `--query` (required string): search query (business name + address)
  - `--max` (int64, default 10): max results
- **Behavior**: Searches for existing Google locations that match the query. Used to find locations before claiming them.
- **Output**: JSON array of Google locations; text table RESOURCE, TITLE, ADDRESS
- **Test**: httptest mock POST `/v1/googleLocations:search`, assert query in body

---

## Location Attributes

### `gog mybusiness-info location-attributes get-google-updated`

- **API method**: `locations.attributes.getGoogleUpdated`
- **Struct**: `MyBusinessInfoLocationAttrsGetGoogleUpdatedCmd`
- **Args/Flags**:
  - `name` (required arg): location attributes resource name (e.g., `locations/123/attributes`)
- **Behavior**: Returns attributes that Google has auto-updated (may differ from what the business set).
- **Output**: JSON object with attributes array
- **Test**: httptest mock GET `/v1/{name}:getGoogleUpdated`

### `gog mybusiness-info location-attributes get`

- **API method**: `locations.getAttributes`
- **Struct**: `MyBusinessInfoLocationAttrsGetCmd`
- **Args/Flags**:
  - `name` (required arg): location attributes resource name
- **Output**: JSON object with `attributes` array; text table ATTRIBUTE_ID, VALUE_TYPE, VALUES
- **Test**: httptest mock GET `/v1/{name}`

### `gog mybusiness-info location-attributes update`

- **API method**: `locations.updateAttributes`
- **Struct**: `MyBusinessInfoLocationAttrsUpdateCmd`
- **Args/Flags**:
  - `name` (required arg): location attributes resource name
  - `--attributes-json` (required string): JSON string or @file path containing attributes array
- **Behavior**: Full replace of all attributes. Accepts raw JSON input because attribute structures are complex and varied.
- **Output**: JSON object of updated attributes
- **Test**: httptest mock PATCH `/v1/{name}`, assert attributes body

---

## Locations

### `gog mybusiness-info locations delete`

- **API method**: `locations.delete`
- **Struct**: `MyBusinessInfoLocationsDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): location resource name
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Permanently deletes the location.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/{name}`

### `gog mybusiness-info locations get-google-updated`

- **API method**: `locations.getGoogleUpdated`
- **Struct**: `MyBusinessInfoLocationsGetGoogleUpdatedCmd`
- **Args/Flags**:
  - `name` (required arg): location resource name
- **Behavior**: Returns the Google-updated version of the location (may differ from business-set data).
- **Output**: JSON object with location and diff fields
- **Test**: httptest mock GET `/v1/{name}:getGoogleUpdated`

### `gog mybusiness-info locations patch`

- **API method**: `locations.patch`
- **Struct**: `MyBusinessInfoLocationsPatchCmd`
- **Args/Flags**:
  - `name` (required arg): location resource name
  - `--title` (string): business name
  - `--phone` (string): primary phone
  - `--website` (string): website URL
  - `--address-lines` (string): comma-separated address lines
  - `--locality` (string): city
  - `--region` (string): state/province
  - `--postal-code` (string): postal code
  - `--country` (string): country code
  - `--category-id` (string): primary category ID
- **Behavior**: `flagProvided()` for updateMask. Only send changed fields.
- **Output**: JSON object of updated location
- **Test**: httptest mock PATCH `/v1/{name}?updateMask=...`

---

## Implementation Notes

1. The command prefix is `gog mybusiness-info` to distinguish from `gog mybusiness` (account management). Consider if these should be unified under a single `gog mybusiness` with sub-resources.
2. Categories and chains are reference data endpoints -- they are read-only and not tied to a specific business.
3. The `--attributes-json` flag for attribute updates accepts complex nested JSON. Support both inline JSON and `@filepath` syntax (read from file if prefixed with `@`).
4. Google-updated variants return what Google thinks the data should be, which may conflict with business-set values. The output should clearly indicate these are Google's values.
5. Total new test count: minimum 13 test functions.
