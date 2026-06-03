package topology

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestDiscoverFunctionsBuildsDeterministicTopology(t *testing.T) {
	t.Parallel()

	grandchild := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	childA := &ast.FunctionExpr{
		Stmts: []ast.Stmt{&ast.LocalAssignStmt{
			Names: []string{"grandchild"},
			Exprs: []ast.Expr{grandchild},
		}},
	}
	childB := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	rootFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"a"}, Exprs: []ast.Expr{childA}},
			&ast.LocalAssignStmt{Names: []string{"b"}, Exprs: []ast.Expr{childB}},
		},
	}
	bindings := bind.Bind(rootFn, nil)
	root := cfg.BuildWithBindings(rootFn, bindings)
	graphA := cfg.BuildWithBindings(childA, bindings)
	graphB := cfg.BuildWithBindings(childB, bindings)
	graphGrandchild := cfg.BuildWithBindings(grandchild, bindings)

	topo := DiscoverFunctions(FunctionDiscoveryInput{
		Root: root,
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			switch fn {
			case childA:
				return graphA
			case childB:
				return graphB
			case grandchild:
				return graphGrandchild
			default:
				return nil
			}
		},
	})

	rootRef := ref.FuncRef{GraphID: root.ID()}
	refA := ref.FuncRef{GraphID: graphA.ID()}
	refB := ref.FuncRef{GraphID: graphB.ID()}
	refGrandchild := ref.FuncRef{GraphID: graphGrandchild.ID()}
	wantRefs := []ref.FuncRef{rootRef, refA, refB, refGrandchild}
	gotRefs := topo.Refs()
	if len(gotRefs) != len(wantRefs) {
		t.Fatalf("Refs = %+v, want %+v", gotRefs, wantRefs)
	}
	for i := range wantRefs {
		if gotRefs[i] != wantRefs[i] {
			t.Fatalf("Refs = %+v, want %+v", gotRefs, wantRefs)
		}
	}
	if got := topo.Graph(refA); got != graphA {
		t.Fatalf("Graph(refA) = %p, want %p", got, graphA)
	}
	if got, ok := topo.RefForGraph(graphGrandchild); !ok || got != refGrandchild {
		t.Fatalf("RefForGraph(grandchild) = %+v/%v, want %+v/true", got, ok, refGrandchild)
	}
	if got, ok := topo.RefForFunction(childB); !ok || got != refB {
		t.Fatalf("RefForFunction(childB) = %+v/%v, want %+v/true", got, ok, refB)
	}
	nestedSymbols := make(map[*ast.FunctionExpr]cfg.SymbolID)
	for _, nested := range root.NestedFunctions() {
		nestedSymbols[nested.Func] = nested.Symbol
	}
	if sym := nestedSymbols[childA]; sym == 0 {
		t.Fatal("childA declaration symbol not recorded")
	} else if got, ok := topo.RefForSymbol(sym); !ok || got != refA {
		t.Fatalf("RefForSymbol(childA declaration symbol) = %+v/%v, want %+v/true", got, ok, refA)
	}
	if got := topo.Function(refA); got != childA {
		t.Fatalf("Function(refA) = %p, want %p", got, childA)
	}
	if got := topo.NestedRefs(rootRef); len(got) != 2 || got[0] != refA || got[1] != refB {
		t.Fatalf("NestedRefs(root) = %+v, want [%+v %+v]", got, refA, refB)
	}
	if got, ok := topo.ParentRef(refGrandchild); !ok || got != refA {
		t.Fatalf("ParentRef(grandchild) = %+v/%v, want %+v/true", got, ok, refA)
	}
	if got := topo.ParentChain(refGrandchild); len(got) != 2 || got[0] != refA || got[1] != rootRef {
		t.Fatalf("ParentChain(grandchild) = %+v, want [%+v %+v]", got, refA, rootRef)
	}
}

func TestDiscoverFunctionsMapsSymbolsAndMethodDefs(t *testing.T) {
	t.Parallel()

	methodFn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   []ast.Stmt{},
	}
	rootFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"T"},
				Exprs: []ast.Expr{&ast.TableExpr{}},
			},
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Receiver: &ast.IdentExpr{Value: "T"},
					Method:   "foo",
				},
				Func: methodFn,
			},
		},
	}
	bindings := bind.Bind(rootFn, nil)
	root := cfg.BuildWithBindings(rootFn, bindings)
	methodGraph := cfg.BuildWithBindings(methodFn, bindings)

	topo := DiscoverFunctions(FunctionDiscoveryInput{
		Root: root,
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			if fn == methodFn {
				return methodGraph
			}
			return nil
		},
	})
	methodRef := ref.FuncRef{GraphID: methodGraph.ID()}
	methodDef := topo.MethodDef(methodRef)
	if methodDef == nil || methodDef.FuncExpr != methodFn || !methodDef.IsMethod {
		t.Fatalf("MethodDef(methodRef) = %#v, want method definition for methodFn", methodDef)
	}
	if methodDef.Symbol == 0 {
		t.Fatal("method definition symbol not recorded by CFG")
	}
	if got, ok := topo.RefForSymbol(methodDef.Symbol); !ok || got != methodRef {
		t.Fatalf("RefForSymbol(method definition symbol) = %+v/%v, want %+v/true", got, ok, methodRef)
	}
	sym, ok := bindings.FuncLitSymbol(methodFn)
	if !ok || sym == 0 {
		t.Fatal("method function literal symbol not recorded by bindings")
	}
	if got, ok := topo.RefForSymbol(sym); !ok || got != methodRef {
		t.Fatalf("RefForSymbol(method symbol) = %+v/%v, want %+v/true", got, ok, methodRef)
	}
}

func TestDiscoverFunctionsDoesNotTreatFieldDefinitionAsMethod(t *testing.T) {
	t.Parallel()

	chunk, err := parse.ParseString(`
local M = {}
function M.decode(source: string): string
	return source
end
`, "field-function.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rootFn := &ast.FunctionExpr{Stmts: chunk}
	bindings := bind.Bind(rootFn, nil)
	root := cfg.BuildWithBindings(rootFn, bindings)
	nested := root.NestedFunctions()
	if len(nested) != 1 || nested[0].Func == nil || nested[0].Symbol == 0 {
		t.Fatalf("nested functions = %#v, want one symbolized field function", nested)
	}
	fieldFn := nested[0].Func
	fieldGraph := cfg.BuildWithBindings(fieldFn, bindings)

	topo := DiscoverFunctions(FunctionDiscoveryInput{
		Root: root,
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			if fn == fieldFn {
				return fieldGraph
			}
			return nil
		},
	})
	fieldRef := ref.FuncRef{GraphID: fieldGraph.ID()}
	if methodDef := topo.MethodDef(fieldRef); methodDef != nil {
		t.Fatalf("MethodDef(fieldRef) = %#v, want nil for dotted field function", methodDef)
	}
	if got, ok := topo.RefForSymbol(nested[0].Symbol); !ok || got != fieldRef {
		t.Fatalf("RefForSymbol(field definition symbol) = %+v/%v, want %+v/true", got, ok, fieldRef)
	}
	sym, ok := bindings.FuncLitSymbol(fieldFn)
	if !ok || sym == 0 {
		t.Fatal("field function literal symbol not recorded by bindings")
	}
	if got, ok := topo.RefForSymbol(sym); !ok || got != fieldRef {
		t.Fatalf("RefForSymbol(field literal symbol) = %+v/%v, want %+v/true", got, ok, fieldRef)
	}
}

func TestDiscoverFunctionsMapsParsedLocalFunctionSymbol(t *testing.T) {
	t.Parallel()

	chunk, err := parse.ParseString(`
local function get_db()
	return 1
end
`, "local-function.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	rootFn := &ast.FunctionExpr{Stmts: chunk}
	bindings := bind.Bind(rootFn, nil)
	root := cfg.BuildWithBindings(rootFn, bindings)
	nested := root.NestedFunctions()
	if len(nested) != 1 || nested[0].Func == nil || nested[0].Symbol == 0 {
		t.Fatalf("nested functions = %#v, want one symbolized local function", nested)
	}
	childGraph := cfg.BuildWithBindings(nested[0].Func, bindings)

	topo := DiscoverFunctions(FunctionDiscoveryInput{
		Root: root,
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			if fn == nested[0].Func {
				return childGraph
			}
			return nil
		},
	})
	childRef := ref.FuncRef{GraphID: childGraph.ID()}
	if got, ok := topo.RefForSymbol(nested[0].Symbol); !ok || got != childRef {
		t.Fatalf("RefForSymbol(local function symbol) = %+v/%v, want %+v/true", got, ok, childRef)
	}
	if syms := topo.SymbolsForRef(childRef); len(syms) != 1 || syms[0] != nested[0].Symbol {
		t.Fatalf("SymbolsForRef(child) = %+v, want [%d]", syms, nested[0].Symbol)
	}
}

func TestNewFunctionTopologyDeduplicatesRows(t *testing.T) {
	t.Parallel()

	fnRef := ref.FuncRef{GraphID: 7}
	topo := NewFunctionTopology([]FunctionEntry{
		{Ref: fnRef, Symbols: []cfg.SymbolID{11}},
		{Ref: fnRef, Symbols: []cfg.SymbolID{12}},
	})
	refs := topo.Refs()
	if len(refs) != 1 || refs[0] != fnRef {
		t.Fatalf("Refs = %+v, want [%+v]", refs, fnRef)
	}
	for _, sym := range []cfg.SymbolID{11, 12} {
		if got, ok := topo.RefForSymbol(sym); !ok || got != fnRef {
			t.Fatalf("RefForSymbol(%d) = %+v/%v, want %+v/true", sym, got, ok, fnRef)
		}
	}
}
