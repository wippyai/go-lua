package step

import (
	"testing"

	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

func TestProposalMergePreservesAuthenticatedEmptyApplyResults(t *testing.T) {
	fixture := testfixture.New(t, 0xD3)
	session, ok := New(fixture.Mounted(), fixture.LeftRoot(), fixture.Geometry())
	if !ok {
		t.Fatal("proposal Merge evaluator session")
	}
	node, ok := fixture.ProposalMergeNode()
	if !ok || node.Kind() != algebra.KindMerge {
		t.Fatal("proposal Merge node")
	}
	value, ok := session.executeNode(node)
	if !ok || !value.available() || value.kind != algebra.KindMerge || len(value.batches) != 0 || len(value.applications) != 1 {
		t.Fatalf("proposal Merge result ok=%t batches=%d applications=%d", ok, len(value.batches), len(value.applications))
	}
	if !value.applications[0].Available() || value.applications[0].Len() != 0 {
		t.Fatal("proposal Merge collapsed authenticated empty Apply results")
	}
}
