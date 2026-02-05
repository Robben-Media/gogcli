package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/bigquery/v2"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newBigqueryService = googleapi.NewBigquery

type BigqueryCmd struct {
	Query    BigqueryQueryCmd    `cmd:"" name:"query" group:"Read" help:"Run a SQL query"`
	Datasets BigqueryDatasetsCmd `cmd:"" name:"datasets" group:"Read" help:"List datasets in a project"`
	Tables   BigqueryTablesCmd   `cmd:"" name:"tables" group:"Read" help:"List tables in a dataset"`
	Schema   BigquerySchemaCmd   `cmd:"" name:"schema" group:"Read" help:"Get table schema"`
	Jobs     BigqueryJobsCmd     `cmd:"" name:"jobs" group:"Read" help:"List jobs in a project"`
}

// --- query ---

type BigqueryQueryCmd struct {
	Project      string `name:"project" required:"" help:"Google Cloud project ID"`
	SQL          string `name:"sql" required:"" help:"SQL query to execute"`
	MaxResults   int64  `name:"max-results" help:"Maximum number of rows to return" default:"100"`
	UseLegacySQL bool   `name:"use-legacy-sql" help:"Use legacy SQL syntax instead of standard SQL"`
}

func (c *BigqueryQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("--project required")
	}
	sql := strings.TrimSpace(c.SQL)
	if sql == "" {
		return usage("--sql required")
	}

	svc, err := newBigqueryService(ctx, account)
	if err != nil {
		return err
	}

	useLegacy := c.UseLegacySQL
	req := bigquery.QueryRequest{
		Query:        sql,
		UseLegacySql: &useLegacy,
		MaxResults:   c.MaxResults,
	}

	resp, err := svc.Jobs.Query(project, &req).Do()
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	if !resp.JobComplete {
		return fmt.Errorf("query is still running (job %s); use jobs command to check status", resp.JobReference.JobId)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"schema":    resp.Schema,
			"rows":      resp.Rows,
			"totalRows": resp.TotalRows,
		})
	}

	if resp.Schema == nil || len(resp.Schema.Fields) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	// Print column headers from schema
	headers := make([]string, len(resp.Schema.Fields))
	for i, field := range resp.Schema.Fields {
		headers[i] = field.Name
	}
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print rows
	for _, row := range resp.Rows {
		vals := make([]string, len(row.F))
		for i, cell := range row.F {
			if cell.V != nil {
				vals[i] = fmt.Sprintf("%v", cell.V)
			}
		}
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}

	u.Err().Printf("Total rows: %d", resp.TotalRows)
	return nil
}

// --- datasets ---

type BigqueryDatasetsCmd struct {
	Project string `name:"project" required:"" help:"Google Cloud project ID"`
	Max     int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
}

func (c *BigqueryDatasetsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("--project required")
	}

	svc, err := newBigqueryService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Datasets.List(project).MaxResults(c.Max).Do()
	if err != nil {
		return fmt.Errorf("list datasets: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"datasets":      resp.Datasets,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Datasets) == 0 {
		u.Err().Println("No datasets")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "DATASET_ID\tLOCATION\tFRIENDLY_NAME")
	for _, ds := range resp.Datasets {
		datasetID := ""
		if ds.DatasetReference != nil {
			datasetID = ds.DatasetReference.DatasetId
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", datasetID, ds.Location, ds.FriendlyName)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- tables ---

type BigqueryTablesCmd struct {
	Project string `name:"project" required:"" help:"Google Cloud project ID"`
	Dataset string `name:"dataset" required:"" help:"Dataset ID"`
	Max     int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
}

func (c *BigqueryTablesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("--project required")
	}
	dataset := strings.TrimSpace(c.Dataset)
	if dataset == "" {
		return usage("--dataset required")
	}

	svc, err := newBigqueryService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Tables.List(project, dataset).MaxResults(c.Max).Do()
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"tables":        resp.Tables,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Tables) == 0 {
		u.Err().Println("No tables")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TABLE_ID\tTYPE\tCREATION_TIME")
	for _, tbl := range resp.Tables {
		tableID := ""
		if tbl.TableReference != nil {
			tableID = tbl.TableReference.TableId
		}
		fmt.Fprintf(w, "%s\t%s\t%d\n", tableID, tbl.Type, tbl.CreationTime)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// --- schema ---

type BigquerySchemaCmd struct {
	Project string `name:"project" required:"" help:"Google Cloud project ID"`
	Dataset string `name:"dataset" required:"" help:"Dataset ID"`
	Table   string `name:"table" required:"" help:"Table ID"`
}

func (c *BigquerySchemaCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("--project required")
	}
	dataset := strings.TrimSpace(c.Dataset)
	if dataset == "" {
		return usage("--dataset required")
	}
	table := strings.TrimSpace(c.Table)
	if table == "" {
		return usage("--table required")
	}

	svc, err := newBigqueryService(ctx, account)
	if err != nil {
		return err
	}

	tbl, err := svc.Tables.Get(project, dataset, table).Do()
	if err != nil {
		return fmt.Errorf("get table: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"schema": tbl.Schema,
		})
	}

	if tbl.Schema == nil || len(tbl.Schema.Fields) == 0 {
		u.Err().Println("No schema fields")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTYPE\tMODE")
	for _, field := range tbl.Schema.Fields {
		fmt.Fprintf(w, "%s\t%s\t%s\n", field.Name, field.Type, field.Mode)
	}
	return nil
}

// --- jobs ---

type BigqueryJobsCmd struct {
	Project     string `name:"project" required:"" help:"Google Cloud project ID"`
	Max         int64  `name:"max" aliases:"limit" help:"Max results" default:"20"`
	StateFilter string `name:"state-filter" help:"Filter by job state: running, done, pending"`
}

func (c *BigqueryJobsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("--project required")
	}

	svc, err := newBigqueryService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Jobs.List(project).MaxResults(c.Max)
	stateFilter := strings.TrimSpace(c.StateFilter)
	if stateFilter != "" {
		call = call.StateFilter(stateFilter)
	}

	resp, err := call.Do()
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"jobs":          resp.Jobs,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Jobs) == 0 {
		u.Err().Println("No jobs")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "JOB_ID\tTYPE\tSTATE\tCREATION_TIME")
	for _, job := range resp.Jobs {
		jobID := ""
		if job.JobReference != nil {
			jobID = job.JobReference.JobId
		}
		jobType := ""
		state := ""
		creationTime := ""
		if job.Statistics != nil {
			creationTime = fmt.Sprintf("%d", job.Statistics.CreationTime)
		}
		if job.Configuration != nil {
			jobType = job.Configuration.JobType
		}
		if job.Status != nil {
			state = job.Status.State
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", jobID, jobType, state, creationTime)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

// Ensure bigquery.Service is used to avoid import cycle lint errors.
var _ *bigquery.Service
