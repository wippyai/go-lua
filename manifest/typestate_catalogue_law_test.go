package manifest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/typestate"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

func typestateProvider(t *testing.T, identity, path string, definition typestate.Definition) Provider {
	t.Helper()
	return Provider{
		Identity: identity, Mount: MountModule,
		Declaration: func() *moduleio.Manifest {
			declaration := moduleio.New(path)
			if err := declaration.DefineTypestateProtocol(definition); err != nil {
				t.Fatal(err)
			}
			return declaration
		},
	}
}

func connectionMachine() typestate.Definition {
	return typestate.Definition{
		Protocol:    "connection",
		States:      []typestate.State{"open", "closed"},
		FinalStates: []typestate.State{"closed"},
		Transitions: []typestate.TransitionDecl{{From: "open", To: "closed"}},
	}
}

// A protocol name is catalogue-wide vocabulary. Two providers may restate the
// identical machine, and the sealed catalogue answers with one definition.
func TestCatalogueMergesIdenticalTypestateRedeclaration(t *testing.T) {
	catalogue, err := Seal(
		typestateProvider(t, "left", "left", connectionMachine()),
		typestateProvider(t, "right", "right", connectionMachine()),
	)
	if err != nil {
		t.Fatal(err)
	}
	protocols := catalogue.TypestateProtocols()
	if len(protocols) != 1 {
		t.Fatalf("catalogue protocols = %+v, want the one connection machine", protocols)
	}
	declared, ok := protocols["connection"]
	if !ok {
		t.Fatal("the catalogue holds no connection protocol")
	}
	if len(declared.States) != 2 || !declared.IsFinal("closed") || !declared.AllowsTransition("open", "closed") {
		t.Fatalf("connection machine = %+v, want two states with the declared closing edge", declared)
	}
}

// Two providers that disagree about one protocol name state two machines under
// one identity. The catalogue refuses instead of letting declaration order pick
// a winner.
func TestCatalogueRefusesDisagreeingTypestateRedeclaration(t *testing.T) {
	divergent := connectionMachine()
	divergent.States = append(divergent.States, "draining")
	_, err := Seal(
		typestateProvider(t, "left", "left", connectionMachine()),
		typestateProvider(t, "right", "right", divergent),
	)
	if err == nil {
		t.Fatal("a disagreeing protocol redeclaration sealed without refusal")
	}
	if !strings.Contains(err.Error(), "redeclares typestate protocol") {
		t.Fatalf("refusal = %v, want it to name the conflicting redeclaration", err)
	}
}
