package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type changeDeclarations struct {
	exact   [3]carrier.Unit
	summary carrier.Unit
	strong  [2]carrier.Target
	weak    carrier.Target
}

func changeFixture(t testing.TB) (*Binding[uint64, uint64], carrier.State, shape.Slot, *carrier.Composition, support.Mask, support.Mask, support.Mask, changeDeclarations) {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("support work")
	}
	all := work.True()
	on, ok := work.Literal(1, true)
	if !ok {
		t.Fatal("on region")
	}
	off, ok := work.Literal(1, false)
	if !ok || !work.Seal() {
		t.Fatal("off region")
	}
	var declared changeDeclarations
	config := testAlgebraInput[uint64, uint64]{
		KeyEnd:      3,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		LessOrEq: func(left, right uint64) bool { return left <= right },
		declare: func(binding *Binding[uint64, uint64]) bool {
			var ok bool
			for key := range declared.exact {
				declared.exact[key], ok = binding.DeclareExact(uint64(key))
				if !ok {
					return false
				}
			}
			declared.summary, ok = binding.DeclareSummary([]uint64{0, 1})
			if !ok {
				return false
			}
			declared.strong[0], ok = binding.DeclareStrong(declared.exact[0])
			if !ok {
				return false
			}
			declared.strong[1], ok = binding.DeclareStrong(declared.exact[1])
			if !ok {
				return false
			}
			declared.weak, ok = binding.DeclareWeak([]carrier.Unit{declared.exact[0], declared.exact[1]})
			return ok
		},
	}
	binding, state, slot, composition, _ := bindingState(t, manager, config, all)
	return binding, state, slot, composition, all, on, off, declared
}

func TestDirectWriteChangeSetIsExactAndClosesSummaryUnits(t *testing.T) {
	binding, state, slot, composition, all, on, off, declared := changeFixture(t)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.strong[0], on, 7) || !patch.Write(declared.strong[1], off, 9) {
		t.Fatal("disjoint direct writes")
	}
	publication, ok := patch.Accept(work)
	if !ok {
		t.Fatal("publication")
	}
	next, changes, ok := work.Commit(state, []carrier.Patch{publication})
	if !ok || !support.Empty(changes.Added()) || !support.Empty(changes.Removed()) || changes.Count() != 3 {
		t.Fatalf("change set shape: ok=%t count=%d", ok, changes.Count())
	}
	wantUnits := []carrier.Unit{declared.exact[0], declared.exact[1], declared.summary}
	wantRegions := []support.Mask{on, off, all}
	for index := range wantUnits {
		row, present := changes.At(index)
		if !present || !row.Unit().Same(wantUnits[index]) || !row.Region().Equal(wantRegions[index]) {
			t.Fatalf("change row %d did not retain canonical unit/region", index)
		}
	}
	for index := 0; index < changes.Count(); index++ {
		row, _ := changes.At(index)
		if row.Unit().Same(declared.exact[2]) {
			t.Fatal("unrelated exact unit was invalidated")
		}
	}
	factor, present := changes.FactorAt(0)
	if changes.FactorCount() != 1 || !present || factor.Slot() != slot || !factor.Region().Equal(all) {
		t.Fatal("direct write did not publish its exact whole-Factor region")
	}
	root, _ := next.HandleAt(slot)
	if value, _, valid := observedExactValue(binding, work, root, declared.exact[0], on, func(atom guard.Atom) bool { return atom == 1 }); !valid || value != 7 {
		t.Fatalf("high key 0 = %d/%t", value, valid)
	}
	if value, _, valid := observedExactValue(binding, work, root, declared.exact[1], off, func(guard.Atom) bool { return false }); !valid || value != 9 {
		t.Fatalf("low key 1 = %d/%t", value, valid)
	}
}

func TestDirectWriteTracksNetDifferenceAndErasesRestoredRegion(t *testing.T) {
	binding, state, slot, composition, all, on, off, declared := changeFixture(t)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.strong[0], all, 7) || !patch.Write(declared.strong[0], on, 0) {
		t.Fatal("write then regional restore")
	}
	publication, ok := patch.Accept(work)
	if !ok {
		t.Fatal("publication")
	}
	_, changes, ok := work.Commit(state, []carrier.Patch{publication})
	if !ok || changes.Count() != 2 {
		t.Fatalf("net regional changes = %d/%t, want exact+summary", changes.Count(), ok)
	}
	for index, unit := range []carrier.Unit{declared.exact[0], declared.summary} {
		row, present := changes.At(index)
		if !present || !row.Unit().Same(unit) || !row.Region().Equal(off) {
			t.Fatalf("net row %d retained restored high region", index)
		}
	}
	factor, present := changes.FactorAt(0)
	if changes.FactorCount() != 1 || !present || factor.Slot() != slot || !factor.Region().Equal(off) {
		t.Fatal("net direct write did not erase restored Factor region")
	}

	restore := binding.Begin(work, state)
	if restore == nil || !restore.Write(declared.strong[0], on, 7) || !restore.Write(declared.strong[0], on, 0) {
		t.Fatal("complete restore")
	}
	noOp, ok := restore.Accept(work)
	if !ok {
		t.Fatal("no-op publication")
	}
	unchanged, empty, ok := work.Commit(state, []carrier.Patch{noOp})
	if !ok || !empty.Empty() {
		t.Fatal("restored write emitted a semantic change")
	}
	left, _ := state.HandleAt(slot)
	right, _ := unchanged.HandleAt(slot)
	if left != right {
		t.Fatal("restored write did not retain predecessor root identity")
	}
}

func TestAbsentDefaultWriteHasEmptyChangeSetAndRejectsUnproducedChange(t *testing.T) {
	binding, state, slot, composition, all, _, _, declared := changeFixture(t)
	work := newWork(t, composition)
	if _, ok := work.Accept(state, carrier.ChangeHandle{}); ok {
		t.Fatal("carrier accepted an unproduced change handle")
	}
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.strong[0], all, 0) {
		t.Fatal("absent default write")
	}
	publication, ok := patch.Accept(work)
	if !ok {
		t.Fatal("default publication")
	}
	unchanged, changes, ok := work.Commit(state, []carrier.Patch{publication})
	if !ok || !changes.Empty() || changes.FactorCount() != 0 {
		t.Fatal("absent Default emitted a change")
	}
	before, _ := state.HandleAt(slot)
	after, _ := unchanged.HandleAt(slot)
	if before != after {
		t.Fatal("absent Default replaced predecessor root identity")
	}
}

func TestWeakTargetChangeSetIncludesAllReverseSummaryClosure(t *testing.T) {
	binding, state, _, composition, all, _, _, declared := changeFixture(t)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.weak, all, 4) {
		t.Fatal("weak target")
	}
	publication, ok := patch.Accept(work)
	if !ok {
		t.Fatal("weak publication")
	}
	_, changes, ok := work.Commit(state, []carrier.Patch{publication})
	if !ok || changes.Count() != 3 {
		t.Fatalf("weak closure count = %d/%t", changes.Count(), ok)
	}
	for index, unit := range []carrier.Unit{declared.exact[0], declared.exact[1], declared.summary} {
		row, present := changes.At(index)
		if !present || !row.Unit().Same(unit) || !row.Region().Equal(all) {
			t.Fatalf("weak closure row %d", index)
		}
	}
	factor, present := changes.FactorAt(0)
	if changes.FactorCount() != 1 || !present || !factor.Region().Equal(all) {
		t.Fatal("weak write did not retain exact Factor delta")
	}
}

// TestTransformClosurePublishesOnlyItsExactReverseDependencies proves the
// transformed-carry substrate is one ordinary typed Patch: it maps the
// precompiled closure, lets a later strong result overwrite one mapped cell,
// and publishes only the net exact/summarized ChangeSet.  No State scan or
// second carry representation participates.
func TestTransformClosurePublishesOnlyItsExactReverseDependencies(t *testing.T) {
	binding, state, slot, composition, all, _, _, declared := changeFixture(t)
	work := newWork(t, composition)
	seed := binding.Begin(work, state)
	if seed == nil || !seed.Write(declared.strong[0], all, 1) || !seed.Write(declared.strong[1], all, 1) {
		t.Fatal("transform seed")
	}
	seedPatch, accepted := seed.Accept(work)
	if !accepted {
		t.Fatal("transform seed accept")
	}
	seeded, _, committed := work.Commit(state, []carrier.Patch{seedPatch})
	if !committed {
		t.Fatal("transform seed commit")
	}
	closure, closed := binding.TransformClosure([]carrier.Target{declared.strong[0], declared.strong[1]})
	reversed, reverseClosed := binding.TransformClosure([]carrier.Target{declared.strong[1], declared.strong[0]})
	if !closed || !reverseClosed || len(closure.keys) != len(reversed.keys) {
		t.Fatal("transform closure order")
	}
	for index := range closure.keys {
		if closure.keys[index] != reversed.keys[index] {
			t.Fatal("transform closure depends on target declaration order")
		}
	}
	patch := binding.Begin(work, seeded)
	transformCalls := 0
	if patch == nil || !patch.Transform(closure, all, func(value uint64) (uint64, bool) {
		transformCalls++
		if value == 0 {
			return 0, true
		}
		if value == 1 {
			return 2, true
		}
		return value, true
	}) || !patch.Write(declared.strong[0], all, 9) {
		t.Fatal("transform plus result write")
	}
	// One invocation checks Default; the equal carried terminal is then mapped
	// once for both keys by the candidate-local terminal memo.
	if transformCalls != 2 {
		t.Fatalf("transform calls = %d, want default plus one carried terminal", transformCalls)
	}
	publication, accepted := patch.Accept(work)
	if !accepted {
		t.Fatal("transform accept")
	}
	next, changes, committed := work.Commit(seeded, []carrier.Patch{publication})
	if !committed || changes.Count() != 3 || changes.FactorCount() != 1 {
		t.Fatalf("transform changes = commit:%t rows:%d factors:%d", committed, changes.Count(), changes.FactorCount())
	}
	for index, unit := range []carrier.Unit{declared.exact[0], declared.exact[1], declared.summary} {
		row, present := changes.At(index)
		if !present || !row.Unit().Same(unit) || !row.Region().Equal(all) {
			t.Fatalf("transform change row %d", index)
		}
	}
	factor, present := changes.FactorAt(0)
	if !present || factor.Slot() != slot || !factor.Region().Equal(all) {
		t.Fatal("transform factor region")
	}
	root, rootOK := next.HandleAt(slot)
	if !rootOK {
		t.Fatal("transform root")
	}
	for key, want := range []uint64{9, 2} {
		got, valuePresent, valid := observedExactValue(binding, work, root, declared.exact[key], all, func(guard.Atom) bool { return false })
		if !valid || !valuePresent || got != want {
			t.Fatalf("transform value[%d] = %d/%t/%t, want %d", key, got, valuePresent, valid, want)
		}
	}
	// Reapplying the idempotent map is the local recurrence law: it reaches
	// the same post-fixpoint with no Fact or reader invalidation, rather than
	// relying on an iteration budget to stop repeated carry transport.
	stable := binding.Begin(work, next)
	if stable == nil || !stable.Transform(closure, all, func(value uint64) (uint64, bool) {
		if value == 0 {
			return 0, true
		}
		if value == 1 {
			return 2, true
		}
		return value, true
	}) {
		t.Fatal("stable transform")
	}
	stablePatch, stableAccepted := stable.Accept(work)
	if !stableAccepted {
		t.Fatal("stable transform accept")
	}
	stabilized, stableChanges, stableCommitted := work.Commit(next, []carrier.Patch{stablePatch})
	if !stableCommitted || !stableChanges.Empty() {
		t.Fatalf("stable transform emitted change: commit:%t changes:%d", stableCommitted, stableChanges.Count())
	}
	before, beforeOK := next.HandleAt(slot)
	after, afterOK := stabilized.HandleAt(slot)
	if !beforeOK || !afterOK || before != after {
		t.Fatal("stable transform replaced an unchanged root")
	}

	// Transfers need not be extensive. A descending but monotone owner map is
	// publishable; allocation recency has exactly this shape because Recent and
	// Summary are distinct atoms rather than an ascending approximation step.
	descending := binding.Begin(work, seeded)
	if descending == nil || !descending.Transform(closure, all, func(value uint64) (uint64, bool) {
		if value == 0 {
			return 0, true
		}
		return 0, true
	}) {
		t.Fatal("non-extensive transform was rejected")
	}
	descendingPatch, accepted := descending.Accept(work)
	if !accepted {
		t.Fatal("non-extensive transform was not publishable")
	}
	descended, descendedChanges, committed := work.Commit(seeded, []carrier.Patch{descendingPatch})
	if !committed || descendedChanges.Count() != 3 || descendedChanges.FactorCount() != 1 {
		t.Fatalf("non-extensive transform changes = commit:%t units:%d factors:%d", committed, descendedChanges.Count(), descendedChanges.FactorCount())
	}
	descendedRoot, rootOK := descended.HandleAt(slot)
	if !rootOK {
		t.Fatal("non-extensive transform root")
	}
	for _, key := range []uint64{0, 1} {
		got, present, valid := observedExactValue(binding, work, descendedRoot, declared.exact[key], all, func(guard.Atom) bool { return false })
		if !valid || present || got != 0 {
			t.Fatalf("non-extensive transform value[%d] = %d/%t/%t", key, got, present, valid)
		}
	}
}

func TestTransformClosureStaysSparseAtLargeKeyUniverse(t *testing.T) {
	binding, state, slot, composition, all, unit, target := sparseCatalogFixture(t, 4_096)
	work := newWork(t, composition)
	seed := binding.Begin(work, state)
	if seed == nil || !seed.Write(target, all, 1) {
		t.Fatal("sparse transform seed")
	}
	seedPatch, accepted := seed.Accept(work)
	if !accepted {
		t.Fatal("sparse transform seed accept")
	}
	seeded, _, committed := work.Commit(state, []carrier.Patch{seedPatch})
	if !committed {
		t.Fatal("sparse transform seed commit")
	}
	closure, closed := binding.TransformClosure([]carrier.Target{target})
	if !closed || len(closure.keys) != 1 {
		t.Fatalf("sparse transform closure keys = %d, want 1", len(closure.keys))
	}
	patch := binding.Begin(work, seeded)
	if patch == nil || !patch.Transform(closure, all, func(value uint64) (uint64, bool) {
		if value == 0 {
			return 0, true
		}
		return 2, true
	}) {
		t.Fatal("sparse transform")
	}
	publication, accepted := patch.Accept(work)
	if !accepted {
		t.Fatal("sparse transform accept")
	}
	next, changes, committed := work.Commit(seeded, []carrier.Patch{publication})
	if !committed || changes.Count() != 1 || changes.FactorCount() != 1 {
		t.Fatalf("sparse transform changes = commit:%t units:%d factors:%d", committed, changes.Count(), changes.FactorCount())
	}
	root, rootOK := next.HandleAt(slot)
	if !rootOK {
		t.Fatal("sparse transform root")
	}
	value, present, valid := observedExactValue(binding, work, root, unit, all, func(guard.Atom) bool { return false })
	if !valid || !present || value != 2 {
		t.Fatalf("sparse transform value = %d/%t/%t", value, present, valid)
	}
}

func sparseCatalogFixture(t testing.TB, size uint64) (*Binding[uint64, uint64], carrier.State, shape.Slot, *carrier.Composition, support.Mask, carrier.Unit, carrier.Target) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	var first carrier.Unit
	var target carrier.Target
	config := testAlgebraInput[uint64, uint64]{
		KeyEnd:      size,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		LessOrEq: func(left, right uint64) bool { return left <= right },
		declare: func(binding *Binding[uint64, uint64]) bool {
			for key := uint64(0); key < size; key++ {
				unit, declared := binding.DeclareExact(key)
				if !declared {
					return false
				}
				if key == 0 {
					first = unit
				}
			}
			var declared bool
			target, declared = binding.DeclareStrong(first)
			return declared
		},
	}
	binding, state, slot, composition, _ := bindingState(t, manager, config, whole)
	return binding, state, slot, composition, whole, first, target
}

func TestSparseChangeDoesNotPublishUnrelatedLargeCatalog(t *testing.T) {
	binding, state, _, composition, whole, exact, target := sparseCatalogFixture(t, 4096)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(target, whole, 1) {
		t.Fatal("single-key write")
	}
	publication, ok := patch.Accept(work)
	if !ok {
		t.Fatal("single-key publication")
	}
	_, changes, ok := work.Commit(state, []carrier.Patch{publication})
	row, present := changes.At(0)
	if !ok || changes.Count() != 1 || !present || !row.Unit().Same(exact) || !row.Region().Equal(whole) || changes.FactorCount() != 1 {
		t.Fatalf("large sparse catalog publication = %t/%d/%t", ok, changes.Count(), present)
	}
}

func benchmarkSparseChangeCatalog(b *testing.B, size uint64) {
	b.Helper()
	binding, state, _, composition, whole, exact, target := sparseCatalogFixture(b, size)
	work := newWork(b, composition)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Write(target, whole, 1) {
			b.Fatal("single-key write")
		}
		publication, ok := patch.Accept(work)
		if !ok {
			b.Fatal("single-key publication")
		}
		_, changes, ok := work.Commit(state, []carrier.Patch{publication})
		row, present := changes.At(0)
		if !ok || changes.Count() != 1 || !present || !row.Unit().Same(exact) {
			b.Fatal("inexact sparse publication")
		}
	}
}

func BenchmarkSparseChangeCatalog16(b *testing.B) {
	benchmarkSparseChangeCatalog(b, 16)
}

func BenchmarkSparseChangeCatalog4096(b *testing.B) {
	benchmarkSparseChangeCatalog(b, 4096)
}

func TestCompositionInputOrderDeterminesSlotsAndInitialRoots(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	first, firstFixture := newLawBinding(t, manager, true)
	second, secondFixture := newLawBinding(t, manager, true)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{second, first})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	secondSlot, firstSlot := shape.Slot(0), shape.Slot(1)
	secondRoot, secondOK := state.HandleAt(secondSlot)
	firstRoot, firstOK := state.HandleAt(firstSlot)
	if !secondOK || !firstOK || !second.ValidRoot(secondRoot) || !first.ValidRoot(firstRoot) || first.ValidRoot(secondRoot) || second.ValidRoot(firstRoot) {
		t.Fatal("input order did not determine owned slots")
	}
	work := newWork(t, composition)
	stageBoth := func() (carrier.Patch, carrier.Patch) {
		firstStage := first.Begin(work, state)
		secondStage := second.Begin(work, state)
		if firstStage == nil || secondStage == nil ||
			!firstStage.Write(firstFixture.target(t, 0, carrier.StrongTarget), whole, 1) ||
			!secondStage.Write(secondFixture.target(t, 0, carrier.StrongTarget), whole, 2) {
			t.Fatal("distinct factor stages")
		}
		firstPatch, firstOK := firstStage.Accept(work)
		secondPatch, secondOK := secondStage.Accept(work)
		if !firstOK || !secondOK {
			t.Fatal("distinct factor publications")
		}
		return firstPatch, secondPatch
	}
	firstPatch, secondPatch := stageBoth()
	if _, _, ok := work.Commit(state, []carrier.Patch{firstPatch, secondPatch}); ok {
		t.Fatal("noncanonical publication order was accepted")
	}
	if _, _, ok := work.Commit(state, []carrier.Patch{secondPatch, firstPatch}); ok {
		t.Fatal("rejected batch remained retryable")
	}
	duplicateStage := first.Begin(work, state)
	if duplicateStage == nil || !duplicateStage.Write(firstFixture.target(t, 0, carrier.StrongTarget), whole, 1) {
		t.Fatal("duplicate factor stage")
	}
	duplicate, ok := duplicateStage.Accept(work)
	if !ok {
		t.Fatal("duplicate factor publication")
	}
	if _, _, ok := work.Commit(state, []carrier.Patch{duplicate, duplicate}); ok {
		t.Fatal("duplicate output Factor was accepted")
	}
	firstPatch, secondPatch = stageBoth()
	next, changes, ok := work.Commit(state, []carrier.Patch{secondPatch, firstPatch})
	if !ok || !next.Valid() || changes.Count() != 2 {
		t.Fatalf("canonical batch = %t/%t/%d", ok, next.Valid(), changes.Count())
	}
	for index, slot := range []shape.Slot{secondSlot, firstSlot} {
		row, present := changes.At(index)
		owner, owned := row.Unit().Slot()
		if !present || !owned || owner != slot || !row.Region().Equal(whole) {
			t.Fatalf("canonical batch row %d", index)
		}
	}
}
