package protocol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// requirementInput is protocolInput plus one requirement on the same operation
// input the transition already names: the read half of the same protocol.
func requirementInput() Input {
	input := protocolInput()
	input.Protocols[0].Requirements = []vocabulary.RequirementSpec{{
		Operation: 1,
		Input:     vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal},
		State:     1,
	}}
	return input
}

// A declared requirement seals and answers the read surface with the exact
// Protocol x Operation x InputSource x State relation it was declared as.
func TestDeclaredRequirementSealsAndQueriesBack(t *testing.T) {
	table, err := Compile(requirementInput())
	if err != nil {
		t.Fatal(err)
	}
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed table has no protocol handle")
	}
	if table.ProtocolRequirementCount(protocol) != 1 {
		t.Fatalf("requirement count = %d, want 1", table.ProtocolRequirementCount(protocol))
	}
	operation, source, state, found := table.ProtocolRequirementAt(protocol, 0)
	if !found {
		t.Fatal("sealed protocol has no requirement row")
	}
	if operation != 1 {
		t.Fatalf("requirement operation = %d, want the declared operation", operation)
	}
	if source.Kind != vocabulary.InputSourceValueFormal || source.Ordinal != 0 {
		t.Fatalf("requirement input = %d/%d, want value formal 0", source.Kind, source.Ordinal)
	}
	if name, nameOK := table.StateName(protocol, state); !nameOK || name != "open" {
		t.Fatalf("required state = %q/%v, want open", name, nameOK)
	}
	rows := table.CountRows()
	ids := denominator.GeneratedTargetIDs()
	if got, rowOK := rows.Value(ids.TargetProtocolRequirement); !rowOK || got != 1 {
		t.Fatalf("requirement count row = %d/%v, want 1/true", got, rowOK)
	}
}

// The callable-requirement authority answers every protocol that constrains
// one operation, so a consumer holding an operation handle needs no protocol
// scan of its own. The requirement relation is one kind of that authority's
// closed obligation vocabulary, addressed by the protocol-local row that
// states it.
func TestRequirementIsReachableFromTheCallableAuthority(t *testing.T) {
	table, err := Compile(requirementInput())
	if err != nil {
		t.Fatal(err)
	}
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed table has no protocol handle")
	}
	var found int
	for index := 0; index < table.DemandCount(1); index++ {
		demand, demandOK := table.DemandAt(1, index)
		if !demandOK {
			t.Fatalf("demand %d is unavailable", index)
		}
		if demand.Kind != DemandRequirement {
			continue
		}
		found++
		if demand.Protocol != protocol {
			t.Fatalf("requirement protocol = %d, want %d", demand.Protocol, protocol)
		}
		_, input, state, rowOK := table.ProtocolRequirementAt(demand.Protocol, demand.Row)
		if !rowOK || input.Kind != vocabulary.InputSourceValueFormal || input.Ordinal != 0 {
			t.Fatalf("requirement input = %+v/%v, want value formal 0", input, rowOK)
		}
		if name, nameOK := table.StateName(demand.Protocol, state); !nameOK || name != "open" {
			t.Fatalf("required state = %q/%v, want open", name, nameOK)
		}
	}
	if found != 1 {
		t.Fatalf("the authority carries %d requirement obligations, want the single declared row", found)
	}
	if count := table.DemandCount(0); count != 0 {
		t.Fatalf("DemandCount(invalid operation) = %d, want nothing", count)
	}
}

// The state coordinate is resolved against the protocol's own state machine.
// A requirement naming a state the protocol never declares is refused, so no
// requirement can constrain a state no acquisition or transition can reach.
func TestRequirementOutsideTheDeclaredStateMachineIsRefused(t *testing.T) {
	input := requirementInput()
	input.Protocols[0].Requirements[0].State = 2
	if _, err := Compile(input); err == nil {
		t.Fatal("a requirement on an undeclared state sealed")
	}
	input = requirementInput()
	input.Protocols[0].Requirements[0].State = 0
	if _, err := Compile(input); err == nil {
		t.Fatal("a requirement with no state sealed")
	}
}

// The operation reference is resolved against the sealed operation geometry
// exactly as every other protocol row is.
func TestRequirementOutsideTheOperationTableIsRefused(t *testing.T) {
	input := requirementInput()
	input.Protocols[0].Requirements[0].Operation = 0
	if _, err := Compile(input); err == nil {
		t.Fatal("a requirement with no operation sealed")
	}
	input = requirementInput()
	input.Protocols[0].Requirements[0].Operation = 2
	if _, err := Compile(input); err == nil {
		t.Fatal("a requirement on an unknown operation sealed")
	}
}

// A requirement addresses an input of the operation it names. A coordinate
// outside that operation's own geometry is refused rather than stored as a
// constraint on an argument the operation does not have.
func TestRequirementInputOutsideTheOperationIsRefused(t *testing.T) {
	for _, source := range []vocabulary.InputSource{
		{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1},
		{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0},
		{Kind: vocabulary.InputSourceAllInputs},
		{},
	} {
		input := requirementInput()
		input.Protocols[0].Requirements[0].Input = source
		if _, err := Compile(input); err == nil {
			t.Fatalf("a requirement on input %+v sealed outside the operation geometry", source)
		}
	}
}

// The relation is a set: the same operation, input, and state stated twice is
// a malformed declaration, not two constraints.
func TestDuplicateRequirementIsRefused(t *testing.T) {
	input := requirementInput()
	input.Protocols[0].Requirements = append(input.Protocols[0].Requirements, input.Protocols[0].Requirements[0])
	if _, err := Compile(input); err == nil {
		t.Fatal("a duplicated requirement sealed")
	}
}

// A requirement is not a transition with equal endpoints. It adds no
// transition row and no outcome arm, so it moves no state and discharges no
// obligation; the transition relation of the same protocol is untouched.
func TestRequirementIsNotATransition(t *testing.T) {
	base, err := Compile(protocolInput())
	if err != nil {
		t.Fatal(err)
	}
	table, err := Compile(requirementInput())
	if err != nil {
		t.Fatal(err)
	}
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed table has no protocol handle")
	}
	baseProtocol, baseOK := base.ProtocolAt(0)
	if !baseOK {
		t.Fatal("baseline table has no protocol handle")
	}
	if table.TransitionCount(protocol) != base.TransitionCount(baseProtocol) {
		t.Fatalf("transition count = %d, want the baseline %d", table.TransitionCount(protocol), base.TransitionCount(baseProtocol))
	}
	if table.StateCount(protocol) != base.StateCount(baseProtocol) {
		t.Fatalf("state count = %d, want the baseline %d", table.StateCount(protocol), base.StateCount(baseProtocol))
	}
	if table.EscapeCount(protocol) != base.EscapeCount(baseProtocol) {
		t.Fatalf("escape count = %d, want the baseline %d", table.EscapeCount(protocol), base.EscapeCount(baseProtocol))
	}
	rows, baseline := table.CountRows(), base.CountRows()
	ids := denominator.GeneratedTargetIDs()
	for _, id := range []schema.EntryID{
		ids.TargetProtocolTransition, ids.TargetProtocolTransitionOutcome,
		ids.TargetProtocolState, ids.TargetProtocolAcquisition,
	} {
		got, gotOK := rows.Value(id)
		want, wantOK := baseline.Value(id)
		if !gotOK || !wantOK || got != want {
			t.Fatalf("relation %v = %d/%v, want the baseline %d/%v", id, got, gotOK, want, wantOK)
		}
	}
}
