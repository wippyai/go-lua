package boundary

import "testing"

func TestBoundaryDraftFinalizationConsumesSharedState(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	p := boundaryProgram(t)
	project := boundaryInitialOperationProject(t, p, contract)
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	copyDraft := *draft
	if _, err := draft.Finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := copyDraft.Finalize(); err == nil {
		t.Fatal("copied Draft finalized twice")
	}
	if copyDraft.state == nil || !copyDraft.state.consumed {
		t.Fatal("copied Draft did not share finalization state")
	}
}
