package manifesttarget_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	protocolvalue "github.com/wippyai/go-lua/analysis/program/target/protocol"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// A manifest that declares a typestate protocol, acquires it in a result slot,
// and moves it with lifecycle labels answers the sealed protocol query surface
// with the same relation. The declaration is the only lifecycle authority: no
// member name, return type, or provider identity contributes a row.
func TestManifestTypestateDeclarationSealsIntoProtocolTable(t *testing.T) {
	sealed := sealSessionCatalogue(t)
	table := sealed.Protocols()
	if table.ProtocolCount() != 1 {
		t.Fatalf("sealed protocol count = %d, want 1", table.ProtocolCount())
	}
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed target has no protocol handle")
	}

	if names := sessionStateNames(&table, protocol); len(names) != 2 || names["open"] || !names["closed"] {
		t.Fatalf("protocol states = %v, want open (non-final) and closed (final)", names)
	}

	openOperation := sessionOperation(t, sealed, "open")
	closeOperation := sessionOperation(t, sealed, "close")
	leakOperation := sessionOperation(t, sealed, "leak")

	if table.ProtocolAcquisitionCount(protocol) != 1 {
		t.Fatalf("acquisition count = %d, want 1", table.ProtocolAcquisitionCount(protocol))
	}
	operation, outcome, result, state, found := table.ProtocolAcquisitionAt(protocol, 0)
	if !found {
		t.Fatal("sealed protocol has no acquisition row")
	}
	if operation != openOperation || outcome != 0 || result != 0 {
		t.Fatalf("acquisition = op %d outcome %d result %d, want op %d outcome 0 result 0", operation, outcome, result, openOperation)
	}
	if name, nameOK := table.StateName(protocol, state); !nameOK || name != "open" {
		t.Fatalf("acquired state = %q/%v, want open", name, nameOK)
	}

	if table.TransitionCount(protocol) != 1 {
		t.Fatalf("transition count = %d, want 1", table.TransitionCount(protocol))
	}
	transitionOperation, inputKind, inputOrdinal, from, transitionOK := table.TransitionAt(protocol, 0)
	if !transitionOK {
		t.Fatal("sealed protocol has no transition row")
	}
	if transitionOperation != closeOperation || inputKind != vocabulary.InputSourceValueFormal || inputOrdinal != 0 {
		t.Fatalf("transition subject = op %d %d/%d, want op %d value formal 0", transitionOperation, inputKind, inputOrdinal, closeOperation)
	}
	if name, nameOK := table.StateName(protocol, from); !nameOK || name != "open" {
		t.Fatalf("transition source state = %q/%v, want open", name, nameOK)
	}
	if table.TransitionOutcomeCount(protocol, 0) != 1 {
		t.Fatalf("transition outcome count = %d, want 1", table.TransitionOutcomeCount(protocol, 0))
	}
	transitionOutcome, to, outcomeOK := table.TransitionOutcomeAt(protocol, 0, 0)
	if !outcomeOK || transitionOutcome != 0 {
		t.Fatalf("transition outcome = %d/%v, want the normal arm", transitionOutcome, outcomeOK)
	}
	if name, nameOK := table.StateName(protocol, to); !nameOK || name != "closed" {
		t.Fatalf("transition target state = %q/%v, want closed", name, nameOK)
	}

	// The escape relation always carries the derived opaque row after the
	// authored ones, so one declared escape answers a count of two.
	if table.EscapeCount(protocol) != 2 {
		t.Fatalf("escape count = %d, want the declared escape and the derived opaque row", table.EscapeCount(protocol))
	}
	escapeOperation, escapeKind, escapeOrdinal, escapeOK := table.EscapeAt(protocol, 0)
	if !escapeOK || escapeOperation != leakOperation || escapeKind != vocabulary.InputSourceValueFormal || escapeOrdinal != 0 {
		t.Fatalf("escape = op %d %d/%d/%v, want op %d value formal 0", escapeOperation, escapeKind, escapeOrdinal, escapeOK, leakOperation)
	}
}

// A lifecycle label that names a protocol the manifest never declared is a
// refusal, not a dropped row.
func TestManifestUndeclaredLifecycleProtocolIsRefused(t *testing.T) {
	provider := manifest.Provider{
		Identity: "session", Mount: manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := sessionManifest()
			declaration.DefineFunctionSignature("abandon", signature.Function{
				Type: typ.Func().Param("handle", sessionHandleType()).Build(),
				Effect: effect.Empty.With(lifecycle.Escape{
					Target: effect.ParamRef{Index: 0}, Protocol: "unknown",
				}),
			})
			return declaration
		},
	}
	if _, err := manifest.Seal(append(stdlib.Providers(), provider)...); err == nil {
		t.Fatal("a lifecycle label naming an undeclared protocol sealed without refusal")
	}
}

// The sealed vocabulary acquires a fixed result slot. A manifest that names a
// parameter as its acquisition subject states a relation the sealed contract
// cannot carry, so seal refuses it instead of dropping the declaration.
func TestManifestParameterAcquisitionIsRefused(t *testing.T) {
	provider := manifest.Provider{
		Identity: "session", Mount: manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := sessionManifest()
			declaration.DefineFunctionSignature("adopt", signature.Function{
				Type: typ.Func().Param("handle", sessionHandleType()).Build(),
				Effect: effect.Empty.With(lifecycle.Acquire{
					Target: effect.ParamRef{Index: 0}, Protocol: "session", State: "open",
				}),
			})
			return declaration
		},
	}
	catalogue, err := manifest.Seal(append(stdlib.Providers(), provider)...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifesttarget.SealCatalogue(catalogue); err == nil {
		t.Fatal("a parameter-targeted acquisition sealed instead of being refused")
	}
}

// A protocol whose acquiring member is never declared has no identity in the
// sealed contract, which knows a protocol only by the result slots that create
// it. Seal refuses rather than publishing a machine no operation can enter.
func TestManifestProtocolWithoutAcquisitionIsRefused(t *testing.T) {
	provider := manifest.Provider{
		Identity: "session", Mount: manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := sessionManifest()
			delete(declaration.FunctionOperations, "open")
			return declaration
		},
	}
	catalogue, err := manifest.Seal(append(stdlib.Providers(), provider)...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifesttarget.SealCatalogue(catalogue); err == nil {
		t.Fatal("a protocol with no acquisition sealed instead of being refused")
	}
}

func sealSessionCatalogue(t *testing.T) *contract.Contract {
	t.Helper()
	provider := manifest.Provider{
		Identity: "session", Mount: manifest.MountModule,
		Declaration: sessionManifest,
	}
	catalogue, err := manifest.Seal(append(stdlib.Providers(), provider)...)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func sessionHandleType() typ.Type { return typ.NewInterface("session.Handle", nil) }

func sessionManifest() *manifestwire.Manifest {
	handle := sessionHandleType()
	declaration := manifestwire.New("session")
	declaration.DefineType("Handle", handle)
	if err := declaration.DefineTypestateProtocol(typestate.Definition{
		Protocol:    "session",
		States:      []typestate.State{"open", "closed"},
		FinalStates: []typestate.State{"closed"},
		Transitions: []typestate.TransitionDecl{{From: "open", To: "closed"}},
	}); err != nil {
		panic(err)
	}
	declaration.DefineFunctionSignature("open", signature.Function{
		Type: typ.Func().Returns(handle).Build(),
	})
	declaration.DefineFunctionOperation("open", manifestwire.Operation{
		Acquisitions: []manifestwire.Acquisition{{Outcome: 0, Result: 0, Protocol: "session", State: "open"}},
	})
	declaration.DefineFunctionSignature("close", signature.Function{
		Type: typ.Func().Param("handle", handle).Build(),
		Effect: effect.Empty.With(lifecycle.Transition{
			Target: effect.ParamRef{Index: 0}, Protocol: "session", From: "open", To: "closed",
		}),
	})
	declaration.DefineFunctionSignature("leak", signature.Function{
		Type: typ.Func().Param("handle", handle).Build(),
		Effect: effect.Empty.With(lifecycle.Escape{
			Target: effect.ParamRef{Index: 0}, Protocol: "session",
		}),
	})
	return declaration
}

func sessionOperation(t *testing.T, sealed *contract.Contract, member string) vocabulary.Operation {
	t.Helper()
	operation, ok := sealed.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule, Owner: []string{"session"}, Member: []string{member},
	})
	if !ok {
		t.Fatalf("session.%s is not a sealed operation", member)
	}
	return operation
}

// sessionStateNames reads back the nominal state names with their finality.
func sessionStateNames(table *protocolvalue.Table, protocol vocabulary.Protocol) map[string]bool {
	out := make(map[string]bool, table.StateCount(protocol))
	for index := 0; index < table.StateCount(protocol); index++ {
		state, ok := table.StateAt(protocol, index)
		if !ok {
			continue
		}
		name, nameOK := table.StateName(protocol, state)
		final, finalOK := table.StateFinal(protocol, state)
		if !nameOK || !finalOK {
			continue
		}
		out[name] = final
	}
	return out
}
