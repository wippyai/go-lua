package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// pointExactObservation is deliberately collected through the public,
// callback-free DirectObservation cursor. It keeps no Binding-private root or
// semantic plane, so the transport law below compares only Factor-issued
// typed entries and carrier-owned support rows.
type pointExactObservation struct {
	region  support.Mask
	value   uint64
	present bool
}

func collectPointExactObservation(t testing.TB, binding *Binding[uint64, uint64], work *carrier.Work, point carrier.PointState, unit carrier.Unit, within support.Mask) []pointExactObservation {
	t.Helper()
	var cursor DirectObservation[uint64, uint64]
	if !binding.BeginPointObservation(&cursor, work, point, unit, within) {
		t.Fatal("begin point observation")
	}
	rows := make([]pointExactObservation, 0, 2)
	for {
		row, view, status := cursor.Step()
		switch status {
		case DirectObservationExhausted:
			if !cursor.Close() {
				t.Fatal("close point observation")
			}
			return rows
		case DirectObservationAvailable:
			if !view.Valid() || view.Count() != 1 || !row.Region().Valid() {
				_ = cursor.Close()
				t.Fatal("invalid exact point observation")
			}
			entry, ok := view.At(0)
			if !ok {
				_ = cursor.Close()
				t.Fatal("missing exact point entry")
			}
			value, present := entry.Read()
			rows = append(rows, pointExactObservation{region: row.Region(), value: value, present: present})
		default:
			_ = cursor.Close()
			t.Fatal("point observation refused")
		}
	}
}

func pointExactAt(t testing.TB, rows []pointExactObservation, region support.Mask) (pointExactObservation, bool) {
	t.Helper()
	for _, row := range rows {
		if row.region.Equal(region) {
			return row, true
		}
	}
	return pointExactObservation{}, false
}

func samePointExact(left, right pointExactObservation) bool {
	return left.region.Equal(right.region) && left.value == right.value && left.present == right.present
}

// TestPointTransportObservationPreservesTypedCells proves the missing G8
// semantic half. The source and transported PointState are observed through
// the one Factor-owned Unit, while carrier remains the only owner of root
// lookup and TransportPointState. Both coordinate filtering and a nonidentity
// forget transport are checked; no domain plane or callback is opened.
func TestPointTransportObservationPreservesTypedCells(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on region")
	}
	off, ok := regions.Literal(1, false)
	if !ok {
		t.Fatal("off region")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("regions")
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity transport")
	}
	forget, targetScope := forgetPlan(t, composition, composition.Scope(), 1)
	plan := compositionPlan(t, composition)
	work := newWork(t, composition)

	onValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	offValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
	merged, _, ok := work.MergeContribution(onValue, offValue)
	if !ok {
		t.Fatal("merge source fibers")
	}
	rule, ok := work.AsRuleContribution(merged)
	if !ok {
		t.Fatal("source rule role")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok || !work.OwnsPointState(point) {
		t.Fatal("source point role")
	}
	unit := fixture.unit(t, 0)

	// Coordinate identity narrows the semantic support while retaining the
	// immutable source roots. The typed on-cell must be identical before and
	// after that one carrier transport.
	narrowed, ok := work.TransportPointState(point, whole, identity, on)
	if !ok || !work.OwnsPointState(narrowed) || narrowed.Support().Equal(whole) {
		t.Fatal("coordinate point transport")
	}
	before := collectPointExactObservation(t, binding, work, point, unit, whole)
	after := collectPointExactObservation(t, binding, work, narrowed, unit, on)
	beforeOn, beforeOnOK := pointExactAt(t, before, on)
	afterOn, afterOnOK := pointExactAt(t, after, on)
	if !beforeOnOK || !afterOnOK || beforeOn.value != 4 || !beforeOn.present || !samePointExact(afterOn, beforeOn) {
		t.Fatalf("coordinate typed cell before=%+v after=%+v", beforeOn, afterOn)
	}

	// A nonidentity forget relation is checked with a source-pre restriction,
	// so only the declared off source cell reaches the target cell. This proves
	// semantic preservation through typed reindex, not merely RootHandle reuse.
	routed, ok := work.TransportPointState(point, off, forget, whole)
	if !ok || !work.OwnsPointState(routed) || !routed.Scope().Same(targetScope) {
		t.Fatal("forget point transport")
	}
	routedRows := collectPointExactObservation(t, binding, work, routed, unit, whole)
	routedCell, routedOK := pointExactAt(t, routedRows, whole)
	if !routedOK || routedCell.value != 9 || !routedCell.present {
		t.Fatalf("forgotten typed cell=%+v, want 9/present", routedCell)
	}

	// Keep the initial point live so the test also proves the helper did not
	// accidentally use an unowned State alias while observing the source.
	if !work.OwnsPointState(point) || !work.OwnsState(initial) {
		t.Fatal("source point/state ownership changed")
	}
	if _, ok := point.HandleAt(slot); !ok {
		t.Fatal("source root disappeared")
	}
}

// TestPointObservationRejectsForeignGenerationTypeSlotAndAbsentRoot covers
// the nearest negative fence for the new surface. Every refusal occurs before
// typed entries are emitted: foreign PointState/Unit, a stale generation,
// wrong physical slot, a different generic binding, and a zero PointState.
func TestPointObservationRejectsForeignGenerationTypeSlotAndAbsentRoot(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	local, localState, _, localComposition, localFixture := bindingState(t, manager, transportConfig(0), whole)
	localWork := newWork(t, localComposition)
	localPoint, ok := localWork.EmptyPointState(localState)
	if !ok {
		t.Fatal("local point")
	}
	localUnit := localFixture.unit(t, 0)

	_, foreignState, _, foreignComposition, foreignFixture := bindingState(t, manager, transportConfig(0), whole)
	foreignWork := newWork(t, foreignComposition)
	foreignPoint, ok := foreignWork.EmptyPointState(foreignState)
	if !ok {
		t.Fatal("foreign point")
	}
	var cursor DirectObservation[uint64, uint64]
	if local.BeginPointObservation(&cursor, localWork, foreignPoint, localUnit, whole) {
		t.Fatal("foreign point/generation accepted")
	}
	if local.BeginPointObservation(&cursor, localWork, localPoint, foreignFixture.unit(t, 0), whole) {
		t.Fatal("foreign factor unit accepted")
	}

	// A zero PointState has no carrier role seal or root vector and must not be
	// relabelled as a typed input merely because the Unit is declared.
	if local.BeginPointObservation(&cursor, localWork, carrier.PointState{}, localUnit, whole) {
		t.Fatal("absent point/root accepted")
	}

	// The callback-free cursor's typed view is generation-bound. Closing the
	// source generation revokes both the row and the Observation view.
	if !local.BeginPointObservation(&cursor, localWork, localPoint, localUnit, whole) {
		t.Fatal("local point observation")
	}
	_, view, status := cursor.Step()
	if status != DirectObservationAvailable || !view.Valid() {
		t.Fatal("initial generation observation")
	}
	if !cursor.Close() || view.Valid() {
		t.Fatal("stale generation remained readable")
	}

	// Two typed Factors in one carrier composition provide the nearest slot
	// mismatch without manufacturing or mutating a Unit.
	leftFixture := newTestFixture(1)
	leftInput := transportConfig(0)
	leftInput.declare = leftFixture.declareAllExact
	left, ok := bindTest(leftInput, manager)
	if !ok {
		t.Fatal("left binding")
	}
	rightFixture := newTestFixture(1)
	rightInput := transportConfig(0)
	rightInput.declare = rightFixture.declareAllExact
	right, ok := bindTest(rightInput, manager)
	if !ok {
		t.Fatal("right binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{left, right})
	if !ok {
		t.Fatal("two-factor prepare")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("two-factor attach")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("two-factor state")
	}
	work := newWork(t, composition)
	point, ok := work.EmptyPointState(state)
	if !ok {
		t.Fatal("two-factor point")
	}
	if left.BeginPointObservation(&cursor, work, point, rightFixture.unit(t, 0), whole) {
		t.Fatal("mismatched factor slot accepted")
	}

	// A different generic binding cannot resolve the local slot work, even
	// though it uses the same carrier-level Unit shape and guard manager.
	otherAlgebra, ok := Admit[uint32, uint32](1, 0, lattice.Lattice[uint32]{
		Bottom:   func() uint32 { return 0 },
		Top:      func() uint32 { return 0 },
		Equal:    func(left, right uint32) bool { return left == right },
		Same:     func(left, right uint32) bool { return left == right },
		LessOrEq: func(left, right uint32) bool { return left <= right },
		Join: func(left, right uint32) uint32 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint32) uint32 {
			if left > right {
				return left
			}
			return right
		},
	}, func(_ uint32, _ uint32) bool { return true }, func(value uint32) uint64 { return uint64(value) }, Measure[uint32, uint32]{}, Measure[uint32, uint32]{})
	if !ok {
		t.Fatal("other algebra")
	}
	var otherUnit carrier.Unit
	other, ok := Bind(otherAlgebra, manager, func(binding *Binding[uint32, uint32]) bool {
		var declared bool
		otherUnit, declared = binding.DeclareExact(0)
		return declared
	})
	if !ok {
		t.Fatal("other binding")
	}
	otherPrepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{other})
	if !ok {
		t.Fatal("other prepare")
	}
	if _, ok := otherPrepared.Attach(); !ok {
		t.Fatal("other attach")
	}
	var otherCursor DirectObservation[uint32, uint32]
	if other.BeginPointObservation(&otherCursor, work, point, otherUnit, whole) {
		t.Fatal("mismatched typed binding accepted")
	}
}
