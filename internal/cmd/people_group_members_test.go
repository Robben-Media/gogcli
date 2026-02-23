package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestContactGroupMembersModify(t *testing.T) {
	tests := []struct {
		name          string
		cmd           ContactGroupMembersModifyCmd
		force         bool
		wantErr       bool
		wantAddCount  int
		wantRemoveLen int
	}{
		{
			name:         "adds members to group",
			cmd:          ContactGroupMembersModifyCmd{GroupName: "contactGroups/123", Add: []string{"people/c1", "people/c2"}},
			force:        true,
			wantErr:      false,
			wantAddCount: 2,
		},
		{
			name:          "removes members from group",
			cmd:           ContactGroupMembersModifyCmd{GroupName: "contactGroups/123", Remove: []string{"people/c1"}},
			force:         true,
			wantErr:       false,
			wantRemoveLen: 1,
		},
		{
			name:         "adds and removes members",
			cmd:          ContactGroupMembersModifyCmd{GroupName: "contactGroups/123", Add: []string{"people/c3"}, Remove: []string{"people/c1"}},
			force:        true,
			wantErr:      false,
			wantAddCount: 1,
		},
		{
			name:    "fails with no add or remove",
			cmd:     ContactGroupMembersModifyCmd{GroupName: "contactGroups/123"},
			force:   true,
			wantErr: true,
		},
		{
			name:    "fails with empty group name",
			cmd:     ContactGroupMembersModifyCmd{Add: []string{"people/c1"}},
			force:   true,
			wantErr: true,
		},
		{
			name:         "normalizes member names without prefix",
			cmd:          ContactGroupMembersModifyCmd{GroupName: "contactGroups/123", Add: []string{"c1", "c2"}},
			force:        true,
			wantErr:      false,
			wantAddCount: 2,
		},
		{
			name:         "accepts short group ID",
			cmd:          ContactGroupMembersModifyCmd{GroupName: "123", Add: []string{"people/c1"}},
			force:        true,
			wantErr:      false,
			wantAddCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					// Get existing group
					resp := &people.ContactGroup{
						ResourceName: "contactGroups/123",
						Name:         "Test Group",
						MemberCount:  5,
						Etag:         "etag123",
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
					return
				}

				if r.Method == http.MethodPost {
					// Modify members
					if !testPathHasPrefix(r.URL.Path, "/v1/contactGroups/") || !testPathHasSuffix(r.URL.Path, ":modify") {
						t.Errorf("unexpected modify path: %s", r.URL.Path)
					}

					var req people.ModifyContactGroupMembersRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("failed to decode request: %v", err)
					}

					if tt.wantAddCount > 0 && len(req.ResourceNamesToAdd) != tt.wantAddCount {
						t.Errorf("expected %d members to add, got %d", tt.wantAddCount, len(req.ResourceNamesToAdd))
					}
					if tt.wantRemoveLen > 0 && len(req.ResourceNamesToRemove) != tt.wantRemoveLen {
						t.Errorf("expected %d members to remove, got %d", tt.wantRemoveLen, len(req.ResourceNamesToRemove))
					}

					resp := &people.ModifyContactGroupMembersResponse{
						NotFoundResourceNames: []string{},
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
			if tt.force {
				flags.Force = true
			}

			// Build kong context with args
			args := make([]string, 0, 1+len(tt.cmd.Add)*2+len(tt.cmd.Remove)*2)
			args = append(args, tt.cmd.GroupName)
			for _, add := range tt.cmd.Add {
				args = append(args, "--add", add)
			}
			for _, rm := range tt.cmd.Remove {
				args = append(args, "--remove", rm)
			}
			kc := buildKongWithArgs(t, &tt.cmd, args)

			err = tt.cmd.Run(ctx, kc, flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeMemberNames(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty input",
			input:    []string{},
			expected: nil,
		},
		{
			name:     "already has prefix",
			input:    []string{"people/c1", "people/c2"},
			expected: []string{"people/c1", "people/c2"},
		},
		{
			name:     "needs prefix",
			input:    []string{"c1", "c2"},
			expected: []string{"people/c1", "people/c2"},
		},
		{
			name:     "mixed",
			input:    []string{"people/c1", "c2"},
			expected: []string{"people/c1", "people/c2"},
		},
		{
			name:     "with whitespace",
			input:    []string{"  c1  ", "c2"},
			expected: []string{"people/c1", "people/c2"},
		},
		{
			name:     "skips empty strings",
			input:    []string{"c1", "", "c2"},
			expected: []string{"people/c1", "people/c2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMemberNames(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d items, got %d", len(tt.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("expected %s at index %d, got %s", tt.expected[i], i, v)
				}
			}
		})
	}
}

func testPathHasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
