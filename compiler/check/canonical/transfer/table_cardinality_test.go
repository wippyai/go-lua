package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTableConstructorSeedsContainerCardinalityForStaticMapLiteral(t *testing.T) {
	const sym = cfg.SymbolID(8101)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top(),
	}

	tbl := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "a"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NumberExpr{Value: "1"}},
		{Key: &ast.StringExpr{Value: "b"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NumberExpr{Value: "2"}},
	}}
	tr.seedArrayLiteralLength(&out, flow.SymbolValueKey(sym), tbl, tr.tableLiteralCardinalityLowerBound(&out, tbl))

	key := flow.SymbolPathKey(sym, nil)
	if !out.Rel.HasContainerLowerBound(sym, key, 2) {
		t.Fatalf("relations = %#v, want static constructor cardinality >= 2", out.Rel)
	}
	if lower, _, ok := out.Num.LenBoundsFor(key); ok {
		t.Fatalf("string-keyed map seeded numeric # length lower %d", lower)
	}
}

func TestTableConstructorCardinalityUsesFinalStaticWrite(t *testing.T) {
	const sym = cfg.SymbolID(8102)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top(),
	}

	tbl := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "a"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NumberExpr{Value: "1"}},
		{Key: &ast.StringExpr{Value: "a"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NilExpr{}},
		{Key: &ast.StringExpr{Value: "b"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NumberExpr{Value: "2"}},
	}}
	tr.seedArrayLiteralLength(&out, flow.SymbolValueKey(sym), tbl, tr.tableLiteralCardinalityLowerBound(&out, tbl))

	key := flow.SymbolPathKey(sym, nil)
	if !out.Rel.HasContainerLowerBound(sym, key, 1) {
		t.Fatalf("relations = %#v, want final-write cardinality >= 1", out.Rel)
	}
	if out.Rel.HasContainerLowerBound(sym, key, 2) {
		t.Fatalf("relations = %#v, counted overwritten nil field", out.Rel)
	}
}

func TestTableConstructorCardinalityUsesRuntimeStringKeySemantics(t *testing.T) {
	const sym = cfg.SymbolID(8105)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top(),
	}

	tbl := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "a"}, KeySyntax: ast.AttrKeyDot, Value: &ast.NumberExpr{Value: "1"}},
		{Key: &ast.StringExpr{Value: "a"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NilExpr{}},
	}}
	tr.seedArrayLiteralLength(&out, flow.SymbolValueKey(sym), tbl, tr.tableLiteralCardinalityLowerBound(&out, tbl))

	key := flow.SymbolPathKey(sym, nil)
	if out.Rel.HasContainerLowerBound(sym, key, 1) {
		t.Fatalf("relations = %#v, treated dot and bracket string keys as distinct runtime entries", out.Rel)
	}
}

func TestTableConstructorDynamicKeyDoesNotSeedCardinality(t *testing.T) {
	const sym = cfg.SymbolID(8103)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top().WithContainerLowerBound(sym, flow.SymbolPathKey(sym, nil), 4),
	}

	tbl := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "a"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NumberExpr{Value: "1"}},
		{Key: &ast.IdentExpr{Value: "k"}, KeySyntax: ast.AttrKeyIndex, Value: &ast.NumberExpr{Value: "2"}},
	}}
	tr.seedArrayLiteralLength(&out, flow.SymbolValueKey(sym), tbl, tr.tableLiteralCardinalityLowerBound(&out, tbl))

	key := flow.SymbolPathKey(sym, nil)
	if out.Rel.HasContainerLowerBound(sym, key, 1) {
		t.Fatalf("relations = %#v, dynamic-key constructor kept/seeded cardinality lower bound", out.Rel)
	}
}

func TestTableConstructorCardinalityReadsPreWriteState(t *testing.T) {
	const sym = cfg.SymbolID(8104)
	x := &ast.IdentExpr{Value: "x"}
	in := input.BuildFromFunction(&ast.FunctionExpr{}, nil, nil)
	if in.Graph == nil {
		t.Fatal("test graph did not build")
	}
	in.Graph.Bindings().Bind(x, sym)
	in.Graph.Bindings().SetName(sym, "x")
	tr := New(in, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Nil),
		},
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top(),
	}

	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Name: "x", Symbol: sym}},
		Sources: []ast.Expr{&ast.TableExpr{Fields: []*ast.Field{{
			Key:       &ast.StringExpr{Value: "a"},
			KeySyntax: ast.AttrKeyIndex,
			Value:     x,
		}}}},
	}, nil)

	key := flow.SymbolPathKey(sym, nil)
	if out.Rel.HasContainerLowerBound(sym, key, 1) {
		t.Fatalf("relations = %#v, counted field value from post-write state", out.Rel)
	}
}
