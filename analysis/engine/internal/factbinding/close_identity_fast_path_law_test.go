package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestSameHandleClosePublishesNoTypedDeltaWhileErasingLatentPayload covers
// the identity predecessor cut in CloseContributionUnder. The filtered
// PointState deliberately retains the source root while its support shrinks;
// closing it must remove the latent off-support branch, but the old and
// candidate roots are the same handle, so no Factor or Unit delta can exist
// on the replacement overlap.
func TestSameHandleClosePublishesNoTypedDeltaWhileErasingLatentPayload(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	off, ok := regions.Literal(1, false)
	if !ok {
		t.Fatal("off")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("regions")
	}
	binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(7), whole)
	writePlan := compositionPlan(t, composition)
	selection, ok := composition.SealWidening(nil)
	if !ok {
		t.Fatal("empty widening selection")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work := newWork(t, composition)

	onLegacy := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	offLegacy := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
	sourceLegacy, _, ok := work.MergeContribution(onLegacy, offLegacy)
	if !ok {
		t.Fatal("source")
	}
	source, ok := work.AsRuleContribution(sourceLegacy)
	if !ok {
		t.Fatal("source rule")
	}
	point, ok := work.PointStateFromRuleContribution(source)
	if !ok {
		t.Fatal("source point")
	}
	filtered, ok := work.TransportPointState(point, whole, identity, on)
	if !ok || !filtered.Support().Equal(on) {
		t.Fatal("filtered point")
	}
	filteredRoot, ok := filtered.HandleAt(slot)
	if !ok {
		t.Fatal("filtered root")
	}
	rhs, ok := work.PointRHSFromPointState(filtered)
	if !ok {
		t.Fatal("filtered RHS")
	}

	closed, changes, ok := work.MergeSelectedPointState(carrier.Widen, filtered, rhs, rhs, selection)
	if !ok || !closed.Support().Equal(on) {
		t.Fatal("same-handle close")
	}
	if !changes.Empty() || changes.Count() != 0 || changes.FactorCount() != 0 || !support.Empty(changes.Added()) || !support.Empty(changes.Removed()) {
		t.Fatalf("same-handle close emitted typed/structural evidence: empty=%t rows=%d factors=%d", changes.Empty(), changes.Count(), changes.FactorCount())
	}
	closedRoot, ok := closed.HandleAt(slot)
	if !ok || closedRoot == filteredRoot {
		t.Fatal("same-handle close did not replace latent root")
	}
	if got, present, valid := observedExactValue(binding, work, closedRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("closed on branch = %d/%t/%t, want 4/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, closedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("closed latent branch = %d/%t/%t, want 7/false/true", got, present, valid)
	}
}
