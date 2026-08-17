package sourcecontrol

import (
	"testing"
)

func dominanceAdjacency(nodeCount uint32, offsets, targets []uint32) adjacencyProof {
	adjacency := adjacencyProof{forwardOffsets: offsets, forwardTargets: targets}
	if nodeCount == 0 || uint64(len(offsets)) != uint64(nodeCount)+1 || offsets[0] != 0 ||
		uint64(offsets[len(offsets)-1]) != uint64(len(targets)) {
		return adjacency
	}
	reverseOffsets := make([]uint32, nodeCount+1)
	for from := uint32(0); from < nodeCount; from++ {
		start, end := offsets[from], offsets[from+1]
		if start > end || uint64(end) > uint64(len(targets)) {
			return adjacency
		}
		for edge := start; edge < end; edge++ {
			to := targets[edge]
			if to >= nodeCount {
				return adjacency
			}
			reverseOffsets[to+1]++
		}
	}
	for index := 1; index < len(reverseOffsets); index++ {
		reverseOffsets[index] += reverseOffsets[index-1]
	}
	reverseTargets := make([]uint32, len(targets))
	reverseNext := append([]uint32(nil), reverseOffsets[:len(reverseOffsets)-1]...)
	for from := uint32(0); from < nodeCount; from++ {
		for edge := offsets[from]; edge < offsets[from+1]; edge++ {
			to := targets[edge]
			reverseTargets[reverseNext[to]] = from
			reverseNext[to]++
		}
	}
	adjacency.reverseOffsets = reverseOffsets
	adjacency.reverseTargets = reverseTargets
	return adjacency
}

// referenceReachable is deliberately a slow edge-walking oracle. It does not
// share DFS numbering, reverse rows, or any dominator state with sealDominance.
func referenceReachable(nodeCount uint32, offsets, targets, roots []uint32) []bool {
	reachable := make([]bool, int(nodeCount))
	work := make([]uint32, 0, len(roots))
	for _, root := range roots {
		if !reachable[root] {
			reachable[root] = true
			work = append(work, root)
		}
	}
	for len(work) != 0 {
		node := work[len(work)-1]
		work = work[:len(work)-1]
		for edge := offsets[node]; edge < offsets[node+1]; edge++ {
			target := targets[edge]
			if !reachable[target] {
				reachable[target] = true
				work = append(work, target)
			}
		}
	}
	return reachable
}

// referenceDominates answers dominance by deleting the candidate ancestor and
// asking whether the candidate descendant remains reachable from any virtual
// root child. This is intentionally independent of Lengauer-Tarjan.
func referenceDominates(
	nodeCount uint32,
	offsets, targets, roots []uint32,
	reachable []bool,
	ancestor, descendant uint32,
) bool {
	if ancestor >= nodeCount || descendant >= nodeCount ||
		!reachable[ancestor] || !reachable[descendant] {
		return false
	}
	if ancestor == descendant {
		return true
	}
	seen := make([]bool, int(nodeCount))
	work := make([]uint32, 0, len(roots))
	for _, root := range roots {
		if root != ancestor && !seen[root] {
			seen[root] = true
			work = append(work, root)
		}
	}
	for len(work) != 0 {
		node := work[len(work)-1]
		work = work[:len(work)-1]
		for edge := offsets[node]; edge < offsets[node+1]; edge++ {
			target := targets[edge]
			if target == ancestor || seen[target] {
				continue
			}
			seen[target] = true
			work = append(work, target)
		}
	}
	return !seen[descendant]
}

func assertDominanceMatchesReference(
	t *testing.T,
	nodeCount uint32,
	offsets, targets, roots []uint32,
) {
	t.Helper()
	proof, err := sealDominance(nodeCount, dominanceAdjacency(nodeCount, offsets, targets), roots)
	if err != nil {
		t.Fatalf("sealDominance: %v", err)
	}
	reachable := referenceReachable(nodeCount, offsets, targets, roots)
	for ancestor := uint32(0); ancestor < nodeCount; ancestor++ {
		for descendant := uint32(0); descendant < nodeCount; descendant++ {
			want := referenceDominates(nodeCount, offsets, targets, roots, reachable, ancestor, descendant)
			if got := proof.dominates(ancestor, descendant); got != want {
				t.Fatalf("dominates(%d,%d)=%v, want %v; offsets=%v targets=%v roots=%v", ancestor, descendant, got, want, offsets, targets, roots)
			}
		}
	}
	for node, live := range reachable {
		if !live && (proof.pre[node] != 0 || proof.post[node] != 0) {
			t.Fatalf("unreachable node %d retained interval %d/%d", node, proof.pre[node], proof.post[node])
		}
	}
	if proof.dominates(nodeCount, 0) || proof.dominates(0, nodeCount) {
		t.Fatalf("out-of-range dominance query did not fail closed")
	}
}

func TestDominanceMatchesIndependentOracleOnBoundedGraphs(t *testing.T) {
	const nodeCount = uint32(3)
	// Exhaust every directed graph on three nodes, including self edges, under
	// every nonempty canonical virtual-root set.
	for mask := uint32(0); mask < 1<<(nodeCount*nodeCount); mask++ {
		offsets := make([]uint32, nodeCount+1)
		targets := make([]uint32, 0, nodeCount*nodeCount)
		for from := uint32(0); from < nodeCount; from++ {
			for to := uint32(0); to < nodeCount; to++ {
				if mask&(1<<(from*nodeCount+to)) != 0 {
					targets = append(targets, to)
				}
			}
			offsets[from+1] = uint32(len(targets))
		}
		for rootMask := uint32(1); rootMask < 1<<nodeCount; rootMask++ {
			roots := make([]uint32, 0, nodeCount)
			for node := uint32(0); node < nodeCount; node++ {
				if rootMask&(1<<node) != 0 {
					roots = append(roots, node)
				}
			}
			assertDominanceMatchesReference(t, nodeCount, offsets, targets, roots)
		}
	}
}

func TestDominanceDiamondLoopAndDisconnectedActivations(t *testing.T) {
	t.Run("diamond", func(t *testing.T) {
		// 0 splits to 1/2 and both join at 3; neither branch dominates the join.
		assertDominanceMatchesReference(
			t,
			4,
			[]uint32{0, 2, 3, 4, 4},
			[]uint32{1, 2, 3, 3},
			[]uint32{0},
		)
	})
	t.Run("loop", func(t *testing.T) {
		// 0 -> 1 -> 2 -> 1, with an exit from the loop header to 3.
		assertDominanceMatchesReference(
			t,
			4,
			[]uint32{0, 1, 2, 4, 4},
			[]uint32{1, 2, 1, 3},
			[]uint32{0},
		)
	})
	t.Run("disconnected activation roots", func(t *testing.T) {
		// 0 and 4 are independent activation roots. 6/7 is an unreachable
		// island and must not acquire a proof interval.
		assertDominanceMatchesReference(
			t,
			8,
			[]uint32{0, 1, 2, 3, 3, 4, 5, 6, 6},
			[]uint32{1, 2, 3, 5, 4, 7},
			[]uint32{0, 4},
		)
	})
}

func TestDominanceRejectsMalformedCSRAndRoots(t *testing.T) {
	tests := []struct {
		name      string
		nodeCount uint32
		offsets   []uint32
		targets   []uint32
		roots     []uint32
	}{
		{name: "zero node count", nodeCount: 0, offsets: []uint32{0}},
		{name: "offset length", nodeCount: 2, offsets: []uint32{0, 0}, roots: []uint32{0}},
		{name: "first offset", nodeCount: 1, offsets: []uint32{1, 1}, roots: []uint32{0}},
		{name: "descending offsets", nodeCount: 2, offsets: []uint32{0, 2, 1}, targets: []uint32{1, 1}, roots: []uint32{0}},
		{name: "final offset", nodeCount: 1, offsets: []uint32{0, 0}, targets: []uint32{0}, roots: []uint32{0}},
		{name: "offset beyond targets", nodeCount: 1, offsets: []uint32{0, 2}, targets: []uint32{0}, roots: []uint32{0}},
		{name: "target out of range", nodeCount: 1, offsets: []uint32{0, 1}, targets: []uint32{1}, roots: []uint32{0}},
		{name: "target duplicate", nodeCount: 2, offsets: []uint32{0, 2, 2}, targets: []uint32{1, 1}, roots: []uint32{0}},
		{name: "target noncanonical order", nodeCount: 3, offsets: []uint32{0, 2, 2, 2}, targets: []uint32{2, 1}, roots: []uint32{0}},
		{name: "empty roots", nodeCount: 1, offsets: []uint32{0, 0}},
		{name: "root out of range", nodeCount: 1, offsets: []uint32{0, 0}, roots: []uint32{1}},
		{name: "duplicate roots", nodeCount: 2, offsets: []uint32{0, 0, 0}, roots: []uint32{0, 0}},
		{name: "root noncanonical order", nodeCount: 2, offsets: []uint32{0, 0, 0}, roots: []uint32{1, 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proof, err := sealDominance(test.nodeCount, dominanceAdjacency(test.nodeCount, test.offsets, test.targets), test.roots)
			if err == nil {
				t.Fatalf("malformed input accepted: proof=%+v", proof)
			}
			if proof.dominates(0, 0) {
				t.Fatal("malformed input returned a positive proof")
			}
		})
	}
}

func TestDominanceRejectsMalformedReverseCSR(t *testing.T) {
	const nodeCount = uint32(4)
	offsets := []uint32{0, 2, 3, 4, 4}
	targets := []uint32{1, 2, 3, 3}
	valid := dominanceAdjacency(nodeCount, offsets, targets)
	if proof, err := sealDominance(nodeCount, valid, []uint32{0}); err != nil || !proof.dominates(0, 3) || proof.dominates(1, 3) {
		t.Fatalf("canonical reverse changed dominance truth: proof=%+v err=%v", proof, err)
	}
	tests := []struct {
		name           string
		reverseOffsets []uint32
		reverseTargets []uint32
	}{
		{
			name:           "missing reverse edge",
			reverseOffsets: []uint32{0, 0, 1, 2, 3},
			reverseTargets: []uint32{0, 0, 1},
		},
		{
			name:           "reverse target is not transpose",
			reverseOffsets: append([]uint32(nil), valid.reverseOffsets...),
			reverseTargets: []uint32{0, 0, 1, 1},
		},
		{
			name:           "reverse target out of range",
			reverseOffsets: append([]uint32(nil), valid.reverseOffsets...),
			reverseTargets: []uint32{0, 0, 1, nodeCount},
		},
		{
			name:           "reverse row is not canonical",
			reverseOffsets: append([]uint32(nil), valid.reverseOffsets...),
			reverseTargets: []uint32{0, 0, 2, 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adjacency := valid
			adjacency.reverseOffsets = test.reverseOffsets
			adjacency.reverseTargets = test.reverseTargets
			proof, err := sealDominance(nodeCount, adjacency, []uint32{0})
			if err == nil {
				t.Fatalf("malformed reverse accepted: proof=%+v", proof)
			}
			if proof.dominates(0, 0) {
				t.Fatal("malformed reverse returned a positive proof")
			}
		})
	}
}

func chainCSR(nodeCount int) ([]uint32, []uint32) {
	offsets := make([]uint32, nodeCount+1)
	targets := make([]uint32, 0, nodeCount-1)
	for node := 0; node < nodeCount-1; node++ {
		targets = append(targets, uint32(node+1))
		offsets[node+1] = uint32(len(targets))
	}
	offsets[nodeCount] = uint32(len(targets))
	return offsets, targets
}

func TestDominanceDeepChainIsIterative(t *testing.T) {
	const nodeCount = 8192
	offsets, targets := chainCSR(nodeCount)
	proof, err := sealDominance(nodeCount, dominanceAdjacency(nodeCount, offsets, targets), []uint32{0})
	if err != nil {
		t.Fatalf("sealDominance deep chain: %v", err)
	}
	for _, node := range []uint32{0, 1, nodeCount / 2, nodeCount - 1} {
		if !proof.dominates(0, node) {
			t.Fatalf("root does not dominate chain node %d", node)
		}
	}
	if proof.dominates(nodeCount-1, 0) {
		t.Fatal("chain tail dominates root")
	}
}

func TestDominanceAllocationCountDoesNotGrowPerEdge(t *testing.T) {
	smallOffsets, smallTargets := chainCSR(64)
	largeOffsets, largeTargets := chainCSR(4096)
	var sink dominanceProof
	allocs := func(offsets, targets []uint32) float64 {
		return testing.AllocsPerRun(10, func() {
			proof, err := sealDominance(uint32(len(offsets)-1), dominanceAdjacency(uint32(len(offsets)-1), offsets, targets), []uint32{0})
			if err != nil {
				panic(err)
			}
			sink = proof
		})
	}
	small := allocs(smallOffsets, smallTargets)
	large := allocs(largeOffsets, largeTargets)
	if large > small+4 {
		t.Fatalf("allocation count grew with edges: small=%v large=%v", small, large)
	}
	if sink.dominates(0, 4095) == false {
		t.Fatal("allocation probe did not retain a valid proof")
	}
}
