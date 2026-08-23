package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestCoverageFastPathsReuseCanonicalRows exercises the two allocation cuts
// independently of typed root evaluation.  A canonical row slice is already
// owned by the caller's immutable contribution/finalization boundary, and a
// coverage union that is an exact inclusion must retain the super relation.
func TestCoverageFastPathsReuseCanonicalRows(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	narrow, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("narrow support")
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
	defer work.Close()

	rows := []TargetRegion{{target: operation.target, region: narrow}}
	canonical, ok := work.canonicalCoverage(rows, whole)
	if !ok || len(canonical.targets) != 1 || &canonical.targets[0] != &rows[0] {
		t.Fatal("canonical rows were copied despite already being ordered and clipped")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		result, valid := work.canonicalCoverage(rows, whole)
		if !valid || len(result.targets) != 1 || &result.targets[0] != &rows[0] {
			t.Fatal("canonical row reuse changed across runs")
		}
	}); allocations != 0 {
		t.Fatalf("canonical fast path allocated: %v", allocations)
	}

	left := slotCoverage{targets: []TargetRegion{{target: operation.target, region: whole}}}
	right := slotCoverage{targets: []TargetRegion{{target: operation.target, region: narrow}}}
	merged, ok := work.mergeSlotCoverage(left, right, whole)
	if !ok || len(merged.targets) != 1 || &merged.targets[0] != &left.targets[0] {
		t.Fatal("coverage inclusion did not reuse the super relation")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		result, valid := work.mergeSlotCoverage(left, right, whole)
		if !valid || len(result.targets) != 1 || &result.targets[0] != &left.targets[0] {
			t.Fatal("coverage inclusion reuse changed across runs")
		}
	}); allocations != 0 {
		t.Fatalf("coverage inclusion fast path allocated: %v", allocations)
	}
}

func TestPointFoldCoverageUnionReusesDominantHeaderAndKeepsPresentDefault(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("support work")
	}
	narrow, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("narrow support")
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
	defer work.Close()

	baseRows := []TargetRegion{{target: operation.target, region: whole}}
	narrowRows := []TargetRegion{{target: operation.target, region: narrow}}
	base := newContributionCoverage(composition, []slotCoverage{{targets: baseRows}})
	term := PointState{coverage: newContributionCoverage(composition, []slotCoverage{{targets: narrowRows}})}
	result, ok := work.foldCoverageUnion(&pointFoldTransaction{}, base, []pointFoldOperand{{coverage: term.coverage}})
	if !ok {
		t.Fatal("coverage union")
	}
	if len(result.slots) != 1 || &result.slots[0] != &base.slots[0] {
		t.Fatal("dominant operand coverage header was not reused")
	}
	rows := result.slot(shape.Slot(0)).targets
	if len(rows) != 1 || &rows[0] != &baseRows[0] || !rows[0].target.Same(operation.target) || !rows[0].region.SameHandle(whole) {
		t.Fatal("present Default authored row was changed or lost")
	}
}

func TestPointFoldCoverageUnionFindsLaterDominatingOperand(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("support work")
	}
	first, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("first support")
	}
	second, ok := regions.Literal(2, true)
	if !ok || !regions.Seal() {
		t.Fatal("second support")
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
	defer work.Close()
	rows := func(region support.Mask) contributionCoverage {
		return newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: region}}}})
	}
	base := rows(first)
	middle := PointState{coverage: rows(second)}
	dominant := PointState{coverage: rows(whole)}
	result, ok := work.foldCoverageUnion(&pointFoldTransaction{}, base, []pointFoldOperand{{coverage: middle.coverage}, {coverage: dominant.coverage}})
	if !ok {
		t.Fatal("coverage union")
	}
	if len(result.slots) != 1 || &result.slots[0] != &dominant.coverage.slots[0] {
		t.Fatal("later dominating operand was not reused")
	}
	resultRows := result.slot(shape.Slot(0)).targets
	if len(resultRows) != 1 || !resultRows[0].region.SameHandle(whole) || !resultRows[0].target.Same(operation.target) {
		t.Fatal("dominant Present(Default) row changed")
	}
}
