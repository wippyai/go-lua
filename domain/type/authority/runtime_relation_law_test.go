package typeauthority

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/subtype"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// TestRuntimeSealedRelationMaterializesCanonicalJudgment is the sealed
// judgment law. Runtime owns no structural proof of its own: it must
// reproduce the canonical checker's answer for every ordered pair of the
// sealed closed universe, including the structural children the descriptor
// algebra observes but no caller named directly.
func TestRuntimeSealedRelationMaterializesCanonicalJudgment(t *testing.T) {
	runtime, _, sources := runtimeRelationFixture(runtimeRelationCorpus())
	if len(runtime.closedRows) == 0 {
		t.Fatal("sealed relation universe is empty")
	}
	for leftPosition, leftRow := range runtime.closedRows {
		left, leftOK := runtime.InnerAtIndex(leftRow)
		if !leftOK {
			t.Fatalf("closed universe position %d is not an owned row", leftPosition)
		}
		for rightPosition, rightRow := range runtime.closedRows {
			right, rightOK := runtime.InnerAtIndex(rightRow)
			if !rightOK {
				t.Fatalf("closed universe position %d is not an owned row", rightPosition)
			}
			expected := subtype.IsSubtype(sources[leftRow-1], sources[rightRow-1])
			answer, decided := runtime.Subtype(left, right)
			if !decided || answer != expected {
				t.Fatalf("sealed relation row %d <: row %d = %v/%v, canonical %v", leftRow, rightRow, answer, decided, expected)
			}
		}
	}
}

// TestSealAdmittingOneDeclarationDecidesNoPair is the bounded-admission law.
// Admitting a declaration is linear in its node count: the seal publishes the
// closed universe and decides nothing. A seal that materializes the ordered
// pair relation instead pays the square of that node count, each pair proved
// over the full depth of the declaration, so one deeply nested authored type
// never finishes admitting.
//
// The relation then charges exactly one judgment per ordered pair a consumer
// actually asks about, and none for a pair it asks about twice.
func TestSealAdmittingOneDeclarationDecidesNoPair(t *testing.T) {
	const depth = 4096
	deep := typ.Type(typ.String)
	for level := 0; level < depth; level++ {
		deep = typ.NewArray(deep)
	}
	family, err := SealFamily("test/deep-admission", []typ.Type{deep})
	if err != nil {
		t.Fatal(err)
	}
	runtime, inners, _ := sealFamilyRuntime(t, family, nil)
	if len(runtime.closedRows) <= depth {
		t.Fatalf("closed universe = %d rows, the declaration alone has %d nodes", len(runtime.closedRows), depth+1)
	}
	if decided := runtime.relation.judgmentCount(); decided != 0 {
		t.Fatalf("admitting one declaration decided %d ordered pairs, the seal states no judgment", decided)
	}
	deepInner := inners[0]
	unknown, unknownOK := runtime.InnerAtIndex(runtime.unknownRow)
	never, neverOK := runtime.InnerAtIndex(runtime.neverRow)
	if !unknownOK || !neverOK {
		t.Fatal("seed rows")
	}
	asked := []struct {
		left, right RuntimeInner
		want        bool
	}{
		{left: deepInner, right: unknown, want: true},
		{left: never, right: deepInner, want: true},
		{left: unknown, right: deepInner, want: false},
	}
	for index, pair := range asked {
		answer, decided := runtime.Subtype(pair.left, pair.right)
		if !decided || answer != pair.want {
			t.Fatalf("asked pair %d = %t/%t, want %t", index, answer, decided, pair.want)
		}
		if count := runtime.relation.judgmentCount(); count != index+1 {
			t.Fatalf("after %d asked pairs the relation decided %d", index+1, count)
		}
	}
	for _, pair := range asked {
		if _, decided := runtime.Subtype(pair.left, pair.right); !decided {
			t.Fatal("repeated pair undecided")
		}
	}
	if count := runtime.relation.judgmentCount(); count != len(asked) {
		t.Fatalf("repeating every asked pair decided %d judgments, the relation holds %d", count, len(asked))
	}
}

// TestRuntimeSealedRelationDecidesNamedCrossKindRules keeps the named
// decision boundaries of the structural vocabulary executable: every listed
// pair must remain a decided positive judgment of the sealed relation.
func TestRuntimeSealedRelationDecidesNamedCrossKindRules(t *testing.T) {
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

	fixture := make([]runtimeRelationFixtureType, 0, len(tests)*2)
	for _, test := range tests {
		fixture = append(fixture,
			runtimeRelationFixtureType{name: test.name + "/left", value: test.left},
			runtimeRelationFixtureType{name: test.name + "/right", value: test.right},
		)
	}
	runtime, inners, _ := runtimeRelationFixture(fixture)
	for index, test := range tests {
		if !subtype.IsSubtype(test.left, test.right) {
			t.Fatalf("%s fixture no longer states the intended cross-kind subtype", test.name)
		}
		answer, decided := runtime.Subtype(inners[index*2], inners[index*2+1])
		if !decided || !answer {
			t.Fatalf("%s sealed relation = %v/%v", test.name, answer, decided)
		}
	}
}

// TestRuntimeInstantiatedRowsPreserveSubtype keeps instantiation identity in
// the canonical relation. Runtime no longer publishes the old base/argument
// structural planes; those source edges remain construction-only.
func TestRuntimeInstantiatedRowsPreserveBaseAndInvariantArguments(t *testing.T) {
	parameter := typ.NewTypeParam("T", nil)
	base := typ.NewGeneric("Const", []*typ.TypeParam{parameter}, typ.String)
	equalParameter := typ.NewTypeParam("T", nil)
	equalBase := typ.NewGeneric("Const", []*typ.TypeParam{equalParameter}, typ.String)
	otherParameter := typ.NewTypeParam("T", nil)
	otherBase := typ.NewGeneric("OtherConst", []*typ.TypeParam{otherParameter}, typ.String)
	fixture := []runtimeRelationFixtureType{
		{name: "string", value: typ.String},
		{name: "number", value: typ.Number},
		{name: "const-string", value: typ.Instantiate(base, typ.String)},
		{name: "const-number", value: typ.Instantiate(base, typ.Number)},
		{name: "equal-const-number", value: typ.Instantiate(equalBase, typ.Number)},
		{name: "other-const-number", value: typ.Instantiate(otherBase, typ.Number)},
	}
	runtime, inners, _ := runtimeRelationFixture(fixture)

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
		if canonical := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value); canonical != test.want {
			t.Fatalf("%s canonical = %v, want %v", test.name, canonical, test.want)
		}
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != test.want {
			t.Fatalf("%s sealed relation = %v/%v, want %v", test.name, answer, decided, test.want)
		}
	}
}

// TestRuntimeTypeParameterFunctionBindersKeepBinderIdentity proves the dense
// interning of function binders is a structural identity, not a presentation
// name, and that the sealed relation reads that identity.
func TestRuntimeTypeParameterFunctionBindersKeepBinderIdentity(t *testing.T) {
	unconstrained := typ.NewTypeParam("U", nil)
	unconstrainedFunction := typ.Func().TypeParamRef(unconstrained).Param("value", unconstrained).Build()
	constrained := typ.NewTypeParam("T", typ.String)
	constrainedFunction := typ.Func().TypeParamRef(constrained).Param("value", constrained).Build()
	replayed := typ.NewTypeParam("U", nil)
	replayedFunction := typ.Func().TypeParamRef(replayed).Param("value", replayed).Build()

	fixture := []runtimeRelationFixtureType{
		{name: "unconstrained", value: unconstrainedFunction},
		{name: "constrained", value: constrainedFunction},
		{name: "unconstrained-replay", value: replayedFunction},
	}
	runtime, inners, _ := runtimeRelationFixture(fixture)
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
		if canonical := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value); canonical != test.want {
			t.Fatalf("%s canonical = %v, want %v", test.name, canonical, test.want)
		}
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != test.want {
			t.Fatalf("%s sealed relation = %v/%v, want %v", test.name, answer, decided, test.want)
		}
	}
}

// TestRuntimeSelfRewriteFamiliesRemainDecided keeps the Self-carrying meta,
// recursive, and generic families inside the sealed closed universe.
func TestRuntimeSelfRewriteFamiliesRemainDecided(t *testing.T) {
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

	fixture := []runtimeRelationFixtureType{
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
	runtime, inners, _ := runtimeRelationFixture(fixture)
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
		if canonical := subtype.IsSubtype(fixture[test.left].value, fixture[test.right].value); canonical != test.want {
			t.Fatalf("%s canonical = %v, want %v", test.name, canonical, test.want)
		}
		answer, decided := runtime.Subtype(inners[test.left], inners[test.right])
		if !decided || answer != test.want {
			t.Fatalf("%s sealed relation = %v/%v, want %v", test.name, answer, decided, test.want)
		}
	}
}

// TestRuntimeSubtypeIsAllocationFreeAndOwnerFenced states the hot contract of
// the sealed relation: an owner-local lookup is a constant-time bit read and a
// foreign or unsealed handle stays undecided.
func TestRuntimeSubtypeIsAllocationFreeAndOwnerFenced(t *testing.T) {
	runtime, inners, _ := runtimeRelationFixture(runtimeRelationCorpus())
	foreign, foreignInners, _ := runtimeRelationFixture(runtimeRelationCorpus())
	if answer, decided := runtime.Subtype(foreignInners[0], inners[1]); answer || decided {
		t.Fatalf("foreign inner lookup = %v/%v, want false/false", answer, decided)
	}
	if answer, decided := runtime.Subtype(RuntimeInner{}, inners[1]); answer || decided {
		t.Fatalf("zero inner lookup = %v/%v, want false/false", answer, decided)
	}
	if answer, decided := foreign.Subtype(inners[0], foreignInners[1]); answer || decided {
		t.Fatalf("reverse foreign lookup = %v/%v, want false/false", answer, decided)
	}
	for index := range inners {
		left, right := inners[index], inners[(index+1)%len(inners)]
		if allocations := testing.AllocsPerRun(200, func() {
			_, _ = runtime.Subtype(left, right)
		}); allocations != 0 {
			t.Fatalf("sealed relation lookup %d allocated %.2f objects/run", index, allocations)
		}
	}
}

// TestRuntimeSubtypeIsConcurrent proves the sealed relation is immutable: a
// lookup never installs state and therefore never changes an answer under
// concurrent readers.
func TestRuntimeSubtypeIsConcurrent(t *testing.T) {
	fixture := []runtimeRelationFixtureType{
		{name: "integer", value: typ.Integer},
		{name: "number", value: typ.Number},
		{name: "record-narrow", value: typetable.NewRecord().ReadonlyField("extra", typ.Boolean).ReadonlyField("value", typ.String).Build()},
		{name: "record-wide", value: typetable.NewRecord().ReadonlyField("value", typ.String).Build()},
		{name: "union", value: typeexpr.Union(typ.String, typ.Number)},
	}
	runtime, inners, _ := runtimeRelationFixture(fixture)
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
		t.Fatalf("concurrent lookup %d changed answer", index)
	default:
	}
}

func TestRuntimeDynamicTripleKeepsStructuralIdentity(t *testing.T) {
	empty := typ.NewInterface("Empty", nil)
	fixture := []runtimeRelationFixtureType{
		{name: "any", value: typ.Any},
		{name: "empty-interface", value: empty},
		{name: "table-top", value: typ.BuiltinTableTopMarker()},
		{name: "empty-interface-duplicate", value: typ.NewInterface("Empty", nil)},
	}
	runtime, inners, _ := runtimeRelationFixture(fixture)
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

// BenchmarkRuntimeSealSubtypeRelation measures the one-time cost the sealed
// relation moves the quadratic structural work into. It is reported per
// universe atom so a growing vocabulary stays attributable.
func BenchmarkRuntimeSealSubtypeRelation(b *testing.B) {
	corpus := runtimeRelationCorpus()
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		runtime, _, _ := runtimeRelationFixture(corpus)
		if iteration == 0 {
			b.ReportMetric(float64(len(runtime.closedRows)), "atoms")
		}
	}
}

func BenchmarkRuntimeSealedRelationLookup(b *testing.B) {
	runtime, inners, _ := runtimeRelationFixture(runtimeRelationCorpus())
	left, right := inners[0], inners[1]
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, decided := runtime.Subtype(left, right); !decided {
			b.Fatal("sealed relation lookup is undecided")
		}
	}
}

type runtimeRelationFixtureType struct {
	name  string
	value typ.Type
}

// runtimeRelationCorpus is the structural vocabulary the sealed relation must
// decide. It deliberately spans every supported outer form so a row-addressing
// defect cannot hide behind a narrow fixture.
func runtimeRelationCorpus() []runtimeRelationFixtureType {
	method := typ.Func().Returns(typ.String).Build()
	recursiveNode := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("value", typ.String).OptField("next", self).Build()
	})
	parameter := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{parameter}, typetable.NewRecord().Field("value", parameter).Build())
	return []runtimeRelationFixtureType{
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
		{name: "intersection-records", value: typeexpr.Intersection(typetable.NewRecord().Field("name", typ.String).Build(), typetable.NewRecord().Field("count", typ.Number).Build())},
		{name: "intersection-tablelike", value: typeexpr.Intersection(typetable.NewRecord().Field("name", typ.String).Build(), typ.NewInterface("Marker", nil))},
		{name: "tuple", value: typ.NewTuple(typ.Integer, typ.String)},
		{name: "record-narrow", value: typetable.NewRecord().Field("name", typ.String).Field("count", typ.Integer).Build()},
		{name: "record-wide", value: typetable.NewRecord().Field("name", typ.String).Build()},
		{name: "record-optional", value: typetable.NewRecord().OptField("name", typ.String).Build()},
		{name: "record-empty", value: typetable.NewRecord().Build()},
		{name: "record-map", value: typetable.NewRecord().MapComponent(typ.String, typ.Integer).Build()},
		{name: "record-method", value: typetable.NewRecord().Field("run", method).Build()},
		{name: "record-readonly-present", value: typetable.NewRecord().Field("name", typeexpr.Optional(typ.String)).Build()},
		{name: "record-meta-narrow", value: typetable.NewRecord().Metatable(typetable.NewRecord().Field("tag", typ.String).Build()).Build()},
		{name: "record-meta-wide", value: typetable.NewRecord().Metatable(typetable.NewRecord().Build()).Build()},
		{name: "map", value: typ.NewMap(typ.String, typ.Integer)},
		{name: "map-wide-value", value: typ.NewMap(typ.String, typ.Number)},
		{name: "readonly-map", value: typ.NewReadonlyMap(typ.String, typ.Number)},
		{name: "readonly-present-map", value: typ.NewReadonlyMap(typ.String, typ.String)},
		{name: "array", value: typ.NewArray(typ.Integer)},
		{name: "array-wide", value: typ.NewArray(typ.Number)},
		{name: "interface-method", value: typ.NewInterface("Runner", []typ.Method{{Name: "run", Type: method}})},
		{name: "interface-empty", value: typ.NewInterface("Empty", nil)},
		{name: "table-top", value: typ.BuiltinTableTopMarker()},
		{name: "function-implementation", value: typ.Func().Param("value", typ.Any).Returns(typ.Integer).Build()},
		{name: "function-contract", value: typ.Func().Param("value", typ.Number).Returns(typ.Number).Build()},
		{name: "meta-string", value: typ.NewMeta(typ.String)},
		{name: "recursive-node", value: recursiveNode},
		{name: "recursive-view", value: typetable.NewRecord().Field("value", typ.String).Build()},
		{name: "generic-box", value: box},
		{name: "instantiated-box-string", value: typ.Instantiate(box, typ.String)},
		{name: "instantiated-box-number", value: typ.Instantiate(box, typ.Number)},
	}
}

// runtimeRelationFixture runs the production seal pipeline over a raw type
// vocabulary. It stops where SealRuntime stops for the relation: the closed
// universe is published over the construction graphs, and the test keeps those
// graphs as the canonical oracle input.
func runtimeRelationFixture(values []runtimeRelationFixtureType) (*Runtime, []RuntimeInner, []typ.Type) {
	authority := &Authority{linkID: identity.ContentID{3}, artifact: &artifactAuthority{}}
	inputs := make([]RuntimeInput, len(values))
	for index, value := range values {
		input, ok := authority.RuntimeInputForType(value.value)
		if !ok {
			panic("mint RuntimeInput")
		}
		inputs[index] = input
	}
	canonical, err := canonicalRuntimeInputs(authority, inputs)
	if err != nil {
		panic(err)
	}
	runtime := &Runtime{sourceID: authority.LinkID()}
	builder := runtimeBuilder{runtime: runtime}
	canonicalInners, err := builder.ingest(canonical)
	if err != nil {
		panic(err)
	}
	inners := make([]RuntimeInner, len(values))
	for canonicalIndex, input := range canonical {
		for _, position := range input.positions {
			inners[position] = canonicalInners[canonicalIndex]
		}
	}
	for _, step := range []func() error{builder.sealRuntimeKinds, builder.sealCanonical, builder.sealDescriptors, runtime.sealRanks, builder.sealClosedUniverse} {
		if err := step(); err != nil {
			panic(err)
		}
	}
	return runtime, inners, append([]typ.Type(nil), builder.construction...)
}
