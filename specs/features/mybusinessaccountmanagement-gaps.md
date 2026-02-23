# My Business Account Management v1 -- Gap Coverage Spec

**API**: My Business Account Management v1 (`mybusinessaccountmanagement/v1`)
**Current coverage**: 1 method (accounts.list)
**Gap**: 15 missing methods
**Service factory**: `newMyBusinessAccountManagementService` (to be created or existing)

## Overview

Adding 15 missing methods to the My Business Account Management CLI commands. Covers accounts, account admins, account invitations, location admins, and location transfer to achieve full Discovery API parity.

**Error handling**: All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete operations and location transfer require `confirmDestructive()`.

---

## Accounts

### `gog mybusiness accounts create`

- **API method**: `accounts.create`
- **Struct**: `MyBusinessAccountsCreateCmd`
- **Args/Flags**:
  - `--name` (required string): account display name
  - `--type` (string): account type (PERSONAL, LOCATION_GROUP, etc.)
  - `--primary-owner` (string): primary owner email
- **Output**: JSON object of created account
- **Test**: httptest mock POST `/v1/accounts`, assert request body

### `gog mybusiness accounts get`

- **API method**: `accounts.get`
- **Struct**: `MyBusinessAccountsGetCmd`
- **Args/Flags**:
  - `name` (required arg): account resource name (e.g., `accounts/123`)
- **Output**: JSON object; text shows NAME, ACCOUNT_NAME, TYPE, ROLE, STATE
- **Test**: httptest mock GET `/v1/accounts/{id}`

### `gog mybusiness accounts patch`

- **API method**: `accounts.patch`
- **Struct**: `MyBusinessAccountsPatchCmd`
- **Args/Flags**:
  - `name` (required arg): account resource name
  - `--account-name` (string): display name
  - `--primary-owner` (string): primary owner
- **Behavior**: `flagProvided()` for updateMask. Only send changed fields.
- **Output**: JSON object of updated account
- **Test**: httptest mock PATCH `/v1/accounts/{id}?updateMask=...`, assert mask and body

---

## Account Admins

### `gog mybusiness account-admins create`

- **API method**: `accounts.admins.create`
- **Struct**: `MyBusinessAccountAdminsCreateCmd`
- **Args/Flags**:
  - `parent` (required arg): account resource name
  - `--admin` (required string): admin email address
  - `--role` (string, default "MANAGER"): OWNER, MANAGER, SITE_MANAGER
- **Output**: JSON object of created admin
- **Test**: httptest mock POST `/v1/{parent}/admins`

### `gog mybusiness account-admins delete`

- **API method**: `accounts.admins.delete`
- **Struct**: `MyBusinessAccountAdminsDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): admin resource name (e.g., `accounts/123/admins/456`)
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/{name}`

### `gog mybusiness account-admins list`

- **API method**: `accounts.admins.list`
- **Struct**: `MyBusinessAccountAdminsListCmd`
- **Args/Flags**:
  - `parent` (required arg): account resource name
- **Output**: JSON array of admins; text table NAME, ADMIN, ROLE
- **Note**: This endpoint does not support pagination.
- **Test**: httptest mock GET `/v1/{parent}/admins`

### `gog mybusiness account-admins patch`

- **API method**: `accounts.admins.patch`
- **Struct**: `MyBusinessAccountAdminsPatchCmd`
- **Args/Flags**:
  - `name` (required arg): admin resource name
  - `--role` (string): new role
- **Behavior**: `flagProvided()` for updateMask.
- **Output**: JSON object of updated admin
- **Test**: httptest mock PATCH `/v1/{name}?updateMask=...`

---

## Account Invitations

### `gog mybusiness account-invitations accept`

- **API method**: `accounts.invitations.accept`
- **Struct**: `MyBusinessAccountInvitationsAcceptCmd`
- **Args/Flags**:
  - `name` (required arg): invitation resource name (e.g., `accounts/123/invitations/456`)
- **Behavior**: POST to accept the invitation.
- **Output**: Empty on success, stderr confirmation message
- **Test**: httptest mock POST `/v1/{name}:accept`

### `gog mybusiness account-invitations decline`

- **API method**: `accounts.invitations.decline`
- **Struct**: `MyBusinessAccountInvitationsDeclineCmd`
- **Args/Flags**:
  - `name` (required arg): invitation resource name
- **Behavior**: POST to decline the invitation.
- **Output**: Empty on success
- **Test**: httptest mock POST `/v1/{name}:decline`

### `gog mybusiness account-invitations list`

- **API method**: `accounts.invitations.list`
- **Struct**: `MyBusinessAccountInvitationsListCmd`
- **Args/Flags**:
  - `parent` (required arg): account resource name
  - `--filter` (string): target type filter (e.g., `target_type=ACCEPT_INVITATION`)
- **Output**: JSON array of invitations; text table NAME, TARGET_ACCOUNT, TARGET_TYPE, ROLE
- **Note**: This endpoint does not support pagination.
- **Test**: httptest mock GET `/v1/{parent}/invitations`

---

## Location Admins

### `gog mybusiness location-admins create`

- **API method**: `locations.admins.create`
- **Struct**: `MyBusinessLocationAdminsCreateCmd`
- **Args/Flags**:
  - `parent` (required arg): location resource name (e.g., `locations/123`)
  - `--admin` (required string): admin email
  - `--role` (string, default "MANAGER"): MANAGER or SITE_MANAGER
- **Output**: JSON object of created admin
- **Test**: httptest mock POST `/v1/{parent}/admins`

### `gog mybusiness location-admins delete`

- **API method**: `locations.admins.delete`
- **Struct**: `MyBusinessLocationAdminsDeleteCmd`
- **Args/Flags**:
  - `name` (required arg): admin resource name (e.g., `locations/123/admins/456`)
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/{name}`

### `gog mybusiness location-admins list`

- **API method**: `locations.admins.list`
- **Struct**: `MyBusinessLocationAdminsListCmd`
- **Args/Flags**:
  - `parent` (required arg): location resource name
- **Output**: JSON array; text table NAME, ADMIN, ROLE
- **Note**: No pagination on this endpoint.
- **Test**: httptest mock GET `/v1/{parent}/admins`

### `gog mybusiness location-admins patch`

- **API method**: `locations.admins.patch`
- **Struct**: `MyBusinessLocationAdminsPatchCmd`
- **Args/Flags**:
  - `name` (required arg): admin resource name
  - `--role` (string): new role
- **Behavior**: `flagProvided()` for updateMask.
- **Output**: JSON object of updated admin
- **Test**: httptest mock PATCH `/v1/{name}?updateMask=...`

---

## Locations

### `gog mybusiness locations transfer`

- **API method**: `locations.transfer`
- **Struct**: `MyBusinessLocationsTransferCmd`
- **Args/Flags**:
  - `name` (required arg): location resource name
  - `--destination-account` (required string): destination account resource name
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Transfers location ownership to another account.
- **Output**: Empty on success
- **Test**: httptest mock POST `/v1/{name}:transfer`, assert destination in body

---

## Implementation Notes

1. The My Business APIs do not require Workspace accounts -- they work with standard Google accounts that manage business profiles.
2. Several endpoints (admins list, invitations list) do not support pagination. Do not add --max/--page flags to these.
3. The `locations.transfer` is a high-impact operation -- ensure `confirmDestructive()` with a clear warning message about permanent ownership transfer.
4. Resource names follow the pattern `accounts/{accountId}`, `accounts/{accountId}/admins/{adminId}`, `locations/{locationId}`, etc.
5. Total new test count: minimum 15 test functions.
