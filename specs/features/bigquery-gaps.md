# BigQuery v2 -- Gap Coverage Spec

## Overview

**API**: BigQuery API v2
**Go package**: `google.golang.org/api/bigquery/v2`
**Service factory**: `newBigqueryService` (existing in `bigquery.go`)
**Currently implemented**: `datasets.list`, `jobs.list`, `jobs.query`, `tables.get` (schema), `tables.list`
**Missing methods**: 42

## Why

BigQuery is one of the most heavily used Google Cloud services. The current implementation only covers read-only listing and basic queries. Missing methods prevent users from managing datasets, controlling jobs, working with ML models, managing row-level security, inserting data, and handling IAM policies -- all essential for production BigQuery administration from the CLI.

---

## Resource: Datasets (6 methods)

### datasets.get

| Field | Value |
|-------|-------|
| CLI | `gog bigquery datasets get --project <id> --dataset <id>` |
| API | `Datasets.Get(projectId, datasetId)` |
| Flags | `--project` (required), `--dataset` (required) |
| Output JSON | `{"dataset": {...}}` |
| Output TSV | `datasetId`, `location`, `friendlyName`, `description`, `creationTime`, `lastModifiedTime`, `defaultTableExpirationMs` |

### datasets.insert

| Field | Value |
|-------|-------|
| CLI | `gog bigquery datasets create --project <id> --dataset <id>` |
| API | `Datasets.Insert(projectId, &Dataset{...})` |
| Flags | `--project` (required), `--dataset` (required), `--friendly-name`, `--description`, `--location` (e.g. US, EU, us-east1), `--default-table-expiration-ms`, `--default-partition-expiration-ms` |
| Output JSON | `{"dataset": {...}}` |
| Output TSV | `Created dataset: {projectId}:{datasetId}` |
| Notes | CLI name is `create` even though API method is `insert` for consistency |

### datasets.patch

| Field | Value |
|-------|-------|
| CLI | `gog bigquery datasets patch --project <id> --dataset <id>` |
| API | `Datasets.Patch(projectId, datasetId, &Dataset{...})` |
| Flags | `--project` (required), `--dataset` (required), `--friendly-name`, `--description`, `--default-table-expiration-ms`, `--default-partition-expiration-ms` |
| Patch logic | `flagProvided()` to only include changed fields in request body |
| Output JSON | `{"dataset": {...}}` |

### datasets.update

| Field | Value |
|-------|-------|
| CLI | `gog bigquery datasets update --project <id> --dataset <id>` |
| API | `Datasets.Update(projectId, datasetId, &Dataset{...})` |
| Notes | Full replace semantics. All writable fields required. Consider whether to expose this or only expose patch. If exposed, warn user about full replace. |

### datasets.delete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery datasets delete --project <id> --dataset <id>` |
| API | `Datasets.Delete(projectId, datasetId)` |
| Flags | `--project` (required), `--dataset` (required), `--delete-contents` (bool, deletes all tables in dataset), `--force` |
| Guard | `confirmDestructive(ctx, flags, "delete dataset {projectId}:{datasetId}")` |
| Output JSON | `{"deleted": true, "dataset": "{projectId}:{datasetId}"}` |

### datasets.undelete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery datasets undelete --project <id> --dataset <id>` |
| API | `Datasets.Undelete(projectId, datasetId, &UndeleteDatasetRequest{...})` |
| Flags | `--project` (required), `--dataset` (required), `--deletion-time` (required, timestamp of when dataset was deleted) |
| Output JSON | `{"dataset": {...}}` |

---

## Resource: Jobs (5 methods)

### jobs.get

| Field | Value |
|-------|-------|
| CLI | `gog bigquery jobs get --project <id> --job <id>` |
| API | `Jobs.Get(projectId, jobId)` |
| Flags | `--project` (required), `--job` (required), `--location` |
| Output JSON | `{"job": {...}}` |
| Output TSV | `jobId`, `type`, `state`, `creationTime`, `startTime`, `endTime`, `totalBytesProcessed` |

### jobs.cancel

| Field | Value |
|-------|-------|
| CLI | `gog bigquery jobs cancel --project <id> --job <id>` |
| API | `Jobs.Cancel(projectId, jobId)` |
| Flags | `--project` (required), `--job` (required), `--location` |
| Output JSON | `{"job": {...}}` (returns updated job resource) |
| Output TSV | `Cancelled job: {jobId}` |

### jobs.delete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery jobs delete --project <id> --job <id>` |
| API | `Jobs.Delete(projectId, jobId)` |
| Flags | `--project` (required), `--job` (required), `--location`, `--force` |
| Guard | `confirmDestructive()` |
| Output JSON | `{"deleted": true, "jobId": "..."}` |

### jobs.getQueryResults

| Field | Value |
|-------|-------|
| CLI | `gog bigquery jobs results --project <id> --job <id>` |
| API | `Jobs.GetQueryResults(projectId, jobId)` |
| Flags | `--project` (required), `--job` (required), `--location`, `--max-results`, `--page-token`, `--start-index`, `--timeout-ms` |
| Output JSON | `{"schema": {...}, "rows": [...], "totalRows": "...", "jobComplete": true}` |
| Output TSV | Dynamic columns from schema (same as existing query output) |

### jobs.insert

| Field | Value |
|-------|-------|
| CLI | `gog bigquery jobs submit --project <id> --type <type>` |
| API | `Jobs.Insert(projectId, &Job{...})` |
| Flags | `--project` (required), `--type` (required: query, load, extract, copy), then type-specific flags below |
| Output JSON | `{"job": {...}}` |

**Type-specific flags for jobs.insert**:

Query job:
- `--sql` (required), `--destination-table` (project.dataset.table), `--write-disposition` (WRITE_TRUNCATE, WRITE_APPEND, WRITE_EMPTY), `--use-legacy-sql`

Load job:
- `--source-uris` (comma-separated GCS URIs), `--destination-table` (required), `--source-format` (CSV, NEWLINE_DELIMITED_JSON, AVRO, PARQUET, ORC), `--write-disposition`, `--skip-leading-rows`, `--autodetect` (bool)

Extract job:
- `--source-table` (required, project.dataset.table), `--destination-uris` (required, comma-separated GCS URIs), `--destination-format` (CSV, NEWLINE_DELIMITED_JSON, AVRO), `--compression` (GZIP, DEFLATE, SNAPPY, NONE)

Copy job:
- `--source-table` (required), `--destination-table` (required), `--write-disposition`

Notes: This is the most complex command. Consider breaking into subcommands: `gog bigquery jobs submit-query`, `gog bigquery jobs submit-load`, etc. for better UX.

---

## Resource: Models (4 methods)

Path pattern: `projects/{projectId}/datasets/{datasetId}/models/{modelId}`

### models.list

| Field | Value |
|-------|-------|
| CLI | `gog bigquery models list --project <id> --dataset <id>` |
| API | `Models.List(projectId, datasetId)` |
| Flags | `--project` (required), `--dataset` (required), `--max-results`, `--page-token` |
| Output JSON | `{"models": [...], "nextPageToken": "..."}` |
| Output TSV | `MODEL_ID`, `MODEL_TYPE`, `CREATION_TIME`, `LAST_MODIFIED_TIME` |

### models.get

| Field | Value |
|-------|-------|
| CLI | `gog bigquery models get --project <id> --dataset <id> --model <id>` |
| API | `Models.Get(projectId, datasetId, modelId)` |
| Output JSON | `{"model": {...}}` |
| Output TSV | `modelId`, `modelType`, `description`, `creationTime`, `expirationTime` |

### models.patch

| Field | Value |
|-------|-------|
| CLI | `gog bigquery models patch --project <id> --dataset <id> --model <id>` |
| Flags | `--description`, `--friendly-name`, `--expiration-time`, `--labels` (key=value pairs) |
| Patch logic | `flagProvided()` to detect changed fields |

### models.delete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery models delete --project <id> --dataset <id> --model <id>` |
| Guard | `confirmDestructive()` with `--force` |
| Output JSON | `{"deleted": true}` |

---

## Resource: Projects (2 methods)

### projects.list

| Field | Value |
|-------|-------|
| CLI | `gog bigquery projects list` |
| API | `Projects.List()` |
| Flags | `--max-results`, `--page-token` |
| Output JSON | `{"projects": [...], "nextPageToken": "..."}` |
| Output TSV | `PROJECT_ID`, `FRIENDLY_NAME`, `NUMERIC_ID` |

### projects.getServiceAccount

| Field | Value |
|-------|-------|
| CLI | `gog bigquery projects service-account --project <id>` |
| API | `Projects.GetServiceAccount(projectId)` |
| Output JSON | `{"email": "...", "kind": "..."}` |
| Output TSV | `email` |

---

## Resource: Routines (8 methods)

Path pattern: `projects/{projectId}/datasets/{datasetId}/routines/{routineId}`

### routines.list

| Field | Value |
|-------|-------|
| CLI | `gog bigquery routines list --project <id> --dataset <id>` |
| Flags | `--project` (required), `--dataset` (required), `--max-results`, `--page-token`, `--filter` (routine type filter) |
| Output JSON | `{"routines": [...], "nextPageToken": "..."}` |
| Output TSV | `ROUTINE_ID`, `ROUTINE_TYPE`, `LANGUAGE`, `CREATION_TIME` |

### routines.get

| Field | Value |
|-------|-------|
| CLI | `gog bigquery routines get --project <id> --dataset <id> --routine <id>` |
| Output JSON | `{"routine": {...}}` |
| Output TSV | `routineId`, `routineType`, `language`, `definitionBody` (truncated) |

### routines.insert

| Field | Value |
|-------|-------|
| CLI | `gog bigquery routines create --project <id> --dataset <id>` |
| Flags | `--project` (required), `--dataset` (required), `--routine-id` (required), `--routine-type` (SCALAR_FUNCTION, PROCEDURE, TABLE_VALUED_FUNCTION), `--language` (SQL, JAVASCRIPT), `--definition-body` (required, the SQL/JS body), `--arguments` (JSON array of argument definitions) |
| Output JSON | `{"routine": {...}}` |

### routines.update

| Field | Value |
|-------|-------|
| CLI | `gog bigquery routines update --project <id> --dataset <id> --routine <id>` |
| Notes | Full replace. All writable fields required. |

### routines.delete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery routines delete --project <id> --dataset <id> --routine <id>` |
| Guard | `confirmDestructive()` with `--force` |

### routines.getIamPolicy / setIamPolicy / testIamPermissions

See IAM Policy section below.

---

## Resource: Row Access Policies (8 methods)

Path pattern: `projects/{projectId}/datasets/{datasetId}/tables/{tableId}/rowAccessPolicies/{policyId}`

### rowAccessPolicies.list

| Field | Value |
|-------|-------|
| CLI | `gog bigquery row-access-policies list --project <id> --dataset <id> --table <id>` |
| Flags | `--project`, `--dataset`, `--table` (all required), `--page-size`, `--page-token` |
| Output JSON | `{"rowAccessPolicies": [...], "nextPageToken": "..."}` |
| Output TSV | `POLICY_ID`, `FILTER_PREDICATE`, `CREATION_TIME` |

### rowAccessPolicies.get

| Field | Value |
|-------|-------|
| CLI | `gog bigquery row-access-policies get --project <id> --dataset <id> --table <id> <policyId>` |

### rowAccessPolicies.insert

| Field | Value |
|-------|-------|
| CLI | `gog bigquery row-access-policies create --project <id> --dataset <id> --table <id>` |
| Flags | `--filter-predicate` (required, SQL boolean expression), `--grantees` (required, comma-separated list) |

### rowAccessPolicies.update

| Field | Value |
|-------|-------|
| CLI | `gog bigquery row-access-policies update --project <id> --dataset <id> --table <id> <policyId>` |
| Flags | `--filter-predicate`, `--grantees` |

### rowAccessPolicies.delete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery row-access-policies delete --project <id> --dataset <id> --table <id> <policyId>` |
| Guard | `confirmDestructive()` with `--force` |

### rowAccessPolicies.batchDelete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery row-access-policies batch-delete --project <id> --dataset <id> --table <id>` |
| Flags | `--policy-ids` (required, comma-separated), `--force` |
| Guard | `confirmDestructive()` |

### rowAccessPolicies.getIamPolicy / testIamPermissions

See IAM Policy section below.

---

## Resource: Table Data (2 methods)

### tabledata.list

| Field | Value |
|-------|-------|
| CLI | `gog bigquery tabledata list --project <id> --dataset <id> --table <id>` |
| API | `Tabledata.List(projectId, datasetId, tableId)` |
| Flags | `--project`, `--dataset`, `--table` (all required), `--max-results`, `--page-token`, `--start-index`, `--selected-fields` (comma-separated) |
| Output JSON | `{"rows": [...], "totalRows": "...", "pageToken": "..."}` |
| Output TSV | Dynamic columns from table schema |

### tabledata.insertAll

| Field | Value |
|-------|-------|
| CLI | `gog bigquery tabledata insert --project <id> --dataset <id> --table <id>` |
| API | `Tabledata.InsertAll(projectId, datasetId, tableId, &TableDataInsertAllRequest{...})` |
| Flags | `--project`, `--dataset`, `--table` (all required), `--rows` (required, JSON array or read from stdin), `--skip-invalid-rows`, `--ignore-unknown-values`, `--template-suffix` |
| Output JSON | `{"insertErrors": [...]}` (empty array on success) |
| Notes | Support reading rows from stdin: `cat rows.json \| gog bigquery tabledata insert ...` |

---

## Resource: Tables (7 methods)

### tables.insert

| Field | Value |
|-------|-------|
| CLI | `gog bigquery tables create --project <id> --dataset <id>` |
| Flags | `--project`, `--dataset` (required), `--table-id` (required), `--friendly-name`, `--description`, `--schema` (JSON or comma-separated `name:type` pairs), `--time-partitioning-type` (DAY, HOUR, MONTH, YEAR), `--time-partitioning-field`, `--clustering-fields`, `--expiration-time`, `--view-query` (creates a view), `--materialized-view-query` |
| Output JSON | `{"table": {...}}` |

### tables.patch

| Field | Value |
|-------|-------|
| CLI | `gog bigquery tables patch --project <id> --dataset <id> --table <id>` |
| Flags | `--friendly-name`, `--description`, `--expiration-time`, `--labels` (key=value pairs), `--schema` (for adding columns) |
| Patch logic | `flagProvided()` |

### tables.update

| Field | Value |
|-------|-------|
| CLI | `gog bigquery tables update --project <id> --dataset <id> --table <id>` |
| Notes | Full replace. Consider exposing only patch. |

### tables.delete

| Field | Value |
|-------|-------|
| CLI | `gog bigquery tables delete --project <id> --dataset <id> --table <id>` |
| Guard | `confirmDestructive()` with `--force` |
| Output JSON | `{"deleted": true}` |

### tables.getIamPolicy / setIamPolicy / testIamPermissions

See IAM Policy section below.

---

## IAM Policy Methods (standard pattern)

The following resources share the same IAM pattern: routines, rowAccessPolicies, tables.

### getIamPolicy

| Field | Value |
|-------|-------|
| CLI | `gog bigquery <resource> get-iam-policy --project <id> --dataset <id> --<resource-id-flag> <id>` |
| API | `{Resource}.GetIamPolicy(resourceName, &GetIamPolicyRequest{})` |
| Output JSON | `{"policy": {"bindings": [...], "etag": "...", "version": 1}}` |
| Output TSV | `ROLE`, `MEMBERS` (one row per binding) |

### setIamPolicy

| Field | Value |
|-------|-------|
| CLI | `gog bigquery <resource> set-iam-policy --project <id> --dataset <id> --<resource-id-flag> <id>` |
| Flags | `--policy` (required, JSON file path or stdin), `--force` |
| Guard | `confirmDestructive()` (IAM changes can lock out access) |
| Notes | Read policy first (get-iam-policy), modify, then set. Warn about etag conflicts. |

### testIamPermissions

| Field | Value |
|-------|-------|
| CLI | `gog bigquery <resource> test-iam-permissions --project <id> --dataset <id> --<resource-id-flag> <id>` |
| Flags | `--permissions` (required, comma-separated list of permission strings) |
| Output JSON | `{"permissions": [...]}` (only permissions the caller has) |
| Output TSV | one permission per line |

---

## Kong Struct Layout

```go
type BigqueryCmd struct {
    // ... existing fields ...
    DatasetsAdmin BigqueryDatasetsAdminCmd `cmd:"" name:"datasets" group:"Admin" help:"Dataset admin operations"`
    Models        BigqueryModelsCmd        `cmd:"" name:"models" group:"Read" help:"ML model operations"`
    Projects      BigqueryProjectsCmd      `cmd:"" name:"projects" group:"Read" help:"Project operations"`
    Routines      BigqueryRoutinesCmd      `cmd:"" name:"routines" group:"Read" help:"Routine (UDF/procedure) operations"`
    RowPolicies   BigqueryRowPoliciesCmd   `cmd:"" name:"row-access-policies" group:"Admin" help:"Row access policy operations"`
    Tabledata     BigqueryTabledataCmd     `cmd:"" name:"tabledata" group:"Read" help:"Table data operations"`
    TablesAdmin   BigqueryTablesAdminCmd   `cmd:"" name:"tables-admin" group:"Admin" help:"Table admin operations"`
}
```

Note: Existing `datasets`, `tables`, `schema` commands remain as-is. New dataset admin ops go under a restructured `datasets` subcommand group, or we add `datasets get/create/delete/patch` as sub-subcommands alongside the existing `datasets` list.

Preferred approach: Restructure `BigqueryDatasetsCmd` to be a parent with `list` as the default/existing subcommand, then add `get`, `create`, `delete`, `patch`, `undelete` as siblings.

---

## Test Requirements

### Test patterns

1. **Dataset CRUD**: Standard mock responses, verify `deleteContents` query param for delete
2. **Jobs**: Test each job type submission (query, load, extract, copy) separately; verify correct `configuration` block in request
3. **Jobs cancel**: Verify response includes updated job state
4. **Jobs results**: Reuse existing query output formatting logic; verify pagination params
5. **Models CRUD**: Standard pattern
6. **Routines**: Test SQL body encoding in request, verify argument JSON parsing
7. **Row access policies**: Test filter predicate encoding, batch delete with multiple IDs
8. **Table data insert**: Test stdin reading, verify `insertErrors` handling
9. **IAM methods**: Test with mock policy bindings, verify etag round-trip

### Factory injection

Use existing: `var newBigqueryService = googleapi.NewBigquery`

### Test file organization

- `bigquery_datasets_test.go` -- dataset CRUD, undelete
- `bigquery_jobs_test.go` -- jobs get/cancel/delete/results/insert
- `bigquery_models_test.go` -- model CRUD
- `bigquery_routines_test.go` -- routine CRUD + IAM
- `bigquery_row_policies_test.go` -- row access policy CRUD + IAM
- `bigquery_tabledata_test.go` -- tabledata list/insert
- `bigquery_tables_admin_test.go` -- table create/patch/delete + IAM
- `bigquery_projects_test.go` -- project list, service account
