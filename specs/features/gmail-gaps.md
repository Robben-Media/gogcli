# Gmail v1 -- Gap Coverage Spec

## Overview

**API**: Gmail API v1
**Go package**: `google.golang.org/api/gmail/v1`
**Service factory**: `newGmailService` (existing in `gmail.go`)
**Currently implemented**: 45 methods -- drafts, history, labels (list/get/create/delete/modify), messages (get/list/send/trash/batch/attachments), settings (delegates/filters/forwarding/sendas/autoforward/vacation), threads, watch
**Missing methods**: 34

## Why

Missing methods prevent users from permanently deleting messages (vs trash), managing S/MIME certificates for send-as aliases, configuring IMAP/POP/language settings, handling client-side encryption identities and keypairs (enterprise), and performing label patch/update operations. These gaps block security-focused email administration and enterprise compliance workflows.

---

## Resource: Users (1 method)

### users.getProfile

| Field | Value |
|-------|-------|
| CLI | `gog gmail profile` |
| API | `Users.GetProfile("me")` |
| Output JSON | `{"profile": {...}}` |
| Output TSV | `emailAddress`, `messagesTotal`, `threadsTotal`, `historyId` |

---

## Resource: Labels -- Additional Methods (2 methods)

### labels.patch

| Field | Value |
|-------|-------|
| CLI | `gog gmail labels patch <labelIdOrName>` |
| API | `Users.Labels.Patch("me", labelId, &Label{...})` |
| Args | `labelIdOrName` (positional, required) |
| Flags | `--name`, `--label-list-visibility` (labelShow, labelShowIfUnread, labelHide), `--message-list-visibility` (show, hide), `--background-color`, `--text-color` |
| Patch logic | `flagProvided()` -- only send fields that were explicitly provided |
| Resolve | Use existing `fetchLabelNameToID()` to resolve name to ID |
| Output JSON | `{"label": {...}}` |
| Output TSV | `id`, `name`, `type`, `labelListVisibility`, `messageListVisibility` |

### labels.update

| Field | Value |
|-------|-------|
| CLI | `gog gmail labels update <labelIdOrName>` |
| API | `Users.Labels.Update("me", labelId, &Label{...})` |
| Args | `labelIdOrName` (positional, required) |
| Flags | `--name` (required), `--label-list-visibility` (required), `--message-list-visibility` (required), `--background-color`, `--text-color` |
| Notes | Full replace semantics -- all writable fields required. Print warning if optional fields omitted. |
| Output JSON | `{"label": {...}}` |
| Notes | Prefer `patch` over `update` in most cases. Consider adding help text: "Use 'labels patch' to update individual fields." |

---

## Resource: Messages -- Additional Methods (5 methods)

### messages.delete (PERMANENT)

| Field | Value |
|-------|-------|
| CLI | `gog gmail messages delete <messageId>` |
| API | `Users.Messages.Delete("me", messageId)` |
| Args | `messageId` (positional, required) |
| Flags | `--force` |
| Guard | `confirmDestructive(ctx, flags, "PERMANENTLY delete message {messageId} (this cannot be undone)")` |
| Output JSON | `{"deleted": true, "messageId": "..."}` |
| Output TSV | `Permanently deleted message: {messageId}` |
| Notes | This is NOT the same as trash. The message is permanently and irrecoverably deleted. The confirmation message must emphasize this. |

### messages.import

| Field | Value |
|-------|-------|
| CLI | `gog gmail messages import` |
| API | `Users.Messages.Import("me", &Message{...})` with media upload |
| Flags | `--file` (required, path to RFC 2822 .eml file or `-` for stdin), `--internal-date-source` (receivedTime, dateHeader), `--never-mark-spam` (bool), `--process-for-calendar` (bool), `--labels` (comma-separated label names/IDs to apply) |
| Output JSON | `{"message": {...}}` |
| Output TSV | `id`, `threadId`, `labelIds` |
| Notes | Imports a message into the mailbox as if it was received via SMTP. Does not send it. |

### messages.insert

| Field | Value |
|-------|-------|
| CLI | `gog gmail messages insert` |
| API | `Users.Messages.Insert("me", &Message{...})` with media upload |
| Flags | `--file` (required, path to RFC 2822 .eml file or `-` for stdin), `--internal-date-source` (receivedTime, dateHeader), `--labels` (comma-separated) |
| Output JSON | `{"message": {...}}` |
| Notes | Directly inserts a message into the mailbox (no SMTP processing). Similar to import but skips spam/filter processing. |

### messages.modify

| Field | Value |
|-------|-------|
| CLI | `gog gmail messages modify <messageId>` |
| API | `Users.Messages.Modify("me", messageId, &ModifyMessageRequest{...})` |
| Args | `messageId` (positional, required) |
| Flags | `--add-labels` (comma-separated label names/IDs), `--remove-labels` (comma-separated label names/IDs) |
| Resolve | Use existing `fetchLabelNameToID()` + `resolveLabelIDs()` |
| Output JSON | `{"message": {...}}` |
| Notes | Similar to existing `labels modify` but operates on a single message (not thread). Deduplicate label resolution logic. |

### messages.untrash

| Field | Value |
|-------|-------|
| CLI | `gog gmail messages untrash <messageId>` |
| API | `Users.Messages.Untrash("me", messageId)` |
| Args | `messageId` (positional, required) |
| Output JSON | `{"message": {...}}` |
| Output TSV | `Restored message: {messageId}` |

---

## Resource: Settings -- Additional Methods (6 methods)

### settings.getImap

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings imap get` |
| API | `Users.Settings.GetImap("me")` |
| Output JSON | `{"imapSettings": {...}}` |
| Output TSV | `enabled`, `autoExpunge`, `expungeBehavior`, `maxFolderSize` |

### settings.updateImap

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings imap update` |
| API | `Users.Settings.UpdateImap("me", &ImapSettings{...})` |
| Flags | `--enabled` (bool), `--auto-expunge` (bool), `--expunge-behavior` (archive, deleteForever, trash), `--max-folder-size` (int) |
| Output JSON | `{"imapSettings": {...}}` |

### settings.getPop

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings pop get` |
| API | `Users.Settings.GetPop("me")` |
| Output JSON | `{"popSettings": {...}}` |
| Output TSV | `accessWindow`, `disposition` |

### settings.updatePop

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings pop update` |
| API | `Users.Settings.UpdatePop("me", &PopSettings{...})` |
| Flags | `--access-window` (disabled, fromNowOn, allMail), `--disposition` (archive, deleteForever, leaveInInbox, markRead, trash) |

### settings.getLanguage

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings language get` |
| API | `Users.Settings.GetLanguage("me")` |
| Output JSON | `{"languageSettings": {...}}` |
| Output TSV | `displayLanguage` |

### settings.updateLanguage

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings language update` |
| API | `Users.Settings.UpdateLanguage("me", &LanguageSettings{...})` |
| Flags | `--display-language` (required, BCP 47 language tag, e.g. en, fr, ja) |

---

## Resource: Send As -- Additional Method (1 method)

### sendAs.patch

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings sendas patch <sendAsEmail>` |
| API | `Users.Settings.SendAs.Patch("me", sendAsEmail, &SendAs{...})` |
| Args | `sendAsEmail` (positional, required) |
| Flags | `--display-name`, `--reply-to-address`, `--signature`, `--is-default` (bool), `--treat-as-alias` (bool) |
| Patch logic | `flagProvided()` |
| Output JSON | `{"sendAs": {...}}` |

---

## Resource: Send As S/MIME Info (5 methods)

Path pattern: `users/me/settings/sendAs/{sendAsEmail}/smimeInfo/{smimeInfoId}`

### smimeInfo.list

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings sendas smime list <sendAsEmail>` |
| API | `Users.Settings.SendAs.SmimeInfo.List("me", sendAsEmail)` |
| Args | `sendAsEmail` (positional, required) |
| Output JSON | `{"smimeInfo": [...]}` |
| Output TSV | `ID`, `ISSUER_CN`, `IS_DEFAULT`, `EXPIRATION` |

### smimeInfo.get

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings sendas smime get <sendAsEmail> <smimeInfoId>` |
| Output JSON | `{"smimeInfo": {...}}` |
| Output TSV | `id`, `issuerCn`, `isDefault`, `expiration`, `pem` (truncated) |

### smimeInfo.insert

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings sendas smime insert <sendAsEmail>` |
| API | `Users.Settings.SendAs.SmimeInfo.Insert("me", sendAsEmail, &SmimeInfo{...})` |
| Flags | `--pkcs12` (required, base64 of PKCS#12 file or path to .p12 file), `--encrypted-key-password` (password for the PKCS#12 file), `--is-default` (bool) |
| Output JSON | `{"smimeInfo": {...}}` |
| Notes | Read .p12 file and base64-encode it if a file path is provided |

### smimeInfo.delete

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings sendas smime delete <sendAsEmail> <smimeInfoId>` |
| Guard | `confirmDestructive()` with `--force` |

### smimeInfo.setDefault

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings sendas smime set-default <sendAsEmail> <smimeInfoId>` |
| API | `Users.Settings.SendAs.SmimeInfo.SetDefault("me", sendAsEmail, smimeInfoId)` |
| Output JSON | `{"setDefault": true, "smimeInfoId": "..."}` |

---

## Resource: CSE Identities (5 methods)

Path pattern: `users/me/settings/cse/identities/{cseIdentityId}`

CSE = Client-Side Encryption, a Google Workspace Enterprise feature.

### cseIdentities.create

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse identities create` |
| API | `Users.Settings.Cse.Identities.Create("me", &CseIdentity{...})` |
| Flags | `--primary-key-pair-id` (required, CSE keypair ID), `--email-address` (required) |
| Output JSON | `{"cseIdentity": {...}}` |

### cseIdentities.delete

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse identities delete <identityId>` |
| Guard | `confirmDestructive()` with `--force` |

### cseIdentities.get

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse identities get <identityId>` |
| Output JSON | `{"cseIdentity": {...}}` |
| Output TSV | `emailAddress`, `primaryKeyPairId` |

### cseIdentities.list

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse identities list` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"cseIdentities": [...], "nextPageToken": "..."}` |
| Output TSV | `EMAIL_ADDRESS`, `PRIMARY_KEY_PAIR_ID` |

### cseIdentities.patch

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse identities patch <identityId>` |
| Flags | `--primary-key-pair-id` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: CSE Keypairs (6 methods)

Path pattern: `users/me/settings/cse/keypairs/{keypairId}`

### cseKeypairs.create

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse keypairs create` |
| API | `Users.Settings.Cse.Keypairs.Create("me", &CseKeyPair{...})` |
| Flags | `--pem` (required, PEM-encoded public key or path to .pem file) |
| Output JSON | `{"cseKeyPair": {...}}` |
| Notes | Read .pem file if path provided |

### cseKeypairs.disable

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse keypairs disable <keypairId>` |
| API | `Users.Settings.Cse.Keypairs.Disable("me", keypairId, &DisableCseKeyPairRequest{})` |
| Output JSON | `{"cseKeyPair": {...}}` |
| Notes | Disables but does not delete the keypair |

### cseKeypairs.enable

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse keypairs enable <keypairId>` |
| API | `Users.Settings.Cse.Keypairs.Enable("me", keypairId, &EnableCseKeyPairRequest{})` |
| Output JSON | `{"cseKeyPair": {...}}` |

### cseKeypairs.get

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse keypairs get <keypairId>` |
| Output JSON | `{"cseKeyPair": {...}}` |
| Output TSV | `keypairId`, `enablementState`, `pem` (truncated), `subjectEmailAddresses` |

### cseKeypairs.list

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse keypairs list` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"cseKeyPairs": [...], "nextPageToken": "..."}` |
| Output TSV | `KEYPAIR_ID`, `ENABLEMENT_STATE`, `SUBJECT_EMAILS` |

### cseKeypairs.obliterate

| Field | Value |
|-------|-------|
| CLI | `gog gmail settings cse keypairs obliterate <keypairId>` |
| API | `Users.Settings.Cse.Keypairs.Obliterate("me", keypairId, &ObliterateCseKeyPairRequest{})` |
| Flags | `--force` |
| Guard | `confirmDestructive(ctx, flags, "PERMANENTLY obliterate CSE keypair {keypairId} (messages encrypted with this key will become unreadable)")` |
| Output JSON | `{"obliterated": true, "keypairId": "..."}` |
| Notes | This permanently destroys the private key material. Messages encrypted with this keypair become permanently unreadable. The confirmation must strongly emphasize irreversibility. |

---

## Resource: Threads -- Additional Methods (3 methods)

### threads.delete (PERMANENT)

| Field | Value |
|-------|-------|
| CLI | `gog gmail threads delete <threadId>` |
| API | `Users.Threads.Delete("me", threadId)` |
| Args | `threadId` (positional, required) |
| Flags | `--force` |
| Guard | `confirmDestructive(ctx, flags, "PERMANENTLY delete thread {threadId} and all its messages (this cannot be undone)")` |
| Output JSON | `{"deleted": true, "threadId": "..."}` |

### threads.trash

| Field | Value |
|-------|-------|
| CLI | `gog gmail threads trash <threadId>` |
| API | `Users.Threads.Trash("me", threadId)` |
| Args | `threadId` (positional, required) |
| Output JSON | `{"thread": {...}}` |
| Output TSV | `Trashed thread: {threadId}` |

### threads.untrash

| Field | Value |
|-------|-------|
| CLI | `gog gmail threads untrash <threadId>` |
| API | `Users.Threads.Untrash("me", threadId)` |
| Args | `threadId` (positional, required) |
| Output JSON | `{"thread": {...}}` |
| Output TSV | `Restored thread: {threadId}` |

---

## Kong Struct Layout

```go
// Additions to GmailCmd struct:
type GmailCmd struct {
    // ... existing fields ...
    Profile GmailProfileCmd `cmd:"" name:"profile" group:"Read" help:"Get user profile info"`
}

// Additions to GmailLabelsCmd:
type GmailLabelsCmd struct {
    // ... existing fields ...
    Patch  GmailLabelsPatchCmd  `cmd:"" name:"patch" help:"Patch label (partial update)"`
    Update GmailLabelsUpdateCmd `cmd:"" name:"update" help:"Update label (full replace)"`
}

// Additions to GmailSettingsCmd -- add subcommand groups:
type GmailSettingsCmd struct {
    // ... existing fields ...
    Imap     GmailImapCmd     `cmd:"" name:"imap" group:"Protocol" help:"IMAP settings"`
    Pop      GmailPopCmd      `cmd:"" name:"pop" group:"Protocol" help:"POP settings"`
    Language GmailLanguageCmd `cmd:"" name:"language" group:"General" help:"Language settings"`
    Cse      GmailCseCmd      `cmd:"" name:"cse" group:"Security" help:"Client-side encryption"`
}

// New top-level message operations under GmailMessagesCmd or directly on GmailCmd:
// messages.delete, messages.import, messages.insert, messages.modify, messages.untrash

// Thread operations:
// threads.delete, threads.trash, threads.untrash
```

---

## Test Requirements

### Test patterns

1. **Profile**: Simple mock, verify `"me"` user ID in request path
2. **Labels patch vs update**: Verify patch sends only provided fields; update sends all fields. Test `updateMask` for patch.
3. **Messages delete**: Verify `confirmDestructive()` blocks without `--force`; verify permanent delete API call (not trash)
4. **Messages import/insert**: Mock media upload; verify RFC 2822 content in request body; test stdin reading
5. **Messages modify**: Verify label resolution; test both `--add-labels` and `--remove-labels`
6. **Messages untrash**: Verify correct API path
7. **Settings IMAP/POP/Language**: Test get (verify response parsing) and update (verify request body)
8. **SendAs patch**: Verify `flagProvided()` logic; test partial updates
9. **S/MIME**: Test .p12 file reading and base64 encoding; test setDefault; test delete guard
10. **CSE identities/keypairs**: Test CRUD; verify obliterate confirmation is extra-strong; test enable/disable state transitions
11. **Threads delete/trash/untrash**: Verify correct API paths; test permanent delete guard vs simple trash

### Factory injection

Use existing: `var newGmailService = googleapi.NewGmail`

### Test file organization

- `gmail_profile_test.go` -- profile
- `gmail_labels_patch_test.go` -- labels patch/update
- `gmail_messages_extra_test.go` -- delete, import, insert, modify, untrash
- `gmail_settings_protocol_test.go` -- IMAP, POP, language
- `gmail_sendas_extra_test.go` -- sendAs patch
- `gmail_smime_test.go` -- S/MIME CRUD
- `gmail_cse_test.go` -- CSE identities and keypairs
- `gmail_threads_extra_test.go` -- threads delete, trash, untrash
