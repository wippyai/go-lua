package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// carryExclusionOperation issues two targets in one Factor slot. It gives the
// carrier law a partial candidate surface without importing a typed domain
// Binding: one direct target is excluded while the sibling remains authored.
type carryExclusionOperation struct {
	*carryOnlyOperation
	first  Target
	second Target
}

func (operation *carryExclusionOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.carryOnlyOperation == nil || operation.prepared {
		return nil, false
	}
	issuer, ok := NewIssuer()
	if !ok {
		return nil, false
	}
	first, firstOK := issuer.IssueTarget(1, StrongTarget)
	second, secondOK := issuer.IssueTarget(2, StrongTarget)
	if !firstOK || !secondOK {
		return nil, false
	}
	operation.issuer = issuer
	operation.first = first
	operation.second = second
	operation.prepared = true
	return operation, true
}

func (operation *carryExclusionOperation) DeclaredTarget(target Target) bool {
	return operation != nil && operation.prepared && (operation.first.Same(target) || operation.second.Same(target))
}

func (operation *carryExclusionOperation) ValidTarget(target Target) bool {
	return operation != nil && operation.issuer.Live() && operation.DeclaredTarget(target)
}

func (operation *carryExclusionOperation) ValidRoot(root RootHandle) bool {
	if operation == nil || !operation.issuer.Live() {
		return false
	}
	id, ok := operation.issuer.ResolveRoot(root)
	return ok && (id == 1 || id == 2 || id == 3)
}

func TestSealCarryExclusionsMasksOnlyTheMemberStrongTarget(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("support regions")
	}
	candidate, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("candidate support")
	}
	operation := &carryExclusionOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	plan, ok := composition.SealContribution(1, []shape.Slot{0}, []ContributionSource{{Slot: 0, Input: 0}})
	if !ok {
		t.Fatal("carry plan")
	}
	source := ContributionSource{Slot: 0, Input: 0}
	originalAlias := plan
	sealedPlan, ok := plan.SealCarryExclusions(map[ContributionSource][]Target{source: {operation.first}})
	if !ok {
		t.Fatal("sealed exclusion")
	}
	if sealedPlan.value != originalAlias.value || len(originalAlias.value.carryExclusions) != 1 || !originalAlias.value.carryExclusions[0].target.Same(operation.first) {
		t.Fatal("strong target was not sealed")
	}
	if _, ok := originalAlias.SealCarryExclusions(nil); ok {
		t.Fatal("retained plan alias bypassed the exclusion seal")
	}
	plan = sealedPlan

	rows := []TargetRegion{
		// The excluded target exists only on the candidate region. There is no
		// baseline row for it elsewhere to reset or compensate.
		{target: operation.first, region: candidate},
		{target: operation.second, region: whole},
	}
	filtered := plan.value.carryRows(source, rows)
	if len(filtered) != 1 || !filtered[0].target.Same(operation.second) {
		t.Fatal("carry exclusion did not preserve the partial candidate surface")
	}
	if got := plan.value.carryRows(source, []TargetRegion{{target: operation.second, region: whole}}); len(got) != 1 {
		t.Fatal("unrelated target was filtered")
	}

	// Run the same relation through Finish. The source carries both target
	// rows, but the sealed exclusion leaves the result's authored surface
	// sparse rather than manufacturing a Default patch for the excluded one.
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	before, ok := initial.HandleAt(0)
	if !ok {
		t.Fatal("predecessor root")
	}
	after, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("source root")
	}
	change, ok := operation.issuer.IssueChange(before, after, nil, support.Mask{}, nil, nil, nil)
	if !ok {
		t.Fatal("source change")
	}
	patch, ok := work.Accept(initial, change)
	if !ok {
		t.Fatal("source patch")
	}
	sourceState, _, ok := work.Commit(initial, []Patch{patch})
	if !ok {
		t.Fatal("source commit")
	}
	sourceCoverage := newContributionCoverage(composition, []slotCoverage{{targets: rows}})
	sourceContribution, ok := work.admitContribution(sourceState, sourceCoverage)
	if !ok {
		t.Fatal("source contribution")
	}
	sourceRule, ok := work.AsRuleContribution(sourceContribution)
	if !ok {
		t.Fatal("source rule")
	}
	sourcePoint, ok := work.PointStateFromRuleContribution(sourceRule)
	if !ok {
		t.Fatal("source point")
	}
	// The same member contribution also authors a concrete patch for the
	// excluded target. Finish must retain that effect row while physically
	// masking only the stale carried row; the sibling remains carried.
	patchedBase, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{sourcePoint}, whole)
	if !ok {
		t.Fatal("patched carry base")
	}
	patchedBefore, ok := patchedBase.State().HandleAt(0)
	if !ok {
		t.Fatal("patched predecessor root")
	}
	patchedAfter, ok := operation.issuer.IssueRoot(3)
	if !ok {
		t.Fatal("patched root")
	}
	patchChange, ok := operation.issuer.IssueChange(patchedBefore, patchedAfter, nil, support.Mask{}, nil, nil, nil)
	if !ok {
		t.Fatal("patched change")
	}
	patched, ok := work.AcceptAuthoredRows(patchedBase.State(), patchChange, []TargetRegion{{target: operation.first, region: candidate}})
	if !ok {
		t.Fatal("patched authored row")
	}
	patchedResult, ok := work.FinishRuleContribution(patchedBase, []Patch{patched})
	if !ok || !patchedResult.Valid() {
		t.Fatal("patched finish")
	}
	patchedRows := patchedResult.value.coverage.slot(0).targets
	if len(patchedRows) != 2 || !patchedRows[0].target.Same(operation.first) || !patchedRows[0].region.Equal(candidate) || !patchedRows[1].target.Same(operation.second) || !patchedRows[1].region.Equal(whole) {
		t.Fatal("Finish did not publish the patch and sibling carry")
	}

	base, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{sourcePoint}, whole)
	if !ok {
		t.Fatal("carry base")
	}
	result, ok := work.FinishRuleContribution(base, nil)
	if !ok || !result.Valid() {
		t.Fatal("finish")
	}
	finished := result.value.coverage.slot(0).targets
	if len(finished) != 1 || !finished[0].target.Same(operation.second) || !finished[0].region.Equal(whole) {
		t.Fatal("Finish did not retain exactly the sibling target surface")
	}
}

func TestSealCarryExclusionsRejectsForeignOrNonStrongTargets(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	operation := &carryExclusionOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	plan, ok := composition.SealContribution(1, nil, []ContributionSource{{Slot: 0, Input: 0}})
	if !ok {
		t.Fatal("carry plan")
	}
	source := ContributionSource{Slot: 0, Input: 0}
	if _, ok := plan.SealCarryExclusions(map[ContributionSource][]Target{{Slot: 1, Input: 0}: {operation.first}}); ok {
		t.Fatal("undeclared carry source accepted")
	}
	weak, ok := operation.issuer.IssueTarget(3, WeakTarget)
	if !ok {
		t.Fatal("weak target")
	}
	if _, ok := plan.SealCarryExclusions(map[ContributionSource][]Target{source: {weak}}); ok {
		t.Fatal("weak target accepted as strong exclusion")
	}
	foreignOperation := &carryExclusionOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	foreign, ok := attachTestComposition(t, []FactorOperation{foreignOperation})
	if !ok {
		t.Fatal("foreign composition")
	}
	foreignPlan, ok := foreign.SealContribution(1, nil, []ContributionSource{{Slot: 0, Input: 0}})
	if !ok {
		t.Fatal("foreign plan")
	}
	if _, ok := plan.SealCarryExclusions(map[ContributionSource][]Target{source: {foreignOperation.first}}); ok {
		t.Fatal("foreign target accepted")
	}
	sealedForeign, ok := foreignPlan.SealCarryExclusions(nil)
	if !ok {
		t.Fatal("empty exclusion seal")
	}
	if _, ok := sealedForeign.SealCarryExclusions(map[ContributionSource][]Target{{Slot: 0, Input: 0}: {foreignOperation.first}}); ok {
		t.Fatal("sealed exclusion relation was overwritten")
	}
	sealedPlan, ok := plan.SealCarryExclusions(nil)
	if !ok {
		t.Fatal("empty exclusion seal")
	}
	if _, ok := sealedPlan.SealCarryExclusions(map[ContributionSource][]Target{source: {operation.first}}); ok {
		t.Fatal("empty exclusion relation was overwritten")
	}
}
