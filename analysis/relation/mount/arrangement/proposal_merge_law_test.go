package arrangement_test

import (
	"testing"

	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
)

func TestMergeBindingSealsExistingApplyAuthority(t *testing.T) {
	fixture := testfixture.New(t, 0xD5)
	node, ok := fixture.ProposalMergeNode()
	if !ok {
		t.Fatal("proposal Merge node")
	}
	binding, ok := node.Merge()
	children := node.Children()
	if !ok || len(children) != 2 {
		t.Fatal("proposal Merge binding")
	}
	applyBinding, ok := children[0].Apply()
	if !ok {
		t.Fatal("proposal Merge first child is not the sealed Apply")
	}
	operations := binding.ProposalOperations()
	if len(operations) != 1 || operations[0] != applyBinding.Operation() || !binding.AcceptsProposal(children[0].Digest(), applyBinding.Operation()) {
		t.Fatalf("proposal authorities=%v, want exact Apply operation %v", operations, applyBinding.Operation())
	}

	ordinary, ok := fixture.MergeNode()
	if !ok {
		t.Fatal("ordinary Merge node")
	}
	ordinaryBinding, ok := ordinary.Merge()
	if !ok || len(ordinaryBinding.ProposalOperations()) != 0 {
		t.Fatal("ordinary tuple Merge acquired proposal authority")
	}
}
