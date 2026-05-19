package returnsummary

import (
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"testing"
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
