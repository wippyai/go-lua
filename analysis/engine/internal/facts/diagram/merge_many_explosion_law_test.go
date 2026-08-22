package diagram

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestMergeSoleManyPreviousComplexityIsIndependentOfPreviousLane proves that
// the predecessor is only an outer sparse-key reconstruction input. The
// previous root has a deliberately deep/shared FDD, while the two semantic
// operands and their authored supports stay small and interleaved. Replacing
// that predecessor with a terminal-only root must not change the fused
// per-column tuple state count or the semantic result.
func TestMergeSoleManyPreviousComplexityIsIndependentOfPreviousLane(t *testing.T) {
	const (
		depth = 32
		lanes = 3
	)
	atoms := make([]guard.Atom, lanes*depth)
	for index := range atoms {
		atoms[index] = guard.Atom(index + 1)
	}
	lane := func(index int) []guard.Atom {
		result := make([]guard.Atom, depth)
		for level := range result {
			result[level] = atoms[level*lanes+index]
		}
		return result
	}
	manager, err := guard.New(atoms)
	if err != nil {
		t.Fatal(err)
	}
	setup := support.New(manager)
	if setup == nil {
		t.Fatal("support setup")
	}
	leftSupport, ok := setup.Literal(lane(1)[0], true)
	if !ok {
		t.Fatal("left support")
	}
	rightSupport, ok := setup.Literal(lane(2)[0], true)
	if !ok || !setup.Seal() {
		t.Fatal("right support")
	}

	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	lowID, ok := values.Admit(1)
	if !ok {
		t.Fatal("low terminal")
	}
	highID, ok := values.Admit(2)
	if !ok || !values.Seal() {
		t.Fatal("high terminal")
	}
	facts, ok := New(Config[uint64, uint64, uint8]{Factors: []uint64{1}, Terminals: values, Guards: manager})
	if !ok {
		t.Fatal("diagram")
	}

	sealColumn := func(builder *Builder[uint64, uint64, uint8], value *node[uint8]) Root[uint64, uint64, uint8] {
		t.Helper()
		keys := makeKey(uint64(7), value, nil, nil)
		candidate := Root[uint64, uint64, uint8]{
			diagram: facts,
			root:    makeFactor(uint64(1), 0, keys, nil, nil),
			count:   1,
			lease:   builder.lease,
		}
		sealed, valid := builder.Seal(candidate)
		if !valid {
			t.Fatal("column seal")
		}
		return sealed
	}
	buildPrevious := func(order []guard.Atom, deep bool) Root[uint64, uint64, uint8] {
		t.Helper()
		builder := facts.Begin()
		if builder == nil {
			t.Fatal("previous builder")
		}
		low, high := builder.terminal(lowID), builder.terminal(highID)
		if !deep {
			return sealColumn(builder, low)
		}
		states := make([]*node[uint8], len(order))
		for index := len(states) - 1; index >= 0; index-- {
			left, right := low, high
			if index+1 < len(states) {
				left = states[index+1]
			}
			if index+4 < len(states) {
				right = states[index+4]
			}
			states[index] = builder.decision(order[index], left, right)
		}
		return sealColumn(builder, states[0])
	}
	previousSmall := buildPrevious(nil, false)
	previousLarge := buildPrevious(lane(0), true)

	buildOperand := func(order []guard.Atom, highFirst bool) Root[uint64, uint64, uint8] {
		builder := facts.Begin()
		if builder == nil {
			t.Fatal("operand builder")
		}
		low, high := builder.terminal(lowID), builder.terminal(highID)
		if highFirst {
			low, high = high, low
		}
		return sealColumn(builder, builder.decision(order[0], low, high))
	}
	left := buildOperand(lane(1), false)
	right := buildOperand(lane(2), true)

	run := func(previous Root[uint64, uint64, uint8]) (Root[uint64, uint64, uint8], int) {
		t.Helper()
		builder := facts.Begin()
		if builder == nil {
			t.Fatal("merge builder")
		}
		scratch := NewSoleScratch[uint64, uint8]()
		maxStates := 0
		scratch.SetCheckpoint(func() bool {
			if len(scratch.manyStates) > maxStates {
				maxStates = len(scratch.manyStates)
			}
			return true
		})
		regions := support.New(manager)
		if regions == nil {
			t.Fatal("regions")
		}
		merged, valid := builder.MergeSoleFactorMany(previous, []Root[uint64, uint64, uint8]{left, right}, scratch, regions, func(_ uint64, values []terminal.ID[uint8]) (terminal.ID[uint8], bool) {
			if len(values) == 0 {
				return terminal.ID[uint8]{}, false
			}
			return values[0], true
		}, func(key uint64, output []support.Mask) bool {
			if key != 7 || len(output) != 2 {
				return false
			}
			output[0], output[1] = leftSupport, rightSupport
			return true
		})
		if !valid {
			builder.Discard()
			t.Fatal("many merge")
		}
		merged, valid = builder.Seal(merged)
		if !valid {
			t.Fatal("merged seal")
		}
		scratch.Clear()
		return merged, maxStates
	}

	small, smallStates := run(previousSmall)
	large, largeStates := run(previousLarge)
	if smallStates != largeStates || smallStates == 0 {
		t.Fatalf("width-two fused fold predecessor dependence: small=%d large=%d", smallStates, largeStates)
	}
	if !facts.Equal(small, large) {
		t.Fatal("predecessor topology changed folded semantic result")
	}
	for _, result := range []Root[uint64, uint64, uint8]{small, large} {
		if got, present, valid := facts.At(result, 1, 7, func(atom guard.Atom) bool { return atom == lane(1)[0] }); !valid || !present || got != highID {
			t.Fatalf("left authored meaning = %v/%t/%t, want high terminal", got, present, valid)
		}
		if got, present, valid := facts.At(result, 1, 7, func(atom guard.Atom) bool { return atom == lane(2)[0] }); !valid || !present || got != lowID {
			t.Fatalf("right authored meaning = %v/%t/%t, want low terminal", got, present, valid)
		}
	}
}

func TestSeedSoleManyPredecessorReusesSharedCrossEdgeLaw(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	lowID, lowOK := values.Admit(1)
	highID, highOK := values.Admit(2)
	if !lowOK || !highOK || !values.Seal() {
		t.Fatal("terminals")
	}
	facts, ok := New(Config[uint64, uint64, uint8]{Factors: []uint64{1}, Terminals: values, Guards: manager})
	if !ok {
		t.Fatal("diagram")
	}
	prior := facts.Begin()
	if prior == nil {
		t.Fatal("prior builder")
	}
	low, high := prior.terminal(lowID), prior.terminal(highID)
	shared := prior.decision(3, low, high)
	left := prior.decision(2, low, shared)
	root := prior.decision(1, left, shared)
	// Retain this intentionally shared root as an immutable predecessor: the
	// right child is also reached from the left subtree. A parallel child push
	// would encounter it gray and falsely report a cycle.
	keys := makeKey(uint64(7), root, nil, nil)
	sealed, sealedOK := prior.Seal(Root[uint64, uint64, uint8]{diagram: facts, root: makeFactor(uint64(1), 0, keys, nil, nil), count: 1, lease: prior.lease})
	if !sealedOK || sealed.root == nil {
		t.Fatal("prior seal")
	}

	builder := facts.Begin()
	if builder == nil {
		t.Fatal("candidate builder")
	}
	seeded := builder.seedSoleManyPredecessor(root, NewSoleScratch[uint64, uint8]())
	if !seeded || builder.imports[root] != root || builder.decisions[decisionKey[uint8]{atom: 1, low: left, high: shared}] != root {
		builder.Discard()
		t.Fatalf("shared predecessor seed seeded=%t root=%t", seeded, builder.imports[root] == root)
	}
	// Every reachable predecessor node is registered under its own identity,
	// so a fold cell that resolves to one of these terminals republishes the
	// predecessor node instead of allocating an equal duplicate.
	for _, node := range []*node[uint8]{low, high, shared, left, root} {
		if builder.imports[node] != node {
			builder.Discard()
			t.Fatal("shared predecessor omitted a reachable node")
		}
	}
	if builder.terminals[lowID] != low || builder.terminals[highID] != high {
		builder.Discard()
		t.Fatal("shared predecessor omitted a terminal leaf registration")
	}
	builder.Discard()
}
