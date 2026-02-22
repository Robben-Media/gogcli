package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecthomas/kong"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestContactGroupsList(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(svc *people.Service)
		cmd        ContactGroupsListCmd
		wantErr    bool
		wantOutput string
	}{
		{
			name: "lists contact groups",
			setup: func(svc *people.Service) {
				// Setup is handled by the mock server
			},
			cmd: ContactGroupsListCmd{Max: 10},
		},
		{
			name: "lists with page token",
			setup: func(svc *people.Service) {
				// Setup is handled by the mock server
			},
			cmd:     ContactGroupsListCmd{Max: 10, Page: "token123"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/contactGroups" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.URL.Query().Get("pageSize") != "10" {
					t.Errorf("unexpected pageSize: %s", r.URL.Query().Get("pageSize"))
				}
				if tt.cmd.Page != "" && r.URL.Query().Get("pageToken") != tt.cmd.Page {
					t.Errorf("unexpected pageToken: %s", r.URL.Query().Get("pageToken"))
				}

				resp := &people.ListContactGroupsResponse{
					ContactGroups: []*people.ContactGroup{
						{ResourceName: "contactGroups/1", Name: "Family", MemberCount: 5, GroupType: "USER_CONTACT_GROUP"},
						{ResourceName: "contactGroups/2", Name: "Work", MemberCount: 10, GroupType: "USER_CONTACT_GROUP"},
					},
					TotalItems:    2,
					NextPageToken: "",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
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

			kc := buildKong(t, &tt.cmd)
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
			_ = kc
		})
	}
}

func TestContactGroupsGet(t *testing.T) {
	tests := []struct {
		name         string
		cmd          ContactGroupsGetCmd
		resourceName string
		wantErr      bool
	}{
		{
			name:         "gets contact group by full resource name",
			cmd:          ContactGroupsGetCmd{ResourceName: "contactGroups/123"},
			resourceName: "contactGroups/123",
			wantErr:      false,
		},
		{
			name:         "gets contact group by short ID",
			cmd:          ContactGroupsGetCmd{ResourceName: "123"},
			resourceName: "contactGroups/123",
			wantErr:      false,
		},
		{
			name:    "empty resource name fails",
			cmd:     ContactGroupsGetCmd{ResourceName: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/v1/" + tt.resourceName
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
				}

				resp := &people.ContactGroup{
					ResourceName:  tt.resourceName,
					Name:          "Test Group",
					FormattedName: "Test Group",
					GroupType:     "USER_CONTACT_GROUP",
					MemberCount:   5,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
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

func TestContactGroupsBatchGet(t *testing.T) {
	tests := []struct {
		name          string
		cmd           ContactGroupsBatchGetCmd
		wantErr       bool
		wantPathCheck bool
	}{
		{
			name:          "batch gets contact groups",
			cmd:           ContactGroupsBatchGetCmd{ResourceNames: []string{"contactGroups/1", "contactGroups/2"}},
			wantErr:       false,
			wantPathCheck: true,
		},
		{
			name:    "empty resource names fails",
			cmd:     ContactGroupsBatchGetCmd{ResourceNames: []string{}},
			wantErr: true,
		},
		{
			name:    "too many resource names fails",
			cmd:     ContactGroupsBatchGetCmd{ResourceNames: make([]string, 201)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/contactGroups:batchGet" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}

				resp := &people.BatchGetContactGroupsResponse{
					Responses: []*people.ContactGroupResponse{
						{
							ContactGroup: &people.ContactGroup{ResourceName: "contactGroups/1", Name: "Group 1"},
						},
						{
							ContactGroup: &people.ContactGroup{ResourceName: "contactGroups/2", Name: "Group 2"},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
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

func TestContactGroupsCreate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ContactGroupsCreateCmd
		wantErr bool
	}{
		{
			name:    "creates contact group",
			cmd:     ContactGroupsCreateCmd{Name: "New Group"},
			wantErr: false,
		},
		{
			name:    "empty name fails",
			cmd:     ContactGroupsCreateCmd{Name: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/contactGroups" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}

				var req people.CreateContactGroupRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}
				if req.ContactGroup.Name != tt.cmd.Name {
					t.Errorf("unexpected name: got %s, want %s", req.ContactGroup.Name, tt.cmd.Name)
				}

				resp := &people.ContactGroup{
					ResourceName: "contactGroups/new123",
					Name:         req.ContactGroup.Name,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
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

func TestContactGroupsUpdate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     ContactGroupsUpdateCmd
		wantErr bool
	}{
		{
			name:    "updates contact group name",
			cmd:     ContactGroupsUpdateCmd{ResourceName: "contactGroups/123", Name: "Updated Name"},
			wantErr: false,
		},
		{
			name:    "empty resource name fails",
			cmd:     ContactGroupsUpdateCmd{ResourceName: "", Name: "New Name"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					// Get existing group
					resp := &people.ContactGroup{
						ResourceName: "contactGroups/123",
						Name:         "Old Name",
						Etag:         "etag123",
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
					return
				}

				if r.Method == http.MethodPut {
					// Update group
					if !testPathHasPrefix(r.URL.Path, "/v1/contactGroups/") {
						t.Errorf("unexpected path: %s", r.URL.Path)
					}

					var req people.UpdateContactGroupRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("failed to decode request: %v", err)
					}

					resp := &people.ContactGroup{
						ResourceName: "contactGroups/123",
						Name:         req.ContactGroup.Name,
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
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

			// Build kong context with positional argument
			kc := buildKongWithArgs(t, &tt.cmd, []string{tt.cmd.ResourceName, "--name", tt.cmd.Name})

			err = tt.cmd.Run(ctx, kc, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContactGroupsDelete(t *testing.T) {
	tests := []struct {
		name           string
		cmd            ContactGroupsDeleteCmd
		force          bool
		wantErr        bool
		wantDeletePath bool
	}{
		{
			name:           "deletes contact group with force",
			cmd:            ContactGroupsDeleteCmd{ResourceName: "contactGroups/123"},
			force:          true,
			wantErr:        false,
			wantDeletePath: true,
		},
		{
			name:    "empty resource name fails",
			cmd:     ContactGroupsDeleteCmd{ResourceName: ""},
			force:   true,
			wantErr: true,
		},
		{
			name:           "deletes with short ID",
			cmd:            ContactGroupsDeleteCmd{ResourceName: "123"},
			force:          true,
			wantErr:        false,
			wantDeletePath: true,
		},
		{
			name:           "deletes contacts with group",
			cmd:            ContactGroupsDeleteCmd{ResourceName: "contactGroups/123", DeleteContacts: true},
			force:          true,
			wantErr:        false,
			wantDeletePath: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					// Get existing group for confirmation message
					resp := &people.ContactGroup{
						ResourceName: "contactGroups/123",
						Name:         "Test Group",
						MemberCount:  5,
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
					return
				}

				if r.Method == http.MethodDelete {
					if tt.wantDeletePath && !testPathHasPrefix(r.URL.Path, "/v1/contactGroups/") {
						t.Errorf("unexpected delete path: %s", r.URL.Path)
					}
					// Return empty JSON object for successful delete
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte("{}"))
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

func buildKong(t *testing.T, cmd interface{}) *kong.Context {
	t.Helper()
	parser, err := kong.New(cmd)
	if err != nil {
		t.Fatalf("failed to create kong parser: %v", err)
	}
	ctx, err := parser.Parse([]string{})
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	return ctx
}

func buildKongWithArgs(t *testing.T, cmd interface{}, args []string) *kong.Context {
	t.Helper()
	parser, err := kong.New(cmd)
	if err != nil {
		t.Fatalf("failed to create kong parser: %v", err)
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	return ctx
}

func testPathHasPrefix(path, prefix string) bool {
	return len(path) >= len(prefix) && path[:len(prefix)] == prefix
}
