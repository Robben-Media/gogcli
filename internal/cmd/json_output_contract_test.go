package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This contract check intentionally covers direct production call sites in
// internal/cmd only; integration tests own command-specific output semantics.
func TestInternalCmdProductionWriteJSONCallsDeclareResultsContract(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob command files: %v", err)
	}

	fset := token.NewFileSet()
	for _, file := range files {
		if filepath.Ext(file) != ".go" || strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		outfmtAliases := importedPackageAliases(parsed, "github.com/steipete/gogcli/internal/outfmt")
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isPackageCall(call.Fun, outfmtAliases, "WriteJSON") {
				return true
			}
			if len(call.Args) != 3 {
				t.Errorf("%s: WriteJSON must receive context, writer, and explicit result contract", fset.Position(call.Pos()))
				return true
			}
			if !isContextIdentifier(call.Args[0]) {
				t.Errorf("%s: WriteJSON first argument must be a context identifier", fset.Position(call.Args[0].Pos()))
			}
			contract, ok := call.Args[2].(*ast.CallExpr)
			if !ok || !(isPackageCall(contract.Fun, outfmtAliases, "DirectResult") || isPackageCall(contract.Fun, outfmtAliases, "PrimaryResult")) {
				t.Errorf("%s: WriteJSON output must use outfmt.DirectResult or outfmt.PrimaryResult", fset.Position(call.Args[2].Pos()))
			}
			return true
		})
	}
}

func isContextIdentifier(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name != ""
}

func importedPackageAliases(file *ast.File, importPath string) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != importPath {
			continue
		}

		alias := filepath.Base(path)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias != "." && alias != "_" {
			aliases[alias] = struct{}{}
		}
	}
	return aliases
}

func isPackageCall(expr ast.Expr, aliases map[string]struct{}, name string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = aliases[pkg.Name]
	return ok
}
