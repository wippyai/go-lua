package calltarget

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func targetLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/schema/program/call-target-law", []byte(label))
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func TestTargetRequiresAndReturnsAllFiveProofIdentities(t *testing.T) {
	allocation, body := targetLawID(t, "allocation"), targetLawID(t, "body")
	context, function, formal := targetLawID(t, "context"), targetLawID(t, "function"), targetLawID(t, "formal")
	target, ok := NewTarget(allocation, body, context, function, formal)
	if !ok || !target.Available() {
		t.Fatal("complete target refused")
	}
	if target.AllocationID() != allocation || target.BodyID() != body || target.ContextID() != context || target.FunctionID() != function || target.FormalID() != formal {
		t.Fatal("target accessors changed proof geometry")
	}
	if Family().Definition() != programcatalog.CallTarget() {
		t.Fatal("target family drifted from manifest")
	}
}

func TestTargetFailsClosedForIncompleteProof(t *testing.T) {
	parts := [5]identity.ContentID{targetLawID(t, "allocation"), targetLawID(t, "body"), targetLawID(t, "context"), targetLawID(t, "function"), targetLawID(t, "formal")}
	for missing := range parts {
		row := parts
		row[missing] = identity.ContentID{}
		target, ok := NewTarget(row[0], row[1], row[2], row[3], row[4])
		if ok || target.Available() || target.AllocationID().Available() || target.BodyID().Available() || target.ContextID().Available() || target.FunctionID().Available() || target.FormalID().Available() {
			t.Fatalf("incomplete target missing field %d was admitted", missing)
		}
	}
}

func TestViewReadsCanonicalPlaneAndRejectsAmbiguousRelations(t *testing.T) {
	allocation := targetLawID(t, "view-allocation")
	body := targetLawID(t, "view-body")
	first, firstOK := NewTarget(allocation, body, targetLawID(t, "view-context-1"), targetLawID(t, "view-function-1"), targetLawID(t, "view-formal-1"))
	second, secondOK := NewTarget(allocation, targetLawID(t, "view-body-2"), targetLawID(t, "view-context-2"), targetLawID(t, "view-function-2"), targetLawID(t, "view-formal-2"))
	if !firstOK || !secondOK {
		t.Fatal("complete view fixtures refused")
	}
	catalog := targetLawID(t, "view-catalog")
	builder := snapshot.NewFrozen(catalog, identity.StoreID(1))
	if !Family().Put(&builder, []Target{first, second}, catalog) {
		t.Fatal("publish target plane")
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal target plane: %v", err)
	}
	state, stateOK := programstate.New(frozen, catalog)
	view, viewOK := NewView(state)
	if !stateOK || !viewOK {
		t.Fatal("open target view")
	}
	if count, published := view.Count(); !published || count != 2 {
		t.Fatalf("count = %d/%v", count, published)
	}
	if got, held := view.ForBody(body); !held || got != first {
		t.Fatal("unique body relation not resolved")
	}
	if _, held := view.ForAllocation(allocation); held {
		t.Fatal("ambiguous allocation relation resolved")
	}
	if _, ok := NewView(programstate.State{}); ok {
		t.Fatal("zero state opened a target view")
	}
}
