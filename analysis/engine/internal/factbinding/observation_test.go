package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type observationDeclarations struct {
	exact   [3]carrier.Unit
	summary carrier.Unit
	target  [3]carrier.Target
}

type observedRow struct {
	row         carrier.ObservationRow
	region      support.Mask
	handle      carrier.ObservationHandle
	observation Observation[uint64]
	entries     []ObservationEntry[uint64]
}

func TestObservationPreservesStoredAndAbsentEntries(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, on, off, _, _ := observationMasks(t, manager)
	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	base, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("base root")
	}

	initial := observe(t, binding, slotWork, base, declared.exact[0], whole)
	if len(initial) != 1 || len(initial[0].entries) != 1 || !sameEntry(initial[0].entries[0], 0, false) {
		t.Fatal("absent exact key did not preserve Default/absent token")
	}
	if initial[0].observation.Valid() || initial[0].observation.Count() != 0 {
		t.Fatal("completed callback left observation live")
	}
	if _, live := binding.ResolveObservation(slotWork, initial[0].row); live {
		t.Fatal("escaped completed row remained resolvable")
	}

	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.target[0], on, 1) {
		t.Fatal("staged stored branch")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	state = commit(t, work, state, candidate)
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	rows := observe(t, binding, slotWork, root, declared.exact[0], whole)
	if len(rows) != 2 {
		t.Fatalf("exact rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if len(row.entries) != 1 {
			t.Fatal("exact row did not retain one entry")
		}
		entry := row.entries[0]
		switch {
		case row.region.Equal(on):
			if !sameEntry(entry, 1, true) {
				value, present := entry.Read()
				t.Fatalf("stored entry = %d/%t, want 1/true", value, present)
			}
		case row.region.Equal(off):
			if !sameEntry(entry, 0, false) {
				value, present := entry.Read()
				t.Fatalf("absent entry = %d/%t, want 0/false", value, present)
			}
		default:
			t.Fatal("exact observation did not partition support")
		}
	}
}

func TestSummaryCoalescesEveryEqualEntrySequenceInCanonicalOrder(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	whole, onOne, offOne, onTwo, offTwo := observationMasks(t, manager)
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	lowLow, ok := regions.And(offOne, offTwo)
	if !ok {
		t.Fatal("low-low")
	}
	lowHigh, ok := regions.And(offOne, onTwo)
	if !ok {
		t.Fatal("low-high")
	}
	highLow, ok := regions.And(onOne, offTwo)
	if !ok {
		t.Fatal("high-low")
	}
	highHigh, ok := regions.And(onOne, onTwo)
	if !ok {
		t.Fatal("high-high")
	}
	first, ok := regions.Or(lowLow, highHigh)
	if !ok {
		t.Fatal("first sequence")
	}
	second, ok := regions.Or(lowHigh, highLow)
	if !ok {
		t.Fatal("second sequence")
	}
	atomThree, ok := regions.Literal(3, true)
	if !ok || !regions.Seal() {
		t.Fatal("atom three")
	}

	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil ||
		!patch.Write(declared.target[0], lowLow, 1) ||
		!patch.Write(declared.target[0], lowHigh, 2) ||
		!patch.Write(declared.target[0], highLow, 2) ||
		!patch.Write(declared.target[0], highHigh, 1) ||
		!patch.Write(declared.target[1], lowLow, 8) ||
		!patch.Write(declared.target[1], lowHigh, 4) ||
		!patch.Write(declared.target[1], highLow, 4) ||
		!patch.Write(declared.target[1], highHigh, 8) {
		t.Fatal("summary writes")
	}
	// A third declared key varies on atom three only. A whole-plane scan would
	// split the two summary groups again; the declared summary excludes it.
	if !patch.Write(declared.target[2], atomThree, 16) {
		t.Fatal("unselected key write")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	state = commit(t, work, state, candidate)
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	rows := observe(t, binding, slotWork, root, declared.summary, whole)
	if len(rows) != 2 {
		t.Fatalf("coalesced summary rows = %d, want 2", len(rows))
	}
	assertSequence := func(row observedRow, region support.Mask, left, right uint64) {
		if !row.region.Equal(region) || len(row.entries) != 2 {
			t.Fatal("summary region or sequence width")
		}
		for index, want := range []uint64{left, right} {
			if !sameEntry(row.entries[index], want, true) {
				value, present := row.entries[index].Read()
				t.Fatalf("entry %d = %d/%t, want %d/true", index, value, present, want)
			}
		}
	}
	assertSequence(rows[0], first, 1, 8)
	assertSequence(rows[1], second, 2, 4)
	// The same deterministic low/high discovery order is retained on every
	// generation; maps assist lookup but never select emission order.
	again := observe(t, binding, slotWork, root, declared.summary, whole)
	for index := range rows {
		if !rows[index].region.Equal(again[index].region) || !sameEntries(rows[index].entries, again[index].entries) {
			t.Fatal("summary emission was not canonical")
		}
	}
}

func TestExactCoalescesEqualNonadjacentCells(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	whole, onOne, offOne, onTwo, offTwo := observationMasks(t, manager)
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	lowLow, ok := regions.And(offOne, offTwo)
	if !ok {
		t.Fatal("low-low")
	}
	lowHigh, ok := regions.And(offOne, onTwo)
	if !ok {
		t.Fatal("low-high")
	}
	highLow, ok := regions.And(onOne, offTwo)
	if !ok {
		t.Fatal("high-low")
	}
	highHigh, ok := regions.And(onOne, onTwo)
	if !ok {
		t.Fatal("high-high")
	}
	stored, ok := regions.Or(lowLow, highHigh)
	if !ok {
		t.Fatal("stored diagonal")
	}
	absent, ok := regions.Or(lowHigh, highLow)
	if !ok || !regions.Seal() {
		t.Fatal("absent diagonal")
	}
	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.target[0], stored, 1) {
		t.Fatal("exact diagonal write")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	state = commit(t, work, state, candidate)
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	rows := observe(t, binding, slotWork, root, declared.exact[0], whole)
	if len(rows) != 2 {
		t.Fatalf("exact coalesced rows = %d, want 2", len(rows))
	}
	if !rows[0].region.Equal(stored) || len(rows[0].entries) != 1 || !sameEntry(rows[0].entries[0], 1, true) {
		t.Fatal("stored diagonal was not exact-coalesced")
	}
	if !rows[1].region.Equal(absent) || len(rows[1].entries) != 1 || !sameEntry(rows[1].entries[0], 0, false) {
		t.Fatal("absent diagonal was not exact-coalesced")
	}
}

func TestExactObservationRefinesHighLowAndNestedSourceRegions(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	_, onOne, offOne, onTwo, _ := observationMasks(t, manager)
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	highNested, ok := regions.And(onOne, onTwo)
	if !ok {
		t.Fatal("high nested")
	}
	lowNested, ok := regions.And(offOne, onTwo)
	if !ok || !regions.Seal() {
		t.Fatal("low nested")
	}
	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	// Key zero creates the Product-style source split. The second exact key
	// has the opposite branch, proving an ObserveUnder result never returns
	// that other key's whole-plane FDD cell outside its supplied source.
	if patch == nil || !patch.Write(declared.target[0], onOne, 1) || !patch.Write(declared.target[1], offOne, 2) {
		t.Fatal("cross-key writes")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	state = commit(t, work, state, candidate)
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	for _, source := range []struct {
		name   string
		region support.Mask
	}{
		{name: "high", region: onOne},
		{name: "low", region: offOne},
		{name: "high nested", region: highNested},
		{name: "low nested", region: lowNested},
	} {
		t.Run(source.name, func(t *testing.T) {
			assertExactObservationCover(t, binding, slotWork, root, declared.exact[1], source.region)
		})
	}
}

func TestConstantSummaryEmitsOneRawSequence(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, _, _, _, _ := observationMasks(t, manager)
	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	root, ok := state.HandleAt(slot)
	if !ok || !slotWork.BeginObservation() {
		t.Fatal("begin constant summary")
	}
	rows := observeLive(t, binding, slotWork, root, declared.summary, whole)
	if len(rows) != 1 || len(rows[0].entries) != 2 || !sameEntry(rows[0].entries[0], 0, false) || !sameEntry(rows[0].entries[1], 0, false) {
		t.Fatal("constant summary did not preserve its raw absent sequence")
	}
	if !slotWork.EndObservation() {
		t.Fatal("end constant summary")
	}
}

func TestObservationGenerationSpansSequentialUnitsAndCloseInvalidatesAll(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, _, _, _, _ := observationMasks(t, manager)
	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	if slotWork.ObserveUnder(root, declared.exact[0], whole, func(carrier.ObservationRow) bool { return true }) {
		t.Fatal("observation opened an implicit generation")
	}
	if !slotWork.BeginObservation() {
		t.Fatal("begin generation")
	}
	first := observeLive(t, binding, slotWork, root, declared.exact[0], whole)
	if len(first) != 1 || !first[0].observation.Valid() {
		t.Fatal("first unit did not stay live")
	}
	if _, live := binding.ResolveObservation(slotWork, first[0].row); !live {
		t.Fatal("first unit did not resolve after its own visit")
	}
	second := observeLive(t, binding, slotWork, root, declared.exact[1], whole)
	if len(second) != 1 || !first[0].observation.Valid() || !second[0].observation.Valid() {
		t.Fatal("sequential units did not share the live generation")
	}
	if entry, readable := first[0].observation.At(0); !readable || !sameEntry(entry, 0, false) {
		t.Fatal("second unit overwrote first unit's typed entry scratch")
	}
	if !slotWork.EndObservation() {
		t.Fatal("end generation")
	}
	if first[0].observation.Valid() || second[0].observation.Valid() {
		t.Fatal("end left sequential observations live")
	}
	if first[0].row.Region().Valid() || second[0].row.Region().Valid() {
		t.Fatal("end left an escaped row with a live support region")
	}
	if !slotWork.BeginObservation() {
		t.Fatal("begin next generation")
	}
	if _, live := binding.ResolveObservation(slotWork, first[0].row); live {
		t.Fatal("next generation accepted an old row")
	}
	if !slotWork.EndObservation() {
		t.Fatal("end empty generation")
	}
}

func TestObservationAbortAndGenerationReuseRejectEscapedRows(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, _, _, _, _ := observationMasks(t, manager)
	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	var escaped observedRow
	if !slotWork.BeginObservation() {
		t.Fatal("begin aborted generation")
	}
	if slotWork.ObserveUnder(root, declared.exact[0], whole, func(row carrier.ObservationRow) bool {
		escaped = resolveRow(t, binding, slotWork, row)
		return false
	}) {
		t.Fatal("aborted visit succeeded")
	}
	if escaped.observation.Valid() || escaped.observation.Count() != 0 {
		t.Fatal("aborted observation remained live")
	}
	if _, live := binding.ResolveObservation(slotWork, escaped.row); live {
		t.Fatal("aborted row remained resolvable")
	}
	if escaped.row.Region().Valid() {
		t.Fatal("aborted row retained a live support region")
	}
	fresh := observe(t, binding, slotWork, root, declared.exact[0], whole)
	if len(fresh) != 1 || escaped.handle == fresh[0].handle {
		t.Fatal("generation reuse aliased an escaped observation handle")
	}
}

func TestObservationRejectsForeignRootUnitWorkAndIssuer(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, _, _, _, _ := observationMasks(t, manager)
	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	foreignBinding, foreignState, _, foreignComposition, foreignDeclared := newObservationBinding(t, manager)
	foreignRoot, ok := foreignState.HandleAt(0)
	if !ok {
		t.Fatal("foreign root")
	}
	if !slotWork.BeginObservation() {
		t.Fatal("begin foreign-root generation")
	}
	if slotWork.ObserveUnder(foreignRoot, declared.exact[0], whole, func(carrier.ObservationRow) bool { return true }) {
		t.Fatal("foreign root observed")
	}
	if !slotWork.BeginObservation() {
		t.Fatal("begin foreign-unit generation")
	}
	if slotWork.ObserveUnder(root, foreignDeclared.exact[0], whole, func(carrier.ObservationRow) bool { return true }) {
		t.Fatal("foreign unit observed")
	}
	otherWork := newWork(t, composition)
	otherSlotWork, ok := otherWork.SlotWork(slot)
	if !ok {
		t.Fatal("other slot work")
	}
	foreignWork := newWork(t, foreignComposition)
	foreignSlotWork, ok := foreignWork.SlotWork(0)
	if !ok {
		t.Fatal("foreign slot work")
	}
	if !slotWork.BeginObservation() {
		t.Fatal("begin source generation")
	}
	if !slotWork.ObserveUnder(root, declared.exact[0], whole, func(row carrier.ObservationRow) bool {
		if _, live := binding.ResolveObservation(otherSlotWork, row); live {
			t.Fatal("foreign work resolved live row")
		}
		if _, live := foreignBinding.ResolveObservation(foreignSlotWork, row); live {
			t.Fatal("foreign issuer resolved live row")
		}
		return true
	}) {
		t.Fatal("source observation")
	}
	if !slotWork.EndObservation() {
		t.Fatal("end source generation")
	}
}

func BenchmarkObserveExact(b *testing.B) {
	benchmarkObservation(b, false, false, false)
}

// BenchmarkObserveExactBranched identifies the remaining exact-read floor
// when a real FDD split requires freshly published support cells. The
// one-piece BenchmarkObserveExact is the ordinary no-refinement hot path.
func BenchmarkObserveExactBranched(b *testing.B) {
	benchmarkObservation(b, false, true, false)
}

// BenchmarkObserveExactAligned proves that a source region which implies one
// FDD branch is retained as that branch's exact sealed mask without building
// a redundant intersection candidate.
func BenchmarkObserveExactAligned(b *testing.B) {
	benchmarkObservation(b, false, true, true)
}

func BenchmarkObserveSummary(b *testing.B) {
	benchmarkObservation(b, true, false, false)
}

func benchmarkObservation(b *testing.B, summary, branched, aligned bool) {
	atoms := []guard.Atom(nil)
	if branched {
		atoms = []guard.Atom{1}
	}
	manager, err := guard.New(atoms)
	if err != nil {
		b.Fatal(err)
	}
	whole, onOne, _, _, _ := observationMasks(b, manager)
	binding, state, slot, composition, declared := newObservationBinding(b, manager)
	work := newWork(b, composition)
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		b.Fatal("slot work")
	}
	if branched {
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Write(declared.target[0], onOne, 1) {
			b.Fatal("branched write")
		}
		candidate, accepted := patch.Accept(work)
		if !accepted {
			b.Fatal("branched accept")
		}
		state = commit(b, work, state, candidate)
	}
	within := whole
	if aligned {
		within = onOne
	}
	root, ok := state.HandleAt(slot)
	if !ok {
		b.Fatal("root")
	}
	unit := declared.exact[0]
	if summary {
		unit = declared.summary
	}
	visit := func(row carrier.ObservationRow) bool {
		observation, ok := binding.ResolveObservation(slotWork, row)
		if !ok || observation.Count() == 0 {
			return false
		}
		_, ok = observation.At(0)
		return ok
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !slotWork.BeginObservation() {
			b.Fatal("begin")
		}
		if !slotWork.ObserveUnder(root, unit, within, visit) {
			b.Fatal("observe")
		}
		if !slotWork.EndObservation() {
			b.Fatal("end")
		}
	}
}

func newObservationBinding(t testing.TB, manager *guard.Manager) (*Binding[uint64, uint64], carrier.State, shape.Slot, *carrier.Composition, observationDeclarations) {
	t.Helper()
	declared := observationDeclarations{}
	binding, ok := bindTest(observationInput(&declared), manager)
	if !ok {
		t.Fatal("binding")
	}
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("composition")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	return binding, state, 0, composition, declared
}

func observationInput(declared *observationDeclarations) testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd:  3,
		Default: 0,
		AdmitAt: func(_ uint64, _ uint64) bool { return true },
		Equal:   func(left, right uint64) bool { return left == right },
		// Force every terminal and sequence through one bucket: coalescing must
		// still use Factor Equal plus presence, never a fingerprint verdict.
		Fingerprint: func(uint64) uint64 { return 0 },
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
			for key := range declared.exact {
				unit, ok := binding.DeclareExact(uint64(key))
				if !ok {
					return false
				}
				declared.exact[key] = unit
			}
			var ok bool
			declared.summary, ok = binding.DeclareSummary([]uint64{0, 1})
			if !ok {
				return false
			}
			for index, unit := range declared.exact {
				target, accepted := binding.DeclareStrong(unit)
				if !accepted {
					return false
				}
				declared.target[index] = target
			}
			return true
		},
	}
}

func observationMasks(t testing.TB, manager *guard.Manager) (whole, onOne, offOne, onTwo, offTwo support.Mask) {
	t.Helper()
	work := support.New(manager)
	if work == nil {
		t.Fatal("support work")
	}
	whole = work.True()
	if _, ok := manager.Rank(1); ok {
		onOne, ok = work.Literal(1, true)
		if !ok {
			t.Fatal("atom one")
		}
		offOne, ok = work.Literal(1, false)
		if !ok {
			t.Fatal("not atom one")
		}
	}
	if _, ok := manager.Rank(2); ok {
		onTwo, ok = work.Literal(2, true)
		if !ok {
			t.Fatal("atom two")
		}
		offTwo, ok = work.Literal(2, false)
		if !ok {
			t.Fatal("not atom two")
		}
	}
	if !work.Seal() {
		t.Fatal("support seal")
	}
	return whole, onOne, offOne, onTwo, offTwo
}

func observe(t testing.TB, binding *Binding[uint64, uint64], work carrier.SlotWork, root carrier.RootHandle, unit carrier.Unit, within support.Mask) []observedRow {
	t.Helper()
	if !work.BeginObservation() {
		t.Fatal("begin observation")
	}
	rows := observeLive(t, binding, work, root, unit, within)
	if !work.EndObservation() {
		t.Fatal("end observation")
	}
	return rows
}

// observedExactValue reads one declared exact Unit through the same
// generation-bound SlotWork observation protocol used by the evaluator.  It
// deliberately retains neither a Binding-private root nor a diagram/value
// lookup path in tests.  The caller supplies both the declared Unit and the
// requested support/valuation.
func observedExactValue(binding *Binding[uint64, uint64], work *carrier.Work, root carrier.RootHandle, unit carrier.Unit, within support.Mask, valuation func(guard.Atom) bool) (uint64, bool, bool) {
	if binding == nil || work == nil || !within.Valid() || valuation == nil {
		return 0, false, false
	}
	slot, ok := unit.Slot()
	if !ok {
		return 0, false, false
	}
	slotWork, ok := work.SlotWork(slot)
	if !ok || !slotWork.BeginObservation() {
		return 0, false, false
	}
	var value uint64
	var present, found bool
	completed := slotWork.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
		if !row.Region().Matches(valuation) {
			return true
		}
		observation, resolved := binding.ResolveObservation(slotWork, row)
		if !resolved || observation.Count() != 1 || found {
			return false
		}
		entry, ok := observation.At(0)
		if !ok {
			return false
		}
		value, present = entry.Read()
		found = true
		return true
	})
	if !completed {
		return 0, false, false
	}
	if !slotWork.EndObservation() {
		return 0, false, false
	}
	return value, present, found
}

func observeLive(t testing.TB, binding *Binding[uint64, uint64], work carrier.SlotWork, root carrier.RootHandle, unit carrier.Unit, within support.Mask) []observedRow {
	t.Helper()
	rows := make([]observedRow, 0)
	if !work.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
		rows = append(rows, resolveRow(t, binding, work, row))
		return true
	}) {
		t.Fatal("observe")
	}
	return rows
}

func resolveRow(t testing.TB, binding *Binding[uint64, uint64], work carrier.SlotWork, row carrier.ObservationRow) observedRow {
	t.Helper()
	observation, ok := binding.ResolveObservation(work, row)
	if !ok || !observation.Valid() {
		t.Fatal("resolve observation")
	}
	entries := make([]ObservationEntry[uint64], observation.Count())
	for index := range entries {
		entry, ok := observation.At(index)
		if !ok {
			t.Fatal("entry")
		}
		entries[index] = entry
	}
	return observedRow{row: row, region: row.Region(), handle: row.Handle(), observation: observation, entries: entries}
}

func assertExactObservationCover(t *testing.T, binding *Binding[uint64, uint64], work carrier.SlotWork, root carrier.RootHandle, unit carrier.Unit, within support.Mask) {
	t.Helper()
	if !work.BeginObservation() {
		t.Fatal("begin observation")
	}
	rows := observeLive(t, binding, work, root, unit, within)
	union := support.New(within.Manager())
	if union == nil {
		t.Fatal("cover work")
	}
	covered := union.False()
	for _, row := range rows {
		if !row.region.Valid() || support.Empty(row.region) || !row.region.Entails(within) {
			union.Discard()
			t.Fatal("exact observation escaped its source region")
		}
		var ok bool
		covered, ok = union.Or(covered, row.region)
		if !ok {
			union.Discard()
			t.Fatal("cover union")
		}
	}
	if !union.Seal() || !covered.Entails(within) || !within.Entails(covered) {
		t.Fatal("exact observation did not exactly cover its source region")
	}
	if !work.EndObservation() {
		t.Fatal("end observation")
	}
}

func sameEntries(left, right []ObservationEntry[uint64]) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftValue, leftPresent := left[index].Read()
		rightValue, rightPresent := right[index].Read()
		if leftPresent != rightPresent || leftValue != rightValue {
			return false
		}
	}
	return true
}

func sameEntry(entry ObservationEntry[uint64], value uint64, present bool) bool {
	got, stored := entry.Read()
	return got == value && stored == present
}
