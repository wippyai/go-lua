package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestUntrustedContributionAdmissionStillRejectsMalformedCoverage keeps the
// deep admission boundary exercised independently of the private constructed
// cut. A caller that supplies an authored row still has to prove its target,
// region, and canonical relation; the hot helper is not that boundary.
func TestUntrustedContributionAdmissionStillRejectsMalformedCoverage(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	operation := &carryOnlyOperation{guards: manager}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()

	// The outer vector has the right length, but the row has no issued Target
	// capability. This must fail before any seal is attached.
	malformed := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{region: whole}}}})
	if _, ok := work.admitContribution(state, malformed); ok {
		t.Fatal("untrusted malformed coverage crossed deep admission")
	}
}

// TestConstructedContributionAdmissionIsAllocationFree exercises the exact
// hot operation used after carrier-owned commit/transport construction. A
// wide composition makes a deep State.Valid scan visible in review while the
// constructed cut remains an O(1), seal-reusing admission.
func TestConstructedContributionAdmissionIsAllocationFree(t *testing.T) {
	_, _, composition, _, state := contributionFixture(t, 64)
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	coverage := contributionCoverage{composition: composition}
	var admitted Contribution
	allocations := testing.AllocsPerRun(1000, func() {
		var ok bool
		admitted, ok = work.admitConstructedContribution(state, coverage)
		if !ok {
			panic("constructed contribution admission failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("constructed contribution admission allocated: %v", allocations)
	}
	if !work.admittedContribution(admitted) {
		t.Fatal("constructed contribution was not admitted")
	}
}
