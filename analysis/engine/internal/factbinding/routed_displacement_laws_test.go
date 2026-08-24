package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// routedRule stages one write at the fixture's strong Target and closes it as
// an authored rule result. routed selects the routed entry, whose row states
// the complete value of the coordinate its own selected read observed.
func routedRule(t testing.TB, fixture lineageLawFixture, region support.Mask, value uint64, routed bool) carrier.RuleContribution {
	t.Helper()
	base, ok := fixture.work.BeginContribution(fixture.plan, fixture.composition.Scope(), nil, fixture.whole)
	if !ok {
		t.Fatal("begin routed contribution")
	}
	patch := fixture.binding.Begin(fixture.work, base.State())
	if patch == nil {
		t.Fatal("begin routed patch")
	}
	staged := patch.Write(fixture.target, region, value)
	if routed {
		staged = patch.WriteRouted(fixture.target, region, value)
	}
	if !staged {
		t.Fatal("stage routed write")
	}
	accepted, ok := patch.Accept(fixture.work)
	if !ok {
		t.Fatal("accept routed write")
	}
	contribution, ok := fixture.work.FinishContribution(base, []carrier.Patch{accepted})
	if !ok {
		t.Fatal("finish routed contribution")
	}
	rule, ok := fixture.work.AsRuleContribution(contribution)
	if !ok {
		t.Fatal("routed rule")
	}
	return rule
}

// TestPointFoldRoutedWriteSupersedesTransportedPredecessor proves the
// flow-sensitive law a routed output declares. Its own selected read observed
// the predecessor at this coordinate and its reducer answered it, so the
// published cell after the operation is that answer alone. The predecessor
// reaches the fold as a baseline of a different published generation, which
// is exactly the identity lineage cannot name across a transport, and the
// displacement supersedes it inside the region it read anyway.
func TestPointFoldRoutedWriteSupersedesTransportedPredecessor(t *testing.T) {
	fixture := newLineageLawFixture(t)
	folded := fixture.foldRules(t, routedRule(t, fixture, fixture.on, lineageProven, true))
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageProven)
	assertLineageValue(t, fixture, root, func(guard.Atom) bool { return false }, lineageRefuted)
}

// TestPointFoldOrdinaryStrongWriteJoinsTransportedPredecessor holds the other
// half of the same boundary. An exact strong write declares no read of its
// own destination, so it remains one more operand reaching the cell and the
// transported predecessor keeps its region.
func TestPointFoldOrdinaryStrongWriteJoinsTransportedPredecessor(t *testing.T) {
	fixture := newLineageLawFixture(t)
	folded := fixture.foldRules(t, routedRule(t, fixture, fixture.on, lineageProven, false))
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageUnknown)
}

// TestPointFoldRoutedWriteDisplacesOnlyTheRegionItRead proves the bound. The
// displacement is the region the routed write's own read observed, so a
// predecessor outside it is untouched.
func TestPointFoldRoutedWriteDisplacesOnlyTheRegionItRead(t *testing.T) {
	fixture := newLineageLawFixture(t)
	folded := fixture.foldRules(t, routedRule(t, fixture, fixture.off, lineageProven, true))
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageRefuted)
	assertLineageValue(t, fixture, root, func(guard.Atom) bool { return false }, lineageProven)
}

// TestPointFoldMarksDisplacedRHSInadmissibleAsAccumulator proves the
// precondition an incremental Region head fold depends on. Reuse is sound
// because folding the moved operands onto a retained row equals folding every
// operand onto Init, and that equality belongs to the join. A row a routed
// write displaced inside is therefore marked, and only that row.
func TestPointFoldMarksDisplacedRHSInadmissibleAsAccumulator(t *testing.T) {
	fixture := newLineageLawFixture(t)
	if joined := fixture.foldRules(t, routedRule(t, fixture, fixture.on, lineageProven, false)); joined.Displaced() {
		t.Fatal("a pure join was marked as displaced")
	}
	displaced := fixture.foldRules(t, routedRule(t, fixture, fixture.on, lineageProven, true))
	if !displaced.Displaced() {
		t.Fatal("a routed displacement was admitted as a join accumulator")
	}
}

// TestPublishedPointCarriesNoDisplacement proves the mark never outlives the
// fold that consumed it. Publication mints one generation and restamps every
// row as a baseline of it, so a later sibling fold sees a predecessor surface
// and not somebody else's authored displacement.
func TestPublishedPointCarriesNoDisplacement(t *testing.T) {
	fixture := newLineageLawFixture(t)
	rule := routedRule(t, fixture, fixture.on, lineageProven, true)
	point, ok := fixture.work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("publish routed contribution")
	}
	rhs, ok := fixture.work.PointRHSFromPointState(fixture.source)
	if !ok || !fixture.work.BeginPointRHSFold(fixture.source, rhs) || !fixture.work.AddPointFoldEnvironment(point) {
		t.Fatal("published-point fold")
	}
	folded, _, ok := fixture.work.FinishPointRHSFold()
	if !ok {
		t.Fatal("finish published-point fold")
	}
	if folded.Displaced() {
		t.Fatal("a published predecessor still carried an authored displacement")
	}
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageUnknown)
}

// TestRoutedWriteRefusesAWeakTarget keeps the routed entry at the authority
// its declaration proves. A routed output is a strong singleton coordinate;
// a may-alias surface has no predecessor of its own to supersede.
func TestRoutedWriteRefusesAWeakTarget(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
	whole := transportWhole(t, manager)
	binding, _, _, composition, fixture := bindingState(t, manager, lineageLawConfig(), whole)
	plan := compositionPlan(t, composition)
	work := newWork(t, composition)
	t.Cleanup(func() { work.Close() })
	base, ok := work.BeginContribution(plan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin weak contribution")
	}
	patch := binding.Begin(work, base.State())
	if patch == nil {
		t.Fatal("begin weak patch")
	}
	if patch.WriteRouted(fixture.target(t, 0, carrier.WeakTarget), whole, lineageProven) {
		t.Fatal("a weak target accepted a routed displacement")
	}
	if !patch.Discard() {
		t.Fatal("discard weak patch")
	}
}
