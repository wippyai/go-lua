package refinement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCanBeFalseUsesTypeWitness(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().Field("kind", typ.String).Build()
	if CanBeFalse(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)) {
		t.Fatal("record value can be false")
	}
	if !CanBeFalse(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)) {
		t.Fatal("boolean value cannot be false")
	}
	if CanBeFalse(reg, typevalue.FromType(reg, typ.LiteralString("auto"))) {
		t.Fatal("exact string literal value can be false")
	}
	if !CanBeFalse(reg, product.Top()) {
		t.Fatal("unknown value cannot be false")
	}
}

func TestCanBeTruthyUsesLuaTruthiness(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().Field("kind", typ.String).Build()
	if !CanBeTruthy(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)) {
		t.Fatal("record value cannot be truthy")
	}
	if CanBeTruthy(reg, typevalue.Nil(reg)) {
		t.Fatal("nil value can be truthy")
	}
	if CanBeTruthy(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)) {
		t.Fatal("false value can be truthy")
	}
	if !CanBeTruthy(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typeexpr.Optional(typ.String)), typeexpr.Optional(typ.String))) {
		t.Fatal("optional string cannot be truthy")
	}
	if !CanBeTruthy(reg, product.Top()) {
		t.Fatal("unknown value cannot be truthy")
	}
}

func TestCanBeFalsyUsesLuaTruthiness(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().Field("kind", typ.String).Build()
	if CanBeFalsy(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)) {
		t.Fatal("record value can be falsy")
	}
	if !CanBeFalsy(reg, typevalue.Nil(reg)) {
		t.Fatal("nil value cannot be falsy")
	}
	if !CanBeFalsy(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)) {
		t.Fatal("false value cannot be falsy")
	}
	if !CanBeFalsy(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typeexpr.Optional(typ.String)), typeexpr.Optional(typ.String))) {
		t.Fatal("optional string cannot be falsy")
	}
	if !CanBeFalsy(reg, product.Top()) {
		t.Fatal("unknown value cannot be falsy")
	}
}

func TestLiteralTypeRequiresLiteralWitness(t *testing.T) {
	reg := standard.Registry()
	lit := typ.LiteralString("ready")
	got, ok := LiteralType(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit))
	if !ok || !typ.TypeEquals(got, lit) {
		t.Fatalf("LiteralType = %v/%v, want %v", got, ok, lit)
	}
	if got, ok := LiteralType(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)); ok {
		t.Fatalf("LiteralType(string) = %v, want !ok", got)
	}
}

func TestMergeDeclaredContractDoesNotMaskComputedVariantOrigin(t *testing.T) {
	reg := standard.Registry()
	msg := typetable.NewRecord().
		Field("kind", typ.LiteralString("msg")).
		Field("value", typ.String).
		Build()
	timer := typetable.NewRecord().
		Field("kind", typ.LiteralString("timer")).
		Field("value", typ.Number).
		Build()
	declaredType := typeexpr.Union(msg, timer)
	value := typevalue.FromType(reg, msg)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, declaredType), declaredType)

	got := MergeDeclaredContract(reg, value, declared)

	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, msg) {
		t.Fatalf("merged contract type = %v/%v, want computed variant %v", gotType, ok, msg)
	}
	declaredFamily, _, ok := variant.OriginOfType(declaredType)
	if !ok {
		t.Fatalf("declared type has no variant origin")
	}
	gotOrigin := product.Get(reg, got, variantorigin.Key)
	if gotOrigin.IsBottom() || gotOrigin.IsTop() || gotOrigin.Family() != declaredFamily || gotOrigin.CasesLen() != 1 {
		t.Fatalf("merged contract origin = %v, want one case in declared family %x", gotOrigin, declaredFamily)
	}
}

func TestMergeDeclaredContractAdoptsDeclaredEvidenceForUnknownValue(t *testing.T) {
	reg := standard.Registry()
	declaredType := typeexpr.Union(typ.Number, typ.String)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, declaredType), declaredType)

	got := MergeDeclaredContract(reg, product.Top(), declared)

	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, declaredType) {
		t.Fatalf("merged unknown contract type = %v/%v, want %v", gotType, ok, declaredType)
	}
}

func TestMergeDeclaredContractCarriesExplicitTopEvidenceOntoNilSource(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.FromType(reg, typ.Nil)
	declared := typevalue.FromType(reg, typ.Any)

	got := MergeDeclaredContract(reg, value, declared)

	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("merged evidence = %s, want explicit-top from declared any contract", gotEvidence)
	}
}

func TestMergeDeclaredContractClearsStaleAnyEvidenceAfterScalarRuntimeProof(t *testing.T) {
	reg := standard.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	got := MergeDeclaredContract(reg, value, declared)

	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("merged evidence = %s, want trusted top after scalar runtime proof", gotEvidence)
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("merged witness = %v/%v, want string", gotType, ok)
	}
}

func TestMergeDeclaredContractDerivesRuntimeKindFromDeclaredWitness(t *testing.T) {
	reg := standard.Registry()
	declaredType := typ.MaterializeOptional(typ.String)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	declared := typevalue.WithWitness(reg, product.Top(), declaredType)

	got := MergeDeclaredContract(reg, value, declared)

	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("merged evidence = %s, want trusted top after declared scalar witness covers runtime proof", gotEvidence)
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, declaredType) {
		t.Fatalf("merged witness = %v/%v, want %s", gotType, ok, declaredType)
	}
}

func TestDeclaredContractAlreadySatisfied(t *testing.T) {
	reg := standard.Registry()
	nodeType := typetable.NewRecord().Field("id", typ.String).Build()
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, nodeType), nodeType)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, nodeType), nodeType)

	if !DeclaredContractAlreadySatisfied(reg, value, declared) {
		t.Fatalf("identical value and declared contract should already be satisfied")
	}
}

func TestDeclaredContractAlreadySatisfiedRejectsMissingPresence(t *testing.T) {
	reg := standard.Registry()
	nodeType := typetable.NewRecord().Field("id", typ.String).Build()
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, nodeType), nodeType)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typeexpr.Optional(nodeType)), typeexpr.Optional(nodeType))

	if DeclaredContractAlreadySatisfied(reg, value, declared) {
		t.Fatalf("present value should not already satisfy optional declared presence")
	}
}

func TestDeclaredContractAlreadySatisfiedPreservingPresenceAcceptsPresenceOnlyMismatch(t *testing.T) {
	reg := standard.Registry()
	node := typ.NewRecursivePlaceholder("TreeNode")
	node.SetBody(typetable.NewRecord().
		Field("label", typ.String).
		Field("children", typ.NewArray(node)).
		OptField("parent", node).
		Build())
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, node), node)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typeexpr.Optional(node)), typeexpr.Optional(node))

	if !DeclaredContractAlreadySatisfiedPreservingPresence(reg, value, declared) {
		t.Fatalf("preserving-presence contract should accept optional declaration when non-presence facts are already satisfied")
	}
	if DeclaredContractAlreadySatisfied(reg, value, declared) {
		t.Fatalf("full declared contract should still reject the presence mismatch")
	}
}

func TestMeetConstraintRecoversCompatibleWitnessRefinement(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.FromType(reg, typeexpr.Union(typ.String, typ.Number))
	constraint := typevalue.FromType(reg, typ.String)

	got := MeetConstraint(reg, value, constraint)
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("refined type = %v/%v, want string", gotType, ok)
	}
}

func TestMeetConstraintUsesNarrowerWitnessAfterSuccessfulMeet(t *testing.T) {
	reg := standard.Registry()
	valueType := typeexpr.Optional(typ.String)
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, valueType), valueType)
	constraint := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	got := MeetConstraint(reg, value, constraint)
	if !product.Equal(reg, got, constraint) {
		gotType, gotOK := typevalue.WitnessOf(reg, got)
		wantType, wantOK := typevalue.WitnessOf(reg, constraint)
		t.Fatalf("refined witness = %v/%v, want %v/%v", gotType, gotOK, wantType, wantOK)
	}
}

func TestMeetConstraintRejectsDisjointLiteralWitnesses(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("string")), typ.LiteralString("string"))
	constraint := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("boolean")), typ.LiteralString("boolean"))

	got := MeetConstraint(reg, value, constraint)
	if !product.Equal(reg, got, product.Bottom(reg)) {
		gotType, gotOK := typevalue.TypeOf(reg, got)
		t.Fatalf("refined value = %v/%v, want bottom for disjoint literal witnesses", gotType, gotOK)
	}
}

func TestNegatedLiteralContradictsValueUsesExactWitness(t *testing.T) {
	reg := standard.Registry()
	lit := typ.LiteralString("auto")
	constraint := typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit)
	exact := typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit)
	broad := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	if !NegatedLiteralContradictsValue(reg, nil, exact, constraint) {
		t.Fatal("exact literal value should contradict negated same literal")
	}
	if NegatedLiteralContradictsValue(reg, nil, broad, constraint) {
		t.Fatal("broad string should not contradict negated specific string literal")
	}
	if NegatedLiteralContradictsValue(reg, nil, exact, typevalue.FromType(reg, lit)) {
		t.Fatal("constraint without literal witness should not contradict")
	}
}

func TestMeetConstraintNarrowsUnionWitnessByRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	tableType := typetable.NewMap(typ.String, typ.String)
	valueType := typeexpr.Union(typ.String, tableType)
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, valueType), valueType)
	constraint := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

	got := MeetConstraint(reg, value, constraint)
	if product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("runtime-kind refinement collapsed compatible union witness to bottom")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, tableType) {
		t.Fatalf("refined type = %v/%v, want %v", gotType, ok, tableType)
	}
}

func TestMeetConstraintNarrowsScalarUnionWitnessByRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	valueType := typeexpr.Union(typ.Number, typ.String, typ.Boolean)
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, valueType), valueType)
	constraint := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	constraint = typevalue.WithWitness(reg, constraint, typ.Boolean)

	got := MeetConstraint(reg, value, constraint)
	if product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("runtime-kind refinement collapsed compatible scalar union witness to bottom")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Boolean) {
		t.Fatalf("refined type = %v/%v, want boolean", gotType, ok)
	}
}

func TestMeetConstraintKeepsPreciseTableSubtypeForBroadTypeGuardWitness(t *testing.T) {
	reg := standard.Registry()
	tableType := typetable.NewMap(typ.String, typ.String)
	cases := []struct {
		name      string
		valueType typ.Type
		want      typ.Type
	}{
		{
			name:      "union",
			valueType: typeexpr.Union(typ.String, tableType),
			want:      tableType,
		},
		{
			name:      "already table",
			valueType: tableType,
			want:      tableType,
		},
	}
	constraint := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	constraint = typevalue.WithWitness(reg, constraint, typetable.BuiltinTopMarker())

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := typevalue.WithWitness(reg, typevalue.FromType(reg, tc.valueType), tc.valueType)
			got := MeetConstraint(reg, value, constraint)
			if product.Equal(reg, got, product.Bottom(reg)) {
				t.Fatal("broad table guard collapsed compatible precise table witness to bottom")
			}
			gotType, ok := typevalue.TypeOf(reg, got)
			if !ok || !typ.TypeEquals(gotType, tc.want) {
				t.Fatalf("refined type = %v/%v, want %v", gotType, ok, tc.want)
			}
		})
	}
}

func TestMeetConstraintKeepsExplicitAnyReachableForTableTypeGuard(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Any), typ.Any)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	constraint := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	constraint = product.Set(reg, constraint, assertion.Key, assertion.Runtime())
	constraint = typevalue.WithWitness(reg, constraint, typetable.BuiltinTopMarker())

	got := MeetConstraint(reg, value, constraint)
	if product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("table type guard collapsed explicit any value to unreachable")
	}
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("runtime kind = %v, want table", kind)
	}
}

func TestMeetConstraintPreservesConcreteVariantOriginForOpenGenericConstraint(t *testing.T) {
	reg := standard.Registry()
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", tp).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", typ.String).
			Build(),
	))
	openResult := typ.Instantiate(result, tp)
	okPath := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}
	narrowed, ok := variant.NarrowByPathLiteral(openResult, okPath, typ.LiteralBool(true))
	if !ok {
		t.Fatal("open Result<T> did not narrow on ok=true")
	}
	family, cases, ok := variant.OriginByPathLiteral(openResult, okPath, typ.LiteralBool(true))
	if !ok {
		t.Fatal("open Result<T> origin missing")
	}
	constraint := product.Set(reg, typevalue.FromType(reg, narrowed), variantorigin.Key, variantorigin.Of(family, cases))
	concrete := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", typ.LiteralInt(41)).
		Build()
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, concrete), concrete)
	valueOrigin := product.Get(reg, value, variantorigin.Key)

	got := MeetConstraint(reg, value, constraint)
	if product.Equal(reg, got, product.Bottom(reg)) {
		valueType, valueTypeOK := typevalue.TypeOf(reg, value)
		constraintType, constraintTypeOK := typevalue.TypeOf(reg, constraint)
		t.Fatalf("compatible open generic variant refinement collapsed to bottom: valueOrigin=%v constraintOrigin=%v valueType=%v/%v constraintType=%v/%v admits=%v",
			product.Get(reg, value, variantorigin.Key),
			product.Get(reg, constraint, variantorigin.Key),
			valueType, valueTypeOK,
			constraintType, constraintTypeOK,
			openVariantConstraintAdmitsValue(valueType, constraintType, 0),
		)
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, concrete) {
		t.Fatalf("refined type = %v/%v, want concrete payload witness", gotType, ok)
	}
	origin := product.Get(reg, got, variantorigin.Key)
	if origin.IsTop() || origin.IsBottom() || origin.Family() != valueOrigin.Family() {
		t.Fatalf("refined origin = %v, want concrete value origin family %v", origin, valueOrigin)
	}
}

func TestMeetConstraintAdoptsOpenGenericOriginForInstantiatedUnion(t *testing.T) {
	reg := standard.Registry()
	tp := typ.NewTypeParam("T", nil)
	errp := typ.NewTypeParam("E", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp, errp}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", tp).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", errp).
			Build(),
	))
	openResult := typ.Instantiate(result, tp, errp)
	okPath := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}
	narrowed, ok := variant.NarrowByPathLiteral(openResult, okPath, typ.LiteralBool(true))
	if !ok {
		t.Fatal("open Result<T, E> did not narrow on ok=true")
	}
	family, cases, ok := variant.OriginByPathLiteral(openResult, okPath, typ.LiteralBool(true))
	if !ok {
		t.Fatal("open Result<T, E> origin missing")
	}
	constraint := product.Set(reg, typevalue.FromType(reg, narrowed), variantorigin.Key, variantorigin.Of(family, cases))
	concreteResult := typ.Instantiate(result, typ.LiteralInt(41), typ.String)
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, concreteResult), concreteResult)

	got := MeetConstraint(reg, value, constraint)
	if product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("instantiated Result value did not accept compatible open ok-arm origin")
	}
	gotType, ok := typevalue.StructuralTypeOf(reg, typevalue.NewCache(), got, typevalue.StructuralTypeOptions{})
	want := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", typ.LiteralInt(41)).
		Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		valueType, valueTypeOK := typevalue.TypeOf(reg, value)
		constraintType, constraintTypeOK := typevalue.TypeOf(reg, constraint)
		directType, directTypeOK := typevalue.TypeOf(reg, got)
		narrowed, narrowedOK := compatibleValueWitnessType(valueType, constraintType, 0)
		t.Fatalf("structural type = %v/%v direct=%v/%v, want %v (valueType=%v/%v constraintType=%v/%v admits=%v narrowed=%v/%v origin=%v)",
			gotType, ok,
			directType, directTypeOK,
			want,
			valueType, valueTypeOK,
			constraintType, constraintTypeOK,
			openVariantConstraintAdmitsValue(valueType, constraintType, 0),
			narrowed, narrowedOK,
			product.Get(reg, got, variantorigin.Key),
		)
	}
}

func TestMergeDeclaredContractPreservesSelectedObjectLiteralVariant(t *testing.T) {
	reg := standard.Registry()
	intCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("number")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	textCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("string")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	flagCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("boolean")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	cell := typeexpr.Union(intCell, textCell, flagCell)
	actualType := typetable.NewRecord().
		Field("kind", typ.LiteralString("string")).
		Field("raw", typ.LiteralString("x")).
		Build()

	actual := typevalue.WithWitness(reg, typevalue.FromType(reg, actualType), actualType)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, cell), cell)
	merged := MergeDeclaredContract(reg, actual, declared)

	got, ok := typevalue.TypeOf(reg, merged)
	if !ok || !typ.TypeEquals(got, textCell) {
		t.Fatalf("merged type = %v/%v, want selected text cell %v", got, ok, textCell)
	}
}

func TestMergeDeclaredContractDoesNotInferGenericArgFromUnselectedResultArm(t *testing.T) {
	reg := standard.Registry()
	param := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{param}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", param).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", typ.String).
			Build(),
	))
	user := typetable.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()

	actual := typevalue.WithWitness(reg, typevalue.FromType(reg, errorCase), errorCase)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Instantiate(result, user)), typ.Instantiate(result, user))
	merged := MergeDeclaredContract(reg, actual, declared)

	got, ok := typevalue.TypeOf(reg, merged)
	if !ok || !typ.TypeEquals(got, errorCase) {
		t.Fatalf("merged type = %v/%v, want selected error variant %v", got, ok, errorCase)
	}
}
