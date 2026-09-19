package googleops

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Native request packages must not acquire a CLI bridge or output-capture dependency.
func TestNativePackagesDoNotImportCLI(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	forbidden := map[string]bool{
		"os/exec": true,
		"github.com/steipete/gogcli/internal/cmd":    true,
		"github.com/steipete/gogcli/internal/cli":    true,
		"github.com/steipete/gogcli/internal/outfmt": true,
	}

	for _, dir := range []string{"internal/googleops", "internal/googleapi", "internal/mcpcontract", "internal/mcpserver", "internal/accountconnect", "internal/access", "cmd/gog-mcp"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return fmt.Errorf("parse native source %s: %w", path, err)
			}

			for _, spec := range file.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return fmt.Errorf("decode import in %s: %w", path, err)
				}

				if forbidden[imported] {
					t.Errorf("native request package %s imports %s", path, imported)
				}
			}

			return nil
		})
		if err != nil {
			t.Errorf("inspect %s: %v", dir, err)
		}
	}
}
