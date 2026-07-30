package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func functionAtLine(tb testing.TB, bindings *bind.Result, line int) *ast.FunctionExpr {
	tb.Helper()
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func != nil && origin.Func.Line() == line {
			return origin.Func
		}
	}
	tb.Fatalf("function at line %d missing", line)
	return nil
}

// requireLexicalResultByName returns the sole published result for a lexical
// body. Application routes are scheduler detail and must never manufacture a
// second observable body result.
func requireLexicalResultByName(tb testing.TB, root *body.Result, name string) *body.Result {
	tb.Helper()
	if root == nil {
		tb.Fatal("root result missing")
	}
	var matches []*body.Result
	var walk func(*body.Result)
	walk = func(parent *body.Result) {
		for _, child := range parent.FunctionResults() {
			if child == nil {
				continue
			}
			origin, ok := root.FunctionOrigin(child.Function())
			if ok {
				candidate := ""
				if origin.HasTargetSymbol {
					candidate = root.SymbolName(origin.TargetSymbol)
				}
				if candidate == "" {
					candidate = origin.Method
				}
				if candidate == name {
					matches = append(matches, child)
				}
			}
			walk(child)
		}
	}
	walk(root)
	if len(matches) != 1 {
		tb.Fatalf("lexical body %q published %d results, want exactly one", name, len(matches))
	}
	return matches[0]
}

func findLocalFunctionByName(tb testing.TB, bindings *bind.Result, name string) *ast.FunctionExpr {
	tb.Helper()
	for _, origin := range bindings.FunctionOrigins() {
		assign, ok := origin.Stmt.(*ast.LocalAssignStmt)
		if !ok || origin.LocalIndex < 0 || origin.LocalIndex >= len(assign.Names) {
			continue
		}
		if assign.Names[origin.LocalIndex] == name {
			return origin.Func
		}
	}
	tb.Fatalf("local function %q not found", name)
	return nil
}
