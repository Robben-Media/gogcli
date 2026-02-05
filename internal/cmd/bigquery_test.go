package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func TestExecute_BigqueryDatasets_JSON(t *testing.T) {
	origNew := newBigqueryService
	t.Cleanup(func() { newBigqueryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/datasets") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"datasets": []map[string]any{
					{
						"datasetReference": map[string]any{
							"datasetId": "my_dataset",
							"projectId": "proj1",
						},
						"location":     "US",
						"friendlyName": "My Dataset",
					},
				},
				"nextPageToken": "page2",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := bigquery.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBigqueryService = func(context.Context, string) (*bigquery.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "bigquery", "datasets", "--project", "proj1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Datasets []struct {
			DatasetReference struct {
				DatasetID string `json:"datasetId"`
			} `json:"datasetReference"`
			Location     string `json:"location"`
			FriendlyName string `json:"friendlyName"`
		} `json:"datasets"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Datasets) != 1 {
		t.Fatalf("expected 1 dataset, got %d", len(parsed.Datasets))
	}
	if parsed.Datasets[0].DatasetReference.DatasetID != "my_dataset" {
		t.Fatalf("unexpected dataset id: %q", parsed.Datasets[0].DatasetReference.DatasetID)
	}
	if parsed.NextPageToken != "page2" {
		t.Fatalf("unexpected nextPageToken: %q", parsed.NextPageToken)
	}
}

func TestExecute_BigqueryTables_JSON(t *testing.T) {
	origNew := newBigqueryService
	t.Cleanup(func() { newBigqueryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tables") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tables": []map[string]any{
					{
						"tableReference": map[string]any{
							"tableId":   "my_table",
							"datasetId": "my_dataset",
							"projectId": "proj1",
						},
						"type":         "TABLE",
						"creationTime": "1700000000000",
					},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := bigquery.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBigqueryService = func(context.Context, string) (*bigquery.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "bigquery", "tables", "--project", "proj1", "--dataset", "my_dataset"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Tables []struct {
			TableReference struct {
				TableID string `json:"tableId"`
			} `json:"tableReference"`
			Type string `json:"type"`
		} `json:"tables"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(parsed.Tables))
	}
	if parsed.Tables[0].TableReference.TableID != "my_table" {
		t.Fatalf("unexpected table id: %q", parsed.Tables[0].TableReference.TableID)
	}
	if parsed.Tables[0].Type != "TABLE" {
		t.Fatalf("unexpected table type: %q", parsed.Tables[0].Type)
	}
}

func TestExecute_BigqueryQuery_JSON(t *testing.T) {
	origNew := newBigqueryService
	t.Cleanup(func() { newBigqueryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/queries") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobComplete": true,
				"schema": map[string]any{
					"fields": []map[string]any{
						{"name": "name", "type": "STRING"},
						{"name": "age", "type": "INTEGER"},
					},
				},
				"rows": []map[string]any{
					{
						"f": []map[string]any{
							{"v": "Alice"},
							{"v": "30"},
						},
					},
					{
						"f": []map[string]any{
							{"v": "Bob"},
							{"v": "25"},
						},
					},
				},
				"totalRows": "2",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := bigquery.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBigqueryService = func(context.Context, string) (*bigquery.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "bigquery", "query", "--project", "proj1", "--sql", "SELECT name, age FROM users"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Schema struct {
			Fields []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"fields"`
		} `json:"schema"`
		Rows []struct {
			F []struct {
				V any `json:"v"`
			} `json:"f"`
		} `json:"rows"`
		TotalRows uint64 `json:"totalRows"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Schema.Fields) != 2 {
		t.Fatalf("expected 2 schema fields, got %d", len(parsed.Schema.Fields))
	}
	if parsed.Schema.Fields[0].Name != "name" {
		t.Fatalf("unexpected field name: %q", parsed.Schema.Fields[0].Name)
	}
	if len(parsed.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(parsed.Rows))
	}
	if parsed.TotalRows != 2 {
		t.Fatalf("unexpected totalRows: %d", parsed.TotalRows)
	}
}

func TestExecute_BigqueryDatasets_MissingProject(t *testing.T) {
	err := Execute([]string{"--json", "--account", "a@b.com", "bigquery", "datasets"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestExecute_BigqueryJobs_JSON(t *testing.T) {
	origNew := newBigqueryService
	t.Cleanup(func() { newBigqueryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/jobs") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobs": []map[string]any{
					{
						"jobReference": map[string]any{
							"jobId":     "job_123",
							"projectId": "proj1",
						},
						"configuration": map[string]any{
							"jobType": "QUERY",
						},
						"status": map[string]any{
							"state": "DONE",
						},
						"statistics": map[string]any{
							"creationTime": "1700000000000",
						},
					},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := bigquery.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBigqueryService = func(context.Context, string) (*bigquery.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "bigquery", "jobs", "--project", "proj1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Jobs []struct {
			JobReference struct {
				JobID string `json:"jobId"`
			} `json:"jobReference"`
			Configuration struct {
				JobType string `json:"jobType"`
			} `json:"configuration"`
			Status struct {
				State string `json:"state"`
			} `json:"status"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(parsed.Jobs))
	}
	if parsed.Jobs[0].JobReference.JobID != "job_123" {
		t.Fatalf("unexpected job id: %q", parsed.Jobs[0].JobReference.JobID)
	}
	if parsed.Jobs[0].Status.State != "DONE" {
		t.Fatalf("unexpected job state: %q", parsed.Jobs[0].Status.State)
	}
}

func TestExecute_BigquerySchema_JSON(t *testing.T) {
	origNew := newBigqueryService
	t.Cleanup(func() { newBigqueryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tables/my_table") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema": map[string]any{
					"fields": []map[string]any{
						{"name": "id", "type": "INTEGER", "mode": "REQUIRED"},
						{"name": "email", "type": "STRING", "mode": "NULLABLE"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := bigquery.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newBigqueryService = func(context.Context, string) (*bigquery.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "bigquery", "schema", "--project", "proj1", "--dataset", "my_dataset", "--table", "my_table"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Schema struct {
			Fields []struct {
				Name string `json:"name"`
				Type string `json:"type"`
				Mode string `json:"mode"`
			} `json:"fields"`
		} `json:"schema"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Schema.Fields) != 2 {
		t.Fatalf("expected 2 schema fields, got %d", len(parsed.Schema.Fields))
	}
	if parsed.Schema.Fields[0].Name != "id" {
		t.Fatalf("unexpected field name: %q", parsed.Schema.Fields[0].Name)
	}
	if parsed.Schema.Fields[1].Mode != "NULLABLE" {
		t.Fatalf("unexpected field mode: %q", parsed.Schema.Fields[1].Mode)
	}
}
