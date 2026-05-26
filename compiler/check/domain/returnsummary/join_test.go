package returnsummary

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

func TestJoinReturnVectors_Empty(t *testing.T) {
	result := typjoin.ReturnVectors(nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestJoinReturnVectors_AEmpty(t *testing.T) {
	b := []typ.Type{typ.String}
	result := typjoin.ReturnVectors(nil, b)
	if len(result) != 1 || result[0] != typ.String {
		t.Errorf("expected [string], got %v", result)
	}
}

func TestJoinReturnVectors_BEmpty(t *testing.T) {
	a := []typ.Type{typ.Number}
	result := typjoin.ReturnVectors(a, nil)
	if len(result) != 1 || result[0] != typ.Number {
		t.Errorf("expected [number], got %v", result)
	}
}

func TestJoinReturnVectors_SameLength(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.Number}
	result := typjoin.ReturnVectors(a, b)
	if len(result) != 1 {
		t.Errorf("expected length 1, got %d", len(result))
	}
}

func TestTypJoinReturnSlot_PreservesUnknownOverNil(t *testing.T) {
	got := typ.JoinReturnSlot(typ.Unknown, typ.Nil)
	if !typ.TypeEquals(got, typ.Unknown) {
		t.Fatalf("typ.JoinReturnSlot(unknown, nil) = %v, want unknown", got)
	}

	got = typ.JoinReturnSlot(typ.Nil, typ.Unknown)
	if !typ.TypeEquals(got, typ.Unknown) {
		t.Fatalf("typ.JoinReturnSlot(nil, unknown) = %v, want unknown", got)
	}
}

func TestReturnSummaryAllNil(t *testing.T) {
	if !AllNil([]typ.Type{typ.Nil}) {
		t.Fatal("expected [nil] to be nil-only")
	}
	if AllNil([]typ.Type{typ.Nil, typ.Unknown}) {
		t.Fatal("expected [nil, unknown] to not be nil-only")
	}
	if AllNil(nil) {
		t.Fatal("expected empty return vector to not be nil-only")
	}
}

func TestReturnSummaryEqual_UsesRecursiveConvergenceEquality(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Build()
	})
	different := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("case")).
			Field("children", typ.NewArray(self)).
			Build()
	})

	if !Equal([]typ.Type{left}, []typ.Type{right}) {
		t.Fatal("equivalent recursive return summaries should compare equal")
	}
	if Equal([]typ.Type{left}, []typ.Type{different}) {
		t.Fatal("recursive return summary equality must still distinguish facts")
	}
}

func TestReturnSummaryMerge_ExplicitNilBranchStaysOptional(t *testing.T) {
	got := Merge([]typ.Type{typ.Nil}, []typ.Type{typ.String})
	want := []typ.Type{typ.NewOptional(typ.String)}
	if !Equal(got, want) {
		t.Fatalf("Merge([nil], [string]) = %v, want %v", got, want)
	}
}

func TestReturnSummaryMerge_EmptyTableBranchDoesNotHideSequenceReturn(t *testing.T) {
	empty := typ.NewRecord().Build()
	array := typ.NewArray(typ.Any)

	got := Merge([]typ.Type{empty}, []typ.Type{array})
	want := []typ.Type{array}
	if !Equal(got, want) {
		t.Fatalf("Merge([{}], [any[]]) = %v, want %v", got, want)
	}

	got = Merge([]typ.Type{array}, []typ.Type{empty})
	if !Equal(got, want) {
		t.Fatalf("Merge([any[]], [{}]) = %v, want %v", got, want)
	}
}

func TestReturnSummaryMerge_MixedArityPadsMissingSlotsWithNil(t *testing.T) {
	dbType := typ.NewRecord().Field("query", typ.Func().Returns(typ.Any).Build()).Build()
	got := Merge([]typ.Type{dbType}, []typ.Type{typ.Nil, typ.LuaError})
	want := []typ.Type{typ.NewOptional(dbType), typ.NewOptional(typ.LuaError)}
	if !Equal(got, want) {
		t.Fatalf("Merge([db], [nil, err]) = %v, want %v", got, want)
	}
}

func TestReturnSummaryMerge_ShorterRefinementKeepsExistingNilability(t *testing.T) {
	dbType := typ.NewRecord().Field("query", typ.Func().Returns(typ.Any).Build()).Build()
	baseline := []typ.Type{typ.NewOptional(dbType), typ.NewOptional(typ.LuaError)}
	got := Merge(baseline, []typ.Type{dbType})
	if !Equal(got, baseline) {
		t.Fatalf("Merge([db?, err?], [db]) = %v, want %v", got, baseline)
	}
}

func TestReturnSummaryMerge_RecursiveProductJoinKeepsBranchFieldsOptional(t *testing.T) {
	base := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	withProc := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("proc", typ.Any).
			Build()
	})

	got := Merge([]typ.Type{base}, []typ.Type{withProc})
	if len(got) != 1 {
		t.Fatalf("Merge returned %d slots, want 1: %v", len(got), got)
	}
	if _, ok := got[0].(*typ.Union); ok {
		t.Fatalf("recursive return merge produced raw union: %v", got[0])
	}
	rec, ok := got[0].(*typ.Recursive)
	if !ok {
		t.Fatalf("recursive return merge = %T %[1]v, want recursive product", got[0])
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	proc := body.GetField("proc")
	if proc == nil || !proc.Optional {
		t.Fatalf("proc must stay optional because only one branch returned it: %v", body)
	}
}

func TestReturnSummaryMerge_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()

	got := Merge([]typ.Type{seed}, []typ.Type{solved})
	if len(got) != 1 || !typ.TypeEquals(got[0], solved) {
		t.Fatalf("Merge(function seed, solved function) = %v, want [%v]", got, solved)
	}

	got = Merge([]typ.Type{solved}, []typ.Type{seed})
	if len(got) != 1 || !typ.TypeEquals(got[0], solved) {
		t.Fatalf("Merge(solved function, function seed) = %v, want [%v]", got, solved)
	}
}

func TestReturnSummaryMerge_ReplacesUnsolvedFunctionSeedInsideRecordField(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	weak := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", seed).
		Build()
	strong := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", solved).
		Build()

	got := Merge([]typ.Type{weak}, []typ.Type{strong})
	if len(got) != 1 {
		t.Fatalf("Merge returned %d slots, want 1: %v", len(got), got)
	}
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("Merge(record seed, record solved) = %T %[1]v, want record", got[0])
	}
	field := rec.GetField("get_x")
	if field == nil || !typ.TypeEquals(field.Type, solved) {
		t.Fatalf("merged get_x = %v, want %v", field, solved)
	}
}

func TestReturnSummaryMerge_RefinesUnknownMetatableEvidence(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	weak := typ.NewRecord().Metatable(typ.Unknown).Build()
	strong := typ.NewRecord().Metatable(metatable).Build()

	got := Merge([]typ.Type{weak}, []typ.Type{strong})
	if len(got) != 1 {
		t.Fatalf("Merge returned %d slots, want 1: %v", len(got), got)
	}
	if mt, ok := querycore.Method(got[0], "ready"); !ok {
		t.Fatalf("merged metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}

	got = Merge([]typ.Type{strong}, []typ.Type{weak})
	if len(got) != 1 {
		t.Fatalf("reverse Merge returned %d slots, want 1: %v", len(got), got)
	}
	if mt, ok := querycore.Method(got[0], "ready"); !ok {
		t.Fatalf("reverse merged metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}
}

func TestReturnSummaryWiden_RefinesUnknownMetatableEvidence(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	weak := typ.NewRecord().Metatable(typ.Unknown).Build()
	strong := typ.NewRecord().Metatable(metatable).Build()

	got := WidenForConvergence([]typ.Type{weak}, []typ.Type{strong})
	if len(got) != 1 {
		t.Fatalf("WidenForConvergence returned %d slots, want 1: %v", len(got), got)
	}
	if mt, ok := querycore.Method(got[0], "ready"); !ok {
		t.Fatalf("widened metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}

	got = WidenForConvergence([]typ.Type{strong}, []typ.Type{weak})
	if len(got) != 1 {
		t.Fatalf("reverse WidenForConvergence returned %d slots, want 1: %v", len(got), got)
	}
	if mt, ok := querycore.Method(got[0], "ready"); !ok {
		t.Fatalf("reverse widened metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}
}

func TestReturnSummaryWiden_ReplacesEmptySeedWithMetatableEvidence(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	seed := typ.NewRecord().Build()
	observed := typ.NewRecord().Metatable(metatable).Build()

	got := WidenForConvergence([]typ.Type{seed, typ.Nil}, []typ.Type{observed, typ.Nil})
	if len(got) != 2 {
		t.Fatalf("WidenForConvergence returned %d slots, want 2: %v", len(got), got)
	}
	if mt, ok := querycore.Method(got[0], "ready"); !ok {
		t.Fatalf("widened metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}
}

func TestReturnSummaryWiden_RefinesUnknownMetatableWithSelfReturningPrototype(t *testing.T) {
	weak := typ.NewRecord().Metatable(typ.Unknown).Build()
	query := typ.NewRecord().Build()
	contexts := typ.Func().Param("self", typ.Unknown).Returns(query).Build()
	prototype := typ.NewRecord().Field("contexts", contexts).Build()
	metatable := typ.NewRecord().
		Field("__index", prototype).
		Field("contexts", contexts).
		Build()
	strong := typ.NewRecord().Metatable(metatable).Build()

	got := WidenForConvergence([]typ.Type{weak, typ.Nil}, []typ.Type{strong, typ.Nil})
	if len(got) != 2 {
		t.Fatalf("WidenForConvergence returned %d slots, want 2: %v", len(got), got)
	}
	if mt, ok := querycore.Method(got[0], "contexts"); !ok {
		t.Fatalf("widened metatable method contexts = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}
}

func TestReturnSummaryWiden_ReplacesUnsolvedFunctionSeedInsideRecordField(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	weak := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", seed).
		Build()
	strong := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", solved).
		Build()

	got := WidenForConvergence([]typ.Type{weak}, []typ.Type{strong})
	if len(got) != 1 {
		t.Fatalf("WidenForConvergence returned %d slots, want 1: %v", len(got), got)
	}
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("WidenForConvergence(record seed, record solved) = %T %[1]v, want record", got[0])
	}
	field := rec.GetField("get_x")
	if field == nil || !typ.TypeEquals(field.Type, solved) {
		t.Fatalf("widened get_x = %v, want %v", field, solved)
	}
}

func TestReturnSummaryMerge_RecursiveEquivalenceUsesWideningNotDeepEquality(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})

	if !typ.TypeEquals(left, right) {
		t.Fatalf("test setup expected structurally equivalent recursive products")
	}
	if sameReturnSlotWithoutRecursiveDescent(left, right) {
		t.Fatalf("recursive return slots must enter convergence widening instead of deep equality")
	}
	if !sameReturnSlotWithoutRecursiveDescent(typ.NewArray(typ.String), typ.NewArray(typ.String)) {
		t.Fatalf("non-recursive equal return slots should still use the cheap equality gate")
	}
}

func TestReturnSummaryWiden_NoReturnNilRefinesUnknownSeed(t *testing.T) {
	got := WidenForConvergence([]typ.Type{typ.Unknown}, []typ.Type{typ.Nil})
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Nil) {
		t.Fatalf("WidenForConvergence([unknown], [nil]) = %v, want [nil]", got)
	}
}

func TestReturnSummaryWiden_ConcreteEvidenceReplacesUnknownSeed(t *testing.T) {
	obj := typ.NewRecord().Field("x", typ.Number).Build()
	got := WidenForConvergence([]typ.Type{typ.Unknown}, []typ.Type{obj})
	if len(got) != 1 || !typ.TypeEquals(got[0], obj) {
		t.Fatalf("WidenForConvergence([unknown], [record]) = %v, want [%v]", got, obj)
	}
}

func TestJoinReturnVectors_DifferentLengths(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.Boolean}
	result := typjoin.ReturnVectors(a, b)
	if len(result) != 2 {
		t.Errorf("expected length 2, got %d", len(result))
	}
}

func TestReturnSummaryEqual_Empty(t *testing.T) {
	if !Equal(nil, nil) {
		t.Error("nil slices should be equal")
	}
}

func TestReturnSummaryEqual_DifferentLength(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String, typ.Number}
	if Equal(a, b) {
		t.Error("different lengths should not be equal")
	}
}

func TestReturnSummaryEqual_Same(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.String, typ.Number}
	if !Equal(a, b) {
		t.Error("same types should be equal")
	}
}

func TestReturnSummaryEqual_Different(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.Number}
	if Equal(a, b) {
		t.Error("different types should not be equal")
	}
}

func TestReturnSummaryEqual_IncludesMetatableFactState(t *testing.T) {
	withoutSpec := returnSummaryMetatabledRecord(false)
	withSpec := returnSummaryMetatabledRecord(true)
	if !typ.TypeEquals(withoutSpec, withSpec) {
		t.Fatal("ordinary type equality should ignore method spec inside metatable")
	}
	if Equal([]typ.Type{withoutSpec}, []typ.Type{withSpec}) {
		t.Fatal("return-summary equality must include metatable fact state")
	}
}

func returnSummaryMetatabledRecord(withSpec bool) typ.Type {
	method := typ.Func().Returns(typ.String)
	if withSpec {
		method = method.Spec(contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}))
	}
	metatable := typ.NewRecord().
		Field("__index", typ.NewRecord().Field("run", method.Build()).Build()).
		Build()
	return typ.NewRecord().Metatable(metatable).Build()
}

func TestReturnSummaryRefines_EmptyA(t *testing.T) {
	b := []typ.Type{typ.String}
	if Refines(nil, b) {
		t.Error("empty a should not refine b")
	}
}

func TestReturnSummaryRefines_EmptyB(t *testing.T) {
	a := []typ.Type{typ.String}
	if !Refines(a, nil) {
		t.Error("a should refine empty b")
	}
}

func TestReturnSummaryRefines_Same(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String}
	if !Refines(a, b) {
		t.Error("same types should refine")
	}
}

func TestReturnSummaryRefines_DifferentLength(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.String}
	if Refines(a, b) {
		t.Error("different lengths should not refine")
	}
}

func TestReturnSummaryMerge_ReplacesStaleFalsyKeyArrayElement(t *testing.T) {
	stale := []typ.Type{typ.NewArray(typ.NewUnion(typ.Boolean, typ.String))}
	current := []typ.Type{typ.NewArray(typ.String)}

	got := Merge(stale, current)
	if !Equal(got, current) {
		t.Fatalf("expected truthy-refined key array %v, got %v", current, got)
	}
}

func TestReturnSummaryExtendsRecord_Empty(t *testing.T) {
	if ExtendsRecord(nil, nil) {
		t.Error("empty vectors should not extend")
	}
}

func TestReturnSummaryExtendsRecord_NotRecords(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String}
	if ExtendsRecord(a, b) {
		t.Error("non-records should not extend")
	}
}

func TestReturnSummaryExtendsRecord_RecordExtends(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ExtendsRecord(a, b) {
		t.Error("record with more fields should extend")
	}
}

func TestReturnSummaryElidesOptional_Empty(t *testing.T) {
	if ElidesOptional(nil, nil) {
		t.Error("empty vectors should not elide")
	}
}

func TestReturnSummarySelectPreferred_Refinement(t *testing.T) {
	preferred, ok := SelectPreferred([]typ.Type{typ.String}, []typ.Type{typ.NewOptional(typ.String)})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], typ.String) {
		t.Fatalf("expected refined string return, got %v", preferred)
	}
}

func TestReturnSummarySelectPreferred_AvoidsNilOnlyRegression(t *testing.T) {
	preferred, ok := SelectPreferred([]typ.Type{typ.Nil}, []typ.Type{typ.NewOptional(typ.String)})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], typ.NewOptional(typ.String)) {
		t.Fatalf("expected non-nil summary to be preserved, got %v", preferred)
	}
}

func TestReturnSummarySelectPreferred_RejectsStaleNilOnly(t *testing.T) {
	preferred, ok := SelectPreferred([]typ.Type{typ.NewOptional(typ.String)}, []typ.Type{typ.Nil})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], typ.NewOptional(typ.String)) {
		t.Fatalf("expected new non-nil summary to replace nil-only, got %v", preferred)
	}
}

func TestReturnSummaryWiden_NilValueErrorBranchDoesNotEraseValueBranch(t *testing.T) {
	value := typ.NewRecord().Field("y", typ.Integer).Build()
	err := typ.LiteralString("missing")

	got := WidenForConvergence(
		[]typ.Type{typ.Nil, err},
		[]typ.Type{typ.NewOptional(value), typ.NewOptional(err)},
	)

	want := []typ.Type{typ.NewOptional(value), typ.NewOptional(err)}
	if len(got) != len(want) {
		t.Fatalf("WidenForConvergence returned %d slots, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !typ.TypeEquals(got[i], want[i]) {
			t.Fatalf("slot %d = %v, want %v; full vector %v", i, got[i], want[i], got)
		}
	}
}

func TestReturnSummarySelectPreferred_RecordExtension(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()

	preferred, ok := SelectPreferred([]typ.Type{newRec}, []typ.Type{oldRec})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], newRec) {
		t.Fatalf("expected record extension to be preferred, got %v", preferred)
	}
}

func TestReturnSummaryMerge_FoldsSelfEmbeddingBeforeRecordExtension(t *testing.T) {
	base := typ.NewRecord().Field("x", typ.Number).Build()
	grown := typ.NewRecord().
		Field("x", typ.Number).
		Field("next", typ.NewRecord().Field("value", base).Build()).
		Build()

	got := Merge([]typ.Type{base}, []typ.Type{grown})
	if len(got) != 1 {
		t.Fatalf("Merge() returned %d slots, want 1: %v", len(got), got)
	}
	if _, ok := got[0].(*typ.Recursive); !ok {
		t.Fatalf("Merge() slot = %T %[1]v, want recursive convergence upper bound", got[0])
	}
}

func TestReturnSummarySelectPreferred_RecordExtensionDoesNotEraseOptionalEvidence(t *testing.T) {
	baseline := typ.NewRecord().OptField("name", typ.String).Build()
	candidate := typ.NewRecord().Field("name", typ.String).Field("ready", typ.Boolean).Build()

	if preferred, ok := SelectPreferred([]typ.Type{candidate}, []typ.Type{baseline}); ok {
		t.Fatalf("SelectPreferred() = %v, want structural merge to preserve optional absence evidence", preferred)
	}
}

func TestReturnSummarySelectRefining_Refinement(t *testing.T) {
	refined := []typ.Type{typ.String}
	baseline := []typ.Type{typ.NewOptional(typ.String)}

	got, ok := SelectRefining(refined, baseline)
	if !ok {
		t.Fatal("expected refinement to be selected")
	}
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("expected refined return vector, got %v", got)
	}
}

func TestReturnSummarySelectRefining_DoesNotSelectOlderNarrowerBaseline(t *testing.T) {
	candidate := []typ.Type{typ.Any}
	baseline := []typ.Type{typ.False}

	_, ok := SelectRefining(candidate, baseline)
	if ok {
		t.Fatal("did not expect baseline-narrower relation to select candidate")
	}
}

func TestReturnSummaryFillsNilSlots(t *testing.T) {
	candidate := []typ.Type{typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)), typ.NewArray(typ.Unknown)}
	baseline := []typ.Type{typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)), typ.Nil}
	if !FillsNilSlots(candidate, baseline) {
		t.Fatalf("expected candidate to fill nil slot: candidate=%v baseline=%v", candidate, baseline)
	}
}

func TestReturnSummaryMerge_PrefersCandidateRefinement(t *testing.T) {
	existing := []typ.Type{typ.NewOptional(typ.String)}
	candidate := []typ.Type{typ.String}

	merged := Merge(existing, candidate)
	if len(merged) != 1 || !typ.TypeEquals(merged[0], typ.String) {
		t.Fatalf("expected refined candidate return, got %v", merged)
	}
}

func TestReturnSummaryMerge_PreservesWholeSlotAnyRuntimeOutcome(t *testing.T) {
	existing := []typ.Type{typ.Any}
	candidate := []typ.Type{typ.NewRecord().
		Field("candidates", typ.NewArray(typ.Unknown)).
		Build()}

	merged := Merge(existing, candidate)
	if len(merged) != 1 || !typ.TypeEquals(merged[0], typ.Any) {
		t.Fatalf("expected whole-slot any to remain the return outcome, got %v", merged)
	}
}

func TestReturnSummaryMerge_PreservesDynamicAnyTableOutcome(t *testing.T) {
	dynamicTable := typ.NewMap(typ.Any, typ.Any)
	concrete := typ.NewRecord().Field("raw", typ.String).Build()

	merged := Merge([]typ.Type{dynamicTable}, []typ.Type{concrete})
	if len(merged) != 1 || !typ.TypeEquals(merged[0], dynamicTable) {
		t.Fatalf("expected dynamic table outcome to remain, got %v", merged)
	}
}

func TestReturnSummaryMerge_ReplacesUnknownPlaceholderWithConcreteProof(t *testing.T) {
	existing := []typ.Type{typ.Unknown}
	candidate := []typ.Type{typ.NewRecord().Field("data", typ.String).Build()}

	merged := Merge(existing, candidate)
	if len(merged) != 1 || !typ.TypeEquals(merged[0], candidate[0]) {
		t.Fatalf("expected concrete interpreter proof to replace unknown placeholder, got %v", merged)
	}
}

func TestReturnSummaryMerge_FillsNilSlotWithCandidateEvidence(t *testing.T) {
	existing := []typ.Type{
		typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)),
		typ.Nil,
	}
	candidate := []typ.Type{
		typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)),
		typ.NewArray(typ.Unknown),
	}

	merged := Merge(existing, candidate)
	if len(merged) != 2 {
		t.Fatalf("expected two return slots, got %v", merged)
	}
	if !typ.TypeEquals(merged[1], typ.NewArray(typ.Unknown)) {
		t.Fatalf("expected nil slot to be filled with array evidence, got %v", merged[1])
	}
}

func TestReturnSummaryMerge_PrefersCurrentTruthyMapKeyRefinement(t *testing.T) {
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Unknown)
	candidate := typ.NewMap(typ.String, typ.Unknown)

	merged := Merge([]typ.Type{baseline}, []typ.Type{candidate})
	if len(merged) != 1 || !typ.TypeEquals(merged[0], candidate) {
		t.Fatalf("expected stale falsy map key to refine to %v, got %v", candidate, merged)
	}
}

func TestReturnSummaryMerge_PrefersConcreteMapValueOverSoftPlaceholder(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	baseline := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	candidate := typ.NewMap(typ.String, typ.NewArray(entry))

	merged := Merge([]typ.Type{baseline}, []typ.Type{candidate})
	if len(merged) != 1 || !typ.TypeEquals(merged[0], candidate) {
		t.Fatalf("expected concrete map value evidence %v, got %v", candidate, merged)
	}
}

func TestReturnSummaryMerge_PrefersCurrentTruthyRecordMapKeyRefinement(t *testing.T) {
	entryArray := typ.NewArray(typ.Unknown)
	baseline := typ.NewRecord().
		MapComponent(typ.NewUnion(typ.Nil, typ.String, typ.False), entryArray).
		SetOpen(true).
		Build()
	candidate := typ.NewRecord().
		MapComponent(typ.String, entryArray).
		Build()

	merged := Merge([]typ.Type{baseline}, []typ.Type{candidate})
	if len(merged) != 1 || !typ.TypeEquals(merged[0], candidate) {
		t.Fatalf("expected stale falsy record map key to refine to %v, got %v", candidate, merged)
	}
}

func TestReturnSummaryMerge_PrefersMapOverStaleOpenRecordMapKeyRefinement(t *testing.T) {
	entryArray := typ.NewArray(typ.Unknown)
	baseline := typ.NewRecord().
		MapComponent(typ.NewUnion(typ.Nil, typ.String, typ.False), entryArray).
		SetOpen(true).
		Build()
	candidate := typ.NewMap(typ.String, entryArray)

	merged := Merge([]typ.Type{baseline}, []typ.Type{candidate})
	if len(merged) != 1 || !typ.TypeEquals(merged[0], candidate) {
		t.Fatalf("expected map to replace stale open record map %v, got %v", candidate, merged)
	}
}

func TestReturnSummaryMerge_ElidesOptionalForInterfaceFieldRecords(t *testing.T) {
	txType := typ.NewInterface("sql.Tx", []typ.Method{
		{Name: "rollback", Type: typ.Func().Param("self", typ.Self).Build()},
	})
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{Name: "begin", Type: typ.Func().Param("self", typ.Self).Returns(txType, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "release", Type: typ.Func().Param("self", typ.Self).Build()},
	})

	existing := []typ.Type{
		typ.NewRecord().
			Field("db", typ.NewOptional(dbType)).
			Field("tx", typ.NewOptional(txType)).
			Build(),
	}
	candidate := []typ.Type{
		typ.NewRecord().
			Field("db", dbType).
			Field("tx", txType).
			Build(),
	}

	merged := Merge(existing, candidate)
	if len(merged) != 1 || !typ.TypeEquals(merged[0], candidate[0]) {
		t.Fatalf("expected candidate optional-elision to win, got %v", merged)
	}
}

func TestReturnSummaryApplyToFunctionType_AppliesSummaryToPlaceholderReturns(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.String).
		Returns(typ.Unknown).
		Build()
	summary := []typ.Type{typ.Integer}

	got := ApplyToFunctionType(fn, summary)
	if got == nil || len(got.Returns) != 1 {
		t.Fatalf("expected function return, got %v", got)
	}
	if !typ.TypeEquals(got.Returns[0], typ.Integer) {
		t.Fatalf("expected summary return integer, got %v", got.Returns[0])
	}
}

func TestReturnSummaryAlignFunction_NoopsWhenSummaryAlreadyApplied(t *testing.T) {
	event := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("message")).Field("id", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("tool")).Field("id", typ.String).Build(),
	)
	fn := typ.Func().
		Param("raw", typ.Any).
		Returns(typ.NewOptional(event), typ.NewOptional(typ.String)).
		Build()
	summary := []typ.Type{typ.NewOptional(event), typ.NewOptional(typ.String)}

	aligned, changed := AlignFunction(fn, summary)
	if changed {
		t.Fatalf("expected already-aligned function to be unchanged, got %v", aligned)
	}
	if aligned != fn {
		t.Fatalf("expected AlignFunction to preserve the existing function node")
	}
}

func TestReturnSummaryAlignFunction_NoopsForEquivalentRecursiveEvidence(t *testing.T) {
	base := typ.NewRecord().Field("x", typ.Number).Build()
	grown := typ.NewRecord().
		Field("x", typ.Number).
		Field("next", typ.NewRecord().Field("value", base).Build()).
		Build()
	stable := Merge([]typ.Type{base}, []typ.Type{grown})[0]
	observation := typ.NewRecord().
		Field("x", typ.Number).
		Field("next", typ.NewRecord().Field("value", stable).Build()).
		Build()
	fn := typ.Func().Returns(stable).Build()

	aligned, changed := AlignFunction(fn, []typ.Type{observation})
	if changed {
		t.Fatalf("equivalent recursive evidence should not rebuild function returns: %v", aligned)
	}
	if aligned != fn {
		t.Fatalf("expected AlignFunction to preserve the existing function node")
	}
}

func TestReturnSummaryMerge_MutualRefinementPrefersAliasSurface(t *testing.T) {
	eventStruct := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("message")).Field("id", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("tool")).Field("id", typ.String).Build(),
	)
	eventAlias := typ.NewAlias("Event", eventStruct)
	aliasSummary := []typ.Type{typ.NewOptional(eventAlias), typ.NewOptional(typ.String)}
	structSummary := []typ.Type{typ.NewOptional(eventStruct), typ.NewOptional(typ.String)}

	left := Merge(aliasSummary, structSummary)
	right := Merge(structSummary, aliasSummary)
	if !Equal(left, right) {
		t.Fatalf("equivalent alias/struct summaries must merge commutatively:\nleft=%v\nright=%v", left, right)
	}
	if len(left) != 2 || !typ.TypeEquals(left[0], aliasSummary[0]) {
		t.Fatalf("expected named alias surface to be the canonical representative, got %v", left)
	}
}

func TestReturnSummaryAlignFunction_ReplacesUnknownSlotEvenWithStructuredSibling(t *testing.T) {
	placeholderRecord := typ.NewRecord().SetOpen(true).Build()
	fn := typ.Func().
		Param("items", typ.Any).
		Returns(typ.Unknown, placeholderRecord).
		Build()
	summary := []typ.Type{typ.Integer, placeholderRecord}

	aligned, changed := AlignFunction(fn, summary)
	if !changed {
		t.Fatal("expected summary to replace unknown return slot")
	}
	if aligned == nil || len(aligned.Returns) != 2 {
		t.Fatalf("expected two returns, got %v", aligned)
	}
	if !typ.TypeEquals(aligned.Returns[0], typ.Integer) {
		t.Fatalf("expected integer first return, got %v", aligned.Returns[0])
	}
}

func TestReturnSummaryApplyToFunctionType_DefaultsToUnknownWhenMissing(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Build()
	got := ApplyToFunctionType(fn, nil)
	if got == nil || len(got.Returns) != 1 {
		t.Fatalf("expected one default return, got %v", got)
	}
	if !typ.TypeEquals(got.Returns[0], typ.Unknown) {
		t.Fatalf("expected default unknown return, got %v", got.Returns[0])
	}
}

func TestReturnSummaryNormalize_Empty(t *testing.T) {
	result := Normalize(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestReturnSummaryNormalize_ReplacesNil(t *testing.T) {
	input := []typ.Type{typ.String, nil, typ.Number}
	result := Normalize(input)
	if len(result) != 3 {
		t.Fatalf("expected length 3, got %d", len(result))
	}
	if result[0] != typ.String {
		t.Error("first element should be string")
	}
	if result[1] != typ.Nil {
		t.Error("nil should be replaced with typ.Nil")
	}
	if result[2] != typ.Number {
		t.Error("third element should be number")
	}
}

// Regression tests for recordSuperset map component handling.

func TestRecordSuperset_BothHaveMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Field("x", typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ExtendsRecord(a, b) {
		t.Error("record with same map component and additional fields should extend")
	}
}

func TestRecordSuperset_OldHasNoMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ExtendsRecord(a, b) {
		t.Error("record with additional fields should extend record without map component")
	}
}

func TestReturnSummaryAlignFunction_AppliesStrictRefinement(t *testing.T) {
	fn := typ.Func().
		Param("entries", typ.Any).
		Returns(typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown))).
		Build()

	summary := []typ.Type{
		typ.NewRecord().
			SetOpen(true).
			MapComponent(typ.String, typ.NewArray(typ.Unknown)).
			Build(),
	}

	aligned, changed := AlignFunction(fn, summary)
	if !changed {
		t.Fatal("expected alignment to apply strict refinement summary")
	}
	if aligned == nil || len(aligned.Returns) != 1 {
		t.Fatalf("unexpected aligned returns: %v", aligned)
	}
	got := aligned.Returns[0]
	rec, ok := got.(*typ.Record)
	if !ok || !rec.HasMapComponent() {
		t.Fatalf("expected record map-component return, got %T (%v)", got, got)
	}
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("expected map key string, got %v", rec.MapKey)
	}
}

func TestReturnSummaryAlignFunction_ReplacesOpenTopRecordWithStructuredSummary(t *testing.T) {
	openTop := typ.NewRecord().SetOpen(true).Build()
	fn := typ.Func().Returns(openTop).Build()
	summary := []typ.Type{typ.NewArray(typ.Unknown)}

	aligned, changed := AlignFunction(fn, summary)
	if !changed {
		t.Fatal("expected open-top placeholder to be replaced by structured summary")
	}
	if aligned == nil || len(aligned.Returns) != 1 {
		t.Fatalf("unexpected aligned function: %v", aligned)
	}
	if !typ.TypeEquals(aligned.Returns[0], summary[0]) {
		t.Fatalf("expected %v, got %v", summary[0], aligned.Returns[0])
	}
}

func TestReturnSummaryAlignFunction_DoesNotDowngradeStructuredToPlaceholder(t *testing.T) {
	structured := typ.NewRecord().Field("get_x", typ.Func().Build()).Build()
	fn := typ.Func().Returns(structured).Build()
	summary := []typ.Type{typ.Any}

	aligned, changed := AlignFunction(fn, summary)
	if changed {
		t.Fatalf("expected no downgrade change, got %v", aligned)
	}
	if aligned == nil || len(aligned.Returns) != 1 {
		t.Fatalf("unexpected aligned function: %v", aligned)
	}
	if !typ.TypeEquals(aligned.Returns[0], structured) {
		t.Fatalf("expected %v, got %v", structured, aligned.Returns[0])
	}
}

func TestReturnSummaryMerge_PrefersRuntimePossibleSummaryOverNeverArtifact(t *testing.T) {
	bad := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}
	good := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Unknown).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}

	got := Merge(bad, good)
	if !Equal(got, good) {
		t.Fatalf("Merge(%v, %v) = %v, want %v", bad, good, got, good)
	}
}

func TestReturnSummaryMerge_NeverScanUsesCanonicalUncappedTypeTraversal(t *testing.T) {
	badPayload := typ.Never
	goodPayload := typ.Unknown
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		badPayload = typ.NewRecord().Field("next", badPayload).Build()
		goodPayload = typ.NewRecord().Field("next", goodPayload).Build()
	}
	bad := []typ.Type{
		typ.NewRecord().
			Field("success", typ.True).
			Field("payload", badPayload).
			Build(),
	}
	good := []typ.Type{
		typ.NewRecord().
			Field("success", typ.True).
			Field("payload", goodPayload).
			Build(),
	}

	got := Merge(bad, good)
	if !Equal(got, good) {
		t.Fatalf("Merge() = %v, want %v", got, good)
	}
}

func TestReturnSummaryRepairsNeverRequiresBaselineNever(t *testing.T) {
	candidate := typ.NewRecord().Field("payload", typ.String).Build()
	baseline := typ.NewRecord().Field("payload", typ.Unknown).Build()
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		candidate = typ.NewRecord().Field("next", candidate).Build()
		baseline = typ.NewRecord().Field("next", baseline).Build()
	}

	if RepairsNever([]typ.Type{candidate}, []typ.Type{baseline}) {
		t.Fatal("RepairsNever should be false when baseline contains no never artifact")
	}
}

func TestReturnSummaryAlignFunction_RepairsNestedNeverArtifact(t *testing.T) {
	bad := typ.NewUnion(
		typ.NewRecord().
			Field("success", typ.True).
			Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
			Build(),
		typ.NewRecord().
			Field("success", typ.False).
			Field("error", typ.LiteralString("missing")).
			Build(),
	)
	good := typ.NewUnion(
		typ.NewRecord().
			Field("success", typ.True).
			Field("result", typ.NewRecord().OptField("data", typ.Unknown).Build()).
			Build(),
		typ.NewRecord().
			Field("success", typ.False).
			Field("error", typ.LiteralString("missing")).
			Build(),
	)

	fn := typ.Func().Returns(bad).Build()
	aligned, changed := AlignFunction(fn, []typ.Type{good})
	if !changed {
		t.Fatal("expected never-artifact repair to update function returns")
	}
	if aligned == nil || len(aligned.Returns) != 1 || !typ.TypeEquals(aligned.Returns[0], good) {
		t.Fatalf("aligned returns = %v, want %v", aligned, good)
	}
}

func TestMerge_RefinesRecordFieldsFromDynamicEvidence(t *testing.T) {
	baseline := []typ.Type{
		typ.NewOptional(typ.NewRecord().
			Field("max_tokens", typ.Any).
			Field("output_tokens", typ.Any).
			Build()),
		typ.NewUnion(typ.Nil, typ.LiteralString("not found")),
	}
	candidate := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("max_tokens", typ.Integer).
				Field("output_tokens", typ.Integer).
				Build(),
			typ.NewRecord().
				Field("max_tokens", typ.LiteralInt(0)).
				Field("output_tokens", typ.LiteralInt(0)).
				Build(),
		),
		typ.NewUnion(typ.Nil, typ.LiteralString("not found")),
	}

	got := Merge(baseline, candidate)
	wantFirst := typ.NewOptional(typ.NewRecord().
		Field("max_tokens", typ.Integer).
		Field("output_tokens", typ.Integer).
		Build())
	if !typ.TypeEquals(got[0], wantFirst) {
		t.Fatalf("Merge() first slot = %v, want %v", got[0], wantFirst)
	}
}

func TestMerge_RefinesRecordFieldsWhenCandidateHasFewerReturnSlots(t *testing.T) {
	baseline := []typ.Type{
		typ.NewOptional(typ.NewRecord().
			Field("max_tokens", typ.Any).
			Field("output_tokens", typ.Any).
			Build()),
		typ.NewUnion(typ.Nil, typ.String),
	}
	candidate := []typ.Type{
		typ.NewOptional(typ.NewUnion(
			typ.NewRecord().
				Field("max_tokens", typ.Integer).
				Field("output_tokens", typ.Integer).
				Build(),
			typ.NewRecord().
				Field("max_tokens", typ.LiteralInt(0)).
				Field("output_tokens", typ.LiteralInt(0)).
				Build(),
		)),
	}

	got := Merge(baseline, candidate)
	wantFirst := typ.NewOptional(typ.NewRecord().
		Field("max_tokens", typ.Integer).
		Field("output_tokens", typ.Integer).
		Build())
	if !typ.TypeEquals(got[0], wantFirst) {
		t.Fatalf("Merge() first slot = %v, want %v", got[0], wantFirst)
	}
	if !typ.TypeEquals(got[1], baseline[1]) {
		t.Fatalf("Merge() second slot = %v, want %v", got[1], baseline[1])
	}
}

func TestRecordSuperset_NewHasMapComponentOldDoesNot(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).MapComponent(typ.String, typ.Any).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ExtendsRecord(a, b) {
		t.Error("record with additional map component should extend record without it")
	}
}

func TestRecordSuperset_IncompatibleMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.Number, typ.String).Build()
	newRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if ExtendsRecord(a, b) {
		t.Error("record with incompatible map component should not extend")
	}
}

func TestReturnSummaryMerge_PrefersStructuredCollectionOverOpenTopRecordField(t *testing.T) {
	weak := []typ.Type{
		typ.NewRecord().
			Field("messages", typ.NewRecord().SetOpen(true).Build()).
			Field("system", typ.Nil).
			Build(),
	}
	strong := []typ.Type{
		typ.NewRecord().
			Field("messages", typ.NewArray(
				typ.NewRecord().Field("role", typ.Unknown).Build(),
			)).
			Field("system", typ.Nil).
			Build(),
	}

	merged := Merge(weak, strong)
	if len(merged) != 1 {
		t.Fatalf("expected one return slot, got %d", len(merged))
	}

	ret, ok := merged[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record return, got %T (%v)", merged[0], merged[0])
	}
	msgField := ret.GetField("messages")
	if msgField == nil {
		t.Fatalf("expected messages field in merged return: %v", merged[0])
	}
	if _, ok := msgField.Type.(*typ.Array); !ok {
		t.Fatalf("expected messages field to remain array-like, got %T (%v)", msgField.Type, msgField.Type)
	}
}

func TestReturnSummaryMerge_PromotesTopLevelStructuredOverOpenTop(t *testing.T) {
	weak := []typ.Type{
		typ.NewRecord().SetOpen(true).Build(),
	}
	strong := []typ.Type{
		typ.NewArray(typ.Any),
	}

	merged := Merge(weak, strong)
	if len(merged) != 1 {
		t.Fatalf("expected one return slot, got %d", len(merged))
	}
	if _, ok := merged[0].(*typ.Array); !ok {
		t.Fatalf("expected top-level array after merge, got %T (%v)", merged[0], merged[0])
	}
}
