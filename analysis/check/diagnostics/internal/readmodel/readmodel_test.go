package readmodel

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestValueTypeWitnessPresentProjectsConcreteType(t *testing.T) {
	reg := standard.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = typevalue.WithWitness(reg, value, typeexpr.Optional(typ.String))

	got, ok := New(&body.Result{}).ValueType(value)
	if ok {
		t.Fatalf("ValueType with nil registry result returned %v, want false", got)
	}

	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	got, ok = New(result).ValueType(value)
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestValueTypeAbsentProjectsNil(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	got, ok := New(result).ValueType(product.Absent(reg))
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.Nil)
}

func TestSourceValueReadsAnyAssertionClaimFromLocalAssignmentSource(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local request = ({id = "r1", retries = 2} :: any)
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	assign := stmts[0].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	reader := New(result)

	value, ok := reader.SourceValue(point, fact.Source)
	if !ok {
		t.Fatalf("SourceValue returned false")
	}
	if !reader.ValueHasUntrustedTopOrigin(value) {
		t.Fatalf("SourceValue did not preserve assertion.Any: %v", value)
	}
	got, ok := reader.SourceType(point, fact.Source)
	if !ok || !typ.IsAny(got) {
		t.Fatalf("SourceType = %v/%v, want any", got, ok)
	}
}

func TestValueTypeWithPresenceAddsNilForMaybeWitness(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	value = typevalue.WithWitness(reg, value, typ.String)

	got, ok := New(result).ValueTypeWithPresence(value)
	if !ok {
		t.Fatalf("ValueTypeWithPresence returned false")
	}
	assertSameType(t, got, typeexpr.Optional(typ.String))
}

func TestValueTypeMaybeWitnessStaysConcrete(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	value = typevalue.WithWitness(reg, value, typ.String)

	got, ok := New(result).ValueType(value)
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestExplicitTopWitnessIsNotStructuralAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted explicit-top structural witness")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top structural witness")
	}
}

func TestExplicitTopTypeClaimWitnessIsStructuralAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Type())
	reader := New(result)

	if !reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible rejected explicit-top structural TypeClaim")
	}
	if !reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top structural TypeClaim")
	}
}

func TestExplicitTopScalarRuntimeKindRemainsAdmissibleProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(reg, presentValue(reg), evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	reader := New(result)

	if !reader.ValueAdmissible(value, typ.String) {
		t.Fatalf("ValueAdmissible rejected explicit-top scalar runtime-kind proof")
	}
	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top scalar runtime-kind proof")
	}
}

func TestVariantOriginTypeProjectsStructuralUnion(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	okCase := typetable.NewRecord().
		Field("kind", typ.LiteralString("ok")).
		Field("value", typ.Number).
		Build()
	errCase := typetable.NewRecord().
		Field("kind", typ.LiteralString("err")).
		Field("error", typ.String).
		Build()
	union := typeexpr.Union(okCase, errCase)
	value := typevalue.FromType(reg, union)

	got, ok := New(result).VariantOriginType(value)
	if !ok {
		t.Fatalf("VariantOriginType returned false")
	}
	assertSameType(t, got, union)
}

func TestRuntimeKindProjection(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	reader := New(result)
	for _, tc := range []struct {
		name string
		kind runtimekind.Tag
		want typ.Type
	}{
		{name: "nil", kind: runtimekind.Nil, want: typ.Nil},
		{name: "boolean", kind: runtimekind.Boolean, want: typ.Boolean},
		{name: "number", kind: runtimekind.Number, want: typ.Number},
		{name: "string", kind: runtimekind.String, want: typ.String},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(tc.kind))
			got, ok := reader.ValueType(value)
			if !ok {
				t.Fatalf("ValueType returned false")
			}
			assertSameType(t, got, tc.want)
		})
	}

	for _, tc := range []struct {
		name string
		kind runtimekind.Tag
		want kind.Kind
	}{
		{name: "table", kind: runtimekind.Table, want: kind.Map},
		{name: "function", kind: runtimekind.Function, want: kind.Function},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(tc.kind))
			got, ok := reader.RefineDeclaredType(typ.Unknown, value)
			if !ok {
				t.Fatalf("RefineDeclaredType returned false")
			}
			if got.Kind() != tc.want {
				t.Fatalf("RefineDeclaredType kind = %s, want %s (%v)", got.Kind(), tc.want, got)
			}
		})
	}
}

func TestRefineDeclaredTypeOptionalByPresentEvidence(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)

	got, ok := New(result).RefineDeclaredType(typeexpr.Optional(typ.String), value)
	if !ok {
		t.Fatalf("RefineDeclaredType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestValueTypeUsesOriginTypeWhenWitnessFamilyDoesNotReplay(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.String).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.String).
		Build()
	union := typeexpr.Union(dog, cat)
	dogFamily, dogCases, ok := variant.OriginOfType(dog)
	if !ok {
		t.Fatal("missing dog origin")
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(dogFamily, dogCases))

	got, ok := New(result).ValueType(value)
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, dog)
}

func TestSourceTypeReadsCallSourceThroughBoundary(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local v = Point(data)
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	assign := stmts[2].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	if fact.Source.Kind != sourceprovenance.SourceCall || !fact.Source.HasCallPoint {
		t.Fatalf("local source = %#v, want call source with call point", fact.Source)
	}
	if !fact.Source.Final || !fact.Source.Expanded || fact.Source.Adjusted || fact.Source.OpenTail {
		t.Fatalf("call source shape = final:%v expanded:%v adjusted:%v openTail:%v, want expanded final call source",
			fact.Source.Final, fact.Source.Expanded, fact.Source.Adjusted, fact.Source.OpenTail)
	}

	got, ok := New(result).SourceType(point, fact.Source)
	if !ok {
		t.Fatalf("SourceType returned false")
	}
	if got.Kind() != kind.Record {
		t.Fatalf("SourceType kind = %s, want record (%v)", got.Kind(), got)
	}
}

func TestValueSourceFromASTSourcePreservesVarargBoundarySource(t *testing.T) {
	shape, ok := sourceprovenance.NewSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("vararg shape rejected")
	}
	source, ok := sourceprovenance.NewVarargSource(&ast.Comma3Expr{}, 0, 2, 1, shape)
	if !ok {
		t.Fatal("AST vararg source rejected")
	}

	got, ok := valueSourceFromASTSource(source)
	if !ok {
		t.Fatal("valueSourceFromASTSource returned false")
	}
	if !got.Valid() {
		t.Fatalf("lowered vararg source is invalid: %#v", got)
	}
	if got.Kind != factflow.ValueSourceVararg || got.HasExpr || got.ExprRef != 0 || got.TargetIndex != 2 || got.ResultIndex != 1 || !got.OpenTail {
		t.Fatalf("lowered vararg source = %#v, want boundary vararg without expr ref", got)
	}
}

func TestValueSourceFromASTSourceRejectsMalformedSources(t *testing.T) {
	nilSource := sourceprovenance.NewNilSource(0)
	nilSource.Final = true
	if nilSource.Valid() {
		t.Fatalf("malformed nil source unexpectedly valid: %#v", nilSource)
	}
	if got, ok := valueSourceFromASTSource(nilSource); ok {
		t.Fatalf("malformed nil source lowered to %#v, want false", got)
	}

	shape, ok := sourceprovenance.NewSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("call shape rejected")
	}
	callSource, ok := sourceprovenance.NewCallSource(&ast.FuncCallExpr{}, 0, 0, 0, cfg.Point(42), shape)
	if !ok {
		t.Fatal("call source rejected")
	}
	callSource.HasCallPoint = false
	if callSource.Valid() {
		t.Fatalf("malformed call source unexpectedly valid: %#v", callSource)
	}
	if got, ok := valueSourceFromASTSource(callSource); ok {
		t.Fatalf("malformed call source lowered to %#v, want false", got)
	}
}

func assertSameType(t *testing.T, got, want typ.Type) {
	t.Helper()
	if !typ.SameNodeOrAcyclicEqual(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}

func requireLocalAssignment(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) (cfg.Point, semantics.LocalAssignmentFact) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point, fact
		}
	}
	t.Fatalf("missing local assignment for stmt %p index %d", stmt, index)
	return 0, semantics.LocalAssignmentFact{}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(strings.TrimSpace(src), "readmodel_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}
