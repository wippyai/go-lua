package bind_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

type futureStmt struct {
	ast.StmtBase
}

type futureExpr struct {
	ast.ExprBase
}

type futureTypeExpr struct {
	ast.TypeExprBase
}

func incompleteUseResult(extra ast.Stmt) *bind.Result {
	definition := &ast.LocalAssignStmt{
		Names: []string{"worker"},
		Exprs: []ast.Expr{&ast.FunctionExpr{}},
	}
	call := &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "worker"},
	}}
	return bind.BindChunk([]ast.Stmt{definition, call, extra}, bind.Options{})
}

func requireIncompleteUseScan(t *testing.T, result *bind.Result) {
	t.Helper()
	facts := result.LocalFunctionUseClosures()
	if len(facts) != 1 {
		t.Fatalf("use closures = %#v, want one conservative record", facts)
	}
	fact := facts[0]
	if fact.RuntimeUseScanComplete || fact.BindingStable || fact.ValueDoesNotEscape ||
		fact.DirectCallSetComplete || fact.CallSetComplete {
		t.Fatalf("typed-nil input published positive use evidence: %#v", fact)
	}
}

func TestTypedNilASTFailsClosedWithoutPanicking(t *testing.T) {
	var supportedStmt *ast.ReturnStmt
	var supportedExpr *ast.IdentExpr
	var supportedType *ast.OptionalTypeExpr
	var unsupportedStmt *futureStmt
	var unsupportedExpr *futureExpr
	var unsupportedType *futureTypeExpr

	tests := map[string]ast.Stmt{
		"supported-statement": supportedStmt,
		"supported-runtime-expression": &ast.ReturnStmt{
			Exprs: []ast.Expr{supportedExpr},
		},
		"supported-type-expression": &ast.LocalAssignStmt{
			Names: []string{"value"},
			Types: []ast.TypeExpr{supportedType},
		},
		"unsupported-statement": unsupportedStmt,
		"unsupported-runtime-expression": &ast.ReturnStmt{
			Exprs: []ast.Expr{unsupportedExpr},
		},
		"unsupported-type-expression": &ast.LocalAssignStmt{
			Names: []string{"value"},
			Types: []ast.TypeExpr{unsupportedType},
		},
		"nested-statement": &ast.DoBlockStmt{
			Stmts: []ast.Stmt{supportedStmt},
		},
		"nested-runtime-expression": &ast.ReturnStmt{
			Exprs: []ast.Expr{&ast.TableExpr{Fields: []*ast.Field{{
				Value: supportedExpr,
			}}}},
		},
		"nested-type-expression": &ast.LocalAssignStmt{
			Names: []string{"value"},
			Types: []ast.TypeExpr{&ast.OptionalTypeExpr{
				Inner: supportedType,
			}},
		},
	}

	for name, extra := range tests {
		t.Run(name, func(t *testing.T) {
			requireIncompleteUseScan(t, incompleteUseResult(extra))
		})
	}
}

func TestNestedDirectCallsRecordOnEntry(t *testing.T) {
	definition := &ast.LocalAssignStmt{
		Names: []string{"f"},
		Exprs: []ast.Expr{&ast.FunctionExpr{}},
	}
	innerRead := &ast.IdentExpr{Value: "f"}
	inner := &ast.FuncCallExpr{Func: innerRead}
	middleRead := &ast.IdentExpr{Value: "f"}
	middle := &ast.FuncCallExpr{Func: middleRead, Args: []ast.Expr{inner}}
	outerRead := &ast.IdentExpr{Value: "f"}
	outer := &ast.FuncCallExpr{Func: outerRead, Args: []ast.Expr{middle}}

	result := bind.BindChunk([]ast.Stmt{
		definition,
		&ast.ReturnStmt{Exprs: []ast.Expr{outer}},
	}, bind.Options{})
	target, ok := result.LocalSymbolAt(definition, 0)
	if !ok {
		t.Fatal("local f symbol missing")
	}
	for _, read := range []*ast.IdentExpr{outerRead, middleRead, innerRead} {
		if got, found := result.SymbolOf(read); !found || got != target {
			t.Fatalf("callee identity = %d/%v, want %d", got, found, target)
		}
	}
	reads := result.ReadIdents(target)
	if len(reads) != 3 || reads[0] != outerRead || reads[1] != middleRead || reads[2] != innerRead {
		t.Fatalf("read order = %#v, want outer, middle, inner", reads)
	}
	facts := result.LocalFunctionUseClosures()
	if len(facts) != 1 {
		t.Fatalf("use closures = %#v, want one", facts)
	}
	calls := facts[0].DirectCalls
	if len(calls) != 3 || calls[0] != outer || calls[1] != middle || calls[2] != inner {
		t.Fatalf("direct-call order = %#v, want outer, middle, inner", calls)
	}
}
