package product

import (
	"strconv"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestRefinementReturnsExactSourceRowsAcrossMultipleInputs(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	input := splitOnA(t, initial, regions)

	refinement := input.BeginRefine()
	if refinement == nil ||
		!refinement.Add(0, regions.a) ||
		!refinement.Add(1, regions.notAAndB) ||
		!refinement.Add(1, regions.notAAndNotB) {
		t.Fatal("Add rejected an exact multi-source refinement")
	}
	rows, sources, ok := refinement.Seal()
	if !ok || rows.Count() != 3 || sources.Count() != rows.Count() {
		t.Fatalf("Seal = rows:%d sources:%d ok:%t", rows.Count(), sources.Count(), ok)
	}

	wantSources := []int{0, 1, 1}
	wantCells := []support.Mask{regions.a, regions.notAAndB, regions.notAAndNotB}
	for piece, wantSource := range wantSources {
		gotSource, found := sources.Source(piece)
		if !found || gotSource != wantSource {
			t.Fatalf("source[%d] = %d/%t, want %d", piece, gotSource, found, wantSource)
		}
		gotCell, found := rows.At(piece)
		if !found || !gotCell.Equal(wantCells[piece]) {
			t.Fatalf("cell[%d] did not retain the matching exact support", piece)
		}
	}
	if _, found := sources.Source(-1); found {
		t.Fatal("negative output piece resolved to a source")
	}
	if _, found := sources.Source(rows.Count()); found {
		t.Fatal("out-of-range output piece resolved to a source")
	}
	if sources.SourceCount() != 2 {
		t.Fatalf("SourceCount = %d, want 2", sources.SourceCount())
	}
	for source, want := range [][2]int{{0, 1}, {1, 3}} {
		start, end, found := sources.Range(source)
		if !found || start != want[0] || end != want[1] {
			t.Fatalf("Range(%d) = [%d,%d)/%t, want [%d,%d)", source, start, end, found, want[0], want[1])
		}
	}
	if _, _, found := sources.Range(-1); found {
		t.Fatal("negative source returned a range")
	}
	if _, _, found := sources.Range(sources.SourceCount()); found {
		t.Fatal("out-of-range source returned a range")
	}
}

func TestRefinementIdentityRowsPreserveExactSources(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	input := splitOnA(t, initial, regions)

	refinement := input.BeginRefine()
	if refinement == nil || !refinement.Add(0, regions.a) || !refinement.Add(1, regions.notA) {
		t.Fatal("identity refinement setup")
	}
	rows, sources, ok := refinement.Seal()
	if !ok || rows.Count() != input.Count() || sources.Count() != input.Count() {
		t.Fatalf("identity Seal = rows:%d sources:%d ok:%t", rows.Count(), sources.Count(), ok)
	}
	for index := 0; index < input.Count(); index++ {
		inputCell, _ := input.At(index)
		outputCell, _ := rows.At(index)
		if !outputCell.SameHandle(inputCell) {
			t.Fatalf("identity output %d changed its sealed source cell", index)
		}
		source, found := sources.Source(index)
		if !found || source != index {
			t.Fatalf("identity Source(%d) = %d/%t", index, source, found)
		}
		start, end, found := sources.Range(index)
		if !found || start != index || end != index+1 {
			t.Fatalf("identity Range(%d) = [%d,%d)/%t", index, start, end, found)
		}
	}
}

func TestRefinementAcceptsSemanticallyEqualPieceFromLaterWork(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.a)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	// Build the replacement in a fresh candidate transaction.  The identity
	// fast path may use handle identity, but semantic equality remains the
	// authority when publication did not preserve a physical handle.
	work := support.New(regions.manager)
	if work == nil {
		t.Fatal("fresh support work")
	}
	equal, ok := work.Literal(1, true)
	if !ok || !work.Seal() || !equal.Equal(regions.a) {
		t.Fatal("fresh semantically equal support")
	}
	refinement := initial.BeginRefine()
	if refinement == nil || !refinement.Add(0, equal) {
		t.Fatal("fresh equal replacement was rejected")
	}
	if _, _, ok := refinement.Seal(); !ok {
		t.Fatal("semantically equal replacement did not seal")
	}
}

func TestRefinementRejectsForgedNonEqualSinglePiece(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	refinement := initial.BeginRefine()
	if refinement == nil || !refinement.Add(0, regions.a) {
		t.Fatal("non-equal candidate setup")
	}
	if _, _, ok := refinement.Seal(); ok {
		t.Fatal("non-equal single piece sealed as identity")
	}
}

func TestRefinementIdentityRowsAreRaceSafe(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	input := splitOnA(t, initial, regions)
	var group sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			refinement := input.BeginRefine()
			if refinement == nil || !refinement.Add(0, regions.a) || !refinement.Add(1, regions.notA) {
				t.Error("identity refinement setup")
				return
			}
			if rows, sources, ok := refinement.Seal(); !ok || rows.Count() != 2 || sources.Count() != 2 {
				t.Error("identity refinement seal")
			}
		}()
	}
	group.Wait()
}

func TestRefinementSourceRowsFollowDeterministicCellOrder(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	input := splitOnA(t, initial, regions)

	firstRows, firstSources := refineThree(t, input, regions)
	secondRows, secondSources := refineThree(t, input, regions)
	if firstRows.Count() != secondRows.Count() || firstSources.Count() != secondSources.Count() {
		t.Fatal("same refinement changed output cardinality")
	}
	for piece := 0; piece < firstRows.Count(); piece++ {
		firstCell, _ := firstRows.At(piece)
		secondCell, _ := secondRows.At(piece)
		if !firstCell.Equal(secondCell) {
			t.Fatalf("cell order differed at %d", piece)
		}
		firstSource, _ := firstSources.Source(piece)
		secondSource, _ := secondSources.Source(piece)
		if firstSource != secondSource {
			t.Fatalf("source order differed at %d: %d != %d", piece, firstSource, secondSource)
		}
	}
}

func TestRefinementRejectsEmptyOverlapAndForeignManagerPieces(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	empty, ok := support.FromGuard(regions.manager, regions.manager.False())
	if !ok {
		t.Fatal("could not construct empty support")
	}

	emptyRefinement := initial.BeginRefine()
	if emptyRefinement == nil || emptyRefinement.Add(0, empty) {
		t.Fatal("empty piece was admitted")
	}
	if _, _, ok := emptyRefinement.Seal(); ok {
		t.Fatal("missing exact cover sealed after empty piece rejection")
	}

	overlapRefinement := initial.BeginRefine()
	if overlapRefinement == nil || !overlapRefinement.Add(0, regions.a) || !overlapRefinement.Add(0, regions.all) {
		t.Fatal("overlap setup was rejected before Seal")
	}
	if _, _, ok := overlapRefinement.Seal(); ok {
		t.Fatal("overlapping pieces sealed as an exact partition")
	}

	duplicateRefinement := initial.BeginRefine()
	if duplicateRefinement == nil || !duplicateRefinement.Add(0, regions.a) || !duplicateRefinement.Add(0, regions.a) {
		t.Fatal("duplicate setup")
	}
	if _, _, ok := duplicateRefinement.Seal(); ok {
		t.Fatal("duplicate pieces sealed as an exact partition")
	}

	foreignManager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	foreign, ok := support.True(foreignManager)
	if !ok {
		t.Fatal("could not construct foreign support")
	}
	foreignRefinement := initial.BeginRefine()
	if foreignRefinement == nil || foreignRefinement.Add(0, foreign) {
		t.Fatal("foreign-manager piece was admitted")
	}
	if !foreignRefinement.Add(0, regions.all) {
		t.Fatal("foreign rejection mutated the candidate refinement")
	}
	if _, sources, ok := foreignRefinement.Seal(); !ok || sources.Count() != 1 {
		t.Fatal("valid local replacement did not seal after foreign rejection")
	}
}

func TestRefinementRequiresCanonicalSourceMajorPieceEmission(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	input := splitOnA(t, initial, regions)

	refinement := input.BeginRefine()
	if refinement == nil {
		t.Fatal("BeginRefine returned nil")
	}
	if refinement.Add(1, regions.notAAndB) {
		t.Fatal("Add admitted a source before source 0")
	}
	if !refinement.Add(0, regions.a) || !refinement.Add(1, regions.notAAndB) {
		t.Fatal("Add rejected canonical source-major pieces")
	}
	if refinement.Add(0, regions.a) {
		t.Fatal("Add admitted a piece after its source had closed")
	}
	if !refinement.Add(1, regions.notAAndNotB) {
		t.Fatal("Add rejected the remaining canonical source piece")
	}
	if _, _, ok := refinement.Seal(); !ok {
		t.Fatal("canonical source-major refinement did not seal")
	}
}

func TestSameHandleIdentityRefinementIsAllocationFree(t *testing.T) {
	oneRows, onePieces := newExactRows(t, 1)
	manyRows, manyPieces := newExactRows(t, 64)

	one := testing.AllocsPerRun(50, func() { refineOnce(oneRows, onePieces) })
	many := testing.AllocsPerRun(50, func() { refineOnce(manyRows, manyPieces) })
	if one != 0 || many != 0 {
		t.Fatalf("same-handle identity allocations: one=%0.0f many=%0.0f, want zero", one, many)
	}
}

func TestPrefixQuotientPreservesSourcePrefixAndUnionsExactSupport(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	input := splitOnA(t, initial, regions)
	refinement := input.BeginRefine()
	if refinement == nil ||
		!refinement.Add(0, regions.a) ||
		!refinement.Add(1, regions.notAAndB) ||
		!refinement.Add(1, regions.notAAndNotB) {
		t.Fatal("refinement setup")
	}
	rows, sources, ok := refinement.Seal()
	if !ok {
		t.Fatal("refinement seal")
	}

	// Every output is opaque-equal. The quotient must nevertheless retain the
	// source-0 representative separately from the source-1 representative.
	quotient, representatives, ok := rows.PrefixQuotientWithCheckpoint(sources, nil, func(int) (uint64, bool) { return 0, true }, func(_, _ int) bool { return true })
	if !ok || quotient.Count() != 2 {
		t.Fatalf("PrefixQuotient = rows:%d ok:%t", quotient.Count(), ok)
	}
	if len(representatives) != 2 || representatives[0] != 0 || representatives[1] != 1 {
		t.Fatalf("representatives = %v, want [0 1]", representatives)
	}
	first, firstOK := quotient.At(0)
	second, secondOK := quotient.At(1)
	if !firstOK || !secondOK || !first.Equal(regions.a) || !second.Equal(regions.notA) {
		t.Fatal("prefix quotient did not preserve per-source exact support unions")
	}
}

func TestPrefixQuotientFailsClosedOnCancellationAfterOpaqueComparison(t *testing.T) {
	regions := newRegions(t)
	initial, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("NewRows rejected nonempty support")
	}
	input := splitOnA(t, initial, regions)
	refinement := input.BeginRefine()
	if refinement == nil ||
		!refinement.Add(0, regions.a) ||
		!refinement.Add(1, regions.notAAndB) ||
		!refinement.Add(1, regions.notAAndNotB) {
		t.Fatal("refinement setup")
	}
	rows, sources, ok := refinement.Seal()
	if !ok {
		t.Fatal("refinement seal")
	}

	cancelled := false
	quotient, representatives, ok := rows.PrefixQuotientWithCheckpoint(sources, func() bool { return !cancelled }, func(int) (uint64, bool) { return 0, true }, func(_, _ int) bool {
		cancelled = true
		return true
	})
	if ok || quotient.Count() != 0 || representatives != nil {
		t.Fatal("cancelled prefix quotient published a partial result")
	}
	if rows.Count() != 3 {
		t.Fatal("cancelled prefix quotient changed sealed rows")
	}
}

func TestPrefixQuotientRejectsIncompleteOrEmptySourceMappings(t *testing.T) {
	rows, _ := newExactRows(t, 4)
	invalid := []SourceRows{
		{identity: 3},
		{offsets: []int{0, 2, 2, 4}},
		{offsets: []int{0, 2, 3}},
		{offsets: []int{0, 2, 5}},
	}
	for index, sources := range invalid {
		called := false
		if quotient, representatives, ok := rows.PrefixQuotientWithCheckpoint(sources, nil, func(int) (uint64, bool) { return 0, true }, func(_, _ int) bool {
			called = true
			return true
		}); ok || quotient.Count() != 0 || representatives != nil || called {
			t.Fatalf("invalid source map %d was admitted", index)
		}
	}
}

func TestPrefixQuotientSkipsEqualityAcrossDistinctFingerprintBuckets(t *testing.T) {
	rows, _ := newExactRows(t, 4)
	sources := SourceRows{offsets: []int{0, 4}, sourceID: rows.identity, rowsID: rows.identity}
	comparisons := 0
	quotient, representatives, ok := rows.PrefixQuotientWithCheckpoint(sources, nil, func(index int) (uint64, bool) {
		return uint64(index), true
	}, func(_, _ int) bool {
		comparisons++
		return true
	})
	if !ok || quotient.Count() != 4 || len(representatives) != 4 {
		t.Fatalf("distinct-bucket quotient = rows:%d representatives:%d ok:%t", quotient.Count(), len(representatives), ok)
	}
	if comparisons != 0 {
		t.Fatalf("distinct fingerprints caused %d semantic equality calls", comparisons)
	}
}

// TestPrefixQuotientCollapsesAdversarialEqualPrefixes is the semantic law for
// a sequence of reads whose opaque results are all equal. Each read splits
// every current source on a fresh ordered atom before sealing, then quotients
// the sealed result. Without the quotient, the successive exact partitions
// would grow as 2^reads; equal prefixes must instead keep one true-support row.
func TestPrefixQuotientCollapsesAdversarialEqualPrefixes(t *testing.T) {
	rows, atoms, original := newAdversarialPrefixRows(t)
	for read, atom := range atoms {
		var ok bool
		rows, ok = adversarialEqualPrefixRead(rows, atom)
		if !ok {
			t.Fatalf("read %d did not complete", read+1)
		}
		if rows.Count() != 1 {
			t.Fatalf("read %d left %d rows, want 1", read+1, rows.Count())
		}
		row, found := rows.At(0)
		if !found || !row.Equal(original) {
			t.Fatalf("read %d did not retain the original true support", read+1)
		}
	}
}

func TestCheckpointDropsPartialRefinementBeforeRowsEscape(t *testing.T) {
	rows, pieces := newPartitionRows(t, 4)
	checks := 0
	checkpoint := func() bool {
		checks++
		return checks < 5
	}
	refinement := rows.BeginRefineWithCheckpoint(checkpoint)
	if refinement == nil {
		t.Fatal("refinement")
	}
	for source, parts := range pieces {
		for _, piece := range parts {
			if !refinement.Add(source, piece) {
				if _, _, sealed := refinement.Seal(); sealed {
					t.Fatal("cancelled refinement sealed a partial product")
				}
				if rows.Count() != 4 {
					t.Fatal("cancelled refinement changed source rows")
				}
				return
			}
		}
	}
	t.Fatal("checkpoint did not interrupt refinement")
}

func TestReusableProofWorkCleansUpCancelledCoverAndSealsNextCover(t *testing.T) {
	regions := newRegions(t)
	rows, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("rows")
	}
	proof := support.New(regions.manager)
	if proof == nil {
		t.Fatal("proof")
	}
	active := true
	refinement := rows.BeginRefineWithWork(proof, func() bool { return active })
	if refinement == nil || !refinement.Add(0, regions.a) {
		t.Fatal("cancelled cover setup")
	}
	active = false
	if _, _, ok := refinement.Seal(); ok {
		t.Fatal("cancelled cover sealed")
	}
	active = true
	recovered := rows.BeginRefineWithWork(proof, nil)
	if recovered == nil || !recovered.Add(0, regions.all) {
		t.Fatal("recovered cover setup")
	}
	sealed, _, ok := recovered.Seal()
	if !ok || !sealed.Valid() || sealed.Count() != 1 {
		t.Fatal("reusable proof work did not seal the recovered cover")
	}
}

func TestCrossWorkOwnsExactIntersectionsAndOpaquePairs(t *testing.T) {
	regions := newRegions(t)
	seed, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("seed")
	}
	left := splitOnA(t, seed, regions)
	right := splitOnB(t, seed, regions)
	var cross CrossWork
	result, pairs, ok := cross.Cross(left, right, nil)
	if !ok || !result.Valid() || result.Count() != 4 || pairs.Count() != 4 {
		t.Fatalf("cross result=%d pairs=%d ok=%t", result.Count(), pairs.Count(), ok)
	}
	wantPairs := [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}}
	for index, want := range wantPairs {
		gotLeft, gotRight, pairOK := pairs.At(index)
		if !pairOK || gotLeft != want[0] || gotRight != want[1] {
			t.Fatalf("pair[%d]=(%d,%d)/%t want (%d,%d)", index, gotLeft, gotRight, pairOK, want[0], want[1])
		}
		cell, cellOK := result.At(index)
		leftCell, leftOK := left.At(gotLeft)
		rightCell, rightOK := right.At(gotRight)
		wantCell, wantOK := support.Intersect(leftCell, rightCell)
		if !cellOK || !leftOK || !rightOK || !wantOK || !cell.Equal(wantCell) {
			t.Fatalf("pair[%d] did not retain its exact carrier intersection", index)
		}
	}
	// The reusable buffers are generation-fenced before their next write.
	if _, _, secondOK := cross.Cross(left, right, nil); !secondOK {
		t.Fatal("warm cross")
	}
	if result.Count() != 0 || pairs.Count() != 0 {
		t.Fatal("stale CrossWork views remained readable")
	}
}

func TestCrossWorkRejectsUnauthenticatedOrMismatchedCovers(t *testing.T) {
	regions := newRegions(t)
	seed, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("seed")
	}
	left := splitOnA(t, seed, regions)
	var cross CrossWork
	// A package-local cell slice without a carrier seal is not a cross operand.
	forged := Rows{manager: regions.manager, cells: []support.Mask{regions.a}}
	if _, _, ok := cross.Cross(left, forged, nil); ok {
		t.Fatal("unauthenticated rows crossed")
	}
	// A valid cover over a different declared support cannot silently produce a
	// partial product over the left support.
	other, ok := NewRows(regions.a)
	if !ok {
		t.Fatal("other seed")
	}
	if _, _, ok := cross.Cross(left, other, nil); ok {
		t.Fatal("mismatched declared support crossed")
	}
}

func TestCrossWorkRevokesPriorViewsBeforeEveryFailure(t *testing.T) {
	regions := newRegions(t)
	seed, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("seed")
	}
	left := splitOnA(t, seed, regions)
	right := splitOnB(t, seed, regions)
	other, ok := NewRows(regions.a)
	if !ok {
		t.Fatal("other seed")
	}
	var cross CrossWork

	assertRevoked := func(name string, fail func()) {
		t.Helper()
		prior, pairs, crossed := cross.Cross(left, right, nil)
		if !crossed {
			t.Fatalf("%s setup cross", name)
		}
		fail()
		if prior.Valid() || prior.Count() != 0 || pairs.Count() != 0 {
			t.Fatalf("%s retained the prior carrier views", name)
		}
	}

	assertRevoked("invalid", func() {
		if _, _, crossed := cross.Cross(Rows{}, right, nil); crossed {
			t.Fatal("invalid left crossed")
		}
	})
	assertRevoked("mismatched", func() {
		if _, _, crossed := cross.Cross(left, other, nil); crossed {
			t.Fatal("mismatched support crossed")
		}
	})
	assertRevoked("cancelled", func() {
		if _, _, crossed := cross.Cross(left, right, func() bool { return false }); crossed {
			t.Fatal("cancelled cross published")
		}
	})

	prior, pairs, crossed := cross.Cross(left, right, nil)
	if !crossed {
		t.Fatal("generation exhaustion setup cross")
	}
	cross.generation = ^uint64(0)
	if _, _, crossed = cross.Cross(left, right, nil); crossed {
		t.Fatal("generation-exhausted cross published")
	}
	if prior.Valid() || prior.Count() != 0 || pairs.Count() != 0 {
		t.Fatal("generation exhaustion retained the prior carrier views")
	}
}

func TestCrossPairsAuthenticateExactOperandsAndResult(t *testing.T) {
	regions := newRegions(t)
	seed, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("seed")
	}
	left := splitOnA(t, seed, regions)
	right := splitOnB(t, seed, regions)
	foreignLeft := splitOnA(t, seed, regions)
	var cross CrossWork
	result, pairs, ok := cross.Cross(left, right, nil)
	if !ok || !pairs.ValidFor(left, right, result) {
		t.Fatal("exact CrossPairs authentication")
	}
	if pairs.ValidFor(foreignLeft, right, result) {
		t.Fatal("CrossPairs accepted a foreign same-shaped left Rows")
	}
	foreignResult, _, ok := cross.Cross(left, right, nil)
	if !ok || pairs.ValidFor(left, right, foreignResult) {
		t.Fatal("CrossPairs accepted a foreign result or stale generation")
	}
}

func TestSourceRowsRejectForeignSameShapedProducer(t *testing.T) {
	regions := newRegions(t)
	seed, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("seed")
	}
	refinement := seed.BeginRefine()
	if refinement == nil || !refinement.Add(0, regions.a) || !refinement.Add(0, regions.notA) {
		t.Fatal("source refinement")
	}
	rows, sources, ok := refinement.Seal()
	if !ok {
		t.Fatal("source seal")
	}
	foreignRefinement := seed.BeginRefine()
	if foreignRefinement == nil || !foreignRefinement.Add(0, regions.a) || !foreignRefinement.Add(0, regions.notA) {
		t.Fatal("foreign source refinement")
	}
	foreignRows, _, ok := foreignRefinement.Seal()
	if !ok {
		t.Fatal("foreign source seal")
	}
	if _, _, ok := foreignRows.PrefixQuotientWithCheckpoint(sources, nil, func(int) (uint64, bool) { return 0, true }, func(_, _ int) bool { return true }); ok {
		t.Fatal("SourceRows accepted a foreign same-shaped producer")
	}
	if _, _, ok := rows.PrefixQuotientWithCheckpoint(sources, nil, func(int) (uint64, bool) { return 0, true }, func(_, _ int) bool { return true }); !ok {
		t.Fatal("SourceRows rejected its exact producer")
	}
}

func TestCrossWorkRecomputesOnHandleMismatchAndReusesAfterCheckpointFailure(t *testing.T) {
	regions := newRegions(t)
	seed, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("seed")
	}
	left := splitOnA(t, seed, regions)
	rightB := splitOnB(t, seed, regions)
	rightA := splitOnA(t, seed, regions)
	var cross CrossWork
	first, _, ok := cross.Cross(left, rightB, nil)
	if !ok || first.Count() != 4 {
		t.Fatal("first cross")
	}
	// Same cardinality, different physical support handles: a cached answer is
	// not accepted. The recomputed A×A crossing has only two nonempty pairs.
	second, pairs, ok := cross.Cross(left, rightA, nil)
	if !ok || second.Count() != 2 || pairs.Count() != 2 {
		t.Fatalf("handle-mismatch cross=%d pairs=%d ok=%t", second.Count(), pairs.Count(), ok)
	}
	checks := 0
	cancelledRows, cancelledPairs, ok := cross.Cross(left, rightB, func() bool {
		checks++
		return checks < 2
	})
	if ok || cancelledRows.Count() != 0 || cancelledPairs.Count() != 0 {
		t.Fatal("cancelled cross published a result")
	}
	if _, _, ok := cross.Cross(left, rightB, nil); !ok {
		t.Fatal("CrossWork did not recover after checkpoint failure")
	}
}

func TestCrossWorkWarmHandleHitAllocatesNothing(t *testing.T) {
	regions := newRegions(t)
	seed, ok := NewRows(regions.all)
	if !ok {
		t.Fatal("seed")
	}
	left := splitOnA(t, seed, regions)
	right := splitOnB(t, seed, regions)
	var cross CrossWork
	if _, _, ok := cross.Cross(left, right, nil); !ok {
		t.Fatal("cold cross")
	}
	if _, _, ok := cross.Cross(left, right, nil); !ok {
		t.Fatal("warm cross")
	}
	allocations := testing.AllocsPerRun(50, func() {
		if _, _, ok := cross.Cross(left, right, nil); !ok {
			t.Fatal("warm cross")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm CrossWork allocated %v times", allocations)
	}
}

// BenchmarkSameHandleIdentityRefinementAllocationScaling measures only the
// direct-handle no-op route. General exact-cover work is measured separately
// below and must never be inferred from this benchmark.
func BenchmarkSameHandleIdentityRefinementAllocationScaling(b *testing.B) {
	for _, count := range []int{1, 64} {
		b.Run("sources="+strconv.Itoa(count), func(b *testing.B) {
			rows, pieces := newExactRows(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				refineOnce(rows, pieces)
			}
		})
	}
}

func BenchmarkIdentityRefinement(b *testing.B) {
	rows, pieces := newExactRows(b, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		refineOnce(rows, pieces)
	}
}

func BenchmarkCrossGenerationIdentityRefinement(b *testing.B) {
	regions := newRegions(b)
	rows, ok := NewRows(regions.a)
	if !ok {
		b.Fatal("NewRows rejected nonempty support")
	}
	work := support.New(regions.manager)
	if work == nil {
		b.Fatal("fresh support work")
	}
	piece, ok := work.Literal(1, true)
	if !ok || !work.Seal() || !piece.Equal(regions.a) || piece.SameHandle(regions.a) {
		b.Fatal("cross-generation equal support")
	}
	pieces := []support.Mask{piece}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		refineOnce(rows, pieces)
	}
}

func BenchmarkPartitionRefinement(b *testing.B) {
	regions := newRegions(b)
	rows, ok := NewRows(regions.all)
	if !ok {
		b.Fatal("NewRows rejected nonempty support")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		refinement := rows.BeginRefine()
		if refinement == nil || !refinement.Add(0, regions.a) || !refinement.Add(0, regions.notA) {
			b.Fatal("partition refinement setup")
		}
		if _, _, ok := refinement.Seal(); !ok {
			b.Fatal("partition refinement seal")
		}
	}
}

func BenchmarkGeneralRefinementAllocationScaling(b *testing.B) {
	for _, count := range []int{1, 64} {
		b.Run("sources="+strconv.Itoa(count), func(b *testing.B) {
			rows, pieces := newPartitionRows(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				refinePartitionOnce(rows, pieces)
			}
		})
	}
}

// BenchmarkPrefixQuotientAdversarialEqualPrefixes includes fresh manager and
// initial-row construction in every iteration. It therefore measures the full
// sealed partition-and-reduction path for each read depth, rather than a warm
// setup whose retained BDD state could hide growth in the reduction path.
func BenchmarkPrefixQuotientAdversarialEqualPrefixes(b *testing.B) {
	for _, reads := range []int{1, 4, 8, 12} {
		b.Run("reads="+strconv.Itoa(reads), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				rows, atoms, _ := newAdversarialPrefixRows(b)
				for _, atom := range atoms[:reads] {
					var ok bool
					rows, ok = adversarialEqualPrefixRead(rows, atom)
					if !ok {
						b.Fatal("adversarial equal-prefix read")
					}
				}
			}
		})
	}
}

type regions struct {
	manager      *guard.Manager
	all, a, notA support.Mask
	b, notB      support.Mask
	notAAndB     support.Mask
	notAAndNotB  support.Mask
}

func newRegions(tb testing.TB) regions {
	tb.Helper()
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		tb.Fatal(err)
	}
	work := support.New(manager)
	if work == nil {
		tb.Fatal("support work creation failed")
	}
	all := work.True()
	a, ok := work.Literal(1, true)
	if !ok {
		tb.Fatal("a literal")
	}
	notA, ok := work.Not(a)
	if !ok {
		tb.Fatal("not a")
	}
	b, ok := work.Literal(2, true)
	if !ok {
		tb.Fatal("b literal")
	}
	notB, ok := work.Not(b)
	if !ok {
		tb.Fatal("not b")
	}
	notAAndB, ok := work.And(notA, b)
	if !ok {
		tb.Fatal("not a and b")
	}
	notAAndNotB, ok := work.And(notA, notB)
	if !ok || !work.Seal() {
		tb.Fatal("not a and not b publication")
	}
	return regions{
		manager: manager, all: all, a: a, notA: notA, b: b, notB: notB,
		notAAndB: notAAndB, notAAndNotB: notAAndNotB,
	}
}

func splitOnA(t *testing.T, rows Rows, regions regions) Rows {
	t.Helper()
	refinement := rows.BeginRefine()
	if refinement == nil || !refinement.Add(0, regions.a) || !refinement.Add(0, regions.notA) {
		t.Fatal("first refinement setup")
	}
	result, sources, ok := refinement.Seal()
	if !ok || result.Count() != 2 || sources.Count() != 2 {
		t.Fatal("first refinement seal")
	}
	for piece := 0; piece < sources.Count(); piece++ {
		source, found := sources.Source(piece)
		if !found || source != 0 {
			t.Fatalf("first source[%d] = %d/%t, want 0", piece, source, found)
		}
	}
	return result
}

func splitOnB(t *testing.T, rows Rows, regions regions) Rows {
	t.Helper()
	refinement := rows.BeginRefine()
	if refinement == nil || !refinement.Add(0, regions.b) || !refinement.Add(0, regions.notB) {
		t.Fatal("B refinement setup")
	}
	result, sources, ok := refinement.Seal()
	if !ok || result.Count() != 2 || sources.Count() != 2 {
		t.Fatal("B refinement seal")
	}
	return result
}

func refineThree(t *testing.T, rows Rows, regions regions) (Rows, SourceRows) {
	t.Helper()
	refinement := rows.BeginRefine()
	if refinement == nil ||
		!refinement.Add(0, regions.a) ||
		!refinement.Add(1, regions.notAAndB) ||
		!refinement.Add(1, regions.notAAndNotB) {
		t.Fatal("three-piece refinement setup")
	}
	result, sources, ok := refinement.Seal()
	if !ok {
		t.Fatal("three-piece refinement seal")
	}
	return result, sources
}

func refineOnce(rows Rows, pieces []support.Mask) {
	refinement := rows.BeginRefine()
	if refinement == nil {
		panic("BeginRefine")
	}
	for source, piece := range pieces {
		if !refinement.Add(source, piece) {
			panic("Add")
		}
	}
	if _, _, ok := refinement.Seal(); !ok {
		panic("Seal")
	}
}

func refinePartitionOnce(rows Rows, pieces [][]support.Mask) {
	refinement := rows.BeginRefine()
	if refinement == nil {
		panic("BeginRefine")
	}
	for source, parts := range pieces {
		for _, part := range parts {
			if !refinement.Add(source, part) {
				panic("Add")
			}
		}
	}
	if _, _, ok := refinement.Seal(); !ok {
		panic("Seal")
	}
}

func newExactRows(tb testing.TB, count int) (Rows, []support.Mask) {
	tb.Helper()
	if count <= 0 || count&(count-1) != 0 {
		tb.Fatal("exact-row count must be a positive power of two")
	}
	bits := 0
	for width := count; width > 1; width >>= 1 {
		bits++
	}
	atoms := make([]guard.Atom, bits)
	for bit := range atoms {
		atoms[bit] = guard.Atom(bit + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		tb.Fatal(err)
	}
	work := support.New(manager)
	if work == nil {
		tb.Fatal("support work creation failed")
	}
	declared := work.True()
	if !declared.Valid() {
		tb.Fatal("declared support")
	}
	pieces := make([]support.Mask, count)
	for value := range pieces {
		piece := work.True()
		for bit, atom := range atoms {
			var ok bool
			piece, ok = work.Conjoin(piece, atom, value&(1<<bit) != 0)
			if !ok {
				tb.Fatal("could not construct exact source region")
			}
		}
		pieces[value] = piece
	}
	if !work.Seal() {
		tb.Fatal("could not publish exact source regions")
	}
	return sealedRows(manager, declared, pieces), pieces
}

// newPartitionRows gives each input source one independent split atom.  The
// benchmark therefore exercises the real multi-piece exact-cover proof rather
// than the no-op identity path.
func newPartitionRows(tb testing.TB, count int) (Rows, [][]support.Mask) {
	tb.Helper()
	if count <= 0 || count&(count-1) != 0 {
		tb.Fatal("partition-row count must be a positive power of two")
	}
	bits := 0
	for width := count; width > 1; width >>= 1 {
		bits++
	}
	atoms := make([]guard.Atom, bits+1)
	for bit := range atoms {
		atoms[bit] = guard.Atom(bit + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		tb.Fatal(err)
	}
	work := support.New(manager)
	if work == nil {
		tb.Fatal("support work creation failed")
	}
	declared := work.True()
	if !declared.Valid() {
		tb.Fatal("declared support")
	}
	rows := make([]support.Mask, count)
	pieces := make([][]support.Mask, count)
	for value := range rows {
		parent := work.True()
		for bit := 0; bit < bits; bit++ {
			var ok bool
			parent, ok = work.Conjoin(parent, atoms[bit], value&(1<<bit) != 0)
			if !ok {
				tb.Fatal("could not construct partition source region")
			}
		}
		low, ok := work.Conjoin(parent, atoms[bits], false)
		if !ok {
			tb.Fatal("could not construct low partition piece")
		}
		high, ok := work.Conjoin(parent, atoms[bits], true)
		if !ok {
			tb.Fatal("could not construct high partition piece")
		}
		rows[value] = parent
		pieces[value] = []support.Mask{low, high}
	}
	if !work.Seal() {
		tb.Fatal("could not publish partition source regions")
	}
	return sealedRows(manager, declared, rows), pieces
}

// newAdversarialPrefixRows starts one true-support row over a fixed ordered
// twelve-atom manager. The exact atom order is the read order used below.
func newAdversarialPrefixRows(tb testing.TB) (Rows, []guard.Atom, support.Mask) {
	tb.Helper()
	atoms := make([]guard.Atom, 12)
	for index := range atoms {
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		tb.Fatal(err)
	}
	original, ok := support.True(manager)
	if !ok {
		tb.Fatal("true support")
	}
	rows, ok := NewRows(original)
	if !ok {
		tb.Fatal("true-support rows")
	}
	return rows, atoms, original
}

// adversarialEqualPrefixRead partitions every input row exactly by atom,
// seals that refinement, and applies an opaque equality that places every
// output from a source in one class.
func adversarialEqualPrefixRead(rows Rows, atom guard.Atom) (Rows, bool) {
	if rows.Count() == 0 {
		return Rows{}, false
	}
	first, ok := rows.At(0)
	if !ok {
		return Rows{}, false
	}
	work := support.New(first.Manager())
	if work == nil {
		return Rows{}, false
	}
	pieces := make([][2]support.Mask, rows.Count())
	for source := 0; source < rows.Count(); source++ {
		parent, ok := rows.At(source)
		if !ok {
			work.Discard()
			return Rows{}, false
		}
		low, ok := work.Conjoin(parent, atom, false)
		if !ok {
			work.Discard()
			return Rows{}, false
		}
		high, ok := work.Conjoin(parent, atom, true)
		if !ok {
			work.Discard()
			return Rows{}, false
		}
		pieces[source] = [2]support.Mask{low, high}
	}
	if !work.Seal() {
		work.Discard()
		return Rows{}, false
	}

	refinement := rows.BeginRefine()
	if refinement == nil {
		return Rows{}, false
	}
	for source, split := range pieces {
		if !refinement.Add(source, split[0]) || !refinement.Add(source, split[1]) {
			return Rows{}, false
		}
	}
	partitioned, sources, ok := refinement.Seal()
	if !ok {
		return Rows{}, false
	}
	quotient, _, ok := partitioned.PrefixQuotientWithCheckpoint(sources, nil, func(int) (uint64, bool) { return 0, true }, func(_, _ int) bool { return true })
	return quotient, ok
}
