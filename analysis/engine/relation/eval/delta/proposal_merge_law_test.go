package delta

import (
	"testing"

	physicalapply "github.com/wippyai/go-lua/analysis/engine/relation/apply"
	inputop "github.com/wippyai/go-lua/analysis/engine/relation/operator/input"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	semanticbinding "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestLaterProposalMergePreservesAuthenticatedEmptyApplyResults(t *testing.T) {
	fixture := testfixture.New(t, 0xD3)
	mergeNode, ok := fixture.ProposalMergeNode()
	if !ok {
		t.Fatal("proposal Merge node")
	}
	mergeBinding, ok := mergeNode.Merge()
	children := mergeNode.Children()
	if !ok || len(children) != 2 {
		t.Fatal("proposal Merge binding")
	}
	applyNode := children[0]
	applyBinding, ok := applyNode.Apply()
	if !ok {
		t.Fatal("proposal Apply binding")
	}

	inputs := make([][]tuple.Batch, len(applyNode.Children()))
	for index, child := range applyNode.Children() {
		input, inputOK := child.Input()
		if !inputOK {
			t.Fatal("proposal Apply input")
		}
		reader, readerOK := read.Bind(fixture.LeftRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
		if !readerOK {
			t.Fatal("proposal Apply reader")
		}
		inputs[index], ok = inputop.Execute(input, fixture.Mounted(), reader)
		if !ok {
			t.Fatal("proposal Apply input execution")
		}
	}
	deliveries := applyBinding.Deliveries()
	witnesses := make([]semanticbinding.DenominatorWitness, len(deliveries))
	for index, delivery := range deliveries {
		input := delivery.Requirement().Input()
		witnesses[index], ok = fixture.Mounted().Denominator(input.Denominator)
		if !ok {
			t.Fatal("proposal Apply denominator")
		}
	}
	results, ok := physicalapply.Execute(applyBinding, fixture.Mounted(), inputs, fixture.Geometry(), witness.Scope{}, witnesses)
	if !ok || !results.Available() || results.Len() != 0 {
		t.Fatal("authenticated empty Apply results")
	}
	current, ok := applyValue(applyNode.Digest(), []physicalapply.Results{results})
	if !ok {
		t.Fatal("proposal Apply path value")
	}
	merged, ok := proposalMergeValue(fixture.Mounted(), mergeBinding, current)
	if !ok || !merged.available(fixture.Mounted()) || len(merged.batches) != 0 || len(merged.applications) != 1 || merged.applications[0].Len() != 0 {
		t.Fatal("Later proposal Merge changed authenticated empty results")
	}

	carried, ok := relationValue(children[1].Digest(), algebra.KindColumnProject, []tuple.Batch{})
	if !ok {
		t.Fatal("empty carried path value")
	}
	noOp, ok := proposalMergeValue(fixture.Mounted(), mergeBinding, carried)
	if !ok || !noOp.available(fixture.Mounted()) || len(noOp.batches) != 0 || len(noOp.applications) != 0 {
		t.Fatal("carried proposal-Merge occurrence was not an authenticated no-op")
	}
}
