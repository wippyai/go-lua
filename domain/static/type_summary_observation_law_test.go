package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestBeginTypeSummaryUsesSuppliedValueWidthAndExactClassOwner(t *testing.T) {
	fixture := newTypeFactFixture(t, 141)
	first := BeginTypeSummary(fixture.classes, 3)
	if !first.Valid || len(first.Values) != 3 || len(first.Present) != 3 || first.Rows != 0 || first.owner != fixture.classes || first.width != 3 {
		t.Fatalf("begin = %#v, want a three-cell owner-fenced accumulator", first)
	}
	second := BeginTypeSummary(fixture.classes, 3)
	first.Present[1] = true
	first.Values[1], _ = fixture.classes.TypeFactForOwnTarget(fixture.result)
	if second.Present[1] || second.Values[1].Valid() {
		t.Fatal("independent TypeFact summaries share fold storage")
	}
	for _, width := range []int{0, -1} {
		if got := BeginTypeSummary(fixture.classes, width); got.Valid || got.Values != nil || got.Present != nil {
			t.Fatalf("width %d opened %#v, want the zero observation", width, got)
		}
	}
	if got := BeginTypeSummary(nil, 3); got.Valid {
		t.Fatal("nil ClassSet opened a valid TypeFact summary")
	}
}

func TestAccumulateTypeSummaryJoinsThroughClassSetAndKeepsAbsentCellsAbsent(t *testing.T) {
	fixture := newTypeFactFixture(t, 142)
	classes := fixture.classes
	concrete, ok := classes.TypeFactForOwnTarget(fixture.result)
	if !ok {
		t.Fatal("fixture Target fact unavailable")
	}
	top := classes.TypeTop()
	result := BeginTypeSummary(classes, 2)
	result, ok = AccumulateTypeSummaryRows(classes, result, 2, func(index int) (TypeFact, bool, bool) {
		if index == 0 {
			return concrete, true, true
		}
		return classes.TypeBottom(), false, true
	})
	if !ok || !result.Present[0] || result.Present[1] || result.Rows != 1 || !classes.EqualTypeFact(result.Values[0], concrete) {
		t.Fatalf("first fold = %#v/%t, want one concrete present cell", result, ok)
	}
	result, ok = AccumulateTypeSummaryRows(classes, result, 2, func(index int) (TypeFact, bool, bool) {
		if index == 0 {
			return top, true, true
		}
		return classes.TypeTop(), false, true
	})
	if !ok || !result.Present[0] || result.Rows != 1 || !classes.EqualTypeFact(result.Values[0], top) {
		t.Fatal("second fold did not use ClassSet.JoinTypeFact", result, ok)
	}

	absent := BeginTypeSummary(classes, 2)
	absent, ok = AccumulateTypeSummaryRows(classes, absent, 2, func(index int) (TypeFact, bool, bool) {
		return classes.TypeTop(), false, true
	})
	if !ok || absent.Rows != 0 || absent.Present[0] || absent.Present[1] || absent.Values[0].Valid() || absent.Values[1].Valid() {
		t.Fatalf("all-absent fold = %#v/%t, want zero-row absence", absent, ok)
	}
}

func TestTypeSummaryRejectsForeignFactsAndUnavailableCells(t *testing.T) {
	left := newTypeFactFixture(t, 143)
	right := newTypeFactFixture(t, 144)
	foreign, ok := right.classes.TypeFactForOwnTarget(right.result)
	if !ok {
		t.Fatal("foreign fixture Target fact unavailable")
	}
	result := BeginTypeSummary(left.classes, 1)
	if got, folded := AccumulateTypeSummaryRows(left.classes, result, 1, func(int) (TypeFact, bool, bool) {
		return foreign, true, true
	}); folded || got.Valid || got.Values != nil || got.Present != nil {
		t.Fatal("foreign TypeFact crossed the summary owner fence")
	}
	result = BeginTypeSummary(left.classes, 1)
	if got, folded := AccumulateTypeSummaryRows(left.classes, result, 1, func(int) (TypeFact, bool, bool) {
		return left.classes.TypeBottom(), true, true
	}); folded || got.Valid {
		t.Fatal("present TypeFact bottom was accepted as a solved value")
	}
	result = BeginTypeSummary(left.classes, 1)
	if got, folded := AccumulateTypeSummaryRows(left.classes, result, 1, func(int) (TypeFact, bool, bool) {
		return left.classes.TypeBottom(), false, false
	}); folded || got.Valid {
		t.Fatal("unavailable cell was accepted as absence")
	}
}

func TestTypeSummaryCloneEqualAndFingerprintDetachBothPlanes(t *testing.T) {
	fixture := newTypeFactFixture(t, 145)
	fact, ok := fixture.classes.TypeFactForOwnTarget(fixture.result)
	if !ok {
		t.Fatal("fixture Target fact unavailable")
	}
	source := BeginTypeSummary(fixture.classes, 2)
	source.Values[0], source.Present[0], source.Rows = fact, true, 1
	detached := CloneTypeSummary(source)
	if !EqualTypeSummary(fixture.classes, detached, source) || FingerprintTypeSummary(fixture.classes, detached) != FingerprintTypeSummary(fixture.classes, source) {
		t.Fatal("fresh TypeFact summary clone is not equal and fingerprint-identical")
	}
	source.Values[0], source.Present[1] = TypeFact{}, true
	if !detached.Present[0] || !fixture.classes.EqualTypeFact(detached.Values[0], fact) || detached.Present[1] {
		t.Fatal("mutating the source changed the detached TypeFact summary")
	}
	detached.Values[0], detached.Present[0] = TypeFact{}, false
	if !source.Present[0] {
		t.Fatal("mutating the detached summary changed its source")
	}
	foreign := BeginTypeSummary(newTypeFactFixture(t, 146).classes, 2)
	if EqualTypeSummary(fixture.classes, source, foreign) || FingerprintTypeSummary(fixture.classes, foreign) != 0 {
		t.Fatal("foreign summary crossed equality/fingerprint owner fence")
	}
}

func TestTypeSummaryFoldHotPathDoesNotAllocateForIdempotentClassJoin(t *testing.T) {
	fixture := newTypeFactFixture(t, 147)
	fact, ok := fixture.classes.TypeFactForOwnTarget(fixture.result)
	if !ok {
		t.Fatal("fixture Target fact unavailable")
	}
	result := BeginTypeSummary(fixture.classes, 1)
	result.Values[0], result.Present[0], result.Rows = fact, true, 1
	var sink TypeSummaryObservation
	allocations := testing.AllocsPerRun(100, func() {
		var folded bool
		sink, folded = AccumulateTypeSummaryRows(fixture.classes, result, 1, func(int) (TypeFact, bool, bool) {
			return fact, true, true
		})
		if !folded {
			t.Fatal("idempotent TypeFact fold failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("idempotent TypeFact fold allocations = %v, want zero", allocations)
	}
	if !sink.Valid || !sink.Present[0] {
		t.Fatal("hot fold result was lost")
	}
}

func typeSummaryLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}
