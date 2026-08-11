package typeauthority

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

// TestRuntimeSubtypeMatchesCanonicalAcrossSupportedForms is the executable
// decision-boundary law for the dense row proof. The canonical checker is a
// test-only oracle; Runtime.Subtype must decide the same answer directly for
// every closed, owner-local pair.
func TestRuntimeSubtypeMatchesCanonicalAcrossSupportedForms(t *testing.T) {
	method := typ.Func().Returns(typ.String).Build()
	interfaceMethod := typ.NewInterface("Runner", []typ.Method{{Name: "run", Type: method}})
	recordMethod := typetable.NewRecord().Field("run", method).Build()
	recordNarrow := typetable.NewRecord().Field("name", typ.String).Field("count", typ.Integer).Build()
	recordWide := typetable.NewRecord().Field("name", typ.String).Build()
	recordOptional := typetable.NewRecord().OptField("name", typ.String).Build()
	emptyRecord := typetable.NewRecord().Build()
	recordMap := typetable.NewRecord().MapComponent(typ.String, typ.Integer).Build()
	recordReadonlyPresent := typetable.NewRecord().Field("name", typeexpr.Optional(typ.String)).Build()
	metaNarrow := typetable.NewRecord().Field("tag", typ.String).Build()
	metaWide := typetable.NewRecord().Build()
	recordWithMetaNarrow := typetable.NewRecord().Metatable(metaNarrow).Build()
	recordWithMetaWide := typetable.NewRecord().Metatable(metaWide).Build()

	contravariantFunction := typ.Func().Param("value", typ.Any).Returns(typ.Integer).Build()
	functionContract := typ.Func().Param("value", typ.Number).Returns(typ.Number).Build()

	recursiveNode := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("value", typ.String).OptField("next", self).Build()
	})
	recursiveView := typetable.NewRecord().Field("value", typ.String).Build()

	parameter := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{parameter}, typetable.NewRecord().Field("value", parameter).Build())
	boxString := typ.Instantiate(box, typ.String)
	boxNumber := typ.Instantiate(box, typ.Number)
	boxStringView := typetable.NewRecord().Field("value", typ.String).Build()

	fixture := []runtimeSubtypeFixtureType{
		{name: "nil", value: typ.Nil},
		{name: "boolean", value: typ.Boolean},
		{name: "number", value: typ.Number},
		{name: "integer", value: typ.Integer},
		{name: "string", value: typ.String},
		{name: "any", value: typ.Any},
		{name: "unknown", value: typ.Unknown},
		{name: "never", value: typ.Never},
		{name: "literal-boolean", value: typ.LiteralBool(true)},
		{name: "literal-integer", value: typ.LiteralInt(7)},
		{name: "literal-number", value: typ.LiteralNumber(7.5)},
		{name: "literal-string", value: typ.LiteralString("name")},
		{name: "optional-string", value: typeexpr.Optional(typ.String)},
		{name: "optional-unknown", value: typeexpr.Optional(typ.Unknown)},
		{name: "union-scalar", value: typeexpr.Union(typ.String, typ.Number)},
		{name: "union-literals", value: typeexpr.Union(typ.LiteralString("name"), typ.LiteralString("other"))},
		{name: "union-table-top", value: typeexpr.Union(typ.BuiltinTableTopMarker(), typ.String)},
		{name: "intersection-records", value: typeexpr.Intersection(recordWide, typetable.NewRecord().Field("count", typ.Number).Build())},
		{name: "intersection-tablelike", value: typeexpr.Intersection(recordWide, typ.NewInterface("Marker", nil))},
		{name: "tuple", value: typ.NewTuple(typ.Integer, typ.String)},
		{name: "record-narrow", value: recordNarrow},
		{name: "record-wide", value: recordWide},
		{name: "record-optional", value: recordOptional},
		{name: "record-empty", value: emptyRecord},
		{name: "record-map", value: recordMap},
		{name: "record-method", value: recordMethod},
		{name: "record-readonly-present", value: recordReadonlyPresent},
		{name: "record-meta-narrow", value: recordWithMetaNarrow},
		{name: "record-meta-wide", value: recordWithMetaWide},
		{name: "map", value: typ.NewMap(typ.String, typ.Integer)},
		{name: "map-wide-value", value: typ.NewMap(typ.String, typ.Number)},
		{name: "readonly-map", value: typ.NewReadonlyMap(typ.String, typ.Number)},
		{name: "readonly-present-map", value: typ.NewReadonlyMap(typ.String, typ.String)},
		{name: "array", value: typ.NewArray(typ.Integer)},
		{name: "array-wide", value: typ.NewArray(typ.Number)},
		{name: "interface-method", value: interfaceMethod},
		{name: "interface-empty", value: typ.NewInterface("Empty", nil)},
		{name: "table-top", value: typ.BuiltinTableTopMarker()},
		{name: "function-implementation", value: contravariantFunction},
		{name: "function-contract", value: functionContract},
		{name: "meta-string", value: typ.NewMeta(typ.String)},
		{name: "recursive-node", value: recursiveNode},
		{name: "recursive-view", value: recursiveView},
		{name: "generic-box", value: box},
		{name: "instantiated-box-string", value: boxString},
		{name: "instantiated-box-number", value: boxNumber},
		{name: "instantiated-box-string-view", value: boxStringView},
	}

	runtime, inners := runtimeSubtypeFixture(fixture)
	for leftIndex, left := range fixture {
		for rightIndex, right := range fixture {
			expected := subtype.IsSubtype(left.value, right.value)
			answer, decided := runtime.Subtype(inners[leftIndex], inners[rightIndex])
			if !decided || answer != expected {
				t.Fatalf("Runtime.Subtype(%s, %s) = %v/%v, canonical %v", left.name, right.name, answer, decided, expected)
			}
		}
	}
}

func TestRuntimeSubtypeCrossKindRulesRemainReachable(t *testing.T) {
	method := typ.Func().Returns(typ.String).Build()
	selfMethodImplementation := typ.Func().Param("self", typ.BuiltinTableTopMarker()).Returns(typ.String).Build()
	selfMethodContract := typ.Func().Param("self", typ.Self).Returns(typ.String).Build()
	boxParameter := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParameter}, typetable.NewRecord().Field("value", boxParameter).Build())
	bottomGeneric := typ.NewGeneric("Bottom", nil, typ.Never)
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("value", typ.String).OptField("next", self).Build()
	})
	recursiveTableTop := typ.NewRecursive("TableTop", func(typ.Type) typ.Type { return typ.BuiltinTableTopMarker() })
	recursiveAny := typ.NewRecursive("Dynamic", func(typ.Type) typ.Type { return typ.Any })
	recursiveNever := typ.NewRecursive("Bottom", func(typ.Type) typ.Type { return typ.Never })

	tests := []struct {
		name        string
		left, right typ.Type
	}{
		{name: "record-to-map", left: typetable.NewRecord().Field("name", typ.String).Build(), right: typ.NewMap(typ.String, typ.String)},
		{name: "record-to-interface", left: typetable.NewRecord().Field("run", method).Build(), right: typ.NewInterface("Runner", []typ.Method{{Name: "run", Type: method}})},
		{name: "record-to-interface-self-method", left: typetable.NewRecord().Field("run", selfMethodImplementation).Build(), right: typ.NewInterface("SelfRunner", []typ.Method{{Name: "run", Type: selfMethodContract}})},
		{name: "array-to-map", left: typ.NewArray(typ.Integer), right: typ.NewMap(typ.Integer, typ.Number)},
		{name: "tuple-to-array", left: typ.NewTuple(typ.Integer, typ.Integer), right: typ.NewArray(typ.Number)},
		{name: "tuple-to-map", left: typ.NewTuple(typ.Integer, typ.Integer), right: typ.NewMap(typ.Integer, typ.Number)},
		{name: "tuple-to-readonly-map", left: typ.NewTuple(typ.Integer, typ.Integer), right: typ.NewReadonlyMap(typ.Number, typ.Number)},
		{name: "empty-record-to-array", left: typetable.NewRecord().Build(), right: typ.NewArray(typ.String)},
		{name: "empty-record-to-map", left: typetable.NewRecord().Build(), right: typ.NewMap(typ.String, typ.Number)},
		{name: "map-to-readonly-map", left: typ.NewMap(typ.String, typ.Integer), right: typ.NewReadonlyMap(typ.String, typ.Number)},
		{name: "readonly-present-value", left: typ.NewReadonlyMap(typ.String, typeexpr.Optional(typ.String)), right: typ.NewReadonlyMap(typ.String, typ.String)},
		{name: "record-readonly-present-value", left: typetable.NewRecord().Field("name", typeexpr.Optional(typ.String)).Build(), right: typ.NewReadonlyMap(typ.String, typ.String)},
		{name: "literal-to-primitive", left: typ.LiteralInt(1), right: typ.Number},
		{name: "map-to-record", left: typ.NewMap(typ.String, typ.Integer), right: typetable.NewRecord().MapComponent(typ.String, typ.Integer).Build()},
		{name: "instantiation-expansion", left: typ.Instantiate(box, typ.String), right: typetable.NewRecord().Field("value", typ.String).Build()},
		{name: "recursive-expansion", left: recursive, right: typetable.NewRecord().Field("value", typ.String).Build()},
		{name: "any-to-recursive-table-top", left: typ.Any, right: recursiveTableTop},
		{name: "unknown-to-recursive-any", left: typ.Unknown, right: recursiveAny},
		{name: "recursive-never-to-never", left: recursiveNever, right: typ.Never},
		{name: "instantiated-never-to-never", left: typ.Instantiate(bottomGeneric), right: typ.Never},
	}

	fixture := make([]runtimeSubtypeFixtureType, 0, len(tests)*2)
	for _, test := range tests {
		fixture = append(fixture,
			runtimeSubtypeFixtureType{name: test.name + "/left", value: test.left},
			runtimeSubtypeFixtureType{name: test.name + "/right", value: test.right},
		)
	}
	runtime, inners := runtimeSubtypeFixture(fixture)
	for index, test := range tests {
		if !subtype.IsSubtype(test.left, test.right) {
			t.Fatalf("%s canonical fixture no longer proves the intended cross-kind subtype", test.name)
		}
		answer, decided := runtime.Subtype(inners[index*2], inners[index*2+1])
		if !decided || !answer {
			t.Fatalf("%s Runtime row proof = %v/%v", test.name, answer, decided)
		}
	}
}

func TestRuntimeInstantiatedRowsPreserveBaseAndInvariantArguments(t *testing.T) {
	parameter := typ.NewTypeParam("T", nil)
	base := typ.NewGeneric("Const", []*typ.TypeParam{parameter}, typ.String)
	equalParameter := typ.NewTypeParam("T", nil)
	equalBase := typ.NewGeneric("Const", []*typ.TypeParam{equalParameter}, typ.String)
	otherParameter := typ.NewTypeParam("T", nil)
	otherBase := typ.NewGeneric("OtherConst", []*typ.TypeParam{otherParameter}, typ.String)
	fixture := []runtimeSubtypeFixtureType{
		{name: "string", value: typ.String},
		{name: "number", value: typ.Number},
		{name: "const-string", value: typ.Instantiate(base, typ.String)},
		{name: "const-number", value: typ.Instantiate(base, typ.Number)},
		{name: "equal-const-number", value: typ.Instantiate(equalBase, typ.Number)},
		{name: "other-const-number", value: typ.Instantiate(otherBase, typ.Number)},
	}
	runtime, inners := runtimeSubtypeFixture(fixture)

	for _, instance := range []struct {
		fixtureIndex int
		argument     int
	}{
		{fixtureIndex: 2, argument: 0},
		{fixtureIndex: 3, argument: 1},
		{fixtureIndex: 4, argument: 1},
		{fixtureIndex: 5, argument: 1},
	} {
		row := runtime.rows[inners[instance.fixtureIndex].index-1]
		if row.form != FormInstantiated || !row.base.present || row.arguments.end-row.arguments.start != 1 {
			t.Fatalf("%s row lost base/arguments", fixture[instance.fixtureIndex].name)
		}
		if argument := runtime.arguments[row.arguments.start]; !runtime.Equal(argument, inners[instance.argument]) {
			t.Fatalf("%s argument points at the wrong dense row", fixture[instance.fixtureIndex].name)
		}
	}
	baseRow := runtime.rows[inners[2].index-1].base.inner.index
	if runtime.rows[inners[3].index-1].base.inner.index != baseRow || runtime.rows[inners[4].index-1].base.inner.index != baseRow {
		t.Fatal("structurally equal generic bases did not share one authoritative row")
	}
	if runtime.rows[inners[5].index-1].base.inner.index == baseRow {
		t.Fatal("distinct generic bases collapsed")
	}

	for _, test := range []struct {
		name        string
		left, right int
		want        bool
	}{
		{name: "same-base-string-number", left: 2, right: 3, want: false},
		{name: "same-base-number-string", left: 3, right: 2, want: false},
		{name: "structurally-equal-base-invariant", left: 2, right: 4, want: false},
		{name: "same-expansion-distinct-base", left: 2, right: 5, want: true},
		{name: "instance-expands-to-body", left: 2, right: 0, want: true},
	} {
		canonical := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value)
		if canonical != test.want {
			t.Fatalf("%s canonical = %v, want %v", test.name, canonical, test.want)
		}
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != canonical {
			t.Fatalf("%s Runtime = %v/%v, canonical %v", test.name, answer, decided, canonical)
		}
	}
}

func TestRuntimeTypeParameterFunctionBindersMatchCanonical(t *testing.T) {
	unconstrained := typ.NewTypeParam("U", nil)
	unconstrainedFunction := typ.Func().TypeParamRef(unconstrained).Param("value", unconstrained).Build()
	constrained := typ.NewTypeParam("T", typ.String)
	constrainedFunction := typ.Func().TypeParamRef(constrained).Param("value", constrained).Build()
	replayed := typ.NewTypeParam("U", nil)
	replayedFunction := typ.Func().TypeParamRef(replayed).Param("value", replayed).Build()

	fixture := []runtimeSubtypeFixtureType{
		{name: "unconstrained", value: unconstrainedFunction},
		{name: "constrained", value: constrainedFunction},
		{name: "unconstrained-replay", value: replayedFunction},
	}
	runtime, inners := runtimeSubtypeFixture(fixture)
	if !runtime.Equal(inners[0], inners[2]) {
		t.Fatal("same structural function binder replay did not reuse its dense row")
	}
	for _, test := range []struct {
		name        string
		left, right int
		want        bool
	}{
		{name: "unconstrained-to-constrained", left: 0, right: 1, want: false},
		{name: "constrained-to-unconstrained", left: 1, right: 0, want: false},
		{name: "same-binder-replay-forward", left: 0, right: 2, want: true},
		{name: "same-binder-replay-reverse", left: 2, right: 0, want: true},
	} {
		canonical := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value)
		if canonical != test.want {
			t.Fatalf("%s canonical = %v, want %v", test.name, canonical, test.want)
		}
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != canonical {
			t.Fatalf("%s Runtime = %v/%v, canonical %v", test.name, answer, decided, canonical)
		}
	}
}

func TestRuntimeSelfRewriteActivationMatchesMetaRecursiveAndGeneric(t *testing.T) {
	metaRecursive := typ.NewRecursive("MetaRecord", func(self typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("run", typ.Func().Returns(typ.NewMeta(self)).Build()).Build()
	})
	metaRecord := metaRecursive.Body.(*typ.Record)
	metaMethod := typ.Func().Returns(typ.NewMeta(metaRecursive)).Build()
	metaExpected := typ.Func().Returns(typ.NewMeta(typ.Self)).Build()
	metaInterface := typ.NewInterface("MetaRunner", []typ.Method{{Name: "run", Type: metaExpected}})

	recursiveActual := typ.NewRecursive("RecursiveRunner", func(self typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("next", self).ReadonlyField("run", typ.Func().Returns(self).Build()).Build()
	})
	recursiveRecord := recursiveActual.Body.(*typ.Record)
	recursiveActualMethod := typ.Func().Returns(recursiveActual).Build()
	recursiveExpected := typ.NewRecursive("RecursiveExpected", func(typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("next", typ.Self).Build()
	})
	recursiveMethod := typ.Func().Returns(typeexpr.Intersection(typ.Self, recursiveExpected)).Build()
	recursiveInterface := typ.NewInterface("RecursiveInterface", []typ.Method{{Name: "run", Type: recursiveMethod}})

	genericActual := typ.NewGeneric("G", nil, nil)
	genericBody := typetable.NewRecord().ReadonlyField("run", typ.Func().Returns(genericActual).Build()).Build()
	genericActual.SetBody(genericBody)
	genericRecord := typetable.NewRecord().ReadonlyField("run", typ.Func().Returns(genericActual).Build()).Build()
	genericExpected := typ.NewGeneric("G", nil, typ.Self)
	genericExpectedMethod := typ.Func().Returns(genericExpected).Build()
	genericInterface := typ.NewInterface("GenericInterface", []typ.Method{{Name: "run", Type: genericExpectedMethod}})

	fixture := []runtimeSubtypeFixtureType{
		{name: "meta-method", value: metaMethod},
		{name: "meta-expected", value: metaExpected},
		{name: "meta-record", value: metaRecord},
		{name: "meta-interface", value: metaInterface},
		{name: "recursive-actual", value: recursiveActualMethod},
		{name: "recursive-method", value: recursiveMethod},
		{name: "recursive-record", value: recursiveRecord},
		{name: "recursive-interface", value: recursiveInterface},
		{name: "generic-actual", value: genericActual},
		{name: "generic-expected", value: genericExpected},
		{name: "generic-record", value: genericRecord},
		{name: "generic-interface", value: genericInterface},
	}
	runtime, inners := runtimeSubtypeFixture(fixture)
	for _, test := range []struct {
		name        string
		left, right int
		want        bool
	}{
		{name: "meta-inactive", left: 0, right: 1, want: false},
		{name: "meta-activated", left: 2, right: 3, want: true},
		{name: "recursive-inactive", left: 4, right: 5, want: false},
		{name: "recursive-activated", left: 6, right: 7, want: true},
		{name: "generic-inactive", left: 8, right: 9, want: false},
		{name: "generic-activated", left: 10, right: 11, want: true},
	} {
		canonical := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value)
		if canonical != test.want {
			t.Fatalf("%s canonical = %v, want %v", test.name, canonical, test.want)
		}
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != canonical {
			t.Fatalf("%s Runtime = %v/%v, canonical %v", test.name, answer, decided, canonical)
		}
	}
}

func TestRuntimeSubtypeRecursiveCoinductionTerminates(t *testing.T) {
	positiveLeft := typ.NewRecursive("PositiveLeft", func(self typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("next", self).ReadonlyField("value", typ.String).Build()
	})
	positiveRight := typ.NewRecursive("PositiveRight", func(self typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("next", self).ReadonlyField("value", typ.Any).Build()
	})
	negativeRight := typ.NewRecursive("NegativeRight", func(self typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("next", self).ReadonlyField("value", typ.Number).Build()
	})
	fixture := []runtimeSubtypeFixtureType{
		{name: "positive-left", value: positiveLeft},
		{name: "positive-right", value: positiveRight},
		{name: "negative-right", value: negativeRight},
	}
	runtime, inners := runtimeSubtypeFixture(fixture)
	for _, test := range []struct {
		name        string
		left, right int
	}{
		{name: "productive-cycle-closes", left: 0, right: 1},
		{name: "cycle-does-not-hide-leaf-mismatch", left: 0, right: 2},
	} {
		want := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value)
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != want {
			t.Fatalf("%s = %v/%v, canonical %v", test.name, answer, decided, want)
		}
	}
}

func TestRuntimeSubtypeHotFamiliesAllocateNothing(t *testing.T) {
	functionImplementation := typ.Func().Param("value", typ.Any).Returns(typ.Integer).Build()
	functionContract := typ.Func().Param("value", typ.Number).Returns(typ.Number).Build()
	recursiveLeft := typ.NewRecursive("AllocLeft", func(self typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("next", self).ReadonlyField("value", typ.String).Build()
	})
	recursiveRight := typ.NewRecursive("AllocRight", func(self typ.Type) typ.Type {
		return typetable.NewRecord().ReadonlyField("next", self).ReadonlyField("value", typ.Any).Build()
	})
	parameter := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("AllocBox", []*typ.TypeParam{parameter}, typetable.NewRecord().ReadonlyField("value", parameter).Build())
	fixture := []runtimeSubtypeFixtureType{
		{name: "integer", value: typ.Integer},
		{name: "number", value: typ.Number},
		{name: "literal", value: typ.LiteralInt(7)},
		{name: "record-narrow", value: typetable.NewRecord().ReadonlyField("extra", typ.Integer).ReadonlyField("value", typ.String).Build()},
		{name: "record-wide", value: typetable.NewRecord().ReadonlyField("value", typ.String).Build()},
		{name: "function-implementation", value: functionImplementation},
		{name: "function-contract", value: functionContract},
		{name: "map-narrow", value: typ.NewMap(typ.String, typ.Integer)},
		{name: "map-wide", value: typ.NewMap(typ.String, typ.Number)},
		{name: "tuple-narrow", value: typ.NewTuple(typ.Integer, typ.Integer)},
		{name: "tuple-wide", value: typ.NewTuple(typ.Number, typ.Number)},
		{name: "union", value: typeexpr.Union(typ.LiteralString("left"), typ.LiteralString("right"))},
		{name: "string", value: typ.String},
		{name: "recursive-left", value: recursiveLeft},
		{name: "recursive-right", value: recursiveRight},
		{name: "instantiated", value: typ.Instantiate(box, typ.String)},
		{name: "instantiated-view", value: typetable.NewRecord().ReadonlyField("value", typ.String).Build()},
	}
	tests := []struct {
		family      string
		left, right int
	}{
		{family: "primitive", left: 0, right: 1},
		{family: "literal", left: 2, right: 1},
		{family: "record", left: 3, right: 4},
		{family: "function", left: 5, right: 6},
		{family: "map", left: 7, right: 8},
		{family: "tuple", left: 9, right: 10},
		{family: "union", left: 11, right: 12},
		{family: "recursive", left: 13, right: 14},
		{family: "instantiated", left: 15, right: 16},
	}
	runtime, inners := runtimeSubtypeFixture(fixture)
	for _, test := range tests {
		want := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value)
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != want {
			t.Fatalf("%s proof = %v/%v, canonical %v", test.family, answer, decided, want)
		}
	}
	for _, test := range tests {
		if allocations := testing.AllocsPerRun(1000, func() {
			_, _ = runtime.Subtype(inners[test.left], inners[test.right])
		}); allocations != 0 {
			t.Fatalf("Runtime %s subtype allocated %.2f objects/run", test.family, allocations)
		}
	}
}

func TestRuntimeSubtypeIsConcurrentAndOwnerFenced(t *testing.T) {
	fixture := []runtimeSubtypeFixtureType{
		{name: "integer", value: typ.Integer},
		{name: "number", value: typ.Number},
		{name: "record-narrow", value: typetable.NewRecord().ReadonlyField("extra", typ.Boolean).ReadonlyField("value", typ.String).Build()},
		{name: "record-wide", value: typetable.NewRecord().ReadonlyField("value", typ.String).Build()},
		{name: "union", value: typeexpr.Union(typ.String, typ.Number)},
	}
	runtime, inners := runtimeSubtypeFixture(fixture)
	foreign, foreignInners := runtimeSubtypeFixture(fixture)
	if answer, decided := runtime.Subtype(foreignInners[0], inners[1]); answer || decided {
		t.Fatalf("foreign inner proof = %v/%v, want false/false", answer, decided)
	}
	if answer, decided := runtime.Subtype(RuntimeInner{}, inners[1]); answer || decided {
		t.Fatalf("zero inner proof = %v/%v, want false/false", answer, decided)
	}
	if answer, decided := foreign.Subtype(inners[0], foreignInners[1]); answer || decided {
		t.Fatalf("reverse foreign proof = %v/%v, want false/false", answer, decided)
	}
	tests := []struct {
		left, right int
		answer      bool
	}{
		{left: 0, right: 1, answer: true},
		{left: 1, right: 0, answer: false},
		{left: 2, right: 3, answer: true},
		{left: 4, right: 1, answer: false},
	}
	const workers = 8
	const repetitions = 1000
	failures := make(chan int, 1)
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for repetition := 0; repetition < repetitions; repetition++ {
				for index, test := range tests {
					answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
					if !decided || answer != test.answer {
						select {
						case failures <- index:
						default:
						}
						return
					}
				}
			}
		}()
	}
	wait.Wait()
	select {
	case index := <-failures:
		t.Fatalf("concurrent proof %d changed answer", index)
	default:
	}
}

func TestRuntimeDynamicTripleKeepsStructuralIdentity(t *testing.T) {
	empty := typ.NewInterface("Empty", nil)
	fixture := []runtimeSubtypeFixtureType{
		{name: "any", value: typ.Any},
		{name: "empty-interface", value: empty},
		{name: "table-top", value: typ.BuiltinTableTopMarker()},
		{name: "empty-interface-duplicate", value: typ.NewInterface("Empty", nil)},
	}
	runtime, inners := runtimeSubtypeFixture(fixture)
	if runtime.rows[inners[1].index-1].tableTop {
		t.Fatal("ordinary empty interface was marked as the canonical table top")
	}
	for left := 0; left < 3; left++ {
		for right := 0; right < 3; right++ {
			if left == right {
				continue
			}
			want := subtype.IsSubtype(fixture[left].value, fixture[right].value)
			answer, decided := runtime.Subtype(inners[left], inners[right])
			if !decided || answer != want {
				t.Fatalf("%s <: %s = %v/%v, canonical %v", fixture[left].name, fixture[right].name, answer, decided, want)
			}
		}
	}
	seen := make(map[uint32]struct{}, 3)
	for index := 0; index < 3; index++ {
		canonical, ok := runtime.Canonical(inners[index])
		if !ok {
			t.Fatalf("%s lost structural canonical identity", fixture[index].name)
		}
		id, ok := runtime.Index(canonical)
		if !ok {
			t.Fatalf("%s canonical identity is foreign", fixture[index].name)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("%s collapsed into another structural canonical identity", fixture[index].name)
		}
		seen[id] = struct{}{}
	}
	if !runtime.Equal(inners[1], inners[3]) {
		t.Fatal("equal structural inputs did not reuse one dense identity")
	}
	if answer, decided := runtime.StructuralEqual(inners[1], inners[3]); !decided || !answer {
		t.Fatalf("duplicate structural equality = %v/%v, want true/true", answer, decided)
	}
}

type runtimeSubtypeFixtureType struct {
	name  string
	value typ.Type
}

func runtimeSubtypeFixture(values []runtimeSubtypeFixtureType) (*Runtime, []RuntimeInner) {
	runtime := &Runtime{}
	builder := runtimeBuilder{runtime: runtime, byIdentity: make(map[string]RuntimeInner)}
	if err := builder.seedPrimitives(); err != nil {
		panic(err)
	}
	inners := make([]RuntimeInner, len(values))
	for index, value := range values {
		inner, err := builder.add(runtimePending{value: value.value})
		if err != nil {
			panic(err)
		}
		inners[index] = inner
	}
	if err := builder.close(); err != nil {
		panic(err)
	}
	if err := builder.describe(); err != nil {
		panic(err)
	}
	if err := builder.sealCanonical(); err != nil {
		panic(err)
	}
	return runtime, inners
}
