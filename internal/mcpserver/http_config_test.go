package mcpserver_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpserver"
)

func TestParseHTTPConfigAcceptsDeploymentShape(t *testing.T) {
	t.Parallel()

	token, digest := testBearer(t)
	_ = token

	cfg, err := mcpserver.ParseHTTPConfig(httpConfigJSON(t, map[string]any{
		"host":            "google-mcp.example.com",
		"allowed_origins": []string{"https://google-mcp.example.com"},
		"callers": []map[string]any{
			{
				"id":               "hermes-work",
				"token_sha256":     digest,
				"principal_id":     "jeremy",
				"allow_operations": []string{"accounts_list", "gmail_search", "gmail_get_message", "gmail_get_thread"},
				"grants": []map[string]any{
					{
						"principal_id": "jeremy",
						"account_ids":  []string{"work"},
						"client_names": []string{"native-mcp"},
						"operations":   []string{"gmail:messages.search", "gmail:get", "gmail:thread.get"},
					},
				},
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "google-mcp.example.com" || len(cfg.Callers) != 1 {
		t.Fatalf("%+v", cfg)
	}

	if cfg.Callers[0].ID != "hermes-work" || cfg.Callers[0].PrincipalID != "jeremy" {
		t.Fatalf("caller %+v", cfg.Callers[0])
	}

	if got := cfg.UniquePrincipalIDs(); len(got) != 1 || got[0] != "jeremy" {
		t.Fatalf("principals %#v", got)
	}
}

func TestParseHTTPConfigRejectsMalformed(t *testing.T) {
	t.Parallel()

	_, digestA := testBearer(t, "a")
	_, digestB := testBearer(t, "b")

	tests := []struct {
		name string
		raw  any
	}{
		{name: "unknown field", raw: map[string]any{"host": "google-mcp.example.com", "extra": true, "callers": []map[string]any{minimalCaller("a", digestA)}}},
		{name: "missing host", raw: map[string]any{"callers": []map[string]any{minimalCaller("a", digestA)}}},
		{name: "host with scheme", raw: map[string]any{"host": "https://google-mcp.example.com", "callers": []map[string]any{minimalCaller("a", digestA)}}},
		{name: "empty callers", raw: map[string]any{"host": "google-mcp.example.com", "callers": []map[string]any{}}},
		{name: "duplicate ids", raw: map[string]any{"host": "google-mcp.example.com", "callers": []map[string]any{minimalCaller("a", digestA), minimalCaller("a", digestB)}}},
		{name: "duplicate hash", raw: map[string]any{"host": "google-mcp.example.com", "callers": []map[string]any{minimalCaller("a", digestA), minimalCaller("b", digestA)}}},
		{name: "bad digest", raw: map[string]any{"host": "google-mcp.example.com", "callers": []map[string]any{minimalCaller("a", "zzzz")}}},
		{name: "principal mismatch", raw: map[string]any{"host": "google-mcp.example.com", "callers": []map[string]any{
			{
				"id": "a", "token_sha256": digestA, "principal_id": "jeremy",
				"allow_operations": []string{"accounts_list"},
				"grants": []map[string]any{{
					"principal_id": "other", "account_ids": []string{"work"}, "client_names": []string{"native-mcp"}, "operations": []string{"gmail:get"},
				}},
			},
		}}},
		{name: "unknown operation", raw: map[string]any{"host": "google-mcp.example.com", "callers": []map[string]any{
			{
				"id": "a", "token_sha256": digestA, "principal_id": "jeremy",
				"allow_operations": []string{"not_a_tool"},
			},
		}}},
		{name: "origin with path", raw: map[string]any{"host": "google-mcp.example.com", "allowed_origins": []string{"https://google-mcp.example.com/mcp"}, "callers": []map[string]any{minimalCaller("a", digestA)}}},
		{name: "caller unknown field", raw: map[string]any{"host": "google-mcp.example.com", "callers": []map[string]any{
			{"id": "a", "token_sha256": digestA, "principal_id": "jeremy", "role": "admin"},
		}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := mcpserver.ParseHTTPConfig(mustJSON(t, test.raw))
			if !errors.Is(err, mcpserver.ErrHTTPConfigInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestParseHTTPConfigRejectsTooManyCallers(t *testing.T) {
	t.Parallel()

	callers := make([]map[string]any, 0, 33)

	for i := 0; i < 33; i++ {
		token := strings.Repeat("t", 32) + strconv.Itoa(i)
		sum := sha256.Sum256([]byte(token))
		callers = append(callers, minimalCaller("caller-"+strconv.Itoa(i), hex.EncodeToString(sum[:])))
	}

	_, err := mcpserver.ParseHTTPConfig(mustJSON(t, map[string]any{
		"host":    "google-mcp.example.com",
		"callers": callers,
	}))
	if !errors.Is(err, mcpserver.ErrHTTPConfigInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestParseHTTPConfigAllowsSharedOwner(t *testing.T) {
	t.Parallel()

	_, digestA := testBearer(t, "a")
	_, digestB := testBearer(t, "b")

	cfg, err := mcpserver.ParseHTTPConfig(mustJSON(t, map[string]any{
		"host": "google-mcp.example.com",
		"callers": []map[string]any{
			{
				"id": "hermes-work", "token_sha256": digestA, "principal_id": "jeremy",
				"allow_operations": []string{"accounts_list", "gmail_search"},
				"grants": []map[string]any{{
					"principal_id": "jeremy", "account_ids": []string{"work"}, "client_names": []string{"native-mcp"}, "operations": []string{"gmail:messages.search"},
				}},
			},
			{
				"id": "hermes-mail", "token_sha256": digestB, "principal_id": "jeremy",
				"allow_operations": []string{"accounts_list", "gmail_get_message"},
				"grants": []map[string]any{{
					"principal_id": "jeremy", "account_ids": []string{"personal"}, "client_names": []string{"native-mcp"}, "operations": []string{"gmail:get"},
				}},
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	ids := cfg.UniquePrincipalIDs()
	if len(ids) != 1 || ids[0] != "jeremy" {
		t.Fatalf("%#v", ids)
	}
}

func TestLoadHTTPConfigReadsFile(t *testing.T) {
	t.Parallel()

	_, digest := testBearer(t)

	path := filepath.Join(t.TempDir(), "http-config.json")
	if err := os.WriteFile(path, httpConfigJSON(t, map[string]any{
		"host":    "google-mcp.example.com",
		"callers": []map[string]any{minimalCaller("hermes-work", digest)},
	}), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := mcpserver.LoadHTTPConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Callers[0].ID != "hermes-work" {
		t.Fatalf("%+v", cfg.Callers[0])
	}
}

func TestParseHTTPConfigFillsEmptyGrantPrincipal(t *testing.T) {
	t.Parallel()

	_, digest := testBearer(t)

	cfg, err := mcpserver.ParseHTTPConfig(mustJSON(t, map[string]any{
		"host": "google-mcp.example.com",
		"callers": []map[string]any{{
			"id": "a", "token_sha256": digest, "principal_id": "jeremy",
			"allow_operations": []string{"accounts_list", "gmail_search"},
			"grants": []map[string]any{{
				"account_ids": []string{"work"}, "client_names": []string{"native-mcp"}, "operations": []string{"gmail:messages.search"},
			}},
		}},
	}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Callers[0].Grants[0].PrincipalID != "jeremy" {
		t.Fatalf("%+v", cfg.Callers[0].Grants[0])
	}
}

func minimalCaller(id, digest string) map[string]any {
	return map[string]any{
		"id":               id,
		"token_sha256":     digest,
		"principal_id":     "jeremy",
		"allow_operations": []string{"accounts_list"},
		"grants": []map[string]any{{
			"principal_id": "jeremy",
			"account_ids":  []string{"work"},
			"client_names": []string{"native-mcp"},
			"operations":   []string{"gmail:messages.search"},
		}},
	}
}

func httpConfigJSON(t *testing.T, value map[string]any) []byte {
	t.Helper()
	return mustJSON(t, value)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func testBearer(t *testing.T, extra ...string) (token, digest string) {
	t.Helper()

	token = strings.Repeat("t", 32) + t.Name() + strings.Join(extra, "")
	sum := sha256.Sum256([]byte(token))

	return token, hex.EncodeToString(sum[:])
}
