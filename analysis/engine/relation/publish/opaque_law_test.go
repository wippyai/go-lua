package publish_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// TestPublishRetainsOpaqueRows verifies that an authenticated opaque outcome
// is a write-bearing semantic result. Its rows must cross the ordinary atomic
// publication door with their opaque presence intact; treating Opaque as a
// terminal no-write status would silently erase the fact.
func TestPublishRetainsOpaqueRows(t *testing.T) {
	value := newOpaqueFixture(t)
	opaque, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		t.Fatal("opaque presence")
	}
	keyProposal, ok := binding.NewProposal(value.keyToken, value.keyValue, opaque)
	if !ok {
		t.Fatal("opaque key proposal")
	}
	valueProposal, ok := binding.NewProposal(value.token, value.value, opaque)
	if !ok {
		t.Fatal("opaque value proposal")
	}
	value.worker.result = outcome.Result{Code: outcome.Opaque}
	value.worker.proposals = []binding.Proposal{keyProposal, valueProposal}
	value.worker.emitOpaque = true
	application := value.application(t, outcome.Opaque)
	if !application.Available() || application.Len() != 2 {
		t.Fatalf("opaque application available=%v rows=%d", application.Available(), application.Len())
	}
	settlement := value.door.Publish(value.aggregate, value.readScratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() || settlement.Outcome().Code != outcome.Opaque {
		t.Fatalf("opaque publication available=%v changed=%v outcome=%v", settlement.Available(), settlement.Changed(), settlement.Outcome().Code)
	}
	if !settlement.Next().SuccessorOf(value.aggregate) {
		t.Fatal("opaque publication did not advance the aggregate exactly once")
	}
	if delta, ok := settlement.Delta(); !ok || len(delta.SemanticColumnIDs()) != 2 {
		t.Fatalf("opaque publication semantic columns: delta=%v ok=%v", delta, ok)
	}
}
