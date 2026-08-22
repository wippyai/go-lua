package manifesttarget_test

import (
	"strings"
	"testing"

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

// A requirement the manifest boundary admits has no relation in the sealed
// protocol vocabulary to be carried into. Sealing refuses the catalogue and
// names the missing relation, so a provider's stated constraint is never
// dropped on the way to a target that would then answer nothing about it.
//
// This law is the visible edge of the gap: it turns green into a compile of the
// requirement relation, and until then it holds the drop closed.
func TestSealedTargetRefusesRequirementItCannotCarry(t *testing.T) {
	catalogue, err := manifest.Seal(append(stdlib.Providers(), manifest.Provider{
		Identity: "session", Mount: manifest.MountModule,
		Declaration: requirementSessionManifest,
	})...)
	if err != nil {
		t.Fatalf("the manifest boundary refused a well-formed requirement: %v", err)
	}
	sealed, err := manifesttarget.SealCatalogue(catalogue)
	if err == nil {
		t.Fatalf("a declared requirement sealed into a target that carries no requirement relation: %v", sealed != nil)
	}
	for _, want := range []string{"typestate requirement", "refused rather than dropped"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal = %v, want it to name %q", err, want)
		}
	}
}

// The refusal is scoped to the requirement row alone: the same catalogue
// without it seals, so nothing about acquisition, transition, or escape
// changed.
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

// A requirement is still checked against the declared state machine before it
// reaches the target compiler, so the refusal above is the only thing the
// target layer adds - not the validation.
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
