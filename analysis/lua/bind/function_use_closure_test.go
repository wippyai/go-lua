package bind

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestLocalFunctionUseClosureCertifiesOnlyDirectLocalCalls(t *testing.T) {
	definition := localAssign([]string{"worker"}, function(nil, ret(number("1"))))
	first := &ast.FuncCallExpr{Func: ident("worker")}
	second := &ast.FuncCallExpr{Func: ident("worker")}
	bindings := BindChunk([]ast.Stmt{definition, &ast.FuncCallStmt{Expr: first}, ret(second)}, Options{})

	facts := bindings.LocalFunctionUseClosures()
	if len(facts) != 1 {
		t.Fatalf("use closures = %#v, want one", facts)
	}
	fact := facts[0]
	if fact.FunctionSymbol == 0 || fact.TargetSymbol == 0 || !fact.RuntimeUseScanComplete || !fact.BindingStable || !fact.ValueDoesNotEscape || !fact.CallSetComplete {
		t.Fatalf("use closure = %#v, want complete stable non-escaping proof", fact)
	}
	if !reflect.DeepEqual(fact.DirectCalls, []*ast.FuncCallExpr{first, second}) {
		t.Fatalf("direct calls = %#v, want lexical call order", fact.DirectCalls)
	}
	returned := fact.DirectCalls
	returned[0] = nil
	if bindings.LocalFunctionUseClosures()[0].DirectCalls[0] != first {
		t.Fatal("returned direct-call slice aliases binder storage")
	}
}

func TestLocalFunctionUseClosureParsedSourceTracksWholeUnit(t *testing.T) {
	stmts, err := parse.ParseString(`
		local worker = function(value) return value end
		worker(1)
		return worker(2)
	`, "use_closure.lua")
	if err != nil {
		t.Fatal(err)
	}
	facts := BindChunk(stmts, Options{}).LocalFunctionUseClosures()
	if len(facts) != 1 || !facts[0].CallSetComplete || len(facts[0].DirectCalls) != 2 {
		t.Fatalf("parsed use closures = %#v, want one complete two-call record", facts)
	}
}

func TestLocalFunctionUseClosureFailsClosedForEveryNonCallUse(t *testing.T) {
	tests := map[string]func() []ast.Stmt{
		"alias": func() []ast.Stmt {
			definition := localAssign([]string{"worker"}, function(nil))
			return []ast.Stmt{definition, localAssign([]string{"alias"}, ident("worker"))}
		},
		"argument": func() []ast.Stmt {
			definition := localAssign([]string{"worker"}, function(nil))
			return []ast.Stmt{definition, &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: ident("sink"), Args: []ast.Expr{ident("worker")}}}}
		},
		"return": func() []ast.Stmt {
			definition := localAssign([]string{"worker"}, function(nil))
			return []ast.Stmt{definition, ret(ident("worker"))}
		},
		"capture": func() []ast.Stmt {
			definition := localAssign([]string{"worker"}, function(nil))
			invoke := function(nil, &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: ident("worker")}})
			return []ast.Stmt{definition, localAssign([]string{"invoke"}, invoke)}
		},
		"reassignment": func() []ast.Stmt {
			definition := localAssign([]string{"worker"}, function(nil))
			return []ast.Stmt{definition, &ast.AssignStmt{Lhs: []ast.Expr{ident("worker")}, Rhs: []ast.Expr{function(nil)}}}
		},
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			facts := BindChunk(source(), Options{}).LocalFunctionUseClosures()
			if len(facts) == 0 {
				t.Fatal("missing conservative record")
			}
			fact := facts[0]
			if fact.CallSetComplete {
				t.Fatalf("use closure = %#v, want fail-closed", fact)
			}
			if name == "reassignment" {
				if fact.BindingStable {
					t.Fatalf("reassigned binding certified stable: %#v", fact)
				}
			} else {
				if fact.BindingStable {
					t.Fatalf("non-direct read certified stable: %#v", fact)
				}
				if fact.ValueDoesNotEscape {
					t.Fatalf("non-call use certified non-escaping: %#v", fact)
				}
			}
		})
	}
}

func TestLocalFunctionUseClosureRejectsRecursiveCapture(t *testing.T) {
	call := &ast.FuncCallExpr{Func: ident("worker")}
	definition := localAssign([]string{"worker"}, function(nil, &ast.FuncCallStmt{Expr: call}))
	facts := BindChunk([]ast.Stmt{definition}, Options{}).LocalFunctionUseClosures()
	if len(facts) != 1 {
		t.Fatalf("use closures = %#v, want recursive record", facts)
	}
	fact := facts[0]
	if !fact.RuntimeUseScanComplete || fact.BindingStable || fact.ValueDoesNotEscape || fact.CallSetComplete {
		t.Fatalf("recursive capture proof = %#v, want every positive field degraded", fact)
	}
}

func TestLocalFunctionUseClosureFailsClosedWhenRuntimeScanIsIncomplete(t *testing.T) {
	definition := localAssign([]string{"worker"}, function(nil))
	call := &ast.FuncCallExpr{Func: ident("worker")}
	bindings := BindChunk([]ast.Stmt{definition, &ast.FuncCallStmt{Expr: call}}, Options{})
	b := binder{result: bindings}
	b.invalidateRuntimeUseScan()
	facts := bindings.LocalFunctionUseClosures()
	if len(facts) != 1 {
		t.Fatalf("use closures = %#v, want denied record", facts)
	}
	fact := facts[0]
	if fact.RuntimeUseScanComplete || fact.BindingStable || fact.ValueDoesNotEscape || fact.CallSetComplete {
		t.Fatalf("incomplete scan published positive proof: %#v", fact)
	}
}

func TestLocalFunctionUseClosureIsDeterministicAcrossIndependentBinds(t *testing.T) {
	build := func() []ast.Stmt {
		definition := localAssign([]string{"worker"}, function(nil))
		return []ast.Stmt{definition, &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: ident("worker")}}}
	}
	left := BindChunk(build(), Options{}).LocalFunctionUseClosures()
	right := BindChunk(build(), Options{}).LocalFunctionUseClosures()
	if len(left) != 1 || len(right) != 1 || left[0].FunctionSymbol != right[0].FunctionSymbol || left[0].TargetSymbol != right[0].TargetSymbol || len(left[0].DirectCalls) != len(right[0].DirectCalls) || left[0].CallSetComplete != right[0].CallSetComplete {
		t.Fatalf("independent bind facts differ\nleft:  %#v\nright: %#v", left, right)
	}
}
