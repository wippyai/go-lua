package apply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// TestInvokeRetainsOpaqueRows verifies that Apply carries a nonempty Opaque
// proposal lease exactly like Produced, while terminal no-row outcomes remain
// represented by empty leases.
func TestInvokeRetainsOpaqueRows(t *testing.T) {
	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	fixture := newApplyFixture(t, exact, signature.RequirePresent, signature.ProduceOpaque, 1)
	worker := &scriptedWorker{}
	worker.action = func(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
		proposal, proposalOK := presentProposal(t, fixture, buffer, 0, model.AuthenticatedOpaque)
		if !proposalOK || !buffer.Append(proposal) {
			return refusal(fixture)
		}
		return result(outcome.Opaque)
	}
	application, ok := executorFor(fixture, worker).Invoke(
		frameFor(t, fixture, model.Present), fixture.lineage,
		binding.NewOwnerNamedDestination(fixture.relation),
	)
	if !ok || !application.Available() || application.Outcome().Code != outcome.Opaque || application.Len() != 1 {
		t.Fatalf("opaque application available=%v ok=%v outcome=%v rows=%d", application.Available(), ok, application.Outcome().Code, application.Len())
	}
	batch, ok := application.Proposals()
	if !ok || !batch.Available() {
		t.Fatal("opaque proposal batch unavailable")
	}
	proposal, ok := batch.At(0)
	if !ok || !proposal.Available() || !proposal.Presence().Is(model.AuthenticatedOpaque) {
		t.Fatalf("opaque proposal available=%v presence=%v", proposal.Available(), proposal.Presence().Kind())
	}
}
