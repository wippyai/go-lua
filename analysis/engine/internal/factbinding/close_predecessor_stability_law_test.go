package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestErasingCloseAgainstEqualPredecessorKeepsRootIdentity states the
// predecessor half of the close reuse law.  A recurrence head re-closes the
// same latent exact RHS against the root it published last pass.  Whenever the
// close erases latent payload, the closed plane differs from the candidate, so
// candidate reuse cannot apply; the closed plane is nevertheless equal to the
// published predecessor, and the publication must therefore retain the
// predecessor root.  Minting a fresh root at an unchanged lattice value makes
// every root-identity publication test report a structural change, which turns
// a converged fixpoint into a period-1 limit cycle.
func TestErasingCloseAgainstEqualPredecessorKeepsRootIdentity(t *testing.T) {
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
	selection, ok := composition.SealNarrowing(nil)
	if !ok {
		t.Fatal("empty narrowing selection")
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
	// The exact RHS keeps its latent off-support branch for every pass, so
	// each close physically erases payload and can never reuse the candidate.
	latent, ok := work.TransportPointState(point, whole, identity, on)
	if !ok || !latent.Support().Equal(on) {
		t.Fatal("latent point")
	}
	latentRoot, ok := latent.HandleAt(slot)
	if !ok {
		t.Fatal("latent root")
	}
	rhs, ok := work.PointRHSFromPointState(latent)
	if !ok {
		t.Fatal("latent RHS")
	}

	current := latent
	var published carrier.RootHandle
	for pass := 0; pass < 6; pass++ {
		next, changes, ok := work.MergeSelectedPointState(carrier.Narrow, current, rhs, rhs, selection)
		if !ok {
			t.Fatalf("pass %d close rejected", pass)
		}
		if !next.Support().Equal(on) {
			t.Fatalf("pass %d support", pass)
		}
		root, ok := next.HandleAt(slot)
		if !ok {
			t.Fatalf("pass %d root", pass)
		}
		if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
			t.Fatalf("pass %d on branch = %d/%t/%t, want 4/true/true", pass, got, present, valid)
		}
		if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
			t.Fatalf("pass %d latent branch = %d/%t/%t, want 7/false/true", pass, got, present, valid)
		}
		if pass == 0 {
			if root == latentRoot {
				t.Fatal("first close did not erase the latent root")
			}
			published = root
		} else if root != published {
			t.Fatalf("pass %d minted a new root at an unchanged closed plane", pass)
		}
		if !changes.Empty() || changes.Count() != 0 || changes.FactorCount() != 0 || !support.Empty(changes.Added()) || !support.Empty(changes.Removed()) {
			t.Fatalf("pass %d published evidence at an unchanged plane: empty=%t rows=%d factors=%d", pass, changes.Empty(), changes.Count(), changes.FactorCount())
		}
		current = next
	}
}
