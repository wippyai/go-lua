package subtree

import (
	"testing"

	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
)

// The shared relation fixture intentionally has no correlated Apply child
// whose sealed root contains Join. Join redemption is therefore covered by
// the parent Placement end-to-end gate; these laws cover every generic leaf
// source and keep the evaluator package independent of that domain fixture.

// TestEvaluateDriverRedeemsOneExactPopulationCofiber exercises the direct
// PopulationDriver source. The caller supplies the owner denominator member
// and scope; the evaluator must return exactly that one row, not every reader
// cofiber or the whole population witness.
func TestEvaluateDriverRedeemsOneExactPopulationCofiber(t *testing.T) {
	fixture := testfixture.New(t, 0xF4)
	replay, child := evaluatorChild(t, fixture, fixture.MixedPopulationApplyNode, 0)
	scope, _ := fixture.OverlapScopes()
	session, ok := New(fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch())
	if !ok || !session.Available() {
		t.Fatal("subtree session")
	}
	result, ok := session.Evaluate(replay, child, fixture.RowsLeft()[0], scope)
	if !ok || !result.Available() || !result.PopulationScope().Same(scope) {
		t.Fatalf("driver result=(%v,%v)", ok, result.Available())
	}
	batches := result.Batches()
	if len(batches) != 1 || batches[0].Len() != 1 {
		t.Fatalf("driver batches=%d len=%d", len(batches), func() int {
			if len(batches) == 1 {
				return batches[0].Len()
			}
			return -1
		}())
	}
	value, valueOK := batches[0].At(0)
	row, rowOK := value.SourceAt(0)
	if !valueOK || !rowOK || row != fixture.RowsLeft()[0] {
		t.Fatalf("driver source row=(%v,%v), want=%v", valueOK, rowOK, fixture.RowsLeft()[0])
	}

	_, foreignScope := fixture.OverlapScopes()
	if rejected, accepted := session.Evaluate(replay, child, fixture.RowsLeft()[0], foreignScope); accepted || rejected.Available() {
		t.Fatal("driver accepted a foreign population scope")
	}
}

// TestEvaluatePartitionPreservesEmptyPosting checks both sides of a
// q-specific posting: a populated child and an authenticated empty child are
// distinct successful extents, and neither falls back to the global witness.
func TestEvaluatePartitionPreservesEmptyPosting(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	replay, child := evaluatorChild(t, fixture, fixture.CorrelatedApplyNode, 0)
	scope, _ := fixture.OverlapScopes()
	session, ok := New(fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("subtree session")
	}
	first, firstOK := session.Evaluate(replay, child, fixture.RowsLeft()[0], scope)
	second, secondOK := session.Evaluate(replay, child, fixture.RowsLeft()[1], scope)
	if !firstOK || !first.Available() || !secondOK || !second.Available() {
		t.Fatalf("partition results=(%v,%v)/(%v,%v)", firstOK, first.Available(), secondOK, second.Available())
	}
	firstBatches, secondBatches := first.Batches(), second.Batches()
	if len(firstBatches) != 1 || len(secondBatches) != 1 {
		t.Fatalf("partition batch counts=%d/%d, want=1/1", len(firstBatches), len(secondBatches))
	}
	if firstBatches[0].Len() != 1 || secondBatches[0].Len() != 0 {
		t.Fatalf("partition lengths=%d/%d, want=1/0", firstBatches[0].Len(), secondBatches[0].Len())
	}
	firstWitness, firstWitnessOK := firstBatches[0].DenominatorWitness()
	secondWitness, secondWitnessOK := secondBatches[0].DenominatorWitness()
	if !firstWitnessOK || !secondWitnessOK || firstWitness.Len() != 1 || secondWitness.Len() != 0 {
		t.Fatal("partition did not retain exact posting witnesses")
	}
}

// TestEvaluateSharedAndEmptySourcesUsesMountedDenominators proves that a
// shared child is redeemed from its exact mounted denominator for each owner
// call, while an empty mounted denominator remains one real empty Complete
// range.
func TestEvaluateSharedAndEmptySourcesUsesMountedDenominators(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	scope, _ := fixture.OverlapScopes()
	sharedReplay, shared := evaluatorChild(t, fixture, fixture.SharedCompleteApplyNode, 1)
	session, ok := New(fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("subtree session")
	}
	left, leftOK := session.Evaluate(sharedReplay, shared, fixture.RowsLeft()[0], scope)
	right, rightOK := session.Evaluate(sharedReplay, shared, fixture.RowsLeft()[1], scope)
	if !leftOK || !left.Available() || !rightOK || !right.Available() || len(left.Batches()) != 1 || len(right.Batches()) != 1 {
		t.Fatal("shared result")
	}
	if left.Batches()[0].Len() != len(fixture.RowsRight()) || !left.Batches()[0].Same(right.Batches()[0]) {
		t.Fatal("shared child was not one exact mounted vector")
	}

	emptyReplay, empty := evaluatorChild(t, fixture, fixture.SharedEmptyApplyNode, 1)
	if emptyReplay.Population() != sharedReplay.Population() {
		t.Fatal("empty/shared population authorities diverged")
	}
	emptyResult, emptyOK := session.Evaluate(emptyReplay, empty, fixture.RowsLeft()[0], scope)
	if !emptyOK || !emptyResult.Available() || len(emptyResult.Batches()) != 1 || emptyResult.Batches()[0].Len() != 0 {
		t.Fatal("empty shared Complete was not retained as an empty range")
	}
	if witnessValue, witnessOK := emptyResult.Batches()[0].DenominatorWitness(); !witnessOK || witnessValue.Len() != 0 {
		t.Fatal("empty shared Complete lost its empty witness")
	}
}

// TestEvaluateRefusesForeignPopulationScopeDenominatorAndMissingPosting pins
// the fail-closed authority boundary. A q RowID, denominator, scope, or root
// from another physical authority cannot be used to select a child extent.
func TestEvaluateRefusesForeignPopulationScopeDenominatorAndMissingPosting(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	replay, child := evaluatorChild(t, fixture, fixture.CorrelatedApplyNode, 1)
	leftScope, rightScope := fixture.OverlapScopes()
	session, ok := New(fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("subtree session")
	}
	if result, accepted := session.Evaluate(replay, child, fixture.RowsRight()[0], leftScope); accepted || result.Available() {
		t.Fatal("foreign population row was accepted")
	}
	if result, accepted := session.Evaluate(replay, child, fixture.RowsLeft()[0], rightScope); accepted || result.Available() {
		t.Fatal("foreign population scope was accepted")
	}
	foreignFixture := testfixture.New(t, 0x72)
	foreignReplay, _ := evaluatorChild(t, foreignFixture, foreignFixture.CorrelatedApplyNode, 1)
	if result, accepted := session.Evaluate(foreignReplay, child, fixture.RowsLeft()[0], leftScope); accepted || result.Available() {
		t.Fatal("foreign population replay was accepted")
	}

	// LeftRoot contains the owner q rows but no right rows. The exact right
	// posting therefore cannot be silently replaced by a global or empty
	// fallback when this otherwise valid subtree is evaluated against it.
	leftSession, leftSessionOK := New(fixture.Mounted(), fixture.LeftRoot(), fixture.Geometry(), fixture.Scratch())
	if !leftSessionOK {
		t.Fatal("left-root session")
	}
	if result, accepted := leftSession.Evaluate(replay, child, fixture.RowsLeft()[0], leftScope); accepted || result.Available() {
		t.Fatal("missing right posting was accepted against left root")
	}

	// The direct driver redeems the exact caller scope through ExecuteRow;
	// unlike a shared child, it cannot accept another valid mounted scope as a
	// substitute for the population row's authenticated cofiber.
	driverReplay, driver := evaluatorChild(t, fixture, fixture.MixedPopulationApplyNode, 0)
	if result, accepted := session.Evaluate(driverReplay, driver, fixture.RowsLeft()[0], rightScope); accepted || result.Available() {
		t.Fatal("foreign population scope was accepted by the driver")
	}
}

// TestEvaluateOccurrenceLookupRequiresNodeAndPath prevents a same-digest or
// same-relation shortcut from selecting a sibling occurrence. The accessor is
// intentionally exercised directly because malformed duplicate extents are
// unconstructible outside the mount issuer.
func TestEvaluateOccurrenceLookupRequiresNodeAndPath(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	_, child := evaluatorChild(t, fixture, fixture.CorrelatedApplyNode, 0)
	extent, ok := child.InputAt(0)
	if !ok || !extent.Available() {
		t.Fatal("input extent")
	}
	occurrence := extent.Occurrence()
	path := occurrence.Path()
	if _, ok := child.InputFor(occurrence.Node(), path); !ok {
		t.Fatal("exact node/path occurrence did not resolve")
	}
	wrongPath := append([]uint32(nil), path...)
	if len(wrongPath) == 0 {
		wrongPath = []uint32{1}
	} else if wrongPath[0] == 0 {
		wrongPath[0] = 1
	} else {
		wrongPath[0] = 0
	}
	if _, ok := child.InputFor(occurrence.Node(), wrongPath); ok {
		t.Fatal("wrong occurrence path selected an extent")
	}
	if _, ok := child.InputFor(occurrence.Node(), nil); ok {
		t.Fatal("nil occurrence path selected an extent")
	}
}

type applyNodeFactory func() (arrangement.Node, bool)

func evaluatorChild(t *testing.T, fixture testfixture.Fixture, factory applyNodeFactory, ordinal int) (arrangement.ApplyReplay, arrangement.CorrelatedSubtree) {
	t.Helper()
	node, ok := factory()
	if !ok || !node.Available() {
		t.Fatal("Apply node")
	}
	applyBinding, ok := node.Apply()
	if !ok || !applyBinding.Available() {
		t.Fatal("Apply binding")
	}
	if !applyBinding.Correlation().Available() {
		t.Fatal("Apply correlation")
	}
	replay, ok := applyBinding.Replay()
	if !ok || !replay.Available() {
		t.Fatal("Apply replay")
	}
	child, ok := replay.ChildAt(ordinal)
	if !ok || !child.Available() {
		t.Fatal("Apply child")
	}
	return replay, child
}
