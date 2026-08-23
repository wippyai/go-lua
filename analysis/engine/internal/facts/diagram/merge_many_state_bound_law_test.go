package diagram

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// disjointFoldFixture is one many-way contribution fold whose operands all
// carry the same value under pairwise disjoint single-atom regions. Its join
// is exactly "that value wherever any operand is defined", so the result
// diagram has operand-count many regions. Any correct fold therefore has a
// structural cost bounded by the operands and the result, never by the
// product of the operand region spaces.
type disjointFoldFixture struct {
	facts    *Diagram[uint64, uint64, uint8]
	manager  *guard.Manager
	operands []Root[uint64, uint64, uint8]
	regions  []support.Mask
	previous Root[uint64, uint64, uint8]
	value    terminal.ID[uint8]
}

func newDisjointFoldFixture(t *testing.T, width int) *disjointFoldFixture {
	t.Helper()
	atoms := make([]guard.Atom, width)
	for index := range atoms {
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		t.Fatal(err)
	}
	setup := support.New(manager)
	if setup == nil {
		t.Fatal("support setup")
	}
	regions := make([]support.Mask, width)
	for index, atom := range atoms {
		region, ok := setup.Literal(atom, true)
		if !ok {
			t.Fatal("operand region")
		}
		regions[index] = region
	}
	if !setup.Seal() {
		t.Fatal("region seal")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	valueID, ok := values.Admit(7)
	if !ok || !values.Seal() {
		t.Fatal("terminal admit")
	}
	facts, ok := New(Config[uint64, uint64, uint8]{Factors: []uint64{1}, Terminals: values, Guards: manager})
	if !ok {
		t.Fatal("diagram")
	}
	column := func(build func(builder *Builder[uint64, uint64, uint8]) *node[uint8]) Root[uint64, uint64, uint8] {
		t.Helper()
		builder := facts.Begin()
		if builder == nil {
			t.Fatal("builder")
		}
		value := build(builder)
		keys := makeKey(uint64(1), value, nil, nil)
		sealed, valid := builder.Seal(Root[uint64, uint64, uint8]{
			diagram: facts,
			root:    makeFactor(uint64(1), 0, keys, nil, nil),
			count:   1,
			lease:   builder.token,
		})
		if !valid {
			t.Fatal("column seal")
		}
		return sealed
	}
	fixture := &disjointFoldFixture{facts: facts, manager: manager, regions: regions, value: valueID}
	fixture.previous = column(func(builder *Builder[uint64, uint64, uint8]) *node[uint8] {
		return builder.terminal(terminal.ID[uint8]{})
	})
	fixture.operands = make([]Root[uint64, uint64, uint8], width)
	for index := range fixture.operands {
		fixture.operands[index] = column(func(builder *Builder[uint64, uint64, uint8]) *node[uint8] {
			return builder.terminal(valueID)
		})
	}
	return fixture
}

// fold runs the many-way merge and reports the peak number of live fused
// traversal states the fold materialized.
func (fixture *disjointFoldFixture) fold(t *testing.T) (Root[uint64, uint64, uint8], int) {
	t.Helper()
	builder := fixture.facts.Begin()
	if builder == nil {
		t.Fatal("merge builder")
	}
	scratch := NewSoleScratch[uint64, uint8]()
	peak := 0
	scratch.SetCheckpoint(func() bool {
		if len(scratch.manyStates) > peak {
			peak = len(scratch.manyStates)
		}
		return true
	})
	work := support.New(fixture.manager)
	if work == nil {
		t.Fatal("regions work")
	}
	merged, valid := builder.MergeSoleFactorMany(fixture.previous, fixture.operands, scratch, work, func(_ uint64, values []terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		if len(values) == 0 {
			return terminal.ID[uint8]{}, false
		}
		return values[0], true
	}, func(key uint64, output []support.Mask) bool {
		if key != 1 || len(output) != len(fixture.regions) {
			return false
		}
		copy(output, fixture.regions)
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
	return merged, peak
}

// TestMergeSoleManyStateCountIsBoundedByOperandsAndResult is the structural
// cost law of the many-way contribution fold: the fused traversal may not
// enumerate the product of the operand region spaces. Every operand here
// contributes the same value, so the join distinguishes only "defined" from
// "undefined" and the fold has to materialize a number of states linear in
// the operand count.
func TestMergeSoleManyStateCountIsBoundedByOperandsAndResult(t *testing.T) {
	for _, width := range []int{4, 8, 12, 16} {
		fixture := newDisjointFoldFixture(t, width)
		merged, peak := fixture.fold(t)
		for index := range fixture.regions {
			atom := guard.Atom(index + 1)
			got, present, valid := fixture.facts.At(merged, 1, 1, func(probe guard.Atom) bool { return probe == atom })
			if !valid || !present || got != fixture.value {
				t.Fatalf("width %d operand %d meaning = %v/%t/%t", width, index, got, present, valid)
			}
		}
		if bound := 8 * (width + 1); peak > bound {
			t.Errorf("width %d fused fold materialized %d states, structural bound %d", width, peak, bound)
		} else {
			t.Logf("width %d states %d (bound %d)", width, peak, bound)
		}
	}
}
