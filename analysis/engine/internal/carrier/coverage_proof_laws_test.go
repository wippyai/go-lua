package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// coverageProofFixture publishes one point whose surface carries an authored
// row, which is the surface every later admission is offered.
func coverageProofFixture(t *testing.T) (*Work, PointState) {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	operation := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	t.Cleanup(func() { work.Close() })
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	coverage := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}})
	seed, ok := work.admitContribution(initial, coverage)
	if !ok {
		t.Fatal("seed contribution")
	}
	rule, ok := work.AsRuleContribution(seed)
	if !ok {
		t.Fatal("seed rule")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("seed point")
	}
	return work, point
}

// TestAdmittingAProvenPointWalksNoContributionRows is the cost law behind the
// admitted fast path. A published surface is immutable and its rows were
// walked when it was constructed, so every later admission - and a point is
// admitted again on every refresh that transports, folds, or compares it -
// must answer from the construction proof and walk nothing.
func TestAdmittingAProvenPointWalksNoContributionRows(t *testing.T) {
	work, point := coverageProofFixture(t)
	if !work.admittedPointState(point) {
		t.Fatal("published point was refused")
	}
	DbgCoverageProofRowsReset()
	for admission := 0; admission < 64; admission++ {
		if !work.admittedPointState(point) {
			t.Fatalf("admission %d refused a published point", admission)
		}
	}
	if walked := DbgCoverageProofRows(); walked != 0 {
		t.Fatalf("re-admitting a published point walked %d contribution rows, want 0", walked)
	}
}

// TestAnUnprovenSurfaceStillPaysItsWalk keeps the fast path honest: the
// admission token is the record of a walk that happened, never a way to skip
// one. A surface offered without it is proved in full.
func TestAnUnprovenSurfaceStillPaysItsWalk(t *testing.T) {
	work, point := coverageProofFixture(t)
	unproven := point
	unproven.coverage.proof = nil
	DbgCoverageProofRowsReset()
	if !work.admittedPointState(unproven) {
		t.Fatal("an unproven published surface was refused")
	}
	if walked := DbgCoverageProofRows(); walked == 0 {
		t.Fatal("an unproven surface was admitted without walking its rows")
	}
}

// TestAForeignWorkProofIsNotAProof holds the token to the evaluator that
// issued it. A surface built under another Work carries a token this one did
// not mint, so it is proved again rather than trusted.
func TestAForeignWorkProofIsNotAProof(t *testing.T) {
	work, point := coverageProofFixture(t)
	foreign := point
	foreign.coverage.proof = &contributionSeal{work: nil, composition: work.composition}
	DbgCoverageProofRowsReset()
	if !work.admittedPointState(foreign) {
		t.Fatal("a foreign-token surface was refused instead of proved")
	}
	if walked := DbgCoverageProofRows(); walked == 0 {
		t.Fatal("a foreign admission token was consumed as a proof")
	}
}
