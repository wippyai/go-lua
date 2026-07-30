package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestPreimageFormalIdentitySupportRejectsUnboundForeignOwner(t *testing.T) {
	targetOwner := lexicalidentity.StableLexicalBodyID{1}
	callerOwner := lexicalidentity.StableLexicalBodyID{2}
	target := identity.FormalTerm(identity.NewFormalVarRoot(formal.NewRoot(targetOwner, 1, formal.Input)))
	callerInput := identity.FormalTerm(identity.NewFormalVarRoot(formal.NewRoot(callerOwner, 7, formal.Input)))
	unboundCaller := identity.FormalTerm(identity.NewFormalVarRoot(formal.NewRoot(callerOwner, 8, formal.Input)))
	image, valid := state.NewCoordinateIdentityTermImage([]state.CoordinateIdentityTermBinding{{Source: target, Images: []identity.Term{callerInput}}})
	if !valid {
		t.Fatal("identity image")
	}
	if got, err := preimageFormalIdentitySupport(formalIdentitySupport{unboundCaller}, image, &relationProgramBody{body: targetOwner}); err == nil || len(got) != 0 {
		t.Fatalf("preimage = %#v, %v", got, err)
	}
}

func TestFormalIdentityEnvironmentFoldUsesStableContributionOrder(t *testing.T) {
	owner := lexicalidentity.StableLexicalBodyID{1}
	term := func(ordinal uint64) identity.Term {
		return identity.FormalTerm(identity.NewFormalVarRoot(formal.NewRoot(owner, ordinal, formal.Input)))
	}
	slot := statekey.SymbolValue(symbol.ID(1))
	left := formalIdentityEnvironment{values: map[statekey.Value]formalIdentitySupport{slot: {term(1)}}}
	right := formalIdentityEnvironment{values: map[statekey.Value]formalIdentitySupport{slot: {term(2)}}}
	closure := &formalCoordinateDependencyClosure{}

	forward := newFormalIdentityEnvironmentFold(closure, 0, 3)
	if _, err := forward.set(1, left); err != nil {
		t.Fatal(err)
	}
	if _, err := forward.set(2, right); err != nil {
		t.Fatal(err)
	}
	reverse := newFormalIdentityEnvironmentFold(closure, 0, 3)
	if _, err := reverse.set(2, right); err != nil {
		t.Fatal(err)
	}
	if _, err := reverse.set(1, left); err != nil {
		t.Fatal(err)
	}
	if !closure.formalIdentityEnvironmentEqual(forward.root(), reverse.root()) {
		t.Fatalf("stable contribution order depends on update order: %#v / %#v", forward.root(), reverse.root())
	}

	if _, err := forward.set(1, formalIdentityEnvironment{}); err != nil {
		t.Fatal(err)
	}
	if !closure.formalIdentityEnvironmentEqual(forward.root(), right) {
		t.Fatalf("replaced contribution retained stale identity support: %#v", forward.root())
	}
}
