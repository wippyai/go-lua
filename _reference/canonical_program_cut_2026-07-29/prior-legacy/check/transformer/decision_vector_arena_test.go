package transformer

import (
	"reflect"
	"testing"
)

func decisionTestCofactorAt(kernel *decisionKernel, root decisionRef, variable uint32, high bool) decisionRef {
	node := kernel.nodes[root]
	if !node.terminal && node.variable == variable {
		if high {
			return node.high
		}
		return node.low
	}
	return root
}

func TestDecisionVectorArenaBuildHasCanonicalBalancedIdentity(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	values := []decisionRef{
		kernel.terminal(2),
		kernel.branch(4, kernel.terminal(3), kernel.terminal(4)),
		kernel.terminal(5),
		kernel.branch(2, kernel.terminal(6), kernel.terminal(7)),
		kernel.terminal(8),
	}
	arena := newDecisionVectorArena(&kernel)
	first, err := arena.Build(values)
	if err != nil {
		t.Fatal(err)
	}
	second, err := arena.Build(append([]decisionRef(nil), values...))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equal builds have refs %d and %d", first, second)
	}
	if width, err := arena.Width(first); err != nil || width != len(values) {
		t.Fatalf("Width = %d, %v", width, err)
	}
	if minimum, err := arena.MinVariable(first); err != nil || minimum != 2 {
		t.Fatalf("MinVariable = %d, %v", minimum, err)
	}
	for index, want := range values {
		if got, err := arena.At(first, index); err != nil || got != want {
			t.Fatalf("At(%d) = %d, %v; want %d", index, got, err, want)
		}
	}
	if flattened, err := arena.Flatten(first); err != nil || !reflect.DeepEqual(flattened, values) {
		t.Fatalf("Flatten = %v, %v; want %v", flattened, err, values)
	}
}

func TestDecisionVectorArenaSplitMatchesPointwiseCofactorAndSharesSegments(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	values := []decisionRef{
		kernel.terminal(2),
		kernel.branch(3, kernel.terminal(3), kernel.terminal(4)),
		kernel.branch(0, kernel.terminal(5), kernel.terminal(6)),
		kernel.branch(0,
			kernel.branch(2, kernel.terminal(7), kernel.terminal(8)),
			kernel.terminal(9),
		),
	}
	arena := newDecisionVectorArena(&kernel)
	root, err := arena.Build(values)
	if err != nil {
		t.Fatal(err)
	}
	low, high, err := arena.SplitAt(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if arena.nodes[low].left != arena.nodes[root].left || arena.nodes[high].left != arena.nodes[root].left {
		t.Fatal("SplitAt rebuilt the unaffected left segment")
	}
	lowValues, err := arena.Flatten(low)
	if err != nil {
		t.Fatal(err)
	}
	highValues, err := arena.Flatten(high)
	if err != nil {
		t.Fatal(err)
	}
	for index, value := range values {
		if want := decisionTestCofactorAt(&kernel, value, 0, false); lowValues[index] != want {
			t.Fatalf("low[%d] = %d, want %d", index, lowValues[index], want)
		}
		if want := decisionTestCofactorAt(&kernel, value, 0, true); highValues[index] != want {
			t.Fatalf("high[%d] = %d, want %d", index, highValues[index], want)
		}
	}
}

func TestDecisionVectorArenaLiftBranchMatchesPointwiseAndReconverges(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	values := []decisionRef{
		kernel.branch(0, kernel.terminal(2), kernel.terminal(3)),
		kernel.branch(0, kernel.branch(2, kernel.terminal(4), kernel.terminal(5)), kernel.terminal(6)),
		kernel.terminal(7),
	}
	arena := newDecisionVectorArena(&kernel)
	root, err := arena.Build(values)
	if err != nil {
		t.Fatal(err)
	}
	low, high, err := arena.SplitAt(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := arena.LiftBranch(0, low, high)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt != root {
		t.Fatalf("split/lift ref = %d, want original identity %d", rebuilt, root)
	}
	lowValues, _ := arena.Flatten(low)
	highValues, _ := arena.Flatten(high)
	got, _ := arena.Flatten(rebuilt)
	for index := range got {
		if want := kernel.branch(0, lowValues[index], highValues[index]); got[index] != want {
			t.Fatalf("lift[%d] = %d, want pointwise %d", index, got[index], want)
		}
	}
}

func TestDecisionVectorArenaCofactorLiftLawsAcrossWidths(t *testing.T) {
	for width := 1; width <= 33; width++ {
		kernel := newDecisionKernel()
		kernel.resetBoolean()
		values := make([]decisionRef, width)
		for index := range values {
			low := kernel.terminal(decisionLeaf(2 + index*3))
			high := kernel.terminal(decisionLeaf(3 + index*3))
			switch index % 4 {
			case 0:
				values[index] = low
			case 1:
				values[index] = kernel.branch(0, low, high)
			case 2:
				values[index] = kernel.branch(0, kernel.branch(2, low, high), high)
			default:
				values[index] = kernel.branch(1, low, high)
			}
		}
		arena := newDecisionVectorArena(&kernel)
		root, err := arena.Build(values)
		if err != nil {
			t.Fatalf("width %d Build: %v", width, err)
		}
		low, high, err := arena.SplitAt(root, 0)
		if err != nil {
			t.Fatalf("width %d SplitAt: %v", width, err)
		}
		lowValues, _ := arena.Flatten(low)
		highValues, _ := arena.Flatten(high)
		for index, value := range values {
			if got, want := lowValues[index], decisionTestCofactorAt(&kernel, value, 0, false); got != want {
				t.Fatalf("width %d low[%d] = %d, want %d", width, index, got, want)
			}
			if got, want := highValues[index], decisionTestCofactorAt(&kernel, value, 0, true); got != want {
				t.Fatalf("width %d high[%d] = %d, want %d", width, index, got, want)
			}
		}
		rebuilt, err := arena.LiftBranch(0, low, high)
		if err != nil {
			t.Fatalf("width %d LiftBranch: %v", width, err)
		}
		if rebuilt != root {
			t.Fatalf("width %d split/lift ref = %d, want %d", width, rebuilt, root)
		}
	}
}

func TestDecisionVectorArenaRejectsMalformedWidthsAndReferences(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	arena := newDecisionVectorArena(&kernel)
	empty, err := arena.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if width, err := arena.Width(empty); err != nil || width != 0 {
		t.Fatalf("empty Width = %d, %v", width, err)
	}
	if values, err := arena.Flatten(empty); err != nil || len(values) != 0 {
		t.Fatalf("empty Flatten = %v, %v", values, err)
	}
	one, err := arena.Build([]decisionRef{decisionTrue})
	if err != nil {
		t.Fatal(err)
	}
	two, err := arena.Build([]decisionRef{decisionTrue, decisionFalse})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arena.LiftBranch(0, one, two); err == nil {
		t.Fatal("LiftBranch accepted unequal widths")
	}
	if _, err := arena.At(one, 1); err == nil {
		t.Fatal("At accepted an out-of-range index")
	}
	invalid := decisionVectorRef(len(arena.nodes) + 10)
	if _, err := arena.Width(invalid); err == nil {
		t.Fatal("Width accepted an unknown ref")
	}
	if _, _, err := arena.SplitAt(invalid, 0); err == nil {
		t.Fatal("SplitAt accepted an unknown ref")
	}
}
