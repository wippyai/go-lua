package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

func TestResourceUnreleasedConsumerDistinguishesTypedStateMutations(t *testing.T) {
	identity := shapefact.ScalarTextValue(shapefact.ScalarResource, "op-00000001")
	open, acquired := typestate.AcquirePublication(
		typestate.Resource{ID: typestate.ResourceID(string(identity)), Protocol: typestate.ProtocolConnection},
		typestate.StateOpen,
		typestate.Obligation{Final: typestate.StateClosed},
	)
	if !acquired {
		t.Fatal("construct open resource publication")
	}
	openFact, encoded := lifecyclePublicationFact(open, "op-00000001")
	if !encoded {
		t.Fatal("encode open resource publication")
	}
	diagnostics := resourceUnreleasedDiagnostics(front.Compilation{}, equation.OutputClosure{Values: []equation.Fact{openFact}})
	if len(diagnostics) != 1 {
		t.Fatalf("open resource diagnostics = %d, want 1", len(diagnostics))
	}

	closed, transitioned := open.Transition(typestate.StateClosed)
	if !transitioned {
		t.Fatal("transition resource to closed")
	}
	closedFact, encoded := lifecyclePublicationFact(closed, "op-00000002")
	if !encoded {
		t.Fatal("encode closed resource publication")
	}
	if got := resourceUnreleasedDiagnostics(front.Compilation{}, equation.OutputClosure{Values: []equation.Fact{openFact, closedFact}}); len(got) != 0 {
		t.Fatalf("closed-state mutation retained unreleased diagnostic: %#v", got)
	}

	escaped, changed := open.Escape()
	if !changed {
		t.Fatal("escape resource publication")
	}
	escapedFact, encoded := lifecyclePublicationFact(escaped, "op-00000002")
	if !encoded {
		t.Fatal("encode escaped resource publication")
	}
	if got := resourceUnreleasedDiagnostics(front.Compilation{}, equation.OutputClosure{Values: []equation.Fact{openFact, escapedFact}}); len(got) != 0 {
		t.Fatalf("escaped-locality mutation retained unreleased diagnostic: %#v", got)
	}

	raw := openFact
	raw.Value = []byte(typestate.StateOpen)
	if got := resourceUnreleasedDiagnostics(front.Compilation{}, equation.OutputClosure{Values: []equation.Fact{raw}}); len(got) != 0 {
		t.Fatalf("raw state payload bypassed typed consumer: %#v", got)
	}
}
