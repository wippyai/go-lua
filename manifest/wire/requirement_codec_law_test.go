package wire

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/types/signature"
)

// requirementManifest declares the connection state machine and one member
// that reads a connection without moving it. query is the member the resource
// lifecycle corpus calls on an open connection, so the requirement row this
// boundary carries is the one that corpus needs stated.
func requirementManifest(t *testing.T, requirements []Requirement) *Manifest {
	t.Helper()
	declaration := New("resource")
	handle := typ.NewInterface("resource.Connection", nil)
	declaration.DefineType("Connection", handle)
	if err := declaration.DefineTypestateProtocol(typestate.Definition{
		Protocol:    "connection",
		States:      []typestate.State{"open", "closed"},
		FinalStates: []typestate.State{"closed"},
		Transitions: []typestate.TransitionDecl{{From: "open", To: "closed"}},
	}); err != nil {
		t.Fatal(err)
	}
	declaration.DefineFunctionSignature("query", signature.Function{
		Type: typ.Func().Param("connection", handle).Build(),
	})
	declaration.DefineFunctionOperation("query", Operation{Requirements: requirements})
	return declaration
}

// A requirement names an input and the state that input must be in. The
// declaration crosses the module boundary intact and in canonical order, so a
// consumer reads back exactly the relation the provider wrote, whatever order
// it was authored in.
func TestRequirementRoundTripsThroughManifestCodec(t *testing.T) {
	declared := []Requirement{
		{Input: InputSource{Kind: InputSourceValue, Ordinal: 1}, Protocol: "connection", State: "open"},
		{Input: InputSource{Kind: InputSourceValue, Ordinal: 0}, Protocol: "connection", State: "closed"},
		{Input: InputSource{Kind: InputSourceValue, Ordinal: 0}, Protocol: "connection", State: "open"},
	}
	encoded, err := Encode(requirementManifest(t, declared))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	law, ok := decoded.FunctionOperations["query"]
	if !ok {
		t.Fatal("the decoded manifest holds no operation law for query")
	}
	// CloneOperation canonicalizes the rows, so the decoded order is by
	// (input kind, input ordinal, protocol, state) and is independent of the
	// authoring order.
	want := []Requirement{
		{Input: InputSource{Kind: InputSourceValue, Ordinal: 0}, Protocol: "connection", State: "closed"},
		{Input: InputSource{Kind: InputSourceValue, Ordinal: 0}, Protocol: "connection", State: "open"},
		{Input: InputSource{Kind: InputSourceValue, Ordinal: 1}, Protocol: "connection", State: "open"},
	}
	if len(law.Requirements) != len(want) {
		t.Fatalf("decoded requirements = %+v, want %+v", law.Requirements, want)
	}
	for index := range want {
		if law.Requirements[index] != want[index] {
			t.Fatalf("decoded requirement %d = %+v, want %+v", index, law.Requirements[index], want[index])
		}
	}
}

// A requirement declares no move, so it neither adds a transition edge nor
// discharges an obligation. The state machine the manifest declares is the same
// one before and after the requirement row is authored.
func TestRequirementDeclaresNoTransitionAndNoObligation(t *testing.T) {
	requirement := Requirement{Input: InputSource{Kind: InputSourceValue}, Protocol: "connection", State: "open"}
	encoded, err := Encode(requirementManifest(t, []Requirement{requirement}))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := decoded.TypestateProtocol("connection")
	if !ok {
		t.Fatal("the decoded manifest declares no connection protocol")
	}
	want := typestate.Definition{
		Protocol:    "connection",
		States:      []typestate.State{"closed", "open"},
		FinalStates: []typestate.State{"closed"},
		Transitions: []typestate.TransitionDecl{{From: "open", To: "closed"}},
	}
	got := definition.Normalized()
	if len(got.Transitions) != len(want.Transitions) || got.Transitions[0] != want.Transitions[0] {
		t.Fatalf("declared transitions = %+v, want %+v", got.Transitions, want.Transitions)
	}
	if len(got.FinalStates) != 1 || got.FinalStates[0] != "closed" {
		t.Fatalf("declared final states = %v, want the obligation unchanged", got.FinalStates)
	}
	if !got.AllowsTransition("open", "closed") || got.AllowsTransition("open", "open") {
		t.Fatal("the requirement row added an edge to the state machine")
	}
	if law, lawOK := decoded.FunctionOperations["query"]; !lawOK || len(law.Acquisitions) != 0 {
		t.Fatalf("a requirement row produced an acquisition: %+v/%v", law.Acquisitions, lawOK)
	}
}

// The declared state machine is the only authority on which state may be
// required, and a requirement with no subject constrains nothing. Each such row
// is refused at the boundary rather than carried into a consumer.
func TestRequirementOutsideDeclaredStateMachineIsRefused(t *testing.T) {
	for name, test := range map[string]struct {
		requirement Requirement
		want        string
	}{
		"undeclared protocol": {
			requirement: Requirement{Input: InputSource{Kind: InputSourceValue}, Protocol: "session", State: "open"},
			want:        "is not declared as a typestate FSM",
		},
		"undeclared state": {
			requirement: Requirement{Input: InputSource{Kind: InputSourceValue}, Protocol: "connection", State: "draining"},
			want:        "does not declare required state",
		},
		"missing protocol": {
			requirement: Requirement{Input: InputSource{Kind: InputSourceValue}, State: "open"},
			want:        "has no protocol",
		},
		"missing state": {
			requirement: Requirement{Input: InputSource{Kind: InputSourceValue}, Protocol: "connection"},
			want:        "has no state",
		},
		"missing input": {
			requirement: Requirement{Protocol: "connection", State: "open"},
			want:        "has no input",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Encode(requirementManifest(t, []Requirement{test.requirement}))
			if err == nil {
				t.Fatalf("requirement %+v crossed the manifest boundary", test.requirement)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refusal = %v, want it to name %q", err, test.want)
			}
		})
	}
}
