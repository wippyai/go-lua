package resolve_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRootName_NilGraph(t *testing.T) {
	result := resolve.RootName(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestRootName_ZeroSymbol(t *testing.T) {
	result := resolve.RootName(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestRootNameFromBindings_NilBindings(t *testing.T) {
	result := resolve.RootNameFromBindings(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestRootNameFromBindings_WithSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"myVar"}},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	bindings := bind.Bind(fn, nil)
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) == 0 {
		t.Skip("no param symbols")
	}
	result := resolve.RootNameFromBindings(bindings, paramSyms[0], "fallback")
	if result != "myVar" {
		t.Errorf("expected 'myVar', got %q", result)
	}
}

func TestGetBindings_NilInputs(t *testing.T) {
	result := resolve.GetBindings(nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestGetBindings_NilGraph(t *testing.T) {
	result := resolve.GetBindings(&flow.Inputs{Graph: nil})
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestRootFromSymbol_NilInputs(t *testing.T) {
	result := resolve.RootFromSymbol(nil, 0, "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestClassifyReturnExpr_TrueExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.TrueExpr{})
	if result != flow.ReturnTrue {
		t.Errorf("expected ReturnTrue, got %v", result)
	}
}

func TestClassifyReturnExpr_FalseExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.FalseExpr{})
	if result != flow.ReturnFalse {
		t.Errorf("expected ReturnFalse, got %v", result)
	}
}

func TestClassifyReturnExpr_TrueIdent(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.IdentExpr{Value: "true"})
	if result != flow.ReturnTrue {
		t.Errorf("expected ReturnTrue, got %v", result)
	}
}

func TestClassifyReturnExpr_FalseIdent(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.IdentExpr{Value: "false"})
	if result != flow.ReturnFalse {
		t.Errorf("expected ReturnFalse, got %v", result)
	}
}

func TestClassifyReturnExpr_OtherExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.StringExpr{Value: "hello"})
	if result != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown, got %v", result)
	}
}

func TestResolveSymbolToFunctionLiteral_LocalAssign(t *testing.T) {
	fnLit := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts:   []ast.Stmt{&ast.ReturnStmt{}},
	}
	root := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"f"},
				Exprs: []ast.Expr{fnLit},
			},
		},
	}
	graph := cfg.Build(root)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	var sym cfg.SymbolID
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if sym != 0 || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Name == "f" && source == fnLit {
				sym = target.Symbol
			}
		})
	})
	if sym == 0 {
		t.Fatal("expected non-zero symbol for local function assignment")
	}

	got := resolve.ResolveSymbolToFunctionLiteral(graph, sym)
	if got != fnLit {
		t.Fatalf("ResolveSymbolToFunctionLiteral mismatch: got %p want %p", got, fnLit)
	}
}

func TestResolveExprToTableLiteral_IdentRef(t *testing.T) {
	tbl := &ast.TableExpr{}
	retIdent := &ast.IdentExpr{Value: "t"}
	root := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"t"},
				Exprs: []ast.Expr{tbl},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{retIdent},
			},
		},
	}
	graph := cfg.Build(root)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}

	got := resolve.ResolveExprToTableLiteral(retIdent, graph)
	if got != tbl {
		t.Fatalf("ResolveExprToTableLiteral mismatch: got %p want %p", got, tbl)
	}
}

func TestRef_NilType(t *testing.T) {
	result := resolve.Ref(nil, nil)
	if result != nil {
		t.Error("expected nil for nil type")
	}
}

func TestRef_NonRefType(t *testing.T) {
	result := resolve.Ref(typ.String, nil)
	if result != typ.String {
		t.Error("expected typ.String returned unchanged")
	}
}

func TestRef_NilScope(t *testing.T) {
	ref := typ.NewRef("", "MyType")
	result := resolve.Ref(ref, nil)
	if result != ref {
		t.Error("expected ref returned unchanged for nil scope")
	}
}

func TestRef_ResolvedRef(t *testing.T) {
	sc := scope.New().WithType("MyAlias", typ.String)

	ref := typ.NewRef("", "MyAlias")
	result := resolve.Ref(ref, sc)
	if result != typ.String {
		t.Errorf("expected typ.String, got %v", result)
	}
}

func TestBuildContextSymbolResolver_NilContext(t *testing.T) {
	resolver := resolve.BuildContextSymbolResolver(nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	_, ok := resolver(0, 1)
	if ok {
		t.Error("expected ok=false for nil context")
	}
}

func TestBuildInputSymbolResolver_NilInputs(t *testing.T) {
	resolver := resolve.BuildInputSymbolResolver(nil, nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	_, ok := resolver(0, 1)
	if ok {
		t.Error("expected ok=false for nil inputs")
	}
}

func TestBuildContextTypeKeyResolver_BuiltinType(t *testing.T) {
	resolver := resolve.BuildContextTypeKeyResolver(nil, nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}

	key, ok := resolver("string", nil)
	if !ok {
		t.Error("expected ok=true for builtin 'string'")
	}
	if key.IsZero() {
		t.Error("expected non-zero key for 'string'")
	}
}

func TestBuildContextTypeKeyResolver_UnknownType(t *testing.T) {
	resolver := resolve.BuildContextTypeKeyResolver(nil, nil)
	_, ok := resolver("UnknownType", nil)
	if ok {
		t.Error("expected ok=false for unknown type")
	}
}

func TestBuildEffectLookup_NilContext(t *testing.T) {
	result := resolve.BuildEffectLookup(nil)
	if result != nil {
		t.Error("expected nil for nil context")
	}
}

func TestSynthWithOverlay_NoMatch(t *testing.T) {
	base := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	overlay := map[cfg.SymbolID]typ.Type{}

	synth := resolve.SynthWithOverlay(overlay, nil, base)
	result := synth(&ast.IdentExpr{Value: "x"}, 0)
	if result != typ.String {
		t.Error("expected typ.String from base")
	}
}

func TestSynthWithOverlay_WithMatch(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	retStmt := fn.Stmts[0].(*ast.ReturnStmt)
	ident := retStmt.Exprs[0].(*ast.IdentExpr)
	sym, _ := bindings.SymbolOf(ident)

	base := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	overlay := map[cfg.SymbolID]typ.Type{
		sym: typ.Integer,
	}

	synth := resolve.SynthWithOverlay(overlay, bindings, base)
	result := synth(ident, 0)
	if result != typ.Integer {
		t.Error("expected typ.Integer from overlay")
	}
}

func TestBuildAssignmentTypeResolver_NilInputs(t *testing.T) {
	result := resolve.BuildAssignmentTypeResolver(nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestBuildAssignmentTypeResolver_ZeroSymbol(t *testing.T) {
	inputs := &flow.Inputs{}
	resolver := resolve.BuildAssignmentTypeResolver(inputs)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	result := resolver(0)
	if result != nil {
		t.Error("expected nil for zero symbol")
	}
}

func TestIteratorSourceInfo_Kind(t *testing.T) {
	info := &resolve.IteratorSourceInfo{
		Kind: flow.IterateIndexed,
	}
	if info.Kind != flow.IterateIndexed {
		t.Error("expected IterateIndexed")
	}
}

func TestExtractIteratorSource_EmptyIterExprs(t *testing.T) {
	result := resolve.ExtractIteratorSource(nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil iter exprs")
	}

	result = resolve.ExtractIteratorSource([]ast.Expr{}, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for empty iter exprs")
	}
}

func TestExtractIteratorSource_NonCallExpr(t *testing.T) {
	exprs := []ast.Expr{&ast.IdentExpr{Value: "x"}}
	result := resolve.ExtractIteratorSource(exprs, 0, nil, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for non-call expr")
	}
}

func TestCalleeType_NilInfo(t *testing.T) {
	result := resolve.CalleeType(nil, 0, nil, nil, nil)
	if result != nil {
		t.Error("expected nil for nil info")
	}
}
