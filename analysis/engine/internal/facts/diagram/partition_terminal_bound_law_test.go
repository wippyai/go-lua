package diagram

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// sharedTerminalFixture is one FDD decision function whose node graph is a
// shared DAG: every level holds exactly two nodes, both successors of a level
// are level nodes again, and only two terminals are ever reached. The number
// of root-to-leaf paths is therefore 2^width while the graph holds 2*width+2
// nodes and two distinct terminals.
//
// It is the parity function over width atoms, the canonical witness that a
// decision diagram's path count is exponential in its node count.
type sharedTerminalFixture struct {
	facts   *Diagram[uint64, uint64, uint8]
	manager *guard.Manager
	atoms   []guard.Atom
	value   Value[uint8]
	region  support.Mask
	even    terminal.ID[uint8]
	odd     terminal.ID[uint8]
}

func newSharedTerminalFixture(t *testing.T, width int) *sharedTerminalFixture {
	t.Helper()
	atoms := make([]guard.Atom, width)
	for index := range atoms {
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
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
	even, evenOK := values.Admit(0)
	odd, oddOK := values.Admit(1)
	if !evenOK || !oddOK || !values.Seal() {
		t.Fatal("terminal admit")
	}
	facts, ok := New(Config[uint64, uint64, uint8]{Factors: []uint64{1}, Terminals: values, Guards: manager})
	if !ok {
		t.Fatal("diagram")
	}
	builder := facts.Begin()
	if builder == nil {
		t.Fatal("builder")
	}
	// Build the parity graph bottom up. parity[0] is the node reached once an
	// even number of atoms is set, parity[1] the odd one.
	parity := [2]*node[uint8]{builder.terminal(even), builder.terminal(odd)}
	for level := width - 1; level >= 0; level-- {
		next := [2]*node[uint8]{
			builder.decision(atoms[level], parity[0], parity[1]),
			builder.decision(atoms[level], parity[1], parity[0]),
		}
		parity = next
	}
	root := parity[0]
	if root == nil || root.terminal {
		t.Fatal("parity root")
	}
	builder.Discard()

	work := support.New(manager)
	if work == nil {
		t.Fatal("support")
	}
	region := work.True()
	if !work.Seal() {
		t.Fatal("region seal")
	}
	return &sharedTerminalFixture{
		facts:   facts,
		manager: manager,
		atoms:   atoms,
		value:   Value[uint8]{owner: facts.owner, node: root},
		region:  region,
		even:    even,
		odd:     odd,
	}
}

// TestPartitionValueTerminalsIsBoundedByDistinctTerminals is the structural
// cost law of the value partition: the read publishes one region per distinct
// terminal the value denotes, never one region per root-to-leaf path. The
// node graph is a shared DAG, so a path-enumerating partition delivers 2^width
// pieces where the value takes exactly two values, and every consumer then
// pays that count in Boolean work to re-union what the partition should never
// have split.
func TestPartitionValueTerminalsIsBoundedByDistinctTerminals(t *testing.T) {
	for _, width := range []int{4, 8, 12, 16} {
		fixture := newSharedTerminalFixture(t, width)
		scratch := support.New(fixture.manager)
		if scratch == nil {
			t.Fatal("scratch")
		}
		pieces := 0
		seen := map[terminal.ID[uint8]]int{}
		completed, valid := fixture.facts.PartitionValueTerminals(fixture.value, fixture.region, scratch, func(id terminal.ID[uint8], cell support.Mask) bool {
			pieces++
			seen[id]++
			return true
		})
		if !completed || !valid {
			t.Fatalf("width %d: partition refused", width)
		}
		if len(seen) != 2 {
			t.Fatalf("width %d: partition denotes %d distinct terminals, want 2", width, len(seen))
		}
		if pieces != len(seen) {
			t.Fatalf("width %d: partition emitted %d pieces for %d distinct terminals (paths=%d); one region per terminal is the law", width, pieces, len(seen), 1<<width)
		}
	}
}

// TestPartitionValueTerminalsCoalescesWithoutLosingMeaning is the meaning half
// of the same law: coalescing the pieces of one terminal is exact, so every
// valuation still selects the piece carrying the terminal the decision
// function evaluates to, and the pieces still partition the input region.
func TestPartitionValueTerminalsCoalescesWithoutLosingMeaning(t *testing.T) {
	const width = 4
	fixture := newSharedTerminalFixture(t, width)
	scratch := support.New(fixture.manager)
	if scratch == nil {
		t.Fatal("scratch")
	}
	cells := map[terminal.ID[uint8]][]support.Mask{}
	order := []terminal.ID[uint8]{}
	completed, valid := fixture.facts.PartitionValueTerminals(fixture.value, fixture.region, scratch, func(id terminal.ID[uint8], cell support.Mask) bool {
		if _, known := cells[id]; !known {
			order = append(order, id)
		}
		cells[id] = append(cells[id], cell)
		return true
	})
	if !completed || !valid {
		t.Fatal("partition refused")
	}
	work := support.New(fixture.manager)
	if work == nil {
		t.Fatal("check work")
	}
	for assignment := 0; assignment < 1<<width; assignment++ {
		minterm := work.True()
		parity := 0
		for index, atom := range fixture.atoms {
			set := assignment&(1<<index) != 0
			if set {
				parity ^= 1
			}
			next, ok := work.Conjoin(minterm, atom, set)
			if !ok {
				t.Fatal("minterm")
			}
			minterm = next
		}
		want := fixture.even
		if parity == 1 {
			want = fixture.odd
		}
		selected := 0
		var carried terminal.ID[uint8]
		for id, masks := range cells {
			for _, mask := range masks {
				overlap, ok := work.And(mask, minterm)
				if !ok {
					t.Fatal("overlap")
				}
				if !support.Empty(overlap) {
					selected++
					carried = id
				}
			}
		}
		if selected != 1 {
			t.Fatalf("assignment %d: %d pieces cover the valuation, want exactly 1", assignment, selected)
		}
		if carried != want {
			t.Fatalf("assignment %d: piece carries the wrong terminal", assignment)
		}
	}
	work.Discard()
	if len(order) != 2 || order[0] != fixture.even || order[1] != fixture.odd {
		t.Fatalf("emission order changed: %d terminals", len(order))
	}
}
