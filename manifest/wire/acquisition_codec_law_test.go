package wire

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/types/signature"
)

func acquisitionManifest(t *testing.T, acquisitions []Acquisition) *Manifest {
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
	declaration.DefineFunctionSignature("connect", signature.Function{
		Type: typ.Func().Returns(handle).Build(),
	})
	declaration.DefineFunctionOperation("connect", Operation{Acquisitions: acquisitions})
	return declaration
}

// An acquisition names a result slot of an authored outcome. The declaration
// crosses the module boundary intact, so a consumer reads back exactly the
// relation the provider wrote.
func TestAcquisitionRoundTripsThroughManifestCodec(t *testing.T) {
	declared := []Acquisition{
		{Outcome: 0, Result: 1, Protocol: "connection", State: "open"},
		{Outcome: 0, Result: 0, Protocol: "connection", State: "closed"},
	}
	encoded, err := Encode(acquisitionManifest(t, declared))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	law, ok := decoded.FunctionOperations["connect"]
	if !ok {
		t.Fatal("the decoded manifest holds no operation law for connect")
	}
	// CloneOperation canonicalizes the rows, so the decoded order is by
	// (outcome, result) and is independent of the authoring order.
	want := []Acquisition{
		{Outcome: 0, Result: 0, Protocol: "connection", State: "closed"},
		{Outcome: 0, Result: 1, Protocol: "connection", State: "open"},
	}
	if len(law.Acquisitions) != len(want) {
		t.Fatalf("decoded acquisitions = %+v, want %+v", law.Acquisitions, want)
	}
	for index := range want {
		if law.Acquisitions[index] != want[index] {
			t.Fatalf("decoded acquisition %d = %+v, want %+v", index, law.Acquisitions[index], want[index])
		}
	}
}

// The declared state machine is the only authority on which state may be
// acquired. A row that names a protocol or a state the manifest never declared
// is refused at the boundary rather than carried into a consumer.
func TestAcquisitionOutsideDeclaredStateMachineIsRefused(t *testing.T) {
	for name, test := range map[string]struct {
		acquisition Acquisition
		want        string
	}{
		"undeclared protocol": {
			acquisition: Acquisition{Protocol: "session", State: "open"},
			want:        "is not declared as a typestate FSM",
		},
		"undeclared state": {
			acquisition: Acquisition{Protocol: "connection", State: "draining"},
			want:        "does not declare acquire state",
		},
		"missing protocol": {
			acquisition: Acquisition{State: "open"},
			want:        "has no protocol",
		},
		"missing state": {
			acquisition: Acquisition{Protocol: "connection"},
			want:        "has no state",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Encode(acquisitionManifest(t, []Acquisition{test.acquisition}))
			if err == nil {
				t.Fatalf("acquisition %+v crossed the manifest boundary", test.acquisition)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refusal = %v, want it to name %q", err, test.want)
			}
		})
	}
}
