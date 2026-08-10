package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestContactsBatchCreate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ContactsBatchCreateCmd
		setup   func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "creates multiple contacts from JSON",
			cmd: ContactsBatchCreateCmd{
				ContactsJSON: `[{"names":[{"givenName":"John","familyName":"Doe"}],"emailAddresses":[{"value":"john@example.com"}]},{"names":[{"givenName":"Jane","familyName":"Smith"}],"emailAddresses":[{"value":"jane@example.com"}]}]`,
				ReadMask:     "names,emailAddresses",
			},
			setup: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/people:batchCreateContacts" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}

				resp := &people.BatchCreateContactsResponse{
					CreatedPeople: []*people.PersonResponse{
						{Person: &people.Person{ResourceName: "people/1", Names: []*people.Name{{DisplayName: "John Doe"}}, EmailAddresses: []*people.EmailAddress{{Value: "john@example.com"}}}},
						{Person: &people.Person{ResourceName: "people/2", Names: []*people.Name{{DisplayName: "Jane Smith"}}, EmailAddresses: []*people.EmailAddress{{Value: "jane@example.com"}}}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name: "creates contacts with ContactToCreate wrapper",
			cmd: ContactsBatchCreateCmd{
				ContactsJSON: `[{"contactPerson":{"names":[{"givenName":"Test"}]}}]`,
				ReadMask:     "names",
			},
			setup: func(w http.ResponseWriter, r *http.Request) {
				resp := &people.BatchCreateContactsResponse{
					CreatedPeople: []*people.PersonResponse{
						{Person: &people.Person{ResourceName: "people/1", Names: []*people.Name{{GivenName: "Test"}}}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name: "empty contacts JSON fails",
			cmd: ContactsBatchCreateCmd{
				ContactsJSON: `[]`,
				ReadMask:     "names",
			},
			wantErr: true,
		},
		{
			name: "invalid JSON fails",
			cmd: ContactsBatchCreateCmd{
				ContactsJSON: `invalid`,
				ReadMask:     "names",
			},
			wantErr: true,
		},
		{
			name: "too many contacts fails",
			cmd: ContactsBatchCreateCmd{
				ContactsJSON: generateManyContactsJSON(201),
				ReadMask:     "names",
			},
			wantErr: true,
		},
		{
			name: "no input fails",
			cmd: ContactsBatchCreateCmd{
				ReadMask: "names",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.setup != nil {
					tt.setup(w, r)
				} else {
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()

			svc, err := people.NewService(context.Background(),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
			)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			newPeopleContactsService = func(ctx context.Context, email string) (*people.Service, error) {
				return svc, nil
			}

			u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			if err != nil {
				t.Fatalf("failed to create UI: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			flags := &RootFlags{Account: "a@b.com"}

			err = tt.cmd.Run(ctx, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContactsBatchDelete(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ContactsBatchDeleteCmd
		force   bool
		setup   func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name:  "deletes multiple contacts with force",
			cmd:   ContactsBatchDeleteCmd{ResourceNames: []string{"people/1", "people/2"}},
			force: true,
			setup: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/people:batchDeleteContacts" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("{}"))
			},
			wantErr: false,
		},
		{
			name:  "normalizes resource names without prefix",
			cmd:   ContactsBatchDeleteCmd{ResourceNames: []string{"1", "2"}},
			force: true,
			setup: func(w http.ResponseWriter, r *http.Request) {
				var req people.BatchDeleteContactsRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}
				// Verify normalization happened
				for i, rn := range req.ResourceNames {
					expected := "people/" + string(rune('1'+i))
					if rn != expected {
						t.Errorf("ResourceNames[%d] = %s, want %s", i, rn, expected)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("{}"))
			},
			wantErr: false,
		},
		{
			name:    "empty resource names fails",
			cmd:     ContactsBatchDeleteCmd{ResourceNames: []string{}},
			force:   true,
			wantErr: true,
		},
		{
			name:    "requires confirmation without force",
			cmd:     ContactsBatchDeleteCmd{ResourceNames: []string{"people/1"}},
			force:   false,
			wantErr: true, // confirmation will fail in test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.setup != nil {
					tt.setup(w, r)
				} else {
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()

			svc, err := people.NewService(context.Background(),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
			)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			newPeopleContactsService = func(ctx context.Context, email string) (*people.Service, error) {
				return svc, nil
			}

			u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			if err != nil {
				t.Fatalf("failed to create UI: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			flags := &RootFlags{Account: "a@b.com"}
			if tt.force {
				flags.Force = true
			}

			err = tt.cmd.Run(ctx, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContactsBatchUpdate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ContactsBatchUpdateCmd
		setup   func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "updates multiple contacts",
			cmd: ContactsBatchUpdateCmd{
				ContactsJSON: `{"people/1":{"names":[{"givenName":"Updated"}]},"people/2":{"names":[{"givenName":"Also Updated"}]}}`,
				ReadMask:     "names",
				UpdateMask:   "names",
			},
			setup: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/people:batchUpdateContacts" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}
				resp := &people.BatchUpdateContactsResponse{
					UpdateResult: map[string]people.PersonResponse{
						"people/1": {Person: &people.Person{ResourceName: "people/1"}},
						"people/2": {Person: &people.Person{ResourceName: "people/2"}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name: "empty contacts JSON fails",
			cmd: ContactsBatchUpdateCmd{
				ContactsJSON: `{}`,
				ReadMask:     "names",
				UpdateMask:   "names",
			},
			wantErr: true,
		},
		{
			name: "invalid JSON fails",
			cmd: ContactsBatchUpdateCmd{
				ContactsJSON: `invalid`,
				ReadMask:     "names",
				UpdateMask:   "names",
			},
			wantErr: true,
		},
		{
			name: "no input fails",
			cmd: ContactsBatchUpdateCmd{
				ReadMask:   "names",
				UpdateMask: "names",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.setup != nil {
					tt.setup(w, r)
				} else {
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()

			svc, err := people.NewService(context.Background(),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
			)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			newPeopleContactsService = func(ctx context.Context, email string) (*people.Service, error) {
				return svc, nil
			}

			u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			if err != nil {
				t.Fatalf("failed to create UI: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			flags := &RootFlags{Account: "a@b.com"}

			err = tt.cmd.Run(ctx, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// generateManyContactsJSON generates a JSON array of N contacts for testing.
func generateManyContactsJSON(n int) string {
	contacts := make([]map[string]interface{}, n)
	for i := 0; i < n; i++ {
		contacts[i] = map[string]interface{}{
			"names": []map[string]interface{}{
				{"givenName": "Test"},
			},
		}
	}
	b, _ := json.Marshal(contacts)
	return string(b)
}

func TestContactsBatchGet(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ContactsBatchGetCmd
		setup   func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name: "gets multiple contacts",
			cmd: ContactsBatchGetCmd{
				ResourceNames: []string{"people/1", "people/2"},
				ReadMask:      "names,emailAddresses",
			},
			setup: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/people:batchGet" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}

				resp := &people.GetPeopleResponse{
					Responses: []*people.PersonResponse{
						{Person: &people.Person{ResourceName: "people/1", Names: []*people.Name{{DisplayName: "John Doe"}}, EmailAddresses: []*people.EmailAddress{{Value: "john@example.com"}}}},
						{Person: &people.Person{ResourceName: "people/2", Names: []*people.Name{{DisplayName: "Jane Smith"}}, EmailAddresses: []*people.EmailAddress{{Value: "jane@example.com"}}}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name: "normalizes resource names",
			cmd: ContactsBatchGetCmd{
				ResourceNames: []string{"1", "2"},
				ReadMask:      "names",
			},
			setup: func(w http.ResponseWriter, r *http.Request) {
				// Check that resource names are normalized in the query
				rn := r.URL.Query()["resourceNames"]
				if len(rn) != 2 {
					t.Errorf("expected 2 resource names, got %d", len(rn))
				}

				resp := &people.GetPeopleResponse{
					Responses: []*people.PersonResponse{
						{Person: &people.Person{ResourceName: "people/1"}},
						{Person: &people.Person{ResourceName: "people/2"}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name: "empty resource names fails",
			cmd: ContactsBatchGetCmd{
				ResourceNames: []string{},
				ReadMask:      "names",
			},
			wantErr: true,
		},
		{
			name: "handles not found contacts",
			cmd: ContactsBatchGetCmd{
				ResourceNames: []string{"people/nonexistent"},
				ReadMask:      "names",
			},
			setup: func(w http.ResponseWriter, r *http.Request) {
				resp := &people.GetPeopleResponse{
					Responses: []*people.PersonResponse{
						{Status: &people.Status{Code: 404, Message: "not found"}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: false, // Not found is not an error, just shown in output
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.setup != nil {
					tt.setup(w, r)
				} else {
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()

			svc, err := people.NewService(context.Background(),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
			)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			newPeopleContactsService = func(ctx context.Context, email string) (*people.Service, error) {
				return svc, nil
			}

			u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			if err != nil {
				t.Fatalf("failed to create UI: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)
			flags := &RootFlags{Account: "a@b.com"}

			err = tt.cmd.Run(ctx, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContactsBatchCreate_PlainTSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/people:batchCreateContacts" {
			http.NotFound(w, r)
			return
		}
		resp := &people.BatchCreateContactsResponse{
			CreatedPeople: []*people.PersonResponse{
				{
					Person: &people.Person{
						ResourceName:   "people/1",
						Names:          []*people.Name{{DisplayName: "John\tDoe\nNorth"}},
						EmailAddresses: []*people.EmailAddress{{Value: "john@example.com"}},
					},
				},
				{
					Status: &people.Status{Code: 3, Message: "invalid\targument\r\nline"},
				},
				{
					Person: &people.Person{
						ResourceName: "people/3",
						Names:        []*people.Name{{DisplayName: "Only Name"}},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	usePeopleContactsTestServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--plain", "--account", "a@b.com",
				"contacts", "batch", "create",
				"--contacts-json", `[{"names":[{"givenName":"John"}]},{"names":[{"givenName":"Bad"}]},{"names":[{"givenName":"Only"}]}]`,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	want := "" +
		"REQUEST_KEY\tRESOURCE\tNAME\tEMAIL\tSTATUS\tERROR\n" +
		"0\tpeople/1\tJohn Doe North\tjohn@example.com\tOK\t\n" +
		"1\t\t\t\tERROR\tinvalid argument line\n" +
		"2\tpeople/3\tOnly Name\t\tOK\t\n"
	if out != want {
		t.Fatalf("plain batch create output = %q, want %q", out, want)
	}
}

func TestContactsBatchCreate_PlainEmptyResponseStillEmitsRequestRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&people.BatchCreateContactsResponse{CreatedPeople: []*people.PersonResponse{}})
	}))
	usePeopleContactsTestServer(t, srv)

	out := captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			if execErr := Execute([]string{
				"--plain", "--account", "a@b.com",
				"contacts", "batch", "create",
				"--contacts-json", `[{"names":[{"givenName":"X"}]}]`,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
		if stderr != "" {
			t.Fatalf("unexpected stderr under --plain: %q", stderr)
		}
	})

	want := "REQUEST_KEY\tRESOURCE\tNAME\tEMAIL\tSTATUS\tERROR\n0\t\tX\t\tOK\t\n"
	if out != want {
		t.Fatalf("plain create output with empty response = %q, want %q", out, want)
	}
}

func TestContactsBatchCreate_HumanStillPrintsCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &people.BatchCreateContactsResponse{
			CreatedPeople: []*people.PersonResponse{
				{Person: &people.Person{ResourceName: "people/1", Names: []*people.Name{{DisplayName: "A"}}, EmailAddresses: []*people.EmailAddress{{Value: "a@b.com"}}}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	usePeopleContactsTestServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--account", "a@b.com",
				"contacts", "batch", "create",
				"--contacts-json", `[{"names":[{"givenName":"A"}]}]`,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})
	if !strings.Contains(out, "Created 1 contact(s)") {
		t.Fatalf("human create output missing count prose: %q", out)
	}
	if strings.Contains(out, "REQUEST_KEY") {
		t.Fatalf("human create output unexpectedly used plain schema: %q", out)
	}
}

func TestContactsBatchUpdate_PlainTSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/people:batchUpdateContacts" {
			http.NotFound(w, r)
			return
		}
		resp := &people.BatchUpdateContactsResponse{
			UpdateResult: map[string]people.PersonResponse{
				"people/2": {
					RequestedResourceName: "people/2",
					Person: &people.Person{
						ResourceName:   "people/2",
						Names:          []*people.Name{{DisplayName: "Jane\nSmith"}},
						EmailAddresses: []*people.EmailAddress{{Value: "jane@example.com"}},
					},
				},
				"people/1": {
					RequestedResourceName: "people/1",
					Status:                &people.Status{Code: 5, Message: "not found"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	usePeopleContactsTestServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--plain", "--account", "a@b.com",
				"contacts", "batch", "update",
				"--contacts-json", `{"people/1":{"names":[{"givenName":"A"}]},"people/2":{"names":[{"givenName":"B"}]}}`,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})

	resource := "people/2"
	want := "" +
		"REQUEST_KEY\tRESOURCE\tNAME\tEMAIL\tSTATUS\tERROR\n" +
		"people/1\t\t\t\tERROR\tnot found\n" +
		resource + "\t" + resource + "\tJane Smith\tjane@example.com\tOK\t\n"
	if out != want {
		t.Fatalf("plain batch update output = %q, want %q", out, want)
	}
}

func TestContactsBatchUpdate_PlainEmptyResponseStillEmitsRequestRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&people.BatchUpdateContactsResponse{UpdateResult: map[string]people.PersonResponse{}})
	}))
	usePeopleContactsTestServer(t, srv)

	out := captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			if execErr := Execute([]string{
				"--plain", "--account", "a@b.com",
				"contacts", "batch", "update",
				"--contacts-json", `{"people/1":{"names":[{"givenName":"A"}]}}`,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
		if stderr != "" {
			t.Fatalf("unexpected stderr under --plain: %q", stderr)
		}
	})

	want := "REQUEST_KEY\tRESOURCE\tNAME\tEMAIL\tSTATUS\tERROR\npeople/1\tpeople/1\tA\t\tOK\t\n"
	if out != want {
		t.Fatalf("plain update output with empty response = %q, want %q", out, want)
	}
}

func TestContactsBatchUpdate_HumanStillPrintsCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &people.BatchUpdateContactsResponse{
			UpdateResult: map[string]people.PersonResponse{
				"people/1": {Person: &people.Person{ResourceName: "people/1"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	usePeopleContactsTestServer(t, srv)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{
				"--account", "a@b.com",
				"contacts", "batch", "update",
				"--contacts-json", `{"people/1":{"names":[{"givenName":"A"}]}}`,
			}); execErr != nil {
				t.Fatalf("Execute: %v", execErr)
			}
		})
	})
	if out != "Updated 1 contact(s)\n" {
		t.Fatalf("human update output = %q, want count prose", out)
	}
}

func usePeopleContactsTestServer(t *testing.T, srv *httptest.Server) {
	t.Helper()

	origNew := newPeopleContactsService
	t.Cleanup(func() {
		newPeopleContactsService = origNew
		srv.Close()
	})

	svc, err := people.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }
}
