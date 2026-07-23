package transformer

import (
	"context"
	"reflect"
	"testing"
)

func TestDecisionLeafTuplePartitionPreservesCorrelation(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	leftLow, leftHigh := kernel.terminal(2), kernel.terminal(3)
	rightLow, rightHigh := kernel.terminal(4), kernel.terminal(5)
	left := kernel.branch(0, leftLow, leftHigh)
	right := kernel.branch(0, rightLow, rightHigh)

	regions, err := kernel.partitionLeafTuplesUnderCare(context.Background(), decisionTrue, []decisionRef{left, right})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("regions = %d, want two correlated pairs", len(regions))
	}
	type pair struct{ left, right decisionLeaf }
	cares := make(map[pair]decisionRef, len(regions))
	for _, region := range regions {
		if len(region.leaves) != 2 {
			t.Fatalf("tuple width = %d", len(region.leaves))
		}
		cares[pair{region.leaves[0], region.leaves[1]}] = region.care
	}
	if _, impossible := cares[pair{left: 2, right: 5}]; impossible {
		t.Fatal("partition manufactured impossible low/high pair")
	}
	if _, impossible := cares[pair{left: 3, right: 4}]; impossible {
		t.Fatal("partition manufactured impossible high/low pair")
	}
	if got := cares[pair{left: 2, right: 4}]; got != kernel.branch(0, decisionTrue, decisionFalse) {
		t.Fatalf("low-pair care = %d", got)
	}
	if got := cares[pair{left: 3, right: 5}]; got != kernel.branch(0, decisionFalse, decisionTrue) {
		t.Fatalf("high-pair care = %d", got)
	}
}

func TestDecisionLeafTuplePartitionMergesSameTupleRegion(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	left, right := kernel.terminal(2), kernel.terminal(3)
	regions, err := kernel.partitionLeafTuplesUnderCare(context.Background(), decisionTrue, []decisionRef{
		kernel.branch(0, left, left), kernel.branch(1, right, right),
	})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if len(regions) != 1 || len(regions[0].leaves) != 2 || regions[0].leaves[0] != 2 || regions[0].leaves[1] != 3 || regions[0].care != decisionTrue {
		t.Fatalf("regions = %#v, want one unconditional pair", regions)
	}
}

func TestDecisionLeafTuplePartitionAggregatesReconvergentProductPaths(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	low, high := kernel.terminal(2), kernel.terminal(3)
	left := kernel.branch(0,
		kernel.branch(1, low, high),
		kernel.branch(1, high, low),
	)
	right := kernel.terminal(4)

	regions, err := kernel.partitionLeafTuplesUnderCare(context.Background(), decisionTrue, []decisionRef{left, right})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("regions = %d, want two terminal pairs", len(regions))
	}
	want := map[decisionLeaf]decisionRef{
		2: kernel.branch(0,
			kernel.branch(1, decisionTrue, decisionFalse),
			kernel.branch(1, decisionFalse, decisionTrue),
		),
		3: kernel.branch(0,
			kernel.branch(1, decisionFalse, decisionTrue),
			kernel.branch(1, decisionTrue, decisionFalse),
		),
	}
	for _, region := range regions {
		if len(region.leaves) != 2 || region.leaves[1] != 4 || region.care != want[region.leaves[0]] {
			t.Fatalf("region = %#v, want canonical reconvergent care", region)
		}
	}
}

func TestDecisionLeafTuplePartitionCofactorsEveryDemandByCare(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	care := kernel.branch(0, decisionTrue, decisionFalse)
	first := kernel.branch(0, kernel.terminal(2), kernel.terminal(3))
	// The high value 5 is outside care and must not survive as a demanded
	// semantic combination. The second variable remains live inside care.
	second := kernel.branch(0,
		kernel.branch(1, kernel.terminal(4), kernel.terminal(6)),
		kernel.terminal(5),
	)
	regions, err := kernel.partitionLeafTuplesUnderCare(context.Background(), care, []decisionRef{first, second})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("regions = %d, want two live tuples", len(regions))
	}
	wantCare := map[decisionLeaf]decisionRef{
		4: kernel.branch(0, kernel.branch(1, decisionTrue, decisionFalse), decisionFalse),
		6: kernel.branch(0, kernel.branch(1, decisionFalse, decisionTrue), decisionFalse),
	}
	for _, region := range regions {
		if len(region.leaves) != 2 || region.leaves[0] != 2 || region.leaves[1] == 5 {
			t.Fatalf("region = %#v, contains value outside care", region)
		}
		if got := region.care; got != wantCare[region.leaves[1]] {
			t.Fatalf("leaf %d care = %d, want %d", region.leaves[1], got, wantCare[region.leaves[1]])
		}
	}
}

func TestDecisionLeafTupleJointPartitionMatchesRestrictedIntersection(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	care := kernel.branch(0,
		kernel.branch(1, decisionTrue, decisionFalse),
		kernel.branch(2, decisionFalse, decisionTrue),
	)
	root := kernel.branch(0,
		kernel.branch(1, kernel.terminal(2), kernel.terminal(3)),
		kernel.branch(2, kernel.terminal(4), kernel.terminal(5)),
	)

	want := map[decisionLeaf]decisionRef{
		2: kernel.branch(0, kernel.branch(1, decisionTrue, decisionFalse), decisionFalse),
		5: kernel.branch(0, decisionFalse, kernel.branch(2, decisionFalse, decisionTrue)),
	}

	regions, err := kernel.partitionLeafTuplesUnderCare(context.Background(), care, []decisionRef{root})
	if err != nil {
		t.Fatalf("joint partition: %v", err)
	}
	if len(regions) != len(want) {
		t.Fatalf("joint regions = %d, want %d", len(regions), len(want))
	}
	for _, region := range regions {
		if len(region.leaves) != 1 || region.care != want[region.leaves[0]] {
			t.Fatalf("joint region = %#v, want care by leaf %#v", region, want)
		}
	}
}

func TestDecisionLeafTuplePartitionEmitsDeterministicProductNodeOrder(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()

	// Give the low branch lexicographically larger leaves than the high branch.
	// Stable low-then-high product-node emission must therefore remain visibly
	// different from the removed terminal-row lexicographic sort.
	first := kernel.branch(0, kernel.terminal(9), kernel.terminal(2))
	second := kernel.branch(0, kernel.terminal(8), kernel.terminal(3))
	roots := []decisionRef{first, second}

	want, err := kernel.partitionLeafTuplesUnderCare(context.Background(), decisionTrue, roots)
	if err != nil {
		t.Fatalf("first partition: %v", err)
	}
	if len(want) != 2 || !reflect.DeepEqual(want[0].leaves, []decisionLeaf{9, 8}) || !reflect.DeepEqual(want[1].leaves, []decisionLeaf{2, 3}) {
		t.Fatalf("product-node emission = %#v, want low [9 8] then high [2 3]", want)
	}
	for repeat := 0; repeat < 64; repeat++ {
		got, partitionErr := kernel.partitionLeafTuplesUnderCare(context.Background(), decisionTrue, roots)
		if partitionErr != nil {
			t.Fatalf("repeat %d: %v", repeat, partitionErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("repeat %d drifted: got %#v, want %#v", repeat, got, want)
		}
	}
}
