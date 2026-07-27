package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestObjectLiteralPlanCompositionMatchesConcreteViewAdapter(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	kindSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8101, HasExpr: true}
	payloadSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8102, HasExpr: true}
	start := typetable.NewRecord().
		Field("kind", typ.LiteralString("start")).
		Field("payload", typ.String).
		Build()
	stop := typetable.NewRecord().
		Field("kind", typ.LiteralString("stop")).
		Field("payload", typ.String).
		Build()
	expected := typeexpr.Union(start, stop)
	literalID := testTableIdentity(8100, 8103)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("kind"), kindSource, factflow.SourceSpan{}, ""),
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("payload"), payloadSource, factflow.SourceSpan{}, ""),
	}).WithExpected(typeValues.FromTypeWithWitness(reg, expected)).WithIdentity(literalID)
	valuesBySource := map[factflow.ValueSource]product.Value{
		kindSource:    typeValues.FromTypeWithWitness(reg, typ.LiteralString("stop")),
		payloadSource: typeValues.FromTypeWithWitness(reg, typ.LiteralString("body")),
	}

	plan, ok := CompileObjectLiteralPlanCached(reg, typeValues, lit.View())
	if !ok || !plan.Valid() {
		t.Fatal("CompileObjectLiteralPlan rejected valid literal")
	}
	raw := make([]ObjectLiteralPlanValue, plan.ValueSourceCount())
	observations := make([]ObjectLiteralSourceObservation, plan.ValueSourceCount())
	for i := range raw {
		source, found := plan.ValueSourceAt(i)
		if !found {
			t.Fatalf("ValueSourceAt(%d) returned false", i)
		}
		raw[i] = ObjectLiteralPlanValue{Value: valuesBySource[source], Available: true}
		observations[i], ok = ObserveObjectLiteralSourceCached(reg, typeValues, raw[i].Value, true)
		if !ok {
			t.Fatalf("ObserveObjectLiteralSourceCached(%d) returned false", i)
		}
	}

	fromRaw, ok := ComposeObjectLiteralPlanCached(reg, typeValues, plan, raw)
	if !ok {
		t.Fatal("ComposeObjectLiteralPlanCached returned false")
	}
	fromObservations, ok := ComposeObjectLiteralPlanFromObservationsCached(reg, typeValues, plan, observations)
	if !ok {
		t.Fatal("ComposeObjectLiteralPlanFromObservationsCached returned false")
	}
	fromView, ok := ObjectLiteralValueFromViewCached(reg, typeValues, lit.View(), factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		value, found := valuesBySource[source]
		return value, found
	}))
	if !ok {
		t.Fatal("ObjectLiteralValueFromViewCached returned false")
	}
	if !product.Equal(reg, fromRaw, fromObservations) || !product.Equal(reg, fromRaw, fromView) {
		t.Fatalf("plan composition differs: raw=%v observations=%v view=%v", fromRaw, fromObservations, fromView)
	}
	gotType, typed := typeValues.TypeOf(reg, fromRaw)
	if !typed || !typ.TypeEquals(gotType, stop) {
		t.Fatalf("composed type = %v/%v, want selected stop arm %v", gotType, typed, stop)
	}
	if gotID, identified := product.Get(reg, fromRaw, identity.Key).ID(); !identified || gotID != literalID {
		t.Fatalf("composed identity = %v/%v, want %v", gotID, identified, literalID)
	}
	if gotEscape := product.Get(reg, fromRaw, escape.Key); !escape.Equal(gotEscape, escape.Fresh()) {
		t.Fatalf("composed escape = %v, want fresh", gotEscape)
	}
}

func TestObjectLiteralPlanUniquesDuplicateRawSourceAndResolvesItOnce(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	shared := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8201, HasExpr: true}
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("first"), shared, factflow.SourceSpan{}, ""),
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("second"), shared, factflow.SourceSpan{}, ""),
	})
	plan, ok := CompileObjectLiteralPlanCached(reg, typeValues, lit.View())
	if !ok {
		t.Fatal("CompileObjectLiteralPlan returned false")
	}
	if got := plan.ValueSourceCount(); got != 1 {
		t.Fatalf("unique source count = %d, want 1", got)
	}
	resolved := 0
	got, ok := ObjectLiteralTypeCached(reg, typeValues, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		resolved++
		if source != shared {
			t.Fatalf("resolved unexpected source %v", source)
		}
		return typeValues.FromTypeWithWitness(reg, typ.String), true
	}))
	if !ok {
		t.Fatal("ObjectLiteralTypeCached returned false")
	}
	want := typetable.NewRecord().Field("first", typ.String).Field("second", typ.String).Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object type = %v, want %v", got, want)
	}
	if resolved != 1 {
		t.Fatalf("duplicate source resolved %d times, want 1", resolved)
	}
}

func TestObjectLiteralSourceObservationIsExactConstructorQuotient(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	base := typeValues.FromTypeWithWitness(reg, typ.String)
	fresh := product.Set(reg, base, escape.Key, escape.Fresh())
	escaped := product.Set(reg, base, escape.Key, escape.Escaped())
	freshObservation, ok := ObserveObjectLiteralSourceCached(reg, typeValues, fresh, true)
	if !ok {
		t.Fatal("fresh observation returned false")
	}
	escapedObservation, ok := ObserveObjectLiteralSourceCached(reg, typeValues, escaped, true)
	if !ok {
		t.Fatal("escaped observation returned false")
	}
	if !freshObservation.Equal(escapedObservation) || freshObservation.Fingerprint() != escapedObservation.Fingerprint() {
		t.Fatal("escape-only difference survived the constructor observation quotient")
	}
	numberObservation, ok := ObserveObjectLiteralSourceCached(reg, typeValues, typeValues.FromTypeWithWitness(reg, typ.Number), true)
	if !ok {
		t.Fatal("number observation returned false")
	}
	if freshObservation.Equal(numberObservation) {
		t.Fatal("constructor-relevant type difference collapsed in observation quotient")
	}
	unavailable, ok := ObserveObjectLiteralSourceCached(reg, typeValues, product.Value{}, false)
	if !ok || unavailable.Available() {
		t.Fatalf("unavailable observation = %v/%v, want valid unavailable", unavailable, ok)
	}
	availableBottom, ok := ObserveObjectLiteralSourceCached(reg, typeValues, product.Bottom(reg), true)
	if !ok || !availableBottom.Available() {
		t.Fatalf("available bottom observation = %v/%v, want valid available", availableBottom, ok)
	}
	if unavailable.Equal(availableBottom) {
		t.Fatal("unavailable source collapsed with resolved Bottom")
	}
}

func TestObjectLiteralPlanCloneIdentityAndRegistryFence(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8301, HasExpr: true}
	expected := typetable.NewRecord().Field("value", typ.String).Build()
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("value"), source, factflow.SourceSpan{}, "").
			WithExpected(typeValues.FromTypeWithWitness(reg, typ.String)),
	}).WithExpected(typeValues.FromTypeWithWitness(reg, expected))
	plan, ok := CompileObjectLiteralPlanCached(reg, typeValues, lit.View())
	if !ok {
		t.Fatal("CompileObjectLiteralPlan returned false")
	}
	clone := plan.Clone()
	if !clone.Valid() || !plan.Equal(clone) || plan.Fingerprint() != clone.Fingerprint() {
		t.Fatal("clone changed plan identity")
	}
	if (ObjectLiteralPlan{}).Valid() || !(ObjectLiteralPlan{}).Equal(ObjectLiteralPlan{}) || (ObjectLiteralPlan{}).Fingerprint() != 0 {
		t.Fatal("zero plan validity/equality/fingerprint contract violated")
	}
	foreign, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatalf("foreign registry: %v", err)
	}
	if foreignPlan, compiled := CompileObjectLiteralPlanCached(foreign, typevalue.NewCache(), lit.View()); compiled || foreignPlan.Valid() {
		t.Fatal("plan compiler admitted registry-foreign expected contracts")
	}
	clone.entries[0].segments[0].Name = "mutated"
	clone.entries[0].path[0].Name = "mutated"
	clone.sources[0].ExprRef++
	if plan.entries[0].segments[0].Name != "value" || plan.entries[0].path[0].Name != "value" || plan.sources[0] != source {
		t.Fatal("Clone shared mutable plan storage")
	}
}

func TestObjectLiteralPlanExpectedContractsAffectIdentityAndAreDetached(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8401, HasExpr: true}
	stringRecord := typetable.NewRecord().Field("value", typ.String).Build()
	numberRecord := typetable.NewRecord().Field("value", typ.Number).Build()
	stringLiteral := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("value"), source, factflow.SourceSpan{}, "").
			WithExpected(typeValues.FromTypeWithWitness(reg, typ.String)),
	}).WithExpected(typeValues.FromTypeWithWitness(reg, stringRecord))
	numberLiteral := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("value"), source, factflow.SourceSpan{}, "").
			WithExpected(typeValues.FromTypeWithWitness(reg, typ.Number)),
	}).WithExpected(typeValues.FromTypeWithWitness(reg, numberRecord))
	stringPlan, ok := CompileObjectLiteralPlanCached(reg, typeValues, stringLiteral.View())
	if !ok {
		t.Fatal("string plan compilation returned false")
	}
	numberPlan, ok := CompileObjectLiteralPlanCached(reg, typeValues, numberLiteral.View())
	if !ok {
		t.Fatal("number plan compilation returned false")
	}
	if stringPlan.Equal(numberPlan) {
		t.Fatal("different expected contracts produced equal plans")
	}
	if stringPlan.Fingerprint() == numberPlan.Fingerprint() {
		t.Fatal("different expected contracts collided in the compact test corpus")
	}
	// The plan owns a canonical rematerialization, not the caller's mutable type
	// graph. Mutating the original test record cannot change plan composition.
	stringRecord.Fields[0].Type = typ.Number
	composed, ok := ComposeObjectLiteralPlanCached(reg, typeValues, stringPlan, []ObjectLiteralPlanValue{{
		Value: typeValues.FromTypeWithWitness(reg, typ.LiteralString("body")), Available: true,
	}})
	if !ok {
		t.Fatal("composition after caller mutation returned false")
	}
	composedType, ok := typeValues.TypeOf(reg, composed)
	want := typetable.NewRecord().Field("value", typ.String).Build()
	if !ok || !typ.TypeEquals(composedType, want) {
		t.Fatalf("caller mutation changed retained plan type: %v/%v, want %v", composedType, ok, want)
	}
}

func TestObjectLiteralPlanDetachesRecursiveExpectedContractWithoutRunCache(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8501, HasExpr: true}
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("value", typ.String).
			Field("next", typeexpr.Optional(self)).
			Build()
	})
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(path.NewPlaceholder(0).Field("value"), source, factflow.SourceSpan{}, "").
			WithExpected(typeValues.FromTypeWithWitness(reg, typ.String)),
	}).WithExpected(typeValues.FromTypeWithWitness(reg, node))

	plan, ok := CompileObjectLiteralPlanCached(reg, nil, lit.View())
	if !ok || !plan.Valid() {
		t.Fatal("recursive expected contract has no canonical constructor plan")
	}
	clone := plan.Clone()
	if !clone.Valid() || !plan.Equal(clone) || plan.Fingerprint() != clone.Fingerprint() {
		t.Fatal("recursive expected contract changed across plan ownership boundary")
	}
}
