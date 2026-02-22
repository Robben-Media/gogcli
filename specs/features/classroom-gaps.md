# Classroom v1 -- Gap Coverage Spec

## Overview

**API**: Google Classroom API v1
**Go package**: `google.golang.org/api/classroom/v1`
**Service factory**: `newClassroomService` (existing in `classroom.go`)
**Currently implemented**: 53 methods (courses, students, teachers, coursework, materials, submissions, announcements, topics, invitations, guardians, profiles)
**Missing methods**: 51

## Why

The missing methods cover add-on integrations (required for LTI/third-party tool vendors), rubrics (critical for grading workflows), student groups (collaborative learning), course aliases (LMS integration keys), and registration webhooks (push notifications for course changes). Without these, gogcli cannot support the full Classroom admin and developer workflow.

---

## Resource: Course Aliases (3 methods)

Path pattern: `courses/{courseId}/aliases`

### courses.aliases.create

| Field | Value |
|-------|-------|
| CLI | `gog classroom aliases create --course <courseId>` |
| API | `Courses.Aliases.Create(courseId, &CourseAlias{...})` |
| Flags | `--course` (required), `--alias` (required, must be prefixed with `d:` for domain or `p:` for project) |
| Output JSON | `{"alias": {...}}` |
| Output TSV | `alias` |

### courses.aliases.delete

| Field | Value |
|-------|-------|
| CLI | `gog classroom aliases delete --course <courseId> <alias>` |
| API | `Courses.Aliases.Delete(courseId, alias)` |
| Args | `alias` (positional, required) |
| Flags | `--course` (required), `--force` |
| Guard | `confirmDestructive()` |
| Output JSON | `{"deleted": true, "alias": "..."}` |

### courses.aliases.list

| Field | Value |
|-------|-------|
| CLI | `gog classroom aliases list --course <courseId>` |
| API | `Courses.Aliases.List(courseId)` |
| Flags | `--course` (required), `--page-size`, `--page-token` |
| Output JSON | `{"aliases": [...], "nextPageToken": "..."}` |
| Output TSV | `ALIAS` |

---

## Resource: Course Updates (3 methods)

### courses.update (full replace)

| Field | Value |
|-------|-------|
| CLI | `gog classroom courses update <courseId>` |
| API | `Courses.Update(courseId, &Course{...})` |
| Args | `courseId` (positional, required) |
| Flags | `--name` (required), `--section`, `--description-heading`, `--description`, `--room`, `--owner-id`, `--course-state` (ACTIVE, ARCHIVED, PROVISIONED, DECLINED, SUSPENDED) |
| Output JSON | `{"course": {...}}` |
| Notes | Full replace semantics -- all writable fields must be provided. Different from existing `courses.patch`. Warn user if fields are missing. |

### courses.getGradingPeriodSettings

| Field | Value |
|-------|-------|
| CLI | `gog classroom courses grading-periods get <courseId>` |
| API | `Courses.GetGradingPeriodSettings(courseId)` -- path: `courses/{courseId}/gradingPeriodSettings` |
| Args | `courseId` (positional, required) |
| Output JSON | `{"gradingPeriodSettings": {...}}` |
| Output TSV | key-value pairs of grading period config |

### courses.updateGradingPeriodSettings

| Field | Value |
|-------|-------|
| CLI | `gog classroom courses grading-periods update <courseId>` |
| API | `Courses.UpdateGradingPeriodSettings(courseId, &GradingPeriodSettings{...})` |
| Args | `courseId` (positional, required) |
| Flags | `--grading-periods` (JSON array of grading period objects), `--apply-to-existing-coursework` (bool) |
| Patch logic | `flagProvided()` for `updateMask` |
| Output JSON | `{"gradingPeriodSettings": {...}}` |

---

## Resource: Add-On Attachments (20 methods)

Add-on attachments exist under four parent resource types: announcements, courseWork, courseWorkMaterials, and posts. Each parent has create/delete/get/list/patch operations.

Path patterns:
- `courses/{courseId}/announcements/{announcementId}/addOnAttachments/{attachmentId}`
- `courses/{courseId}/courseWork/{courseWorkId}/addOnAttachments/{attachmentId}`
- `courses/{courseId}/courseWorkMaterials/{materialId}/addOnAttachments/{attachmentId}`
- `courses/{courseId}/posts/{postId}/addOnAttachments/{attachmentId}`

### Implementation strategy

Use a shared parent type flag to reduce code duplication:

```go
type ClassroomAddOnCmd struct {
    Create ClassroomAddOnCreateCmd `cmd:"" name:"create" help:"Create add-on attachment"`
    Delete ClassroomAddOnDeleteCmd `cmd:"" name:"delete" help:"Delete add-on attachment"`
    Get    ClassroomAddOnGetCmd    `cmd:"" name:"get" help:"Get add-on attachment"`
    List   ClassroomAddOnListCmd   `cmd:"" name:"list" help:"List add-on attachments"`
    Patch  ClassroomAddOnPatchCmd  `cmd:"" name:"patch" help:"Patch add-on attachment"`
}
```

### addOnAttachments.create (x4 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons create --course <id> --item-type announcement --item-id <id>` |
| Flags | `--course` (required), `--item-type` (required: announcement, coursework, material, post), `--item-id` (required), `--title` (required), `--uri` (required), `--student-work-uri`, `--teacher-uri`, `--max-points` |
| API routing | Based on `--item-type`, call the corresponding parent's `AddOnAttachments.Create()` |
| Output JSON | `{"addOnAttachment": {...}}` |

### addOnAttachments.delete (x4 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons delete --course <id> --item-type <type> --item-id <id> <attachmentId>` |
| Guard | `confirmDestructive()` with `--force` |
| Output JSON | `{"deleted": true}` |

### addOnAttachments.get (x4 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons get --course <id> --item-type <type> --item-id <id> <attachmentId>` |
| Output JSON | `{"addOnAttachment": {...}}` |
| Output TSV | `id`, `title`, `uri`, `createTime`, `state` |

### addOnAttachments.list (x4 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons list --course <id> --item-type <type> --item-id <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"addOnAttachments": [...], "nextPageToken": "..."}` |
| Output TSV | `ID`, `TITLE`, `URI`, `STATE` |

### addOnAttachments.patch (x4 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons patch --course <id> --item-type <type> --item-id <id> <attachmentId>` |
| Flags | `--title`, `--student-work-uri`, `--teacher-uri`, `--max-points` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Add-On Context (4 methods)

### getAddOnContext (x4 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons context --course <id> --item-type <type> --item-id <id>` |
| API | `Courses.{Parent}.GetAddOnContext(courseId, itemId)` |
| Output JSON | `{"addOnContext": {...}}` |
| Output TSV | `courseId`, `itemId`, `supportsStudentWork`, `studentContext`/`teacherContext` fields |

---

## Resource: Add-On Student Submissions (4 methods)

Available under courseWork and posts parents only.

Path patterns:
- `courses/{courseId}/courseWork/{courseWorkId}/addOnAttachments/{attachmentId}/studentSubmissions/{submissionId}`
- `courses/{courseId}/posts/{postId}/addOnAttachments/{attachmentId}/studentSubmissions/{submissionId}`

### studentSubmissions.get (x2 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons submissions get --course <id> --item-type <type> --item-id <id> --attachment <id> <submissionId>` |
| Flags | `--course`, `--item-type` (coursework, post), `--item-id`, `--attachment` |
| Output JSON | `{"studentSubmission": {...}}` |

### studentSubmissions.patch (x2 parents)

| Field | Value |
|-------|-------|
| CLI | `gog classroom addons submissions patch --course <id> --item-type <type> --item-id <id> --attachment <id> <submissionId>` |
| Flags | `--points-earned`, `--post-submission-state` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Course Work Rubrics (5 methods)

Path pattern: `courses/{courseId}/courseWork/{courseWorkId}/rubrics/{rubricId}`

### rubrics.create

| Field | Value |
|-------|-------|
| CLI | `gog classroom rubrics create --course <id> --coursework <id>` |
| Flags | `--course` (required), `--coursework` (required), `--criteria` (required, JSON array of criterion objects) |
| Output JSON | `{"rubric": {...}}` |
| Notes | Criteria JSON format: `[{"title":"...", "levels":[{"title":"...", "points":5}]}]` |

### rubrics.delete

| Field | Value |
|-------|-------|
| CLI | `gog classroom rubrics delete --course <id> --coursework <id> <rubricId>` |
| Guard | `confirmDestructive()` with `--force` |

### rubrics.get

| Field | Value |
|-------|-------|
| CLI | `gog classroom rubrics get --course <id> --coursework <id> <rubricId>` |
| Output JSON | `{"rubric": {...}}` |
| Output TSV | `id`, `criteriaCount`, `creationTime`, `updateTime` |

### rubrics.list

| Field | Value |
|-------|-------|
| CLI | `gog classroom rubrics list --course <id> --coursework <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"rubrics": [...], "nextPageToken": "..."}` |
| Output TSV | `ID`, `CRITERIA_COUNT`, `CREATION_TIME` |

### rubrics.patch

| Field | Value |
|-------|-------|
| CLI | `gog classroom rubrics patch --course <id> --coursework <id> <rubricId>` |
| Flags | `--criteria` (JSON array) |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Course Work Student Submissions -- modifyAttachments (1 method)

### submissions.modifyAttachments

| Field | Value |
|-------|-------|
| CLI | `gog classroom submissions modify-attachments --course <id> --coursework <id> <submissionId>` |
| API | `Courses.CourseWork.StudentSubmissions.ModifyAttachments(courseId, courseWorkId, submissionId, &ModifyAttachmentsRequest{...})` |
| Flags | `--course` (required), `--coursework` (required), `--add-attachments` (JSON array of Attachment objects) |
| Output JSON | `{"studentSubmission": {...}}` |

---

## Resource: Student Groups (4 methods)

Path pattern: `courses/{courseId}/studentGroups/{studentGroupId}`

### studentGroups.create

| Field | Value |
|-------|-------|
| CLI | `gog classroom student-groups create --course <id>` |
| Flags | `--course` (required), `--title` (required) |
| Output JSON | `{"studentGroup": {...}}` |

### studentGroups.delete

| Field | Value |
|-------|-------|
| CLI | `gog classroom student-groups delete --course <id> <groupId>` |
| Guard | `confirmDestructive()` with `--force` |

### studentGroups.list

| Field | Value |
|-------|-------|
| CLI | `gog classroom student-groups list --course <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"studentGroups": [...], "nextPageToken": "..."}` |
| Output TSV | `ID`, `TITLE` |

### studentGroups.patch

| Field | Value |
|-------|-------|
| CLI | `gog classroom student-groups patch --course <id> <groupId>` |
| Flags | `--title` |
| Patch logic | `flagProvided()` for `updateMask` |

---

## Resource: Student Group Members (3 methods)

Path pattern: `courses/{courseId}/studentGroups/{studentGroupId}/members/{memberId}`

### members.create

| Field | Value |
|-------|-------|
| CLI | `gog classroom student-groups members create --course <id> --group <id>` |
| Flags | `--course` (required), `--group` (required), `--student-id` (required) |
| Output JSON | `{"member": {...}}` |

### members.delete

| Field | Value |
|-------|-------|
| CLI | `gog classroom student-groups members delete --course <id> --group <id> <memberId>` |
| Guard | `confirmDestructive()` with `--force` |

### members.list

| Field | Value |
|-------|-------|
| CLI | `gog classroom student-groups members list --course <id> --group <id>` |
| Flags | `--page-size`, `--page-token` |
| Output JSON | `{"members": [...], "nextPageToken": "..."}` |
| Output TSV | `USER_ID`, `STUDENT_ID` |

---

## Resource: Registrations (2 methods)

### registrations.create

| Field | Value |
|-------|-------|
| CLI | `gog classroom registrations create` |
| API | `Registrations.Create(&Registration{...})` |
| Flags | `--feed-type` (required: COURSE_ROSTER_CHANGES, COURSE_WORK_CHANGES, DOMAIN_ROSTER_CHANGES), `--course-id` (required for course-level feeds), `--topic-name` (required, Cloud Pub/Sub topic) |
| Output JSON | `{"registration": {...}}` |
| Notes | Requires domain-wide delegation or service account with appropriate scopes |

### registrations.delete

| Field | Value |
|-------|-------|
| CLI | `gog classroom registrations delete <registrationId>` |
| API | `Registrations.Delete(registrationId)` |
| Guard | `confirmDestructive()` with `--force` |
| Output JSON | `{"deleted": true}` |

---

## Resource: Guardian Invitations (1 method)

### guardianInvitations.patch

| Field | Value |
|-------|-------|
| CLI | `gog classroom guardian-invitations patch --student <id> <invitationId>` |
| API | `UserProfiles.GuardianInvitations.Patch(studentId, invitationId, &GuardianInvitation{...})` |
| Flags | `--student` (required), `--state` (required: COMPLETE to cancel the invitation) |
| Patch logic | `flagProvided()` for `updateMask` with `state` |
| Output JSON | `{"guardianInvitation": {...}}` |

---

## Kong Struct Layout

```go
// Add to ClassroomCmd struct:
type ClassroomCmd struct {
    // ... existing fields ...
    Aliases        ClassroomAliasesCmd        `cmd:"" name:"aliases" help:"Course aliases"`
    Addons         ClassroomAddOnCmd          `cmd:"" name:"addons" help:"Add-on attachment operations"`
    Rubrics        ClassroomRubricsCmd        `cmd:"" name:"rubrics" help:"Coursework rubrics"`
    StudentGroups  ClassroomStudentGroupsCmd  `cmd:"" name:"student-groups" help:"Student groups"`
    Registrations  ClassroomRegistrationsCmd  `cmd:"" name:"registrations" help:"Push notification registrations"`
}

// Update existing ClassroomCoursesCmd to add:
//   Update         ClassroomCoursesUpdateCmd         `cmd:"" name:"update" help:"Full update a course"`
//   GradingPeriods ClassroomGradingPeriodsCmd        `cmd:"" name:"grading-periods" help:"Grading period settings"`
```

---

## Test Requirements

### Test patterns

1. **Add-on commands**: Test each `--item-type` variant (announcement, coursework, material, post) to ensure correct API path routing
2. **Rubrics**: Test JSON criteria parsing, verify request body structure
3. **Student groups/members**: Standard CRUD tests with nested path validation
4. **Registrations**: Verify Pub/Sub topic name format in request
5. **Course update**: Verify full replace semantics (all fields sent), contrast with existing patch behavior
6. **Guardian invitation patch**: Verify state transition, updateMask

### Factory injection

Use existing: `var newClassroomService = googleapi.NewClassroom`

### Deeply nested path verification

For add-on attachments, verify the mock server receives the correct URL path for each parent type:
- `/v1/courses/C1/announcements/A1/addOnAttachments`
- `/v1/courses/C1/courseWork/W1/addOnAttachments`
- `/v1/courses/C1/courseWorkMaterials/M1/addOnAttachments`
- `/v1/courses/C1/posts/P1/addOnAttachments`

### Test file organization

- `classroom_aliases_test.go` -- alias CRUD
- `classroom_addons_test.go` -- add-on attachments, context, student submissions
- `classroom_rubrics_test.go` -- rubric CRUD
- `classroom_student_groups_test.go` -- groups and members
- `classroom_registrations_test.go` -- registration create/delete
- `classroom_grading_test.go` -- grading period settings, guardian invitation patch
