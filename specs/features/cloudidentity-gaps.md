# Cloud Identity v1 Gap Spec

## Overview

The Cloud Identity v1 API has 60 missing methods. Currently implemented: `groups.lookup`, `groups.memberships.list`, `groups.memberships.searchTransitiveGroups`.

Cloud Identity manages groups, devices, SSO profiles, and organizational policies. The existing implementation lives in `internal/cmd/groups.go` with the service factory `newCloudIdentityService` (maps to `googleapi.NewCloudIdentityGroups`).

Key API patterns:
- Resources use name-based paths: `groups/{groupId}`, `groups/{groupId}/memberships/{membershipId}`, `devices/{deviceId}`, etc.
- Several device operations (wipe, cancelWipe, create, delete) return long-running `Operation` objects that require polling via `operations.get`
- Group lookup uses `groupKey.id` (email) rather than numeric ID
- Membership roles use a nested `MembershipRole` structure with `name` field (OWNER, MANAGER, MEMBER)

Service factory: `newCloudIdentityService` in `internal/cmd/groups.go`, returns `*cloudidentity.Service`.

---

## Long-Running Operations

The following methods return `Operation` objects instead of immediate results. The CLI must implement an operation polling loop:

- `devices.create`
- `devices.delete`
- `devices.wipe`
- `devices.cancelWipe`
- `deviceUsers.approve`
- `deviceUsers.block`
- `deviceUsers.cancelWipe`
- `deviceUsers.delete`
- `deviceUsers.wipe`

Polling pattern:
```go
op, err := svc.Devices.Wipe(name, &cloudidentity.GoogleAppsCloudidentityDevicesV1WipeDeviceRequest{}).Do()
if err != nil {
    return err
}
// Poll until done
for !op.Done {
    time.Sleep(2 * time.Second)
    op, err = svc.Operations.Get(op.Name).Do()
    if err != nil {
        return err
    }
}
if op.Error != nil {
    return fmt.Errorf("operation failed: %s", op.Error.Message)
}
```

The CLI should:
- Print "Operation started: {op.Name}" immediately
- Poll with backoff (2s initial, max 30s) until done
- Support `--no-wait` flag to return the operation name without polling
- Print final result or error when complete

---

## Resource Groups

### 1. Groups

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog groups create` | `--email` (required: group email); `--display-name` (required); `--description` (optional); `--parent` (required: `customers/{customerId}` or `organizations/{orgId}`); `--labels` (optional: `cloudidentity.googleapis.com/groups.discussion_forum` etc.) | JSON object of created group |
| **delete** | `gog groups delete <name>` | `name` (positional: `groups/{groupId}`). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **get** | `gog groups get <name>` | `name` (positional: `groups/{groupId}`) | JSON object, TSV: GROUP_ID, EMAIL, DISPLAY_NAME, DESCRIPTION |
| **getSecuritySettings** | `gog groups security-settings <name>` | `name` (positional: `groups/{groupId}/securitySettings`); `--read-mask` (optional) | JSON object with security settings |
| **list** | `gog groups list` | `--parent` (required: `customers/{customerId}`); `--max`, `--page`; `--view` (optional: BASIC, FULL) | JSON array, TSV: GROUP_ID, EMAIL, DISPLAY_NAME, MEMBER_COUNT |
| **patch** | `gog groups update <name>` | `name` (positional); `--display-name`, `--description` (optional). Uses `flagProvided()`. `--update-mask` auto-computed. | JSON object |
| **search** | `gog groups search` | `--query` (required: CEL query, e.g. `parent == 'customers/C123' && 'group@example.com' in member_key_id`); `--max`, `--page`; `--view` (optional) | JSON array, TSV: GROUP_ID, EMAIL, DISPLAY_NAME |
| **updateSecuritySettings** | `gog groups update-security-settings <name>` | `name` (positional); `--member-restriction-query` (optional); `--update-mask` (required) | JSON object |

**Test requirements:**
- `create`: verify email, displayName, parent in request body
- `delete`: confirmDestructive + force
- `get`: JSON and text output
- `list`: verify parent param, pagination
- `search`: verify CEL query parameter encoding
- `patch`: partial update with flagProvided, verify updateMask
- `getSecuritySettings`/`updateSecuritySettings`: verify readMask/updateMask params

---

### 2. Group Memberships

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **checkTransitiveMembership** | `gog groups memberships check-transitive <parent>` | `parent` (positional: `groups/{groupId}`); `--query` (required: `member_key_id == 'user@example.com'`) | JSON object with `hasMembership` bool |
| **create** | `gog groups memberships create` | `--group` (required: `groups/{groupId}`); `--email` (required: member email); `--role` (optional: OWNER, MANAGER, MEMBER, default MEMBER) | JSON object of created membership |
| **delete** | `gog groups memberships delete <name>` | `name` (positional: `groups/{groupId}/memberships/{membershipId}`). `--force`. `confirmDestructive()`. | JSON object (Operation for some, direct for others) |
| **get** | `gog groups memberships get <name>` | `name` (positional) | JSON object, TSV: MEMBERSHIP_ID, EMAIL, ROLE, TYPE |
| **getMembershipGraph** | `gog groups memberships graph <parent>` | `parent` (positional: `groups/{groupId}`); `--query` (required) | JSON object (Operation) with adjacency list graph |
| **lookup** | `gog groups memberships lookup` | `--group` (required: `groups/{groupId}`); `--email` (required: member email) | JSON object with membership name |
| **modifyMembershipRoles** | `gog groups memberships modify-roles <name>` | `name` (positional); `--add-roles` (optional, repeatable: OWNER, MANAGER, MEMBER); `--remove-roles` (optional, repeatable) | JSON object of updated membership |
| **searchDirectGroups** | `gog groups memberships search-direct` | `--parent` (required: `groups/-`); `--query` (required: `member_key_id == 'user@example.com'`); `--max`, `--page` | JSON array, TSV: GROUP_ID, EMAIL, RELATION_TYPE |
| **searchTransitiveMemberships** | `gog groups memberships search-transitive-memberships <parent>` | `parent` (positional: `groups/{groupId}`); `--max`, `--page` | JSON array, TSV: MEMBERSHIP_ID, EMAIL, ROLE |

**Test requirements:**
- `checkTransitiveMembership`: verify query encoding; verify boolean response
- `create`: verify preferredMemberKey.id and role in request
- `delete`: confirmDestructive
- `modifyMembershipRoles`: verify add/remove role arrays in request body
- `getMembershipGraph`: verify Operation polling; verify graph structure
- `lookup`: verify memberKey.id query param
- `searchDirectGroups`/`searchTransitiveMemberships`: verify pagination + query

---

### 3. Devices

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **cancelWipe** | `gog identity devices cancel-wipe <name>` | `name` (positional: `devices/{deviceId}`); `--customer` (optional) | JSON Operation (poll until complete) |
| **create** | `gog identity devices create` | `--serial-number` (optional); `--device-type` (optional); `--asset-tag` (optional); body via `--device-json` or stdin | JSON Operation (poll until complete) |
| **delete** | `gog identity devices delete <name>` | `name` (positional). `--force`. `confirmDestructive()`. `--customer` (optional) | JSON Operation (poll until complete) |
| **get** | `gog identity devices get <name>` | `name` (positional); `--customer` (optional) | JSON object, TSV: DEVICE_ID, SERIAL, MODEL, OS, STATE |
| **list** | `gog identity devices list` | `--customer` (optional); `--filter` (optional: CEL filter); `--order-by` (optional); `--max`, `--page`; `--view` (optional: COMPANY_INVENTORY, USER_ASSIGNED_DEVICES) | JSON array, TSV: DEVICE_ID, SERIAL, MODEL, OS, STATE |
| **wipe** | `gog identity devices wipe <name>` | `name` (positional). `--force`. `confirmDestructive()` (highly destructive). `--customer` (optional); `--remove-reset-lock` (optional bool) | JSON Operation (poll until complete) |

**Notes:**
- `wipe` is extremely destructive (factory resets the device). The confirmation message should be explicit: "This will FACTORY RESET device {name}. This action cannot be undone."
- `cancelWipe` can only cancel a pending wipe that has not yet been executed.

**Test requirements:**
- `wipe`/`cancelWipe`/`delete`/`create`: verify Operation polling loop; mock operation with done=false then done=true
- `get`/`list`: standard JSON/text output tests
- `wipe`: verify extra-strong confirmDestructive message
- `--no-wait` flag: verify operation name returned without polling

---

### 4. Device Users

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **approve** | `gog identity device-users approve <name>` | `name` (positional: `devices/{deviceId}/deviceUsers/{userId}`); `--customer` (optional) | JSON Operation |
| **block** | `gog identity device-users block <name>` | `name` (positional); `--customer` (optional) | JSON Operation |
| **cancelWipe** | `gog identity device-users cancel-wipe <name>` | `name` (positional); `--customer` (optional) | JSON Operation |
| **delete** | `gog identity device-users delete <name>` | `name` (positional). `--force`. `confirmDestructive()`. `--customer` (optional) | JSON Operation |
| **get** | `gog identity device-users get <name>` | `name` (positional); `--customer` (optional) | JSON object, TSV: USER_ID, EMAIL, MANAGEMENT_STATE, USER_AGENT |
| **list** | `gog identity device-users list <parent>` | `parent` (positional: `devices/{deviceId}`); `--customer` (optional); `--filter` (optional); `--max`, `--page` | JSON array, TSV: USER_ID, EMAIL, STATE |
| **lookup** | `gog identity device-users lookup <parent>` | `parent` (positional: `devices/{deviceId}`); `--android-id` (optional); `--raw-resource-id` (optional); `--user-id` (optional) | JSON object with device user names |
| **wipe** | `gog identity device-users wipe <name>` | `name` (positional). `--force`. `confirmDestructive()`. `--customer` (optional) | JSON Operation |

**Test requirements:**
- All Operation-returning methods: mock polling loop
- `approve`/`block`: verify request body sent
- `lookup`: verify query params
- `wipe`/`delete`: confirmDestructive

---

### 5. Device User Client States

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **get** | `gog identity client-states get <name>` | `name` (positional: `devices/{deviceId}/deviceUsers/{userId}/clientStates/{clientStateId}`); `--customer` (optional) | JSON object |
| **list** | `gog identity client-states list <parent>` | `parent` (positional: `devices/{deviceId}/deviceUsers/{userId}`); `--customer` (optional); `--filter` (optional); `--max`, `--page` | JSON array, TSV: CLIENT_STATE_ID, CUSTOM_ID, HEALTH_SCORE, MANAGED |
| **patch** | `gog identity client-states update <name>` | `name` (positional); `--custom-id` (optional); `--health-score` (optional: VERY_POOR, POOR, NEUTRAL, GOOD, VERY_GOOD); `--score-reason` (optional); `--compliance-state` (optional: COMPLIANT, NON_COMPLIANT); `--etag` (optional); `--customer` (optional). Uses `flagProvided()`. `--update-mask` auto-computed. | JSON object |

**Test requirements:**
- `get`/`list`: standard tests
- `patch`: verify updateMask auto-computation; verify partial update

---

### 6. Customer User Invitations

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **cancel** | `gog identity invitations cancel <name>` | `name` (positional: `customers/{customerId}/userinvitations/{email}`) | JSON Operation |
| **get** | `gog identity invitations get <name>` | `name` (positional) | JSON object, TSV: EMAIL, STATE, UPDATE_TIME |
| **isInvitableUser** | `gog identity invitations check <name>` | `name` (positional: `customers/{customerId}/userinvitations/{email}`) | JSON object with `isInvitableUser` bool |
| **list** | `gog identity invitations list <parent>` | `parent` (positional: `customers/{customerId}`); `--filter` (optional); `--order-by` (optional); `--max`, `--page` | JSON array, TSV: EMAIL, STATE, UPDATE_TIME |
| **send** | `gog identity invitations send <name>` | `name` (positional: `customers/{customerId}/userinvitations/{email}`) | JSON Operation |

**Test requirements:**
- `isInvitableUser`: verify boolean response
- `send`/`cancel`: verify Operation handling
- `list`: verify filter/order-by params

---

### 7. Inbound OIDC SSO Profiles

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog identity oidc-profiles create` | `--client-id` (required); `--client-secret` (required); `--issuer-uri` (required); `--display-name` (optional) | JSON object (Operation) |
| **delete** | `gog identity oidc-profiles delete <name>` | `name` (positional: `inboundSsoAssignments/{profileId}`). `--force`. `confirmDestructive()`. | JSON Operation |
| **get** | `gog identity oidc-profiles get <name>` | `name` (positional) | JSON object |
| **list** | `gog identity oidc-profiles list` | `--filter` (optional); `--max`, `--page` | JSON array, TSV: PROFILE_ID, DISPLAY_NAME, ISSUER_URI |
| **patch** | `gog identity oidc-profiles update <name>` | `name` (positional); `--client-id`, `--client-secret`, `--issuer-uri`, `--display-name` (optional). Uses `flagProvided()`. `--update-mask` auto-computed. | JSON object (Operation) |

**Test requirements:**
- Full CRUD cycle; verify client-id/secret/issuer in request
- `delete`: confirmDestructive (deleting SSO profile can lock out users)

---

### 8. Inbound SAML SSO Profiles

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog identity saml-profiles create` | `--idp-entity-id` (required); `--sso-url` (required); `--idp-certificate` (required, file path); `--display-name` (optional); `--change-password-uri` (optional); `--log-out-redirect-uri` (optional) | JSON object (Operation) |
| **delete** | `gog identity saml-profiles delete <name>` | `name` (positional). `--force`. `confirmDestructive()`. | JSON Operation |
| **get** | `gog identity saml-profiles get <name>` | `name` (positional) | JSON object |
| **list** | `gog identity saml-profiles list` | `--filter` (optional); `--max`, `--page` | JSON array, TSV: PROFILE_ID, DISPLAY_NAME, IDP_ENTITY_ID |
| **patch** | `gog identity saml-profiles update <name>` | `name` (positional); all create fields optional. Uses `flagProvided()`. `--update-mask` auto-computed. | JSON object (Operation) |

**Test requirements:**
- Full CRUD cycle; verify certificate read from file
- `delete`: confirmDestructive (same SSO lockout risk)

---

### 9. SAML IDP Credentials

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **add** | `gog identity saml-credentials add <parent>` | `parent` (positional: `inboundSamlSsoProfiles/{profileId}`); `--pem-data` (required, file path or inline) | JSON object (Operation) |
| **delete** | `gog identity saml-credentials delete <name>` | `name` (positional). `--force`. `confirmDestructive()`. | JSON Operation |
| **get** | `gog identity saml-credentials get <name>` | `name` (positional) | JSON object |
| **list** | `gog identity saml-credentials list <parent>` | `parent` (positional) | JSON array, TSV: CREDENTIAL_ID, NOT_AFTER |

**Test requirements:**
- `add`: verify PEM data read from file; verify parent path
- Full CRUD cycle

---

### 10. Inbound SSO Assignments

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **create** | `gog identity sso-assignments create` | `--customer` (required); `--target-group` (optional: group resource name); `--target-org-unit` (optional: orgunit path); `--sso-profile` (required: SSO profile name); `--sso-mode` (optional: SSO_OFF, SAML_SSO, DOMAIN_WIDE_SAML_IF_ENABLED) | JSON object (Operation) |
| **delete** | `gog identity sso-assignments delete <name>` | `name` (positional). `--force`. `confirmDestructive()`. | JSON Operation |
| **get** | `gog identity sso-assignments get <name>` | `name` (positional) | JSON object |
| **list** | `gog identity sso-assignments list` | `--filter` (optional); `--max`, `--page` | JSON array, TSV: ASSIGNMENT_ID, SSO_PROFILE, TARGET, SSO_MODE |
| **patch** | `gog identity sso-assignments update <name>` | `name` (positional); `--sso-profile`, `--sso-mode` (optional). Uses `flagProvided()`. `--update-mask` auto-computed. | JSON object (Operation) |

**Test requirements:**
- Full CRUD cycle; verify target-group/target-org-unit mutual targeting
- `delete`: confirmDestructive

---

### 11. Policies

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **get** | `gog identity policies get <name>` | `name` (positional: `policies/{policyId}`) | JSON object |
| **list** | `gog identity policies list` | `--filter` (optional); `--max`, `--page` | JSON array, TSV: POLICY_ID, TYPE, SETTING |

**Test requirements:**
- `get`/`list`: standard JSON/text output tests

---

## Command Namespace Design

The existing implementation uses `gog groups` for Cloud Identity groups. New resources need to be organized without breaking existing commands:

| Resource | CLI Namespace | Rationale |
|----------|--------------|-----------|
| Groups (existing) | `gog groups` | Keep existing namespace |
| Group Memberships (partially existing) | `gog groups memberships` | Extend existing |
| Devices | `gog identity devices` | New namespace |
| Device Users | `gog identity device-users` | Under identity |
| Device User Client States | `gog identity client-states` | Under identity |
| Customer User Invitations | `gog identity invitations` | Under identity |
| Inbound OIDC SSO Profiles | `gog identity oidc-profiles` | Under identity |
| Inbound SAML SSO Profiles | `gog identity saml-profiles` | Under identity |
| SAML IDP Credentials | `gog identity saml-credentials` | Under identity |
| Inbound SSO Assignments | `gog identity sso-assignments` | Under identity |
| Policies | `gog identity policies` | Under identity |

This adds a new top-level `IdentityCmd` struct registered in `root.go` alongside the existing `GroupsCmd`.

---

## Edge Cases

1. **Operation polling timeout**: Long-running operations (especially device wipe) can take minutes. The CLI should have a `--timeout` flag (default 5m) and a `--no-wait` flag to skip polling.

2. **Customer ID resolution**: Many endpoints require a `customers/{customerId}` parent. The CLI should support `--customer` flag and optionally auto-resolve the customer ID from the authenticated account using the Directory API.

3. **Group key lookup**: The existing `groups.lookup` uses `groupKey.id` (email address). New group commands should accept either email or group name path, with automatic lookup when an email is provided.

4. **Device wipe safety**: `devices.wipe` factory-resets a device. The confirmation message must be unambiguous about the consequences. Consider requiring `--confirm-wipe` in addition to `--force`.

5. **SSO profile deletion lockout risk**: Deleting an SSO profile or assignment can prevent users from signing in. The confirmation message should warn about this risk.

6. **updateMask computation**: Cloud Identity patch methods require an explicit `updateMask` field listing which fields are being modified. This must be auto-computed from `flagProvided()` checks, e.g.:
   ```go
   var masks []string
   if flagProvided(kctx, "display-name") {
       masks = append(masks, "displayName")
       // ...
   }
   call.UpdateMask(strings.Join(masks, ","))
   ```

7. **Membership role modification**: `modifyMembershipRoles` uses add/remove role arrays rather than a simple set. The CLI must validate that you cannot remove all roles (at least MEMBER must remain).

8. **CEL query syntax**: Several list/search methods accept CEL (Common Expression Language) queries via `--query` or `--filter`. The CLI should pass these through as-is without parsing, but should document example queries in help text.

---

## Test Requirements Summary

Every method requires at minimum:
1. **JSON output test**: httptest mock returning representative JSON
2. **Text output test**: TSV/key-value output verification
3. **Error test**: Missing required args
4. **Delete tests**: confirmDestructive + --force
5. **Operation tests**: For LRO methods, mock operation polling (done=false then done=true)
6. **--no-wait test**: For LRO methods, verify operation name returned without polling

All tests use:
```go
origNew := newCloudIdentityService
t.Cleanup(func() { newCloudIdentityService = origNew })
```

Total test count estimate: ~130 tests (2 per method average across 60 methods, plus extra Operation polling tests).
