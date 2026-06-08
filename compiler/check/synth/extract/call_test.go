package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallQuery_NilSynthesizer(t *testing.T) {
	q := CallQuery{s: nil}

	if _, ok := q.Method(nil, typ.String, "foo"); ok {
		t.Fatal("expected false for nil synthesizer")
	}
	if _, ok := q.Field(nil, typ.String, "foo"); ok {
		t.Fatal("expected false for nil synthesizer")
	}
	if _, ok := q.Index(nil, typ.String, typ.Integer); ok {
		t.Fatal("expected false for nil synthesizer")
	}
	if q.BinaryOp(nil, typ.Integer, "+", typ.Integer) != typ.Unknown {
		t.Fatal("expected unknown for nil synthesizer")
	}
	if q.UnaryOp(nil, "-", typ.Integer) != typ.Unknown {
		t.Fatal("expected unknown for nil synthesizer")
	}
}

func TestStablePrototypeFieldKeyKeepsIndexStructural(t *testing.T) {
	sym := compcfg.SymbolID(7)

	field, ok := stablePrototypeFieldKey(compcfg.AssignTarget{
		Kind:       compcfg.TargetField,
		BaseSymbol: sym,
		FieldPath:  []string{"method"},
	}, sym)
	if !ok || field != (constraint.Segment{Kind: constraint.SegmentField, Name: "method"}) {
		t.Fatalf("field key = %#v,%v; want SegmentField(method),true", field, ok)
	}

	stringIndex, ok := stablePrototypeFieldKey(compcfg.AssignTarget{
		Kind:       compcfg.TargetIndex,
		BaseSymbol: sym,
		Key:        &ast.StringExpr{Value: "method"},
	}, sym)
	if !ok || stringIndex != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "method"}) {
		t.Fatalf("string index key = %#v,%v; want SegmentIndexString(method),true", stringIndex, ok)
	}

	intIndex, ok := stablePrototypeFieldKey(compcfg.AssignTarget{
		Kind:       compcfg.TargetIndex,
		BaseSymbol: sym,
		Key:        &ast.NumberExpr{Value: "1"},
	}, sym)
	if !ok || intIndex != (constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}) {
		t.Fatalf("int index key = %#v,%v; want SegmentIndexInt(1),true", intIndex, ok)
	}
}

func TestAddStablePrototypeFieldPreservesStaticMembers(t *testing.T) {
	builder := typ.NewRecord()
	addStablePrototypeField(builder, constraint.Segment{Kind: constraint.SegmentField, Name: "dot"}, typ.String)
	addStablePrototypeField(builder, constraint.Segment{Kind: constraint.SegmentIndexString, Name: "dot"}, typ.Number)
	addStablePrototypeField(builder, constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}, typ.Boolean)

	rec := builder.Build()
	if rec.GetField("dot") == nil {
		t.Fatalf("missing dot field in %v", rec)
	}
	if rec.GetStaticStringIndex("dot") == nil {
		t.Fatalf("missing static string member in %v", rec)
	}
	if rec.GetStaticIntIndex(1) == nil {
		t.Fatalf("missing static int member in %v", rec)
	}
}

func TestCallQuery_NilTypes(t *testing.T) {
	deps := &Deps{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  nil,
		Scopes: make(api.ScopeMap),
	}
	s := NewSynthesizer(deps, api.SynthModeResolve)
	q := CallQuery{s: s}

	if _, ok := q.Method(deps.Ctx, typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
	if _, ok := q.Field(deps.Ctx, typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
	if _, ok := q.Index(deps.Ctx, typ.String, typ.Integer); ok {
		t.Fatal("expected false for nil types")
	}
}

func TestCallQuery_WithMock(t *testing.T) {
	deps := &Deps{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
	}
	s := NewSynthesizer(deps, api.SynthModeResolve)
	q := CallQuery{s: s}

	rec := typ.NewRecord().
		Field("name", typ.String).
		Build()

	ft, ok := q.Field(deps.Ctx, rec, "name")
	if !ok {
		t.Fatal("expected field lookup to succeed")
	}
	if ft != typ.String {
		t.Fatalf("got %v, want string", ft)
	}
}

func TestGetCallQuery(t *testing.T) {
	s := newTestSynthesizer()
	q := s.GetCallQuery()
	if q == nil {
		t.Fatal("expected non-nil call query")
	}
}

func TestSynthArgs(t *testing.T) {
	exprs := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.StringExpr{Value: "hello"},
	}
	recurse := func(ex ast.Expr) typ.Type {
		switch e := ex.(type) {
		case *ast.NumberExpr:
			return ops.ParseNumber(e.Value)
		case *ast.StringExpr:
			return typ.LiteralString(e.Value)
		}
		return typ.Unknown
	}

	args := synthArgs(exprs, recurse)
	if len(args) != 2 {
		t.Fatalf("got %d args, want 2", len(args))
	}
}

func TestApplyPostCallTransforms_MethodEffectsUseRuntimeReceiverSlot(t *testing.T) {
	s := newTestSynthesizer()
	receiver := typ.NewRecord().
		Field("all", typ.Func().Param("self", typ.Self).Returns(typ.Nil).Build()).
		Build()
	callee := typ.Func().
		Param("self", typ.Self).
		Param("kind", typ.String).
		Returns(typ.Self).
		Effects(effect.Row{Labels: []effect.Label{
			effect.FlowInto{ParamIndex: 0, ReturnIndex: 0},
		}}).
		Build()

	got := s.applyPostCallTransforms(
		callee,
		[]typ.Type{typ.LiteralString("conversation_summary")},
		[]typ.Type{receiver},
		receiver,
		true,
		false,
	)
	if len(got) != 1 || !typ.TypeEquals(got[0], receiver) {
		t.Fatalf("method effect transform = %v, want receiver %v", got, receiver)
	}
}

func TestResolveMethodCallee_Nil(t *testing.T) {
	s := newTestSynthesizer()
	result := s.resolveMethodCallee(nil, "foo")
	if result != nil {
		t.Fatal("expected nil for nil receiver")
	}
}

func TestMethod_NilTypes(t *testing.T) {
	deps := &Deps{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  nil,
		Scopes: make(api.ScopeMap),
	}
	s := NewSynthesizer(deps, api.SynthModeResolve)

	if _, ok := s.Method(typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
}

func TestField_NilTypes(t *testing.T) {
	deps := &Deps{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  nil,
		Scopes: make(api.ScopeMap),
	}
	s := NewSynthesizer(deps, api.SynthModeResolve)

	if _, ok := s.Field(typ.String, "foo"); ok {
		t.Fatal("expected false for nil types")
	}
}

func TestField_WithRecord(t *testing.T) {
	s := newTestSynthesizer()
	rec := typ.NewRecord().
		Field("name", typ.String).
		Build()

	ft, ok := s.Field(rec, "name")
	if !ok {
		t.Fatal("expected field lookup to succeed")
	}
	if ft != typ.String {
		t.Fatalf("got %v, want string", ft)
	}
}
