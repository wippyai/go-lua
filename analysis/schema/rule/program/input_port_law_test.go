package program

import "testing"

// TestJoinsMayShareOnePredecessorPort states the distinction the runtime must
// preserve: Input names a graph predecessor State, while join ordinal names
// one independent relation observation within that State. Arithmetic's left
// and right coordinates are the smallest exact-product witness.
func TestJoinsMayShareOnePredecessorPort(t *testing.T) {
	shared := seq5742Program(
		"input-port",
		[]JoinDecl{
			seq5742Join("input-port/first", []SourceRef{CandidateSource()}, Exact, false, false),
			seq5742Join("input-port/second", []SourceRef{PriorSource(0)}, Exact, false, false),
		},
		[]JoinRef{0, 1},
		[]OutputDecl{seq5742Output("input-port/write", ModeExact, 0)},
	)
	// One port is one predecessor State. Join ordinal, relation, projection,
	// axis, and owner-issued Unit identify each independent observation within
	// it; the port does not identify a Factor.
	shared.Joins[1].Read.Input = shared.Joins[0].Read.Input
	if problem, valid := shared.Check(); !valid {
		t.Fatalf("cross-axis joins sharing predecessor port 0 were rejected: %+v", problem)
	}
}

// TestAPortIsAxisNeutral is the clause the deleted validation denied. A port
// is a graph predecessor State, and a transported PointState carries the whole
// multi-Factor root vector - so which axis a join reads is a property of the
// JOIN, named by its relation, projection and declared axis, never of the port
// it observes that state at.
//
// Store is the concrete witness: an exact Value read and a selected Placement
// read both consume input 0. A rule that made the port carry the axis would
// have to declare two states where the graph has one.
func TestAPortIsAxisNeutral(t *testing.T) {
	for _, test := range []struct {
		name   string
		second JoinDecl
	}{
		{
			name:   "exact-value-and-selected-placement",
			second: seq5742Join("axis-neutral/selected", []SourceRef{PriorSource(0)}, Selected, false, true),
		},
		{
			name:   "two-exact-reads-of-one-axis",
			second: seq5742Join("axis-neutral/exact", []SourceRef{PriorSource(0)}, Exact, false, false),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			first := seq5742Join("axis-neutral/first", []SourceRef{CandidateSource()}, Exact, false, false)
			if test.name == "two-exact-reads-of-one-axis" {
				test.second.Read.Axis = first.Read.Axis
			}
			program := seq5742Program(
				"axis-neutral",
				[]JoinDecl{first, test.second},
				[]JoinRef{0, 1},
				[]OutputDecl{seq5742Output("axis-neutral/write", ModeExact, 0)},
			)
			program.Joins[1].Read.Input = program.Joins[0].Read.Input
			if problem, valid := program.Check(); !valid {
				t.Fatalf("joins sharing one predecessor port were rejected: %+v", problem)
			}
		})
	}
}

// TestACarryMayShareTheJoinPortItCarries states the one sharing that IS the
// declaration. A carry names the input whose prior fact it carries forward, so
// it names that join's port on purpose; refusing it would forbid the ordinary
// carrying rule.
func TestACarryMayShareTheJoinPortItCarries(t *testing.T) {
	carried := seq5742Program(
		"carried-port",
		[]JoinDecl{seq5742Join("carried-port/read", []SourceRef{CandidateSource()}, Exact, false, false)},
		[]JoinRef{0},
		[]OutputDecl{seq5742Output("carried-port/write", ModeExact, 0)},
	)
	carried.Carry = &CarryDecl{Input: carried.Joins[0].Read.Input, Mode: CarryIdentity}
	if problem, valid := carried.Check(); !valid {
		t.Fatalf("a carry on its own join's port was rejected: %+v", problem)
	}
}
