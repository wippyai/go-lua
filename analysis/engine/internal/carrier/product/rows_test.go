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
	sources := SourceRows{offsets: []int{0, 4}}
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
		manager: manager, all: all, a: a, notA: notA,
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
	return Rows{manager: manager, cells: pieces}, pieces
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
	return Rows{manager: manager, cells: rows}, pieces
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
