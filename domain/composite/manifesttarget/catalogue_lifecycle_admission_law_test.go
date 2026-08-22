package manifesttarget_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// The declaration-admission laws of the Target seal.
//
// A signature effect row is a declaration surface: the manifest wire admits a
// label, and the seal decides what judgment carries it. Two label populations
// meet here - the ownership labels, which become formal ownership rows, and
// every other declared capability, which makes the operation carry an
// invocation effect. What must never happen is a third: a label the seal
// recognises as neither, admitted and turned into a generic effect row that no
// declaration asked for. The capability catalog is the authority that closes
// the population, and the seal consults it.

// unclassifiedLabel is a label whose capability the audited catalog does not
// declare. It exists only to prove the seal's refusal; no manifest may carry
// one, which is exactly what the law states.
type unclassifiedLabel struct{}

func (unclassifiedLabel) CapabilityID() string { return "test.unclassified" }
func (unclassifiedLabel) String() string       { return "test.unclassified" }
func (unclassifiedLabel) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(unclassifiedLabel)
	return ok
}

// A label the capability catalog does not declare has no classification, so the
// seal could not know whether it is a formal ownership contract or an
// invocation effect: the formal walk translates the ownership and lifecycle
// families and the invocation walk answers "effectful" for everything else, so
// an unclassified label would silently become a generic effect row.
//
// It cannot reach either walk. The manifest boundary closes the label
// population against the audited catalog and refuses an unaudited capability by
// name, so the seal's two walks together see exactly the declared families.
// This law pins that boundary from the consumer's side: the refusal is what
// makes the seal's walks exhaustive rather than defaulted.
func TestUnauditedEffectLabelNeverReachesTheSeal(t *testing.T) {
	if _, declared := capability.Lookup("test.unclassified"); declared {
		t.Fatal("the probe capability is declared; it cannot demonstrate the refusal")
	}
	handle := typ.NewInterface("probe.Handle", nil)
	provider := manifest.Provider{
		Identity: "probe", Mount: manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("probe")
			declaration.DefineType("Handle", handle)
			declaration.DefineFunctionSignature("touch", signature.Function{
				Type:   typ.Func().Param("handle", handle).Build(),
				Effect: effect.Empty.With(unclassifiedLabel{}),
			})
			return declaration
		},
	}
	_, err := manifest.Seal(append(stdlib.Providers(), provider)...)
	if err == nil {
		t.Fatal("a label outside the audited capability catalog was admitted")
	}
	if !strings.Contains(err.Error(), "test.unclassified") {
		t.Fatalf("refusal %q does not name the capability it refused", err)
	}
}

// Every capability the audited catalog declares is classified by exactly one of
// the seal's two walks. The formal walk owns the ownership and lifecycle
// families; every other declared family is an invocation effect. A family in
// neither would be admitted and carried by nothing, which is the silence this
// law forbids: adding a family to the catalog without deciding which walk owns
// it fails here rather than at a fixture months later.
func TestEveryAuditedCapabilityFamilyIsClassifiedByTheSeal(t *testing.T) {
	formal := map[string]bool{"ownership": true, "lifecycle": true}
	invocation := map[string]bool{
		"returns": true, "postcondition": true, "iteration": true,
		"dispatch": true, "mutation": true, "control": true,
	}
	for _, descriptor := range capability.All() {
		if formal[descriptor.Family] == invocation[descriptor.Family] {
			t.Errorf("capability %q belongs to family %q, which the Target seal classifies as neither a formal ownership contract nor an invocation effect", descriptor.ID, descriptor.Family)
		}
	}
}

// The refusal is exact. Every capability the catalog declares still seals, so
// the law closes the population without narrowing it: the whole canonical
// provider set carries labels from every declared family and seals unchanged.
func TestDeclaredCapabilitiesStillSeal(t *testing.T) {
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifesttarget.SealCatalogue(catalogue); err != nil {
		t.Fatalf("the canonical provider set no longer seals: %v", err)
	}
}

// A lifecycle label naming a protocol no catalogue declares is refused before
// the Target seal ever sees it, and the refusal names the protocol. This pins
// the boundary: the protocol pass is the only consumer of a lifecycle
// declaration, and it runs only when a state machine is declared, so a
// declaration that could reach a pass that never runs must already be gone.
func TestLifecycleDeclarationWithoutStateMachineIsRefusedBeforeSeal(t *testing.T) {
	cases := []struct {
		name  string
		label effect.Label
	}{
		{"transition", lifecycle.Transition{Target: effect.ParamRef{Index: 0}, Protocol: "session", From: "open", To: "closed"}},
		{"escape", lifecycle.Escape{Target: effect.ParamRef{Index: 0}, Protocol: "session"}},
		{"acquire", lifecycle.Acquire{Target: effect.ParamRef{Index: 0}, Protocol: "session", State: "open"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handle := typ.NewInterface("orphan.Handle", nil)
			provider := manifest.Provider{
				Identity: "orphan", Mount: manifest.MountModule,
				Declaration: func() *manifestwire.Manifest {
					declaration := manifestwire.New("orphan")
					declaration.DefineType("Handle", handle)
					declaration.DefineFunctionSignature("close", signature.Function{
						Type:   typ.Func().Param("handle", handle).Build(),
						Effect: effect.Empty.With(testCase.label),
					})
					return declaration
				},
			}
			_, err := manifest.Seal(append(stdlib.Providers(), provider)...)
			if err == nil {
				t.Fatal("a lifecycle declaration with no declared state machine was admitted")
			}
			if !strings.Contains(err.Error(), "session") {
				t.Fatalf("refusal %q does not name the protocol the declaration states", err)
			}
		})
	}
}

// A catalogue that declares no lifecycle relation seals with no protocol table,
// which is the ordinary case for every provider that owns no state machine.
func TestCatalogueWithoutLifecycleDeclarationsSealsWithoutProtocols(t *testing.T) {
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	if table := sealed.Protocols(); table.ProtocolCount() != 0 {
		t.Fatalf("sealed protocol count = %d, want none", table.ProtocolCount())
	}
}
