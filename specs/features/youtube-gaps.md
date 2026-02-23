# YouTube v3 Gap Spec

## Overview

The YouTube Data API v3 has 77 missing methods. Currently implemented: `channels.list`, `commentThreads.list`, `playlistItems.list`, `playlists.list`, `search.list`, `videos.list`.

YouTube uses a flat resource model (no nested paths). Most resources are identified by their `id` field. List operations use `part` parameters to specify which resource properties to include (e.g. `snippet`, `statistics`, `contentDetails`).

Service factory: `newYoutubeService` in `internal/cmd/youtube.go`, returns `*youtube.Service`.

Key API patterns:
- `part` parameter is required on every call (specifies which resource properties to return)
- Pagination via `maxResults` + `pageToken` (mapped to `--max`/`--page`)
- Upload methods (videos.insert, captions.insert, thumbnails.set) use multipart upload with `media.Media()` option
- Rate/report methods return empty responses (HTTP 204)

---

## Resource Groups

### 1. Videos

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt videos delete <videoId>` | `videoId` (positional). `--force`. `confirmDestructive()`. `--on-behalf-of-content-owner` (optional) | JSON `{"deleted": true, "videoId": "..."}` |
| **getRating** | `gog yt videos get-rating` | `--id` (required, comma-separated video IDs); `--on-behalf-of-content-owner` (optional) | JSON array, TSV: VIDEO_ID, RATING |
| **insert** | `gog yt videos upload <file>` | `file` (positional): path to video file; `--title` (required); `--description` (optional); `--tags` (optional, comma-separated); `--category-id` (optional); `--privacy-status` (optional: public, private, unlisted, default "private"); `--notify-subscribers` (optional bool); `--on-behalf-of-content-owner` (optional); `--on-behalf-of-content-owner-channel` (optional) | JSON object of created video. **Multipart upload.** |
| **rate** | `gog yt videos rate <videoId>` | `videoId` (positional); `--rating` (required: like, dislike, none) | JSON `{"rated": true}` (HTTP 204) |
| **reportAbuse** | `gog yt videos report-abuse <videoId>` | `videoId` (positional); `--reason-id` (required); `--secondary-reason-id` (optional); `--comments` (optional); `--language` (optional) | JSON `{"reported": true}` (HTTP 204) |
| **update** | `gog yt videos update <videoId>` | `videoId` (positional); `--title`, `--description`, `--tags`, `--category-id`, `--privacy-status` (all optional). Uses `flagProvided()`. `--part` computed from provided flags. | JSON object of updated video |

**Special handling for upload:**
- `videos upload` uses `media.Media(reader)` on the insert call
- Must set `Content-Type` based on file extension (mp4, mov, avi, etc.)
- Large files benefit from resumable upload; display progress bar on stderr
- Default privacy to "private" for safety

**Test requirements:**
- `delete`: confirmDestructive + force bypass
- `getRating`: mock returning rating list; verify comma-separated ID handling
- `upload`: mock multipart upload endpoint; verify snippet fields in request body; verify media attachment
- `rate`: mock 204 response; verify rating enum validation
- `reportAbuse`: mock 204; verify reasonId in request
- `update`: partial update with flagProvided; verify `part` auto-computed

---

### 2. Channels

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **update** | `gog yt channels update <channelId>` | `channelId` (positional); `--description`, `--default-language`, `--country`, `--keywords` (all optional). Uses `flagProvided()`. `--part` auto-computed. | JSON object of updated channel |

**Test requirements:**
- Verify partial update; verify brandingSettings fields; verify part auto-computation

---

### 3. Comments

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt comments delete <commentId>` | `commentId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt comments insert` | `--parent-id` (required: comment thread ID or comment ID for reply); `--text` (required) | JSON object of created comment |
| **list** | `gog yt comments list` | `--parent-id` (required); `--max`, `--page`; `--text-format` (optional: html, plainText) | JSON array, TSV: COMMENT_ID, AUTHOR, TEXT, LIKES, PUBLISHED |
| **markAsSpam** | `gog yt comments mark-as-spam` | `--id` (required, comma-separated) | JSON `{"marked": true}` (HTTP 204) |
| **setModerationStatus** | `gog yt comments set-moderation-status` | `--id` (required, comma-separated); `--moderation-status` (required: heldForReview, published, rejected); `--ban-author` (optional bool) | JSON `{"updated": true}` (HTTP 204) |
| **update** | `gog yt comments update <commentId>` | `commentId` (positional); `--text` (required) | JSON object of updated comment |

**Test requirements:**
- Full CRUD cycle
- `markAsSpam`: verify comma-separated IDs
- `setModerationStatus`: verify enum validation for moderation status

---

### 4. Comment Threads

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **insert** | `gog yt comment-threads insert` | `--channel-id` (required); `--video-id` (required); `--text` (required) | JSON object of created comment thread |

**Test requirements:**
- Mock POST; verify snippet.topLevelComment.snippet.textOriginal set

---

### 5. Captions

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt captions delete <captionId>` | `captionId` (positional). `--force`. `confirmDestructive()`. `--on-behalf-of-content-owner` (optional) | JSON `{"deleted": true}` |
| **download** | `gog yt captions download <captionId>` | `captionId` (positional); `--tfmt` (optional: sbv, scc, srt, ttml, vtt); `--tlang` (optional: translate to language); `--output` (optional: file path, default stdout) | Raw caption text to stdout or file |
| **insert** | `gog yt captions upload <file>` | `file` (positional); `--video-id` (required); `--language` (required); `--name` (required); `--is-draft` (optional bool). **Multipart upload.** | JSON object of created caption |
| **list** | `gog yt captions list` | `--video-id` (required) | JSON array, TSV: CAPTION_ID, LANGUAGE, NAME, IS_DRAFT |
| **update** | `gog yt captions update <captionId>` | `captionId` (positional); `--name`, `--is-draft` (optional). Uses `flagProvided()`. Optional `--file` for new caption content (multipart upload). | JSON object |

**Special handling for upload/download:**
- `upload` uses multipart with `media.Media(reader)`
- `download` returns raw text body; write to stdout or `--output` file

**Test requirements:**
- `download`: mock returning raw text; verify tfmt/tlang query params; verify file output
- `upload`: mock multipart; verify videoId, language, name in request
- Full CRUD cycle

---

### 6. Channel Banners

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **insert** | `gog yt channel-banners upload <file>` | `file` (positional): image file path. **Multipart upload.** `--on-behalf-of-content-owner` (optional); `--on-behalf-of-content-owner-channel` (optional) | JSON object with `url` field for use in channels.update |

**Test requirements:**
- Mock multipart upload; verify response contains URL

---

### 7. Channel Sections

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt channel-sections delete <sectionId>` | `sectionId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt channel-sections insert` | `--type` (required: allPlaylists, completedEvents, likedPlaylists, likes, liveEvents, multipleChannels, multiplePlaylists, popularUploads, recentActivity, recentPosts, singlePlaylist, subscriptions, upcomingEvents); `--title` (optional); `--position` (optional int); `--playlist-id` (optional, for singlePlaylist); `--channel-ids` (optional, comma-separated for multipleChannels) | JSON object |
| **list** | `gog yt channel-sections list` | `--channel-id` (required); `--mine` (optional bool) | JSON array, TSV: SECTION_ID, TYPE, TITLE, POSITION |
| **update** | `gog yt channel-sections update <sectionId>` | `sectionId` (positional); `--title`, `--position`, `--type` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle; verify type enum validation

---

### 8. Playlists

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt playlists delete <playlistId>` | `playlistId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt playlists insert` | `--title` (required); `--description` (optional); `--privacy-status` (optional: public, private, unlisted); `--tags` (optional, comma-separated); `--default-language` (optional) | JSON object |
| **update** | `gog yt playlists update <playlistId>` | `playlistId` (positional); `--title`, `--description`, `--privacy-status`, `--tags`, `--default-language` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle (list already exists); verify privacy-status enum

---

### 9. Playlist Items

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt playlist-items delete <itemId>` | `itemId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt playlist-items insert` | `--playlist-id` (required); `--video-id` (required); `--position` (optional int); `--note` (optional) | JSON object |
| **update** | `gog yt playlist-items update <itemId>` | `itemId` (positional); `--playlist-id` (required); `--video-id` (required); `--position`, `--note` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle; verify position ordering

---

### 10. Playlist Images

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt playlist-images delete <imageId>` | `imageId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt playlist-images upload <file>` | `file` (positional); `--playlist-id` (required); `--type` (required: hero). **Multipart upload.** | JSON object |
| **list** | `gog yt playlist-images list` | `--playlist-id` (required); `--max`, `--page` | JSON array, TSV: IMAGE_ID, TYPE |
| **update** | `gog yt playlist-images update <imageId>` | `imageId` (positional); `--playlist-id` (required); `--type` (optional). Optional `--file` for new image. Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle; verify multipart upload for insert/update

---

### 11. Subscriptions

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt subscriptions delete <subscriptionId>` | `subscriptionId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt subscriptions insert` | `--channel-id` (required: channel to subscribe to) | JSON object |
| **list** | `gog yt subscriptions list` | `--mine` (optional bool); `--channel-id` (optional); `--for-channel-id` (optional); `--max`, `--page`; `--order` (optional: alphabetical, relevance, unread) | JSON array, TSV: SUBSCRIPTION_ID, CHANNEL_TITLE, CHANNEL_ID |

**Test requirements:**
- Full CRUD cycle; verify list filter combinations

---

### 12. Thumbnails

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **set** | `gog yt thumbnails set <file>` | `file` (positional): image file; `--video-id` (required). **Multipart upload.** | JSON object with thumbnail URLs |

**Test requirements:**
- Mock multipart upload; verify video-id in request; verify response thumbnail URLs

---

### 13. Watermarks

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **set** | `gog yt watermarks set <file>` | `file` (positional): image file; `--channel-id` (required); `--timing-type` (optional: offsetFromStart, offsetFromEnd); `--timing-offset-ms` (optional int64); `--timing-duration-ms` (optional int64). **Multipart upload.** | JSON `{"set": true}` (HTTP 204) |
| **unset** | `gog yt watermarks unset` | `--channel-id` (required). `--force`. `confirmDestructive()`. | JSON `{"unset": true}` (HTTP 204) |

**Test requirements:**
- `set`: mock multipart upload; verify timing parameters
- `unset`: verify confirmDestructive; mock 204

---

### 14. Live Broadcasts

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **bind** | `gog yt live-broadcasts bind <broadcastId>` | `broadcastId` (positional); `--stream-id` (required) | JSON object |
| **delete** | `gog yt live-broadcasts delete <broadcastId>` | `broadcastId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt live-broadcasts insert` | `--title` (required); `--scheduled-start-time` (required, RFC3339); `--scheduled-end-time` (optional); `--description` (optional); `--privacy-status` (optional: public, private, unlisted); `--is-default-broadcast` (optional bool) | JSON object |
| **insertCuepoint** | `gog yt live-broadcasts insert-cuepoint <broadcastId>` | `broadcastId` (positional); `--cue-type` (optional: cueTypeAd); `--duration-secs` (optional int); `--offset-time-ms` (optional int64) | JSON object |
| **list** | `gog yt live-broadcasts list` | `--mine` (optional bool); `--broadcast-status` (optional: active, all, completed, upcoming); `--id` (optional); `--max`, `--page` | JSON array, TSV: BROADCAST_ID, TITLE, STATUS, START_TIME |
| **transition** | `gog yt live-broadcasts transition <broadcastId>` | `broadcastId` (positional); `--broadcast-status` (required: complete, live, testing) | JSON object |
| **update** | `gog yt live-broadcasts update <broadcastId>` | `broadcastId` (positional); `--title`, `--description`, `--scheduled-start-time`, `--scheduled-end-time`, `--privacy-status` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle + bind/transition/insertCuepoint
- `transition`: verify status enum; verify broadcast goes through correct state machine
- `bind`: verify stream-id in request

---

### 15. Live Chat Bans

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt live-chat-bans delete <banId>` | `banId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt live-chat-bans insert` | `--live-chat-id` (required); `--banned-user-channel-id` (required); `--type` (required: permanent, temporary); `--ban-duration-seconds` (optional, for temporary) | JSON object |

**Test requirements:**
- Insert + delete cycle; verify ban type enum; verify duration only sent for temporary

---

### 16. Live Chat Messages

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt live-chat-messages delete <messageId>` | `messageId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt live-chat-messages insert` | `--live-chat-id` (required); `--text` (required); `--type` (optional: textMessageEvent) | JSON object |
| **list** | `gog yt live-chat-messages list` | `--live-chat-id` (required); `--max`, `--page` | JSON array, TSV: MESSAGE_ID, AUTHOR, TEXT, PUBLISHED |
| **transition** | `gog yt live-chat-messages transition` | `--live-chat-id` (required) | JSON object |

**Test requirements:**
- Full CRUD + transition cycle

---

### 17. Live Chat Moderators

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt live-chat-moderators delete <moderatorId>` | `moderatorId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt live-chat-moderators insert` | `--live-chat-id` (required); `--channel-id` (required) | JSON object |
| **list** | `gog yt live-chat-moderators list` | `--live-chat-id` (required); `--max`, `--page` | JSON array, TSV: MODERATOR_ID, CHANNEL_ID, DISPLAY_NAME |

**Test requirements:**
- Insert + delete + list cycle

---

### 18. Live Streams

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt live-streams delete <streamId>` | `streamId` (positional). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt live-streams insert` | `--title` (required); `--description` (optional); `--format` (optional: 1080p, 1440p, 2160p, 240p, 360p, 480p, 720p); `--ingestion-type` (optional: dash, hls, rtmp, webrtc) | JSON object |
| **list** | `gog yt live-streams list` | `--mine` (optional bool); `--id` (optional); `--max`, `--page` | JSON array, TSV: STREAM_ID, TITLE, STATUS, FORMAT |
| **update** | `gog yt live-streams update <streamId>` | `streamId` (positional); `--title`, `--description` (optional). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle; verify format/ingestion-type enums

---

### 19. Members

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt members list` | `--mode` (optional: listMembersByFilteredMember, listMembershipsForChannel); `--max`, `--page`; `--has-access-to-level` (optional); `--filter-by-member-channel-id` (optional, comma-separated) | JSON array, TSV: CHANNEL_ID, DISPLAY_NAME, LEVEL, SINCE |

**Test requirements:**
- Mock list response; verify filter parameters

---

### 20. Memberships Levels

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt memberships-levels list` | (no required args) | JSON array, TSV: LEVEL_ID, DISPLAY_NAME, AMOUNT |

**Test requirements:**
- Mock list; verify simple response

---

### 21. Super Chat Events

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt super-chat-events list` | `--max`, `--page` | JSON array, TSV: EVENT_ID, SUPPORTER, AMOUNT, CURRENCY, COMMENT |

**Test requirements:**
- Mock list; verify pagination

---

### 22. i18n Languages

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt i18n-languages list` | `--hl` (optional: host language for response) | JSON array, TSV: LANGUAGE_CODE, NAME |

**Test requirements:**
- Mock list; verify hl query param

---

### 23. i18n Regions

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt i18n-regions list` | `--hl` (optional) | JSON array, TSV: REGION_CODE, NAME |

**Test requirements:**
- Mock list; verify hl query param

---

### 24. Video Abuse Report Reasons

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt video-abuse-report-reasons list` | `--hl` (optional) | JSON array, TSV: REASON_ID, LABEL, SECONDARY_REASONS |

**Test requirements:**
- Mock list; verify nested secondary reasons in response

---

### 25. Video Categories

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt video-categories list` | `--region-code` (required); `--hl` (optional) | JSON array, TSV: CATEGORY_ID, TITLE, ASSIGNABLE |

**Test requirements:**
- Mock list; verify regionCode query param

---

### 26. Abuse Reports

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **insert** | `gog yt abuse-reports insert` | `--type` (required); `--related-entity-id` (required); `--description` (optional); `--subject-id` (optional) | JSON object |

**Test requirements:**
- Mock POST; verify body fields

---

### 27. Activities

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **list** | `gog yt activities list` | `--mine` (optional bool); `--channel-id` (optional); `--max`, `--page`; `--published-after` (optional RFC3339); `--published-before` (optional RFC3339) | JSON array, TSV: ACTIVITY_ID, TYPE, TITLE, PUBLISHED |

**Test requirements:**
- Mock list; verify date range filtering; verify mine/channel-id mutual exclusion

---

### 28. Third Party Links

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **delete** | `gog yt third-party-links delete <linkId>` | `linkId` (positional); `--type` (required). `--force`. `confirmDestructive()`. | JSON `{"deleted": true}` |
| **insert** | `gog yt third-party-links insert` | `--type` (required); `--linking-token` (required) | JSON object |
| **list** | `gog yt third-party-links list` | `--type` (required); `--id` (optional) | JSON array |
| **update** | `gog yt third-party-links update <linkId>` | `linkId` (positional); `--type` (required). Uses `flagProvided()`. | JSON object |

**Test requirements:**
- Full CRUD cycle; verify type parameter always sent

---

### 29. Tests (YouTube API test endpoint)

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **insert** | `gog yt tests insert` | (body fields TBD per API) | JSON object |

**Note:** This is a testing/debug endpoint in the YouTube API. Low priority.

---

### 30. Video Trainability

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **get** | `gog yt video-trainability get <videoId>` | `videoId` (positional) | JSON object |

**Test requirements:**
- Mock GET; verify videoId in path

---

### 31. Live Chat Messages Stream

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **stream** | `gog yt live-chat stream` | `--live-chat-id` (required); `--poll-interval` (optional, default 5s) | Streaming JSON lines to stdout; each line is a new message. Polls on interval. Ctrl+C to stop. |

**Special handling:** This is a long-polling operation, not a standard request/response. Repeatedly calls `liveChatMessages.list` with the `nextPageToken` and a sleep interval.

**Test requirements:**
- Mock returning messages with pageToken; verify polling loop terminates on context cancel

---

### 32. Update Comment Threads

| Method | CLI Command | Args/Flags | Output |
|--------|-------------|------------|--------|
| **update** | `gog yt comment-threads update <threadId>` | `threadId` (positional); `--text` (required, updates top-level comment text) | JSON object |

**Test requirements:**
- Mock PUT; verify snippet.topLevelComment.snippet.textOriginal in body

---

## Upload Method Summary

Methods requiring multipart upload handling:

| Method | Command | Content-Type |
|--------|---------|-------------|
| videos.insert | `gog yt videos upload` | video/* (mp4, mov, avi, etc.) |
| captions.insert | `gog yt captions upload` | text/plain or application/octet-stream |
| captions.update | `gog yt captions update` | text/plain or application/octet-stream (optional) |
| thumbnails.set | `gog yt thumbnails set` | image/* (jpeg, png) |
| watermarks.set | `gog yt watermarks set` | image/* (jpeg, png) |
| channel-banners.insert | `gog yt channel-banners upload` | image/* (jpeg, png) |
| playlist-images.insert | `gog yt playlist-images upload` | image/* (jpeg, png) |
| playlist-images.update | `gog yt playlist-images update` | image/* (optional) |

Implementation pattern for upload:
```go
f, err := os.Open(filePath)
if err != nil {
    return fmt.Errorf("open file: %w", err)
}
defer f.Close()

call := svc.Videos.Insert([]string{"snippet", "status"}, &youtube.Video{...})
call.Media(f)
resp, err := call.Do()
```

---

## Edge Cases

1. **Part parameter computation**: YouTube requires a `part` parameter listing which resource properties to include. For update commands, `part` should be auto-computed from which flags the user provided (e.g. if only `--title` is set, part = "snippet"; if `--privacy-status` is set, part includes "status").

2. **Quota costs**: YouTube API calls have different quota costs. Upload operations are expensive (1600 units). The CLI should not warn about quotas, but documentation should note it.

3. **Upload resumability**: Large video uploads should use resumable upload. The Google API client handles this automatically when using `media.Media()` with a file reader, but progress reporting requires wrapping the reader.

4. **Rate limiting**: YouTube is aggressive with rate limiting. The existing retry transport in `internal/googleapi/client.go` should handle 403/429 responses.

5. **OAuth scopes**: Different operations require different scopes. Upload requires `youtube.upload`, force-ssl for write operations. The service factory must request appropriate scopes.

6. **Empty 204 responses**: Several methods (rate, reportAbuse, watermarks.set, watermarks.unset, markAsSpam, setModerationStatus) return HTTP 204 with no body. The response handler must not try to decode JSON from these.

7. **Live streaming state machine**: Live broadcasts must transition through states in order: created -> testing -> live -> complete. The CLI should not enforce this but should surface API errors clearly.

---

## Test Requirements Summary

Every method requires at minimum:
1. **JSON output test**: httptest mock returning representative JSON
2. **Text output test**: Verify TSV/key-value output
3. **Error test**: Missing required args
4. **Delete tests**: confirmDestructive + --force
5. **Upload tests**: Verify multipart boundary in request; verify Content-Type

All tests use:
```go
origNew := newYoutubeService
t.Cleanup(func() { newYoutubeService = origNew })
```

Total test count estimate: ~160 tests (2 per method average across 77 methods, plus extra upload tests).
