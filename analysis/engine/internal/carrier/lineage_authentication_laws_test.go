package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestLineageCoverageRejectsForeignPointToken proves that a token copied from
// another live Work cannot cross AcceptAuthoredRows, even when both Works
// share the same Composition and Target layout.
func TestLineageCoverageRejectsForeignPointToken(t *testing.T) {
	manager, err := guard.New(nil)
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
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	first, ok := composition.NewWork()
	if !ok {
		t.Fatal("first work")
	}
	defer first.Close()
	second, ok := composition.NewWork()
	if !ok {
		t.Fatal("second work")
	}
	defer second.Close()
	seedCoverage := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}})
	seed, ok := first.admitContribution(initial, seedCoverage)
	if !ok {
		t.Fatal("seed contribution")
	}
	rule, ok := first.AsRuleContribution(seed)
	if !ok {
		t.Fatal("seed rule")
	}
	point, ok := first.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("seed point")
	}
	before, ok := initial.HandleAt(0)
	if !ok {
		t.Fatal("before root")
	}
	after, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("after root")
	}
	change, ok := operation.issuer.IssueChange(before, after, nil, support.Mask{}, nil, nil, nil)
	if !ok {
		t.Fatal("change")
	}
	foreign := TargetRegion{target: operation.target, region: whole, lineage: point.lineage, role: CoverageEffect}
	if _, accepted := second.AcceptAuthoredRows(initial, change, []TargetRegion{foreign}); accepted {
		t.Fatal("foreign point lineage accepted")
	}
}

// TestLineageCoverageRejectsNilBaselineToken proves that a baseline row must
// carry authenticated provenance; nil remains valid only for ordinary effect
// rows authored without a source point.
func TestLineageCoverageRejectsNilBaselineToken(t *testing.T) {
	manager, err := guard.New(nil)
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
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	coverage := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole, role: CoverageBaseline}}}})
	if _, accepted := work.admitContribution(initial, coverage); accepted {
		t.Fatal("nil baseline lineage accepted")
	}
}
