package cmd

import (
	"encoding/json"
	"net/http"
	"testing"

	"google.golang.org/api/tagmanager/v2"
)

func TestExecute_TagManagerVariablesCreate(t *testing.T) {
	setupTagManagerResourceTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tagmanager/v2/accounts/111/containers/c1/workspaces/7/variables" {
			http.NotFound(w, r)
			return
		}
		var body tagmanager.Variable
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Name != "Lookup" || body.Type != "smm" || len(body.Parameter) != 2 {
			t.Fatalf("unexpected variable: %#v", body)
		}
		parameter := body.Parameter[0]
		if parameter.Type != "list" || len(parameter.List) != 1 || parameter.List[0].Map[0].Key != "key" {
			t.Fatalf("nested parameter not preserved: %#v", parameter)
		}
		if body.Parameter[1].Type != "boolean" || body.Parameter[1].Value != "true" {
			t.Fatalf("boolean parameter not preserved: %#v", body.Parameter[1])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"path":"accounts/111/containers/c1/workspaces/7/variables/3","variableId":"3","name":"Lookup","type":"smm"}`))
	}))

	parameter := `{"type":"list","key":"map","list":[{"type":"map","map":[{"type":"template","key":"key","value":"a"},{"type":"template","key":"value","value":"b"}]}]}`
	booleanParameter := `{"type":"boolean","key":"setDefaultValue","value":"true"}`
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "admin@example.com", "gtm", "variables", "create",
				"--account-id", "111", "--container-id", "c1", "--workspace-id", "7",
				"--name", "Lookup", "--type", "smm", "--parameter", parameter, "--parameter", booleanParameter,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var result struct {
		Variable *tagmanager.Variable `json:"variable"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.Variable == nil || result.Variable.VariableId != "3" {
		t.Fatalf("unexpected output %q: %v", out, err)
	}
}

func TestExecute_TagManagerVariablesLifecycle(t *testing.T) {
	path := "/tagmanager/v2/accounts/111/containers/c1/workspaces/7/variables/3"
	requests := 0
	setupTagManagerResourceTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == path:
			_, _ = w.Write([]byte(`{"path":"accounts/111/containers/c1/workspaces/7/variables/3","variableId":"3","name":"Old","type":"c"}`))
		case r.Method == http.MethodPut && r.URL.Path == path:
			var body tagmanager.Variable
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Name != "New" || body.Type != "c" {
				t.Fatalf("partial update lost fields: %#v", body)
			}
			_, _ = w.Write([]byte(`{"path":"accounts/111/containers/c1/workspaces/7/variables/3","variableId":"3","name":"New","type":"c"}`))
		case r.Method == http.MethodPost && r.URL.Path == path+":revert":
			_, _ = w.Write([]byte(`{"variable":{"path":"accounts/111/containers/c1/workspaces/7/variables/3","variableId":"3","name":"Published","type":"c"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == path:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	resource := "accounts/111/containers/c1/workspaces/7/variables/3"
	for _, args := range [][]string{
		{"--json", "--account", "admin@example.com", "gtm", "variables", "get", resource},
		{"--json", "--account", "admin@example.com", "gtm", "variables", "update", resource, "--name", "New"},
		{"--json", "--account", "admin@example.com", "gtm", "variables", "revert", resource},
		{"--json", "--force", "--account", "admin@example.com", "gtm", "variables", "delete", resource},
	} {
		captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := Execute(args); err != nil {
					t.Fatalf("Execute(%v): %v", args, err)
				}
			})
		})
	}
	if requests != 5 {
		t.Fatalf("requests = %d, want 5", requests)
	}
}
