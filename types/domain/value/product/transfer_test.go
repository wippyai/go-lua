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
	"github.com/wippyai/go-lua/types/typ/unwrap"
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

func TestGradualAnyEvidenceSurvivesDynamicReads(t *testing.T) {
	field, ok := FieldOf(GradualAny(), "anything")
	if !ok || !field.IsGradualTop() {
		t.Fatalf("FieldOf(GradualAny) = %v/%v, want gradual-top evidence", field.ProjectValue(), ok)
	}

	index, ok := IndexOf(GradualAny(), FromType(typ.LiteralString("anything")))
	if !ok || !index.IsGradualTop() {
		t.Fatalf("IndexOf(GradualAny) = %v/%v, want gradual-top evidence", index.ProjectValue(), ok)
	}

	strict, ok := FieldOf(FromType(typ.Any), "anything")
	if !ok {
		t.Fatal("FieldOf(strict any) did not resolve")
	}
	if strict.IsGradualTop() {
		t.Fatal("strict declared any field read must not acquire gradual-top evidence")
	}
}

func TestGradualAnyEvidenceSurvivesKindRefinements(t *testing.T) {
	number := FilterByKind(GradualAny(), kind.Number)
	if !number.IsGradualTop() || !typ.TypeEquals(number.ProjectValue(), typ.Number) || !number.DefinitelyPresent() {
		t.Fatalf("FilterByKind(GradualAny, number) = %v gradual=%v present=%v, want gradual number",
			number.ProjectValue(), number.IsGradualTop(), number.DefinitelyPresent())
	}
	if !GradualAny().Covers(number) {
		t.Fatal("gradual source must semantically cover its type-refined product value")
	}

	nonNil := ExcludeByKind(GradualAny(), kind.Nil)
	if !nonNil.IsGradualTop() || !typ.TypeEquals(nonNil.ProjectValue(), typ.Any) || !nonNil.DefinitelyPresent() {
		t.Fatalf("ExcludeByKind(GradualAny, nil) = %v gradual=%v present=%v, want present gradual any",
			nonNil.ProjectValue(), nonNil.IsGradualTop(), nonNil.DefinitelyPresent())
	}

	nilOnly := FilterByKind(GradualAny(), kind.Nil)
	if !nilOnly.IsGradualTop() || !typ.TypeEquals(nilOnly.ProjectValue(), typ.Nil) {
		t.Fatalf("FilterByKind(GradualAny, nil) = %v gradual=%v, want gradual nil",
			nilOnly.ProjectValue(), nilOnly.IsGradualTop())
	}
}

func TestWithMemberStaticStringIndexDoesNotOverwriteDotField(t *testing.T) {
	base := FromType(typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Any).
		Build())

	got := WithMember(base, value.MemberStringIndex("raw-key"), FromType(typ.Number)).ProjectValue()
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("WithMember projected %T %[1]v, want record", got)
	}
	field := rec.GetField("name")
	if field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("dot field name = %#v, want string", field)
	}
	member := rec.GetStaticStringIndex("raw-key")
	if member == nil || !typ.TypeEquals(member.Type, typ.Number) {
		t.Fatalf("static member [\"raw-key\"] = %#v, want number", member)
	}
}

func TestStaticIndexInstallJoinedWithMissingBranchStaysOptional(t *testing.T) {
	message := typ.NewRecord().
		Field("_topic", typ.String).
		Field("topic", typ.Func().Param("self", typ.Any).Returns(typ.String).Build()).
		Build()
	base := FromType(typ.NewMap(typ.String, message))
	installed := WithMember(base, value.MemberStringIndex("root"), FromType(message))

	joined := Join(base, installed)
	rec, ok := joined.ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("joined static install projected %T %[1]v, want record carrier", joined.ProjectValue())
	}
	member := rec.GetStaticStringIndex("root")
	if member == nil {
		t.Fatalf("joined static install lost [\"root\"] member: %v", rec)
	}
	if !member.Optional {
		t.Fatalf("branch-local [\"root\"] install became definite after join: %v", rec)
	}

	got, ok := MemberOf(joined, value.MemberStringIndex("root"))
	want := typ.NewOptional(message)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("MemberOf(joined, [\"root\"]) = %v, %v; want %v,true", got.ProjectValue(), ok, want)
	}
}

func TestFieldOf_PartialRecordUnionKeepsMissingFieldOptionality(t *testing.T) {
	action := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("y", typ.String).Build(),
	)
	okVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", action).
		Build()
	errVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()

	got, ok := FieldOf(FromType(typ.NewUnion(okVariant, errVariant)), "value")
	want := typ.NewOptional(action)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("FieldOf(value) = %v, %v; want %v,true", got.ProjectValue(), ok, want)
	}
}

func TestMemberOfDistinguishesFieldAndStaticStringIndex(t *testing.T) {
	action := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("y", typ.String).Build(),
	)
	okVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", action).
		Build()
	errVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	av := FromType(typ.NewUnion(okVariant, errVariant))

	fieldRead, fieldOK := MemberOf(av, value.MemberField("value"))
	wantField := typ.NewOptional(action)
	if !fieldOK || !typ.TypeEquals(fieldRead.ProjectValue(), wantField) {
		t.Fatalf("MemberField(value) = %v, %v; want %v,true", fieldRead.ProjectValue(), fieldOK, wantField)
	}
	if indexRead, indexOK := MemberOf(av, value.MemberStringIndex("value")); indexOK {
		t.Fatalf("MemberStringIndex(value) = %v, true; want unresolved exact index on missing union member", indexRead.ProjectValue())
	}
}

func TestRuntimeMemberOfMissingFieldReadsNil(t *testing.T) {
	av := FromType(typ.NewRecord().Field("data", typ.NewRecord().Build()).Build())
	data, ok := RuntimeMemberOf(av, value.MemberField("data"))
	if !ok {
		t.Fatal("RuntimeMemberOf(data) did not resolve present field")
	}
	got, ok := RuntimeMemberOf(data, value.MemberField("max_tokens"))
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Nil) {
		t.Fatalf("RuntimeMemberOf(missing) = %v, %v; want nil,true", got.ProjectValue(), ok)
	}
}

func TestRuntimeMemberOfPartialUnionKeepsPresentAndMissingVariants(t *testing.T) {
	action := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("y", typ.String).Build(),
	)
	okVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", action).
		Build()
	errVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	av := FromType(typ.NewUnion(okVariant, errVariant))

	got, ok := RuntimeMemberOf(av, value.MemberStringIndex("value"))
	want := typ.NewOptional(action)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("RuntimeMemberOf([value]) = %v, %v; want %v,true", got.ProjectValue(), ok, want)
	}
}

func TestRefineCallableValuePreservesOptionalPresence(t *testing.T) {
	base := FromType(typ.NewOptional(typ.Func().Returns(typ.Unknown).Build()))
	sig := typ.Func().Returns(typ.String).Build()

	got := RefineCallableValue(base, sig)
	inner, optional := typ.SplitNilableFieldType(got.ProjectValue())
	if !optional {
		t.Fatalf("RefineCallableValue = %v, want optional function", got.ProjectValue())
	}
	fn, ok := inner.(*typ.Function)
	if !ok {
		t.Fatalf("RefineCallableValue inner = %T, want function", inner)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want [string]", fn.Returns)
	}
}

func TestRefineCallableValueKeepsDefiniteCallableDefinite(t *testing.T) {
	base := FromType(typ.Func().Returns(typ.Unknown).Build())
	sig := typ.Func().Returns(typ.String).Build()

	got := RefineCallableValue(base, sig)
	if _, optional := typ.SplitNilableFieldType(got.ProjectValue()); optional {
		t.Fatalf("RefineCallableValue = %v, want definite function", got.ProjectValue())
	}
	assertProjects(t, "RefineCallableValue definite", got, sig)
}

func TestRefineCallableValueDoesNotInventCallableFromNil(t *testing.T) {
	got := RefineCallableValue(FromType(typ.Nil), typ.Func().Returns(typ.String).Build())
	assertProjects(t, "RefineCallableValue nil", got, typ.Nil)
}

func TestRefineCallableReadTreatsNilAsOptionalCallable(t *testing.T) {
	sig := typ.Func().Returns(typ.String).Build()
	got := RefineCallableRead(FromType(typ.Nil), sig)
	inner, optional := typ.SplitNilableFieldType(got.ProjectValue())
	if !optional {
		t.Fatalf("RefineCallableRead(nil) = %v, want optional callable", got.ProjectValue())
	}
	if fn := unwrap.Function(inner); fn == nil {
		t.Fatalf("RefineCallableRead(nil) inner = %v, want function", inner)
	}
}

func TestRefineCallableReadDoesNotMakeAbsentReadDefinite(t *testing.T) {
	sig := typ.Func().Returns(typ.String).Build()
	got := RefineCallableRead(AbstractValue{}, sig)
	inner, optional := typ.SplitNilableFieldType(got.ProjectValue())
	if !optional {
		t.Fatalf("RefineCallableRead(absent) = %v, want optional callable", got.ProjectValue())
	}
	if fn := unwrap.Function(inner); fn == nil {
		t.Fatalf("RefineCallableRead(absent) inner = %v, want function", inner)
	}
}

func TestRefineCallableReadKeepsDefiniteCallableDefinite(t *testing.T) {
	base := FromType(typ.Func().Returns(typ.Unknown).Build())
	sig := typ.Func().Returns(typ.String).Build()
	got := RefineCallableRead(base, sig)
	if _, optional := typ.SplitNilableFieldType(got.ProjectValue()); optional {
		t.Fatalf("RefineCallableRead(definite) = %v, want definite callable", got.ProjectValue())
	}
	assertProjects(t, "RefineCallableRead definite", got, sig)
}

func TestWithMetatableAttachesPrototypeLookup(t *testing.T) {
	prototype := typ.NewRecord().
		OptField("_session_id", typ.String).
		Field("all", typ.Func().Returns(typ.NewArray(typ.String), typ.Nil).Build()).
		Build()
	meta := typ.NewRecord().Field("__index", prototype).Build()
	instance := FromType(typ.NewRecord().Build())

	got, ok := WithMetatable(instance, FromType(meta))
	if !ok {
		t.Fatal("WithMetatable did not produce a value")
	}
	rec, ok := got.ProjectValue().(*typ.Record)
	if !ok || rec.Metatable == nil {
		t.Fatalf("WithMetatable projected %T %v, want record with metatable", got.ProjectValue(), got.ProjectValue())
	}

	field, ok := RuntimeMemberOf(got, value.MemberField("_session_id"))
	wantField := typ.NewOptional(typ.String)
	if !ok || !typ.TypeEquals(field.ProjectValue(), wantField) {
		t.Fatalf("prototype field read = %v, %v; want %v,true", field.ProjectValue(), ok, wantField)
	}
	method, ok := RuntimeMemberOf(got, value.MemberField("all"))
	if !ok {
		t.Fatal("prototype method read did not resolve")
	}
	if _, ok := method.ProjectValue().(*typ.Function); !ok {
		t.Fatalf("prototype method read = %T %v, want function", method.ProjectValue(), method.ProjectValue())
	}
}

func TestNarrowLengthLowerBoundFiltersImpossibleContainerShapes(t *testing.T) {
	row := typ.NewRecord().Field("text", typ.String).Build()
	rowsArray := typ.NewArray(row)
	base := FromType(typ.NewUnion(typ.NewRecord().Build(), rowsArray, typ.Nil))

	got := NarrowLengthLowerBound(base, 1)
	if !typ.TypeEquals(got.ProjectValue(), rowsArray) {
		t.Fatalf("NarrowLengthLowerBound({}|array|nil, 1) = %v, want %v", got.ProjectValue(), rowsArray)
	}

	empty := NarrowLengthLowerBound(FromType(typ.NewRecord().Build()), 1)
	if !typ.IsNever(empty.ProjectValue()) {
		t.Fatalf("NarrowLengthLowerBound({}, 1) = %v, want never", empty.ProjectValue())
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

func TestWriteIndexForeignFreshEmptyTableSeparatesExactWriteFromIteratorTail(t *testing.T) {
	payload := typ.NewRecord().
		Field("created_at", typ.String).
		Field("last_activity", typ.NewOptional(typ.String)).
		Build()

	got := WriteIndexForeign(FromType(typ.NewFreshEmptyRecord()), FromType(typ.LiteralString("s1")), FromType(payload))
	projected := got.ProjectValue()
	iter := querycore.EntryValueType(projected)
	if !typ.TypeEquals(iter, typ.Any) {
		t.Fatalf("EntryValueType(WriteIndexForeign(fresh{})) = %v, want any; projected=%v", iter, projected)
	}
	exact, ok := querycore.Index(projected, typ.LiteralString("s1"))
	if !ok || !typ.TypeEquals(exact, payload) {
		t.Fatalf("exact read after WriteIndexForeign(fresh{}) = %v/%v, want %v/true; projected=%v", exact, ok, payload, projected)
	}
}

func TestWriteIndexForeignFreshEmptyTableDynamicKeyLearnsIteratorTail(t *testing.T) {
	payload := typ.NewRecord().
		Field("created_at", typ.Number).
		Field("last_activity", typ.Number).
		Build()

	got := WriteIndexForeign(FromType(typ.NewFreshEmptyRecord()), FromType(typ.String), FromType(payload))
	projected := got.ProjectValue()
	iter := querycore.EntryValueType(projected)
	if !typ.TypeEquals(iter, payload) {
		t.Fatalf("EntryValueType(WriteIndexForeign(fresh{}, string)) = %v, want %v; projected=%v", iter, payload, projected)
	}
}

func TestNestedFreshEmptyTableStaticWriteDoesNotInferIteratorTail(t *testing.T) {
	payload := typ.NewRecord().
		Field("created_at", typ.String).
		Field("last_activity", typ.NewOptional(typ.String)).
		Build()
	state := FromType(typ.NewRecord().
		Field("active_sessions", typ.NewFreshEmptyRecord()).
		Build())

	child, ok := RuntimeMemberOf(state, value.MemberField("active_sessions"))
	if !ok {
		t.Fatalf("active_sessions field did not resolve from %v", state.ProjectValue())
	}
	if !child.IsFreshAllocation() {
		t.Fatalf("nested fresh table lost freshness before write: %v", child.ProjectValue())
	}

	updatedChild := WithMember(child, value.MemberStringIndex("s1"), FromType(payload))
	updatedState := WithMember(state, value.MemberField("active_sessions"), updatedChild)
	projectedChild, ok := RuntimeMemberOf(updatedState, value.MemberField("active_sessions"))
	if !ok {
		t.Fatalf("active_sessions field missing after write: %v", updatedState.ProjectValue())
	}
	projected := projectedChild.ProjectValue()
	iter := querycore.EntryValueType(projected)
	if !typ.TypeEquals(iter, typ.Any) {
		t.Fatalf("EntryValueType(nested fresh table after static write) = %v, want any; projected=%v", iter, projected)
	}
	exact, ok := querycore.Index(projected, typ.LiteralString("s1"))
	if !ok || !typ.TypeEquals(exact, payload) {
		t.Fatalf("exact nested [\"s1\"] read = %v/%v, want %v/true; projected=%v", exact, ok, payload, projected)
	}
}

func TestSealedIndexWriteAdmitsUsesDeclaredObligation(t *testing.T) {
	rec := typ.NewRecord().
		Field("count", typ.Integer).
		Field("name", typ.String).
		Build()

	if SealedIndexWriteAdmits(FromType(rec), FromType(typ.String), FromType(typ.Integer)) {
		t.Fatal("sealed heterogeneous record must reject broad string integer write")
	}
	if !SealedIndexWriteAdmits(FromType(rec), FromType(typ.LiteralString("count")), FromType(typ.Integer)) {
		t.Fatal("sealed record must admit compatible exact count write")
	}
	if !IndexWriteAdmits(FromType(rec), FromType(typ.String), FromType(typ.Integer)) {
		t.Fatal("mutable admission law should still allow weakening the same record")
	}
}

func TestWriteSelfDerivedIndexPreservesHeterogeneousRecord(t *testing.T) {
	rec := typ.NewRecord().
		Field("count", typ.LiteralInt(1)).
		Field("name", typ.LiteralString("ready")).
		Build()
	key := typ.NewUnion(typ.LiteralString("count"), typ.LiteralString("name"))
	val := typ.NewUnion(typ.LiteralInt(1), typ.LiteralString("ready"))

	got := WriteSelfDerivedIndex(FromType(rec), FromType(key), FromType(val))
	assertProjects(t, "WriteSelfDerivedIndex", got, rec)
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
		{
			"fresh array to structured element",
			typ.NewFreshArray(),
			typ.NewRecord().Field("routes", typ.NewFreshArray()).Build(),
		},
	}
	for _, c := range cases {
		want := value.MergeForConvergence(c.array, value.AdmitArrayElementMutation(c.array, c.elem, value.JoinContainerValueTypes))
		got := AppendElement(FromType(c.array), FromType(c.elem))
		assertProjects(t, c.name, got, want)
	}
}

func TestJoinConditionalAppendKeepsFreshArrayElementShape(t *testing.T) {
	elem := typ.NewRecord().
		Field("routes", typ.NewFreshArray()).
		Build()
	empty := FromType(typ.NewFreshArray())
	appended := AppendElement(empty, FromType(elem))

	got := Join(empty, appended).ProjectValue()
	arr, ok := unwrap.Alias(got).(*typ.Array)
	if !ok {
		t.Fatalf("Join(empty, appended) = %T %[1]v, want array", got)
	}
	rec, ok := unwrap.Alias(arr.Element).(*typ.Record)
	if !ok || rec.GetField("routes") == nil {
		t.Fatalf("joined element = %T %[1]v, want record with routes", arr.Element)
	}
}

func TestAppendElementCollapsesCompatibleCommandPayloads(t *testing.T) {
	createData := typ.LiteralString("CREATE_DATA")
	nodeInput := typ.LiteralString("NODE_INPUT")
	initial := typ.NewRecord().
		Field("type", createData).
		Field("payload", typ.NewRecord().
			Field("data_id", typ.String).
			Field("data_type", nodeInput).
			Field("content", typ.String).
			Field("content_type", typ.LiteralString("text/plain")).
			Build()).
		Build()
	next := typ.NewRecord().
		Field("type", createData).
		Field("payload", typ.NewRecord().
			Field("data_id", typ.String).
			Field("data_type", nodeInput).
			Field("node_id", typ.String).
			Field("key", typ.String).
			Field("content", typ.LiteralString("")).
			Field("content_type", typ.LiteralString("dataflow/reference")).
			Build()).
		Build()

	appended := AppendElement(FromType(typ.NewArray(initial)), FromType(next))
	got := appended.ProjectValue()
	arr, ok := unwrap.Alias(got).(*typ.Array)
	if !ok {
		t.Fatalf("AppendElement command payloads = %T %[1]v, want array", got)
	}
	elem, ok := unwrap.Alias(arr.Element).(*typ.Record)
	if !ok {
		t.Fatalf("command element = %T %[1]v, want collapsed record", arr.Element)
	}
	if field := elem.GetField("type"); field == nil || !typ.TypeEquals(field.Type, createData) {
		t.Fatalf("command type field = %v, want CREATE_DATA literal", field)
	}
	payloadField := elem.GetField("payload")
	if payloadField == nil {
		t.Fatalf("command payload field missing in %v", elem)
	}
	payload, ok := unwrap.Alias(payloadField.Type).(*typ.Record)
	if !ok {
		t.Fatalf("command payload = %T %[1]v, want collapsed record", payloadField.Type)
	}
	if field := payload.GetField("data_type"); field == nil || !typ.TypeEquals(field.Type, nodeInput) {
		t.Fatalf("payload data_type = %v, want NODE_INPUT literal", field)
	}
	if field := payload.GetField("node_id"); field == nil || !field.Optional {
		t.Fatalf("payload node_id = %v, want optional field", field)
	}
	if field := payload.GetField("content"); field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("payload content = %v, want string widened from literal", field)
	}

	third := typ.NewRecord().
		Field("type", createData).
		Field("payload", typ.NewRecord().
			Field("data_id", typ.String).
			Field("data_type", nodeInput).
			Field("content", typ.LiteralString("{}")).
			Field("content_type", typ.LiteralString("application/json")).
			Build()).
		Build()
	got = AppendElement(appended, FromType(third)).ProjectValue()
	arr, ok = unwrap.Alias(got).(*typ.Array)
	if !ok {
		t.Fatalf("AppendElement third command payload = %T %[1]v, want array", got)
	}
	elem, ok = unwrap.Alias(arr.Element).(*typ.Record)
	if !ok {
		t.Fatalf("third command element = %T %[1]v, want collapsed record", arr.Element)
	}
	payloadField = elem.GetField("payload")
	if payloadField == nil {
		t.Fatalf("third command payload field missing in %v", elem)
	}
	payload, ok = unwrap.Alias(payloadField.Type).(*typ.Record)
	if !ok {
		t.Fatalf("third command payload = %T %[1]v, want collapsed record", payloadField.Type)
	}
	if field := payload.GetField("content_type"); field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("third payload content_type = %v, want string widened from accumulated literals", field)
	}
}

func TestAppendElementPreservesDiscriminatedCommandVariants(t *testing.T) {
	createData := typ.NewRecord().
		Field("type", typ.LiteralString("CREATE_DATA")).
		Field("payload", typ.NewRecord().Field("data_id", typ.String).Build()).
		Build()
	deleteData := typ.NewRecord().
		Field("type", typ.LiteralString("DELETE_DATA")).
		Field("payload", typ.NewRecord().Field("data_id", typ.String).Build()).
		Build()

	got := AppendElement(FromType(typ.NewArray(createData)), FromType(deleteData)).ProjectValue()
	arr, ok := unwrap.Alias(got).(*typ.Array)
	if !ok {
		t.Fatalf("AppendElement discriminants = %T %[1]v, want array", got)
	}
	union, ok := unwrap.Alias(arr.Element).(*typ.Union)
	if !ok || len(union.Members) != 2 {
		t.Fatalf("command variants = %T %[1]v, want two-member discriminated union", arr.Element)
	}
}

func TestAppendElementReplacesEmptyRecordSeedWithArray(t *testing.T) {
	got := AppendElement(FromType(typ.NewRecord().Build()), FromType(typ.String)).ProjectValue()
	want := typ.NewArray(typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AppendElement(empty record seed, string) = %v, want %v", got, want)
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

// TestContainerElementUnionMatchesValueDomain pins that the product-level
// ContainerElementUnion primitive agrees with the value-domain law used for
// channel/send-like mutation effects.
func TestContainerElementUnionMatchesValueDomain(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())
	container := typ.Instantiate(channel, typ.Unknown)
	elem := typ.String

	want := value.MergeForConvergence(container, value.AdmitContainerElementUnion(container, elem))
	got := ContainerElementUnion(FromType(container), FromType(elem))
	assertProjects(t, "ContainerElementUnion", got, want)
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

	dynamic := NarrowTruthy(FromType(typ.Any))
	if !typ.TypeEquals(dynamic.ProjectValue(), typ.Any) || !dynamic.DefinitelyPresent() {
		t.Fatalf("NarrowTruthy(any) = %s presence=%s, want present any", dynamic.ProjectValue(), dynamic.Presence())
	}
	gradual := NarrowTruthy(GradualAny())
	if !gradual.IsGradualTop() || !typ.TypeEquals(gradual.ProjectValue(), typ.Any) || !gradual.DefinitelyPresent() {
		t.Fatalf("NarrowTruthy(GradualAny) = %s gradual=%v presence=%s, want present gradual any", gradual.ProjectValue(), gradual.IsGradualTop(), gradual.Presence())
	}

	svc := typ.NewAlias("Svc", typ.NewRecord().
		Field("go", typ.Func().Build()).
		Build())
	alias := NarrowTruthy(FromType(typ.NewOptional(svc)))
	if _, optional := typ.SplitNilableFieldType(alias.ProjectValue()); optional || !alias.DefinitelyPresent() {
		t.Fatalf("NarrowTruthy(Svc?) = %s presence=%s, want present non-optional alias", alias.ProjectValue(), alias.Presence())
	}

	recursiveSvc := typ.NewRecursive("Svc", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("go", typ.Func().Param("self", self).Build()).
			Build()
	})
	recursiveBase := FromType(typ.NewOptional(recursiveSvc))
	recursive := NarrowTruthy(recursiveBase)
	if !recursiveBase.Covers(recursive) {
		t.Fatalf("NarrowTruthy(recursive Svc?) = %s is not covered by base %s", recursive.ProjectValue(), recursiveBase.ProjectValue())
	}
	if !recursive.DefinitelyPresent() {
		t.Fatalf("NarrowTruthy(recursive Svc?) presence=%s, want present", recursive.Presence())
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

	dynamic := NarrowPresent(FromType(typ.Any))
	if !typ.TypeEquals(dynamic.ProjectValue(), typ.Any) || !dynamic.DefinitelyPresent() {
		t.Fatalf("NarrowPresent(any) = %s presence=%s, want present any", dynamic.ProjectValue(), dynamic.Presence())
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
