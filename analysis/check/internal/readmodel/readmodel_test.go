package readmodel

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
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

func TestGradualTopWitnessIsNotAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.GradualTop())
	reader := New(result)

	if reader.ValueAdmissible(value, typ.String) {
		t.Fatalf("ValueAdmissible accepted gradual-top scalar witness")
	}
	if reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible accepted gradual-top scalar witness")
	}
}

func TestAnyClaimWitnessIsNotAdmissibilityProofWithoutRuntimeValidation(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted any-origin scalar witness")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted any-origin scalar witness")
	}
}

func TestGradualTopRuntimeProofIsAdmissible(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.GradualTop())
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)

	if !reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible rejected gradual-top runtime proof")
	}
	if !reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible rejected gradual-top runtime proof")
	}
}

func TestExplicitTopWitnessWithoutTypeClaimIsNotStructuralAdmissibilityProof(t *testing.T) {
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
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted unclaimed explicit-top structural witness")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted unclaimed explicit-top structural witness")
	}
}

func TestExplicitTopTypeClaimWitnessIsNotStructuralAdmissibilityProof(t *testing.T) {
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

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted explicit-top structural TypeClaim")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top structural TypeClaim")
	}
}

func TestExplicitTopTypeClaimWithAnyOriginIsNotStructuralAdmissibilityProof(t *testing.T) {
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
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.AnyClaim))
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted explicit-top structural TypeClaim with any origin")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top structural TypeClaim with any origin")
	}
}

func TestExplicitTopRuntimeProofIsAdmissibleForStructuralContract(t *testing.T) {
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
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	reader := New(result)

	if !reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top structural runtime proof")
	}
}

func TestExplicitTopScalarRuntimeKindIsNotAdmissibleProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(reg, presentValue(reg), evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	reader := New(result)

	if reader.ValueAdmissible(value, typ.String) {
		t.Fatalf("ValueAdmissible accepted explicit-top scalar runtime-kind fact")
	}
	if reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top scalar runtime-kind fact")
	}
}

func TestRuntimeTableKindDoesNotProveStringKeyMapShape(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)
	want := typetable.NewMap(typ.String, typ.Any)

	if reader.ValueAdmissible(value, want) {
		t.Fatalf("ValueAdmissible accepted runtime table kind as string-key map proof")
	}
	if reader.ValueProofAdmissible(value, want) {
		t.Fatalf("ValueProofAdmissible accepted runtime table kind as string-key map proof")
	}
}

func TestRuntimeTableKindDoesNotProveArrayShape(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)
	want := typ.NewArray(typ.String)

	if reader.ValueAdmissible(value, want) {
		t.Fatalf("ValueAdmissible accepted runtime table kind as array proof")
	}
	if reader.ValueProofAdmissible(value, want) {
		t.Fatalf("ValueProofAdmissible accepted runtime table kind as array proof")
	}
}

func TestExplicitTopScalarWitnessWithoutTypeClaimIsNotAdmissibleProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.MaterializeOptional(typ.String)), typ.MaterializeOptional(typ.String))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	reader := New(result)

	if reader.ValueProofAdmissible(value, typ.MaterializeOptional(typ.String)) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top scalar witness without runtime validation")
	}
}

func TestExplicitTopExactLiteralWitnessIsAdmissibleAsScalarProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("ready")), typ.LiteralString("ready"))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	reader := New(result)

	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top exact literal witness for string")
	}
}

func TestGradualTopExactLiteralWitnessIsAdmissibleAsScalarProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("ready")), typ.LiteralString("ready"))
	value = product.Set(reg, value, evidence.Key, evidence.GradualTop())
	reader := New(result)

	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected gradual-top exact literal witness for string")
	}
}

func TestExplicitTopScalarTypeClaimIsAdmissibleAsUserAssertion(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Type())
	reader := New(result)

	if !reader.ValueHasUntrustedTopOrigin(value) {
		t.Fatalf("ValueHasUntrustedTopOrigin rejected explicit-top scalar TypeClaim")
	}
	if reader.ValueAdmissible(value, typ.String) {
		t.Fatalf("ValueAdmissible accepted explicit-top scalar TypeClaim as trusted proof")
	}
	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top scalar TypeClaim")
	}
}

func TestExplicitTopRuntimeProofIsAdmissibleForScalarContract(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)

	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top scalar runtime proof")
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

func TestForEachCallCarriesSyntaxFreeCallAndCalleeSpans(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function consume(value: string): () end
consume("ok")
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []CallSite
	if !New(result).ForEachCall(func(call CallSite) bool {
		got = append(got, call)
		return true
	}) {
		t.Fatal("ForEachCall returned false, want one call")
	}
	if len(got) != 1 {
		t.Fatalf("calls = %d, want one", len(got))
	}
	call := got[0]
	if call.CallSpan.StartLine == 0 || call.CalleeSpan.StartLine == 0 {
		t.Fatalf("call spans = call:%#v callee:%#v, want syntax-free source ranges", call.CallSpan, call.CalleeSpan)
	}
	if call.CallSpan.StartLine > call.CalleeSpan.StartLine ||
		(call.CallSpan.StartLine == call.CalleeSpan.StartLine && call.CallSpan.StartCol > call.CalleeSpan.StartCol) ||
		call.CallSpan.EndLine < call.CalleeSpan.EndLine ||
		(call.CallSpan.EndLine == call.CalleeSpan.EndLine && call.CallSpan.EndCol < call.CalleeSpan.EndCol) {
		t.Fatalf("call span %#v does not cover callee span %#v", call.CallSpan, call.CalleeSpan)
	}
}

func TestForEachCallReportsArityFromCanonicalContract(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("test")
	m.DefineFunctionSignature("add", signature.Function{
		Type: typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build(),
	})
	result, err := body.CheckFunction(parseFunction(t, `
function f()
	add(1)
	add(1, 2, 3)
end
`), body.Config{
		Registry: reg,
		Globals:  []string{"add"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	var arities []CallArityReport
	New(result).ForEachCall(func(call CallSite) bool {
		if call.Arity.Kind != readapi.CallArityReportNone {
			arities = append(arities, call.Arity)
		}
		return true
	})
	if len(arities) != 2 {
		t.Fatalf("arity reports = %d, want two: %#v", len(arities), arities)
	}
	if arities[0].Kind != readapi.CallArityReportTooFew || arities[0].ExpectedCount != 2 || arities[0].ActualCount != 1 {
		t.Fatalf("too-few report = %#v, want expected 2 actual 1", arities[0])
	}
	if arities[0].CallableName != "add" || arities[0].CallSpan.StartLine == 0 {
		t.Fatalf("too-few report anchors = %#v, want callable name and call source span", arities[0])
	}
	if arities[1].Kind != readapi.CallArityReportTooMany || arities[1].ExpectedCount != 2 || arities[1].ActualCount != 3 {
		t.Fatalf("too-many report = %#v, want expected 2 actual 3", arities[1])
	}
	if arities[1].ExtraSpan.StartLine == 0 {
		t.Fatalf("too-many extra span = %#v, want syntax-free extra-argument span", arities[1].ExtraSpan)
	}
}

func TestForEachCallReportsDirectCalleeCallableMismatches(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local x: number = 42
x()
local maybe: (() -> string)? = nil
maybe()
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var reports []CallCalleeReport
	New(result).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 2 {
		t.Fatalf("callee reports = %d, want two: %#v", len(reports), reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportNotCallable || reports[0].CallableName != "x" || !typ.TypeEquals(reports[0].Type, typ.Number) {
		t.Fatalf("first callee report = %#v, want x not-callable number", reports[0])
	}
	if reports[0].Span.StartLine == 0 {
		t.Fatalf("first callee span = %#v, want source anchor", reports[0].Span)
	}
	if reports[1].Kind != readapi.CallCalleeReportMayBeNil || reports[1].CallableName != "maybe" || !reports[1].Callable {
		t.Fatalf("second callee report = %#v, want maybe possibly-nil callable", reports[1])
	}
}

func TestSourceTypePrefersLocalAssignmentLoweredCallSource(t *testing.T) {
	reg := standard.Registry()
	exportType := typetable.NewRecord().Field("run", typ.Func().Build()).Build()
	m := manifest.New("pkg")
	m.SetExport(exportType)
	m.DefineFunctionSignature("pkg.run", signature.Function{Type: typ.Func().Build()})
	stmts := parseChunk(t, `
local pkg: {run: () -> ()}? = require("pkg")
`)
	result, err := body.CheckChunk(stmts, body.Config{
		Registry: reg,
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
			Manifests:     []*manifest.Manifest{m},
		},
		ModuleExports: importlookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	assign := stmts[0].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	if fact.Source.Kind != sourceprovenance.SourceCall {
		t.Fatalf("local source = %#v, want call source", fact.Source)
	}
	got, ok := New(result).SourceType(point, fact.Source)
	if !ok || !typ.TypeEquals(got, exportType) {
		t.Fatalf("SourceType = %v/%v, want manifest export %v", got, ok, exportType)
	}
}

func TestUntrustedRuntimeTableWitnessIsNotProvenMismatch(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, presentValue(reg), typetable.BuiltinTopMarker())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)

	if reader.ValueWitnessProvenMismatch(value, typ.NewArray(typ.String)) {
		t.Fatalf("ValueWitnessProvenMismatch treated untrusted runtime table as proven array mismatch")
	}
	if reader.ValueProofAdmissible(value, typ.NewArray(typ.String)) {
		t.Fatalf("ValueProofAdmissible accepted untrusted runtime table as array shape")
	}
}

func TestSourceValueReadsGuardedPathExpressionFromSolvedPath(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local block: any = {}
if type(block.items) == "table" then
    local labels: {string} = block.items
end
`)
	result, err := body.CheckChunk(stmts, body.Config{
		Registry: reg,
		Globals:  []string{"type"},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	point, fact := requireLocalAssignmentByName(t, result, "labels")
	reader := New(result)

	value, ok := reader.SourceValue(point, fact.Source)
	if !ok {
		t.Fatalf("SourceValue returned false for guarded path expression")
	}
	got, ok := reader.ValueType(value)
	if !ok || !typetable.IsBuiltinTopMarker(got) {
		t.Fatalf("SourceValue type = %v/%v, want table runtime-kind projection", got, ok)
	}
	if reader.ValueProofAdmissible(value, typ.NewArray(typ.String)) {
		t.Fatalf("SourceValue proof accepted table runtime kind as array shape")
	}
}

func TestSourceValueReadsGuardedAnyParamPathAsUntrusted(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function rows(block: any): {string}?
    if type(block.items) == "table" then
        local labels: {string} = block.items
        return labels
    end
    return nil
end
`)
	result, err := body.CheckFunction(fn, body.Config{
		Registry:   reg,
		Globals:    []string{"type"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	assign := ifStmt.Then[0].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	reader := New(result)

	value, ok := reader.SourceValue(point, fact.Source)
	if !ok {
		t.Fatalf("SourceValue returned false for guarded any-param path")
	}
	got, ok := reader.ValueType(value)
	if !ok || !typetable.IsBuiltinTopMarker(got) {
		t.Fatalf("SourceValue type = %v/%v, want table runtime-kind projection", got, ok)
	}
	if !reader.ValueHasUntrustedTopOrigin(value) {
		t.Fatalf("SourceValue did not preserve untrusted any origin for guarded any-param path")
	}
	if reader.ValueProofAdmissible(value, typ.NewArray(typ.String)) {
		t.Fatalf("SourceValue proof accepted table runtime kind as array shape")
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

func requireLocalAssignmentByName(t *testing.T, result *body.Result, name string) (cfg.Point, semantics.LocalAssignmentFact) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Name == name {
			return point, fact
		}
	}
	t.Fatalf("missing local assignment named %q", name)
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

func parseFunction(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := parseChunk(t, src)
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want one function definition", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	return def.Func
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func TestRuntimeKindReducedTypeNarrowsByRuntimeKindExclusion(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	reader := New(result)
	declared := typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})

	// A runtime kind that excludes Number narrows number | string to string.
	excluded := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	excluded = product.Set(reg, excluded, runtimekind.Key, runtimekind.Top().Without(runtimekind.Number))
	got, ok := reader.RuntimeKindReducedType(excluded, declared)
	if !ok {
		t.Fatalf("RuntimeKindReducedType returned false, want string")
	}
	assertSameType(t, got, typ.String)

	// A top runtime kind imposes no constraint, so nothing is narrowed.
	top := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	if narrowed, ok := reader.RuntimeKindReducedType(top, declared); ok {
		t.Fatalf("RuntimeKindReducedType narrowed under top runtime kind: got %v", narrowed)
	}

	// A non-union declared type cannot have a single kind subtracted.
	if narrowed, ok := reader.RuntimeKindReducedType(excluded, typ.String); ok {
		t.Fatalf("RuntimeKindReducedType narrowed a non-union type: got %v", narrowed)
	}

	tableUnion := typ.MaterializeUnion([]typ.Type{typ.String, typetable.BuiltinTopMarker()})
	withoutTable := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	withoutTable = product.Set(reg, withoutTable, runtimekind.Key, runtimekind.Top().Without(runtimekind.Table))
	got, ok = reader.RuntimeKindReducedType(withoutTable, tableUnion)
	if !ok {
		t.Fatalf("RuntimeKindReducedType returned false for string | table, want string")
	}
	assertSameType(t, got, typ.String)
}
