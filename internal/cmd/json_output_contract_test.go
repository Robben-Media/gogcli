package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestProductionWriteJSONCallsDeclareResultsContract(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob command files: %v", err)
	}

	fset := token.NewFileSet()
	for _, file := range files {
		if filepath.Ext(file) != ".go" || len(file) >= len("_test.go") && file[len(file)-len("_test.go"):] == "_test.go" {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isOutfmtCall(call.Fun, "WriteJSON") {
				return true
			}
			if len(call.Args) != 3 {
				t.Errorf("%s: WriteJSON must receive context, writer, and explicit result contract", fset.Position(call.Pos()))
				return true
			}
			contract, ok := call.Args[2].(*ast.CallExpr)
			if !ok || !(isOutfmtCall(contract.Fun, "DirectResult") || isOutfmtCall(contract.Fun, "PrimaryResult")) {
				t.Errorf("%s: WriteJSON output must use outfmt.DirectResult or outfmt.PrimaryResult", fset.Position(call.Args[2].Pos()))
			}
			return true
		})
	}
}

func isOutfmtCall(expr ast.Expr, name string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "outfmt"
}
