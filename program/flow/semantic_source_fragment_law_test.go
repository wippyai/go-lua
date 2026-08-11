package flow

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestSemanticSourceFragmentRejectsUnavailableView(t *testing.T) {
	if publications, err := SemanticSourceFragment(View{}); err == nil || publications != nil {
		t.Fatalf("SemanticSourceFragment(zero) = %#v, %v; want nil and error", publications, err)
	}
}

func TestSemanticSourceFragmentCanonicalOrderAndZeroRows(t *testing.T) {
	sourceFinalizer, staticFinalizer, moduleFinalizer, draft := emptyAssemblyOwners(t, "flow-semantic-source-empty.lua")
	assembly, err := Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, draft, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		t.Fatal(err)
	}
	_, flowComponent, _, _, err := assembly.Take()
	if err != nil {
		t.Fatal(err)
	}
	publications, err := SemanticSourceFragment(flowComponent.View())
	if err != nil {
		t.Fatalf("SemanticSourceFragment(empty): %v", err)
	}
	want := [...]struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
	}{
		{semanticsource.OriginProgramFlowValues, 0},
		{semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence},
		{semanticsource.OriginProgramFlowLens, 0},
		{semanticsource.OriginProgramFlowStorage, 0},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageBind},
		{semanticsource.OriginProgramFlowConstructors, 0},
		{semanticsource.OriginProgramFlowConstructors, semanticsource.FacetProgramFlowConstructorField},
		{semanticsource.OriginProgramFlowOperators, 0},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet},
		{semanticsource.OriginProgramFlowFunction, 0},
		{semanticsource.OriginProgramFlowFunction, semanticsource.FacetProgramFlowFunctionCapture},
		{semanticsource.OriginProgramFlowCall, 0},
		{semanticsource.OriginProgramFlowCall, semanticsource.FacetProgramFlowDirectCallBinding},
		{semanticsource.OriginProgramFlowControl, 0},
		{semanticsource.OriginProgramFlowControl, semanticsource.FacetProgramFlowGenericFor},
		{semanticsource.OriginProgramFlowClaim, 0},
		{semanticsource.OriginProgramFlowTypeValue, 0},
		{semanticsource.OriginProgramFlowOutcome, 0},
		{semanticsource.OriginProgramFlowTransfer, 0},
	}
	if len(publications) != len(want) {
		t.Fatalf("publication count = %d, want %d", len(publications), len(want))
	}
	for index, publication := range publications {
		token := publication.Definition().Token()
		if token.Origin() != want[index].origin || token.Facet() != want[index].facet {
			t.Fatalf("publication[%d] token = (%d,%d), want (%d,%d)", index, token.Origin(), token.Facet(), want[index].origin, want[index].facet)
		}
		// Empty authored Flow has no rows. Outcome and Transfer are derived
		// whole-Flow projections and retain terminal rows for the empty body.
		if index < 31 && publication.Count() != 0 {
			t.Fatalf("publication[%d] count = %d, want zero", index, publication.Count())
		}
		if index >= 31 && publication.Count() == 0 {
			t.Fatalf("empty Flow lost derived publication %d", index)
		}
	}
}
