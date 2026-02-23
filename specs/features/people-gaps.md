# People API v1 -- Gap Coverage Spec

**API**: People API v1 (`people/v1`)
**Current coverage**: 11 methods (otherContacts.list/search, people.connections.list, people.createContact, people.deleteContact, people.get, people.searchContacts, people.searchDirectoryPeople, people.updateContact, people.listDirectoryPeople, people.batchGet -- verify)
**Gap**: 13 missing methods
**Service factory**: `newPeopleService` (existing)

## Overview

Adding 13 missing methods to the People API v1 CLI commands. Covers contact groups (batch-get/create/delete/get/list/update), group member modification, batch contact operations (batch-create/batch-delete/batch-update/batch-get), and photo management (delete-photo/update-photo) to achieve full Discovery API parity.

All commands follow standard validation: `requireAccount(flags)`, input trimming via `strings.TrimSpace()`, empty checks returning `usage()` errors. Delete operations require `confirmDestructive()`. List commands include `--max` and `--page` flags for pagination. JSON output includes `nextPageToken`. Text output uses TSV-formatted tables. Batch operations support both inline JSON and `@filepath` syntax for complex inputs.

---

## Contact Groups

### `gog contacts groups batch-get`

- **API method**: `contactGroups.batchGet`
- **Struct**: `ContactGroupsBatchGetCmd`
- **Args/Flags**:
  - `--resource-names` (required []string): contact group resource names (e.g., `contactGroups/abc`)
  - `--max-members` (int64, default 0): max members to return per group (0 = none)
  - `--group-fields` (string): field mask for group fields (default: "name,groupType,memberCount")
- **Output**: JSON object `{"responses": [...]}` with group details; text table NAME, TITLE, TYPE, MEMBER_COUNT
- **Test**: httptest mock GET `/v1/contactGroups:batchGet?resourceNames=...`

### `gog contacts groups create`

- **API method**: `contactGroups.create`
- **Struct**: `ContactGroupsCreateCmd`
- **Args/Flags**:
  - `--name` (required string): contact group name
  - `--read-group-fields` (string): field mask for response
- **Output**: JSON object of created group
- **Test**: httptest mock POST `/v1/contactGroups`, assert `contactGroup.name` in body

### `gog contacts groups delete`

- **API method**: `contactGroups.delete`
- **Struct**: `ContactGroupsDeleteCmd`
- **Args/Flags**:
  - `resourceName` (required arg): contact group resource name
  - `--delete-contacts` (bool, default false): also delete contact members
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. If `--delete-contacts` is true, the confirmation message should warn that contacts will also be permanently deleted.
- **Output**: Empty on success
- **Test**: httptest mock DELETE `/v1/{resourceName}?deleteContacts=true|false`

### `gog contacts groups get`

- **API method**: `contactGroups.get`
- **Struct**: `ContactGroupsGetCmd`
- **Args/Flags**:
  - `resourceName` (required arg): contact group resource name
  - `--max-members` (int64): max members to return
  - `--group-fields` (string): field mask
- **Output**: JSON object; text shows NAME, TITLE, TYPE, MEMBER_COUNT, MEMBERS (comma-separated)
- **Test**: httptest mock GET `/v1/{resourceName}`

### `gog contacts groups list`

- **API method**: `contactGroups.list`
- **Struct**: `ContactGroupsListCmd`
- **Args/Flags**:
  - `--max` (int64, default 100, alias `limit`): page size
  - `--page` (string): page token
  - `--group-fields` (string): field mask
- **Output**: JSON array `{"contactGroups": [...], "nextPageToken": "..."}` ; text table NAME, TITLE, TYPE, MEMBER_COUNT
- **Test**: httptest mock GET `/v1/contactGroups`, assert pageSize/pageToken params

### `gog contacts groups update`

- **API method**: `contactGroups.update`
- **Struct**: `ContactGroupsUpdateCmd`
- **Args/Flags**:
  - `resourceName` (required arg): contact group resource name
  - `--name` (required string): new group name
  - `--read-group-fields` (string): field mask for response
- **Output**: JSON object of updated group
- **Test**: httptest mock PUT `/v1/{resourceName}`, assert `contactGroup.name` in body

---

## Contact Group Members

### `gog contacts groups members modify`

- **API method**: `contactGroups.members.modify`
- **Struct**: `ContactGroupMembersModifyCmd`
- **Args/Flags**:
  - `resourceName` (required arg): contact group resource name
  - `--add` ([]string): resource names of contacts to add
  - `--remove` ([]string): resource names of contacts to remove
- **Behavior**: At least one of `--add` or `--remove` must be provided. Both can be used together in a single call.
- **Output**: JSON object with `notFoundResourceNames` and `memberResourceNames`; text shows summary of added/removed/not-found
- **Test**: httptest mock POST `/v1/{resourceName}/members:modify`, assert add/remove arrays in body

---

## People -- Batch Operations

### `gog contacts batch-create`

- **API method**: `people.batchCreateContacts`
- **Struct**: `ContactsBatchCreateCmd`
- **Args/Flags**:
  - `--contacts-json` (required string): JSON array of contact objects, or @filepath
  - `--read-mask` (string): field mask for response (default: "names,emailAddresses,phoneNumbers")
  - `--sources` ([]string): source types (default: ["READ_SOURCE_TYPE_CONTACT"])
- **Behavior**: Accepts up to 200 contacts per batch. Reads JSON from flag or file. Each contact follows the Person schema.
- **Output**: JSON object with `createdPeople` array; text summary "Created N contacts"
- **Test**: httptest mock POST `/v1/people:batchCreateContacts`, assert contacts array in body

### `gog contacts batch-delete`

- **API method**: `people.batchDeleteContacts`
- **Struct**: `ContactsBatchDeleteCmd`
- **Args/Flags**:
  - `--resource-names` (required []string): resource names of contacts to delete
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Deletes up to 500 contacts in a single call.
- **Output**: Empty on success; stderr shows "Deleted N contacts"
- **Test**: httptest mock POST `/v1/people:batchDeleteContacts`, assert resource names in body

### `gog contacts batch-update`

- **API method**: `people.batchUpdateContacts`
- **Struct**: `ContactsBatchUpdateCmd`
- **Args/Flags**:
  - `--contacts-json` (required string): JSON object mapping resource names to Person objects, or @filepath
  - `--update-mask` (required string): comma-separated field mask (e.g., "names,emailAddresses")
  - `--read-mask` (string): field mask for response
  - `--sources` ([]string): source types
- **Behavior**: Accepts up to 200 contacts per batch. The JSON is a map of `{resourceName: Person}`.
- **Output**: JSON object with `updateResult` map; text summary "Updated N contacts"
- **Test**: httptest mock POST `/v1/people:batchUpdateContacts`, assert contacts map in body

### `gog contacts delete-photo`

- **API method**: `people.deleteContactPhoto`
- **Struct**: `ContactsDeletePhotoCmd`
- **Args/Flags**:
  - `resourceName` (required arg): person resource name (e.g., `people/c123`)
  - `--person-fields` (string): field mask for updated person in response
  - `--force`: skip confirmation
- **Behavior**: `confirmDestructive()` required. Removes the contact's photo.
- **Output**: JSON object of updated person; text shows confirmation
- **Test**: httptest mock DELETE `/v1/{resourceName}:deleteContactPhoto`

### `gog contacts batch-get`

- **API method**: `people.getBatchGet`
- **Struct**: `ContactsBatchGetCmd`
- **Args/Flags**:
  - `--resource-names` (required []string): person resource names
  - `--person-fields` (string): field mask (default: "names,emailAddresses,phoneNumbers")
  - `--sources` ([]string): source types
- **Output**: JSON object with `responses` array; text table NAME, EMAIL, PHONE (first of each)
- **Test**: httptest mock GET `/v1/people:batchGet?resourceNames=...`

### `gog contacts update-photo`

- **API method**: `people.updateContactPhoto`
- **Struct**: `ContactsUpdatePhotoCmd`
- **Args/Flags**:
  - `resourceName` (required arg): person resource name
  - `--photo` (required string): path to image file (JPEG/PNG, max 20MB for People API)
  - `--person-fields` (string): field mask for updated person in response
- **Behavior**: Reads image file, base64-encodes it, sends as `photoBytes` in request body.
- **Output**: JSON object of updated person; text shows confirmation
- **Test**: httptest mock PATCH `/v1/{resourceName}:updateContactPhoto`, assert photoBytes in body

---

## Implementation Notes

1. **Batch operations**: All batch endpoints accept/return arrays or maps of contacts. The `--contacts-json` flag should support both inline JSON and `@filepath` syntax for reading from a file.
2. **Photo operations**: `updateContactPhoto` requires base64-encoded image data. Read the file, encode, and send. The API supports JPEG and PNG up to ~20MB.
3. **Contact groups**: System groups (like "myContacts", "starred") cannot be deleted or renamed. The CLI should handle the API error gracefully with a clear message.
4. **Field masks**: The People API uses `personFields` and `readMask` parameters extensively. Default to common fields (names, emails, phones) when not specified.
5. **Resource names**: Contact resource names are `people/c{numericId}`. Contact group names are `contactGroups/{id}`.
6. Total new test count: minimum 13 test functions plus batch edge cases (empty arrays, max limits).
