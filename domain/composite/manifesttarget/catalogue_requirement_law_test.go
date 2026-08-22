package manifesttarget_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// requirementSessionManifest is sessionManifest plus one member that reads the
// handle without moving it: the read half of the same protocol.
func requirementSessionManifest() *manifestwire.Manifest {
	declaration := sessionManifest()
	handle := sessionHandleType()
	declaration.DefineFunctionSignature("query", signature.Function{
		Type: typ.Func().Param("handle", handle).Build(),
	})
	declaration.DefineFunctionOperation("query", manifestwire.Operation{
		Requirements: []manifestwire.Requirement{{
			Input:    manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
			Protocol: "session",
			State:    "open",
		}},
	})
	return declaration
}

// A requirement the manifest boundary admits is carried into the sealed
// protocol table as its own relation, and the sealed target answers the exact
// Protocol x Operation x InputSource x State constraint the provider stated.
func TestDeclaredRequirementSealsIntoTheProtocolTable(t *testing.T) {
	catalogue, err := manifest.Seal(append(stdlib.Providers(), manifest.Provider{
		Identity: "session", Mount: manifest.MountModule,
		Declaration: requirementSessionManifest,
	})...)
	if err != nil {
		t.Fatalf("the manifest boundary refused a well-formed requirement: %v", err)
	}
	sealed, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatalf("a declared requirement did not seal: %v", err)
	}
	table := sealed.Protocols()
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed target has no protocol handle")
	}
	if table.ProtocolRequirementCount(protocol) != 1 {
		t.Fatalf("requirement count = %d, want the declared row", table.ProtocolRequirementCount(protocol))
	}
	operation, input, state, found := table.ProtocolRequirementAt(protocol, 0)
	if !found {
		t.Fatal("the sealed protocol answers no requirement row")
	}
	if operation != sessionOperation(t, sealed, "query") {
		t.Fatalf("requirement operation = %d, want session.query", operation)
	}
	if input.Kind != vocabulary.InputSourceValueFormal || input.Ordinal != 0 {
		t.Fatalf("requirement input = %+v, want value formal 0", input)
	}
	if name, nameOK := table.StateName(protocol, state); !nameOK || name != "open" {
		t.Fatalf("required state = %q/%v, want open", name, nameOK)
	}
	rows := table.RequirementsOf(operation)
	if len(rows) != 1 || rows[0].Protocol != protocol || rows[0].State != state {
		t.Fatalf("RequirementsOf(session.query) = %+v, want the single sealed row", rows)
	}
}

// The requirement is a read, not a move. Carrying it adds no transition row
// and no outcome arm to the protocol that declares it, so nothing about the
// state machine's edges changed.
func TestCarriedRequirementAddsNoTransition(t *testing.T) {
	catalogue, err := manifest.Seal(append(stdlib.Providers(), manifest.Provider{
		Identity: "session", Mount: manifest.MountModule,
		Declaration: requirementSessionManifest,
	})...)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	table := sealed.Protocols()
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed target has no protocol handle")
	}
	baseline := sealSessionCatalogue(t).Protocols()
	baseProtocol, baseOK := baseline.ProtocolAt(0)
	if !baseOK {
		t.Fatal("baseline target has no protocol handle")
	}
	if table.TransitionCount(protocol) != baseline.TransitionCount(baseProtocol) {
		t.Fatalf("transition count = %d, want the baseline %d",
			table.TransitionCount(protocol), baseline.TransitionCount(baseProtocol))
	}
	for index := 0; index < table.TransitionCount(protocol); index++ {
		operation, kind, ordinal, from, transitionOK := table.TransitionAt(protocol, index)
		if !transitionOK {
			t.Fatalf("transition %d is not readable", index)
		}
		if operation == sessionOperation(t, sealed, "query") {
			t.Fatalf("the requirement produced a transition on session.query: %d/%d from %d", kind, ordinal, from)
		}
	}
}

// The requirement relation is additive: the same catalogue without a
// requirement row seals with the acquisition, transition, and escape relations
// exactly as before.
func TestSealedTargetStillSealsWithoutRequirementRows(t *testing.T) {
	sealed := sealSessionCatalogue(t)
	table := sealed.Protocols()
	if table.ProtocolCount() != 1 {
		t.Fatalf("sealed protocol count = %d, want the declaration unaffected", table.ProtocolCount())
	}
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed target has no protocol handle")
	}
	if table.ProtocolAcquisitionCount(protocol) != 1 || table.TransitionCount(protocol) != 1 {
		t.Fatalf("acquisitions=%d transitions=%d, want the declared relations intact",
			table.ProtocolAcquisitionCount(protocol), table.TransitionCount(protocol))
	}
}

// A requirement is checked against the declared state machine before it
// reaches the target compiler. The target layer carries the row; it is not the
// authority on which state may be required.
func TestRequirementIsCheckedAtTheManifestBoundaryNotTheTarget(t *testing.T) {
	declaration := func() *manifestwire.Manifest {
		out := sessionManifest()
		out.DefineFunctionSignature("query", signature.Function{
			Type: typ.Func().Param("handle", sessionHandleType()).Build(),
		})
		out.DefineFunctionOperation("query", manifestwire.Operation{
			Requirements: []manifestwire.Requirement{{
				Input:    manifestwire.InputSource{Kind: manifestwire.InputSourceValue},
				Protocol: "session",
				State:    "draining",
			}},
		})
		return out
	}
	_, err := manifest.Seal(append(stdlib.Providers(), manifest.Provider{
		Identity: "session", Mount: manifest.MountModule, Declaration: declaration,
	})...)
	if err == nil {
		t.Fatal("a requirement on an undeclared state crossed the manifest boundary")
	}
	if !strings.Contains(err.Error(), "does not declare required state") {
		t.Fatalf("refusal = %v, want the state machine to be the authority", err)
	}
}
