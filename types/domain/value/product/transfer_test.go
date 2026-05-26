package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// assertProjects fails unless av projects (losslessly, under the value-domain
// convergence relation) to want. This is the canonical check that a
// product-native transfer primitive matches its typ.Type-level semantics.
func assertProjects(t *testing.T, name string, av AbstractValue, want typ.Type) {
	t.Helper()
	got := av.ProjectValue()
	if !value.SameConvergedFact(want, got) {
		t.Fatalf("%s: got %s, want %s", name, got, want)
	}
}

func recordWith(fields ...typ.Field) typ.Type {
	b := typ.NewRecord()
	for _, f := range fields {
		if f.Optional {
			b.OptField(f.Name, f.Type)
		} else {
			b.Field(f.Name, f.Type)
		}
	}
	return b.Build()
}

// TestFieldOfMatchesQueryCore pins that FieldOf agrees with query/core.Field over
// the lifted record shape, including the missing-field ok=false case.
func TestFieldOfMatchesQueryCore(t *testing.T) {
	rec := recordWith(typ.Field{Name: "x", Type: typ.Number}, typ.Field{Name: "y", Type: typ.String})
	av := FromType(rec)

	for _, name := range []string{"x", "y", "absent"} {
		want, wantOK := querycore.Field(rec, name)
		got, gotOK := FieldOf(av, name)
		if gotOK != wantOK {
			t.Fatalf("FieldOf(%q) ok = %v, want %v", name, gotOK, wantOK)
		}
		if !wantOK {
			continue
		}
		assertProjects(t, "FieldOf "+name, got, want)
	}
}

// TestWithFieldReplacesSlot pins that WithField replaces an existing field slot
// and adds a fresh one, matching typ.ExtendRecordWithField (the value-domain
// field-write primitive the flow field transfer rebuilds the record with).
func TestWithFieldReplacesSlot(t *testing.T) {
	rec := recordWith(typ.Field{Name: "x", Type: typ.Number})
	av := FromType(rec)

	// Replace existing field.
	replaced := WithField(av, "x", FromType(typ.String))
	assertProjects(t, "WithField replace", replaced, typ.ExtendRecordWithField(rec, "x", typ.String))

	// Add a fresh field.
	added := WithField(av, "y", FromType(typ.Boolean))
	assertProjects(t, "WithField add", added, typ.ExtendRecordWithField(rec, "y", typ.Boolean))
}

// TestIndexOfMatchesQueryCore pins that IndexOf agrees with query/core.Index for
// array and map containers, including the optional element nilability that the
// read folds onto its result.
func TestIndexOfMatchesQueryCore(t *testing.T) {
	cases := []struct {
		name      string
		container typ.Type
		key       typ.Type
	}{
		{"array int index", typ.NewArray(typ.Number), typ.Integer},
		{"map string key", typ.NewMap(typ.String, typ.Number), typ.String},
		{"map placeholder key", typ.NewMap(typ.String, typ.Number), typ.Unknown},
	}
	for _, c := range cases {
		want, wantOK := querycore.Index(c.container, c.key)
		got, gotOK := IndexOf(FromType(c.container), FromType(c.key))
		if gotOK != wantOK {
			t.Fatalf("%s: ok = %v, want %v", c.name, gotOK, wantOK)
		}
		if !wantOK {
			continue
		}
		assertProjects(t, c.name, got, want)
	}
}

// TestWriteIndexMatchesValueDomain pins that WriteIndex agrees with the
// value-domain write law the flow map-mutator transfer composes
// (AdmitIndexedWrite then MergeForConvergence), and that the admission predicate
// agrees with value.IndexedWriteAdmits.
func TestWriteIndexMatchesValueDomain(t *testing.T) {
	cases := []struct {
		name      string
		container typ.Type
		key       typ.Type
		val       typ.Type
	}{
		{"empty record to map", recordWith(), typ.String, typ.Number},
		{"map widen value", typ.NewMap(typ.String, typ.Number), typ.String, typ.Boolean},
		{"map widen key", typ.NewMap(typ.String, typ.Number), typ.Integer, typ.Number},
	}
	for _, c := range cases {
		want := value.MergeForConvergence(c.container, value.AdmitIndexedWrite(c.container, c.key, c.val))
		got := WriteIndex(FromType(c.container), FromType(c.key), FromType(c.val))
		assertProjects(t, c.name, got, want)

		wantAdmits := value.IndexedWriteAdmits(c.container, c.key, c.val)
		gotAdmits := IndexWriteAdmits(FromType(c.container), FromType(c.key), FromType(c.val))
		if gotAdmits != wantAdmits {
			t.Fatalf("%s: IndexWriteAdmits = %v, want %v", c.name, gotAdmits, wantAdmits)
		}
	}
}

// TestMutateIndexMatchesValueDomain pins that MutateIndex agrees with the
// value-domain update law (AdmitIndexedValueMutation then MergeForConvergence)
// and that its admission predicate agrees with IndexedValueMutationAdmits.
func TestMutateIndexMatchesValueDomain(t *testing.T) {
	container := typ.NewMap(typ.String, recordWith(typ.Field{Name: "a", Type: typ.Number}))
	key := typ.String
	val := recordWith(typ.Field{Name: "b", Type: typ.String})

	want := value.MergeForConvergence(container, value.AdmitIndexedValueMutation(container, key, val))
	got := MutateIndex(FromType(container), FromType(key), FromType(val))
	assertProjects(t, "MutateIndex", got, want)

	wantAdmits := value.IndexedValueMutationAdmits(container, key, val)
	gotAdmits := IndexMutateAdmits(FromType(container), FromType(key), FromType(val))
	if gotAdmits != wantAdmits {
		t.Fatalf("IndexMutateAdmits = %v, want %v", gotAdmits, wantAdmits)
	}
}

// TestAppendElementMatchesValueDomain pins that AppendElement agrees with the
// value-domain array-element law the flow table-mutator transfer composes.
func TestAppendElementMatchesValueDomain(t *testing.T) {
	cases := []struct {
		name  string
		array typ.Type
		elem  typ.Type
	}{
		{"widen array element", typ.NewArray(typ.Number), typ.String},
		{"empty record to array", recordWith(), typ.Number},
	}
	for _, c := range cases {
		want := value.MergeForConvergence(c.array, value.AdmitArrayElementMutation(c.array, c.elem, typ.JoinPreferNonSoft))
		got := AppendElement(FromType(c.array), FromType(c.elem))
		assertProjects(t, c.name, got, want)
	}
}

// TestAppendMapElementMatchesValueDomain pins that AppendMapElement agrees with
// the value-domain map-array-element law for a keyed table-mutator target.
func TestAppendMapElementMatchesValueDomain(t *testing.T) {
	container := typ.NewMap(typ.String, typ.NewArray(typ.Number))
	key := typ.String
	elem := typ.String

	want := value.MergeForConvergence(container, value.AdmitMapArrayElementMutation(container, key, elem))
	got := AppendMapElement(FromType(container), FromType(key), FromType(elem))
	assertProjects(t, "AppendMapElement", got, want)
}

// TestPhiJoinMatchesProductJoin pins PhiJoin as the product Join reduction: empty
// is Bottom, a single operand is itself, and multiple operands fold under Join
// regardless of order (commutative/associative).
func TestPhiJoinMatchesProductJoin(t *testing.T) {
	if !Equal(PhiJoin(), Bottom()) {
		t.Fatal("PhiJoin() must be Bottom")
	}

	num := FromType(typ.Number)
	if !Equal(PhiJoin(num), num) {
		t.Fatal("PhiJoin of one operand must return it unchanged")
	}

	str := FromType(typ.String)
	boolean := FromType(typ.Boolean)
	joined := PhiJoin(num, str, boolean)
	want := Join(Join(num, str), boolean)
	if !Equal(joined, want) {
		t.Fatal("PhiJoin must fold operands under product Join")
	}

	// Order independence.
	reordered := PhiJoin(boolean, num, str)
	if !Equal(joined, reordered) {
		t.Fatal("PhiJoin must be order independent")
	}
}

// TestCarryForwardMatchesConvergenceMerge pins CarryForward as
// value.MergeForConvergence lifted onto AbstractValue, the store-merge the
// structured-carry transfer uses to seed a new version from predecessor facts.
func TestCarryForwardMatchesConvergenceMerge(t *testing.T) {
	prev := typ.NewArray(typ.Number)
	next := typ.NewArray(typ.String)
	want := value.MergeForConvergence(prev, next)
	got := CarryForward(FromType(prev), FromType(next))
	assertProjects(t, "CarryForward", got, want)
}

// TestNarrowTruthyMatchesNarrow pins NarrowTruthy as narrow.ToTruthy on the
// shape, with the presence transition (nilability removed) reflected natively.
func TestNarrowTruthyMatchesNarrow(t *testing.T) {
	cases := []typ.Type{
		typ.NewOptional(typ.String),
		typ.NewUnion(typ.String, typ.Nil),
		typ.Boolean,
		typ.Number,
	}
	for _, c := range cases {
		want := narrow.ToTruthy(c)
		got := NarrowTruthy(FromType(c))
		assertProjects(t, "NarrowTruthy "+c.String(), got, want)
	}

	// Narrowing a nilable value to truthy makes presence Present.
	got := NarrowTruthy(FromType(typ.NewOptional(typ.String)))
	if !presence.Equal(got.Presence(), presence.Present()) {
		t.Fatalf("NarrowTruthy presence = %s, want present", got.Presence())
	}
}

// TestNarrowFalsyMatchesNarrow pins NarrowFalsy as narrow.ToFalsy on the shape.
func TestNarrowFalsyMatchesNarrow(t *testing.T) {
	cases := []typ.Type{
		typ.NewOptional(typ.String),
		typ.Boolean,
		typ.NewUnion(typ.Number, typ.Nil),
	}
	for _, c := range cases {
		want := narrow.ToFalsy(c)
		got := NarrowFalsy(FromType(c))
		assertProjects(t, "NarrowFalsy "+c.String(), got, want)
	}
}

// TestNarrowPresentMatchesRemoveNil pins NarrowPresent as narrow.RemoveNil on the
// shape, and that the presence axis records the Maybe -> Present transition.
func TestNarrowPresentMatchesRemoveNil(t *testing.T) {
	opt := typ.NewOptional(typ.Number)
	want := narrow.RemoveNil(opt)
	got := NarrowPresent(FromType(opt))
	assertProjects(t, "NarrowPresent", got, want)
	if !presence.Equal(got.Presence(), presence.Present()) {
		t.Fatalf("NarrowPresent presence = %s, want present", got.Presence())
	}
}

// TestFilterAndExcludeByKindMatchNarrow pins the typeof narrowing primitives
// against narrow.FilterByKind / narrow.ExcludeKind.
func TestFilterAndExcludeByKindMatchNarrow(t *testing.T) {
	u := typ.NewUnion(typ.String, typ.Number)

	wantFilter := narrow.FilterByKind(u, kind.String)
	gotFilter := FilterByKind(FromType(u), kind.String)
	assertProjects(t, "FilterByKind", gotFilter, wantFilter)

	wantExclude := narrow.ExcludeKind(u, kind.String)
	gotExclude := ExcludeByKind(FromType(u), kind.String)
	assertProjects(t, "ExcludeByKind", gotExclude, wantExclude)
}

// TestRefineNumericMeet pins that RefineNumeric is the numeric-axis meet: it keeps
// the intersection of intervals, leaves shape/presence otherwise intact, and
// drives the whole value unreachable (presence Bottom via reduceNumericPresence)
// when the refinement is unsatisfiable.
func TestRefineNumericMeet(t *testing.T) {
	base := New(
		shapevalue.Of(typ.Integer),
		presence.Present(),
		numeric.Range(0, 100),
		effectrows.Top(), ownership.Top(), escape.Top(), identityrecursion.Top(), evidence.Top(),
	)

	refined := RefineNumeric(base, numeric.Range(10, 50))
	low, high := refined.Numeric().Interval()
	if low != 10 || high != 50 {
		t.Fatalf("RefineNumeric interval = [%d,%d], want [10,50]", low, high)
	}
	if refined.Presence().IsBottom() {
		t.Fatal("satisfiable refinement must stay reachable")
	}

	// Covered bound keeps the lower (covered) value.
	covered := RefineNumeric(base, numeric.Top())
	cl, ch := covered.Numeric().Interval()
	if cl != 0 || ch != 100 {
		t.Fatalf("meet with Top interval = [%d,%d], want [0,100]", cl, ch)
	}

	// Disjoint refinement is Bottom numeric, which reduces presence to Bottom.
	disjoint := RefineNumeric(base, numeric.Range(200, 300))
	if !disjoint.Numeric().IsBottom() {
		t.Fatal("disjoint numeric refinement must be Bottom")
	}
	if !disjoint.Presence().IsBottom() {
		t.Fatal("Bottom numeric must reduce presence to Bottom")
	}
}
